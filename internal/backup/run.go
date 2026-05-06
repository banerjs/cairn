package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/banerjs/cairn/internal/config"
	"github.com/banerjs/cairn/internal/envelope"
	"github.com/banerjs/cairn/internal/ignore"
	"github.com/banerjs/cairn/internal/manifest"
	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3store"
	"github.com/banerjs/cairn/internal/snapshotid"
	"github.com/banerjs/cairn/internal/version"
	"github.com/google/uuid"

	"filippo.io/age"
)

const zstdDataLevel = 3

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	nn, err := c.r.Read(p)
	c.n += int64(nn)
	return nn, err
}

type walkJob struct {
	rootAbs  string
	relSlash string
	fullPath string
	info     fs.FileInfo
	symlink  bool
	linkTgt  string
}

// Run executes a full backup: GC partial snapshots, walk sources, upload payloads, commit manifest, refresh index.
func Run(ctx context.Context, cfg *config.Config, st *s3store.Store, recipients []age.Recipient, storageClassCLI string, parallelismOverride int, log *slog.Logger) error {
	if err := CleanupPartialSnapshots(ctx, st, cfg.HostID, cfg.CleanupGraceDuration(), log); err != nil {
		return fmt.Errorf("backup: partial gc: %w", err)
	}
	snapID, err := snapshotid.New()
	if err != nil {
		return err
	}
	sc := cfg.S3.StorageClass
	if storageClassCLI != "" {
		sc = storageClassCLI
	}
	workers := cfg.Backup.Parallelism
	if parallelismOverride > 0 {
		workers = parallelismOverride
	}

	var jobs []walkJob
	var dirEntries []manifest.DirEntry

	for _, root := range cfg.Backup.SourceRoots {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("backup: root %q: %w", root, err)
		}
		matcher, err := ignore.Compile(rootAbs, cfg.Backup.Excludes, cfg.Backup.Includes)
		if err != nil {
			return fmt.Errorf("backup: ignore %q: %w", rootAbs, err)
		}
		walkErr := filepath.WalkDir(rootAbs, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(rootAbs, path)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			if d.IsDir() {
				if matcher.Matches(relSlash+"/", true) {
					return filepath.SkipDir
				}
				if relSlash == "." {
					return nil
				}
				info, err := d.Info()
				if err != nil {
					return err
				}
				mode, uid, gid := fileMeta(info)
				dirEntries = append(dirEntries, manifest.DirEntry{
					Path:    relSlash,
					Mode:    mode,
					UID:     uid,
					GID:     gid,
					MtimeNs: info.ModTime().UnixNano(),
				})
				return nil
			}
			if matcher.Matches(relSlash, false) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Mode().Type()&fs.ModeSymlink != 0 {
				tgt, err := os.Readlink(path)
				if err != nil {
					log.Debug("backup: read symlink", "path", path, "err", err)
					return nil
				}
				if cfg.FollowSymlinksEffective() {
					jobs = append(jobs, walkJob{rootAbs: rootAbs, relSlash: relSlash, fullPath: path, info: info, symlink: true, linkTgt: tgt})
					return nil
				}
				info2, err := os.Stat(path)
				if err != nil {
					log.Debug("backup: stat symlink target", "path", path, "err", err)
					return nil
				}
				if info2.Mode().IsDir() {
					log.Debug("backup: skipping symlink to directory", "path", path)
					return nil
				}
				jobs = append(jobs, walkJob{rootAbs: rootAbs, relSlash: relSlash, fullPath: path, info: info2})
				return nil
			}
			switch info.Mode().Type() {
			case fs.ModeDevice, fs.ModeNamedPipe, fs.ModeSocket, fs.ModeIrregular:
				log.Debug("backup: skipping special file", "path", path, "mode", info.Mode().String())
				return nil
			}
			if !info.Mode().IsRegular() {
				log.Debug("backup: skipping non-regular", "path", path)
				return nil
			}
			jobs = append(jobs, walkJob{rootAbs: rootAbs, relSlash: relSlash, fullPath: path, info: info})
			return nil
		})
		if walkErr != nil {
			return fmt.Errorf("backup: walk %q: %w", rootAbs, walkErr)
		}
	}

	jobCh := make(chan walkJob)
	var wg sync.WaitGroup
	var fileMu sync.Mutex
	var files []manifest.FileEntry
	var errMu sync.Mutex
	var firstErr error
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()

	setErr := func(e error) {
		errMu.Lock()
		defer errMu.Unlock()
		if firstErr == nil {
			firstErr = e
			cancel()
		}
	}

	worker := func() {
		defer wg.Done()
		for j := range jobCh {
			if j.symlink {
				t := j.linkTgt
				mode, uid, gid := fileMeta(j.info)
				ent := manifest.FileEntry{
					Path:          j.relSlash,
					Type:          "symlink",
					SizePlain:     0,
					SizeObject:    0,
					Mode:          mode,
					UID:           uid,
					GID:           gid,
					MtimeNs:       j.info.ModTime().UnixNano(),
					SymlinkTarget: &t,
				}
				fileMu.Lock()
				files = append(files, ent)
				fileMu.Unlock()
				continue
			}
			ent, err := uploadRegular(ctx2, st, cfg.HostID, snapID, j, recipients, sc, log)
			if err != nil {
				setErr(err)
				return
			}
			fileMu.Lock()
			files = append(files, ent)
			fileMu.Unlock()
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker()
	}
	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)
	wg.Wait()

	errMu.Lock()
	err = firstErr
	errMu.Unlock()
	if err != nil {
		return fmt.Errorf("backup: upload: %w", err)
	}

	hostOS := runtime.GOOS
	switch hostOS {
	case "linux", "darwin", "windows":
	default:
		hostOS = "linux"
	}
	started := time.Now().UTC().Format(time.RFC3339)

	var plainTotal, objectTotal int64
	for _, f := range files {
		plainTotal += f.SizePlain
		objectTotal += f.SizeObject
	}

	m := &manifest.Manifest{
		Schema:      manifest.SchemaV1,
		SnapshotID:  snapID,
		HostID:      cfg.HostID,
		HostOS:      hostOS,
		CreatedAt:   started,
		CompletedAt: time.Now().UTC().Format(time.RFC3339),
		Tool:        manifest.ToolInfo{Name: version.Name, Version: version.Version},
		Compression: manifest.CompressionInfo{Algorithm: "zstd", Level: zstdDataLevel},
		Encryption: manifest.EncryptionInfo{
			Algorithm:     "age",
			RecipientType: "mlkem768x25519",
			Recipients:    cfg.Encryption.Recipients,
		},
		SourceRoots:  cfg.Backup.SourceRoots,
		StorageClass: sc,
		Files:        files,
		Directories:  dirEntries,
		Stats: manifest.Stats{
			FilesTotal:       len(files),
			BytesPlainTotal:  plainTotal,
			BytesObjectTotal: objectTotal,
		},
	}
	raw, err := manifest.MarshalJSONForManifest(m)
	if err != nil {
		return fmt.Errorf("backup: marshal manifest: %w", err)
	}
	blob, err := envelope.Encrypt(raw, recipients)
	if err != nil {
		return fmt.Errorf("backup: encrypt manifest: %w", err)
	}
	if err := st.PutObject(ctx, paths.ManifestKey(cfg.HostID, snapID), bytes.NewReader(blob), sc); err != nil {
		return fmt.Errorf("backup: put manifest: %w", err)
	}

	if err := RebuildIndex(ctx, st, cfg.HostID, recipients, sc, log); err != nil {
		return fmt.Errorf("backup: index: %w", err)
	}
	log.Info("backup complete", "snapshot_id", snapID, "files", len(files))
	return nil
}

func uploadRegular(ctx context.Context, st *s3store.Store, hostID, snapID string, j walkJob, recipients []age.Recipient, sc string, log *slog.Logger) (manifest.FileEntry, error) {
	_ = log
	objectID := uuid.New().String()
	key := paths.ObjectKey(hostID, snapID, objectID)
	f, err := os.Open(j.fullPath)
	if err != nil {
		return manifest.FileEntry{}, fmt.Errorf("open %s: %w", j.fullPath, err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	tee := io.TeeReader(f, h)
	pr, err := envelope.EncryptReader(tee, recipients, zstdDataLevel)
	if err != nil {
		return manifest.FileEntry{}, err
	}
	cr := &countingReader{r: pr}
	if err := st.PutObject(ctx, key, cr, sc); err != nil {
		return manifest.FileEntry{}, err
	}

	mode, uid, gid := fileMeta(j.info)
	sum := hex.EncodeToString(h.Sum(nil))
	return manifest.FileEntry{
		Path:        j.relSlash,
		Type:        "regular",
		ObjectID:    objectID,
		SizePlain:   j.info.Size(),
		SizeObject:  cr.n,
		SHA256Plain: sum,
		Mode:        mode,
		UID:         uid,
		GID:         gid,
		MtimeNs:     j.info.ModTime().UnixNano(),
	}, nil
}

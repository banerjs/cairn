package restore

import (
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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/banerjs/cairn/internal/config"
	"github.com/banerjs/cairn/internal/envelope"
	"github.com/banerjs/cairn/internal/manifest"
	"github.com/banerjs/cairn/internal/paths"

	"filippo.io/age"
)

// Overrides for deterministic tests (see restore_hooks_test.go).
var (
	mkdirAllRestore = os.MkdirAll
	renameRestore   = os.Rename
	createRestore   = os.Create
	copyRestore     = func(dst io.Writer, src io.Reader) (int64, error) { return io.Copy(dst, src) }
	removeRestore   = os.Remove
	chmodRestore    = os.Chmod
	chtimesRestore  = os.Chtimes
	chownRestore    = os.Chown
	symlinkRestore  = os.Symlink
	euidRestore     = os.Geteuid
	goosRestore     = func() string { return runtime.GOOS }
)

// ObjectGetter reads encrypted snapshot objects (*s3store.Store implements this).
type ObjectGetter interface {
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
}

// Run restores one snapshot under targetRoot using the manifest as authority.
func Run(ctx context.Context, cfg *config.Config, st ObjectGetter, identities []age.Identity, snapshotID, targetRoot string, workers int, log *slog.Logger) error {
	if workers <= 0 {
		workers = cfg.Backup.Parallelism
	}
	rc, err := st.GetObject(ctx, paths.ManifestKey(cfg.HostID, snapshotID))
	if err != nil {
		return fmt.Errorf("restore: manifest: %w", err)
	}
	cipher, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		return fmt.Errorf("restore: read manifest: %w", err)
	}
	plain, err := envelope.Decrypt(cipher, identities)
	if err != nil {
		return fmt.Errorf("restore: decrypt manifest: %w", err)
	}
	m, err := manifest.UnmarshalManifestJSON(plain)
	if err != nil {
		return fmt.Errorf("restore: manifest json: %w", err)
	}
	if err := mkdirAllRestore(targetRoot, 0o750); err != nil {
		return fmt.Errorf("restore: mkdir target: %w", err)
	}

	sort.Slice(m.Directories, func(i, j int) bool {
		return depth(m.Directories[i].Path) > depth(m.Directories[j].Path)
	})
	for _, d := range m.Directories {
		dst := filepath.Join(targetRoot, filepath.FromSlash(d.Path))
		if err := mkdirAllRestore(dst, dirPerm(d)); err != nil {
			return fmt.Errorf("restore: mkdir %s: %w", dst, err)
		}
		if err := chtimesRestore(dst, timeFromNs(d.MtimeNs), timeFromNs(d.MtimeNs)); err != nil {
			log.Debug("restore: chtimes dir", "path", dst, "err", err)
		}
		if goosRestore() != "windows" && euidRestore() == 0 && d.UID != nil && d.GID != nil {
			if err := chownRestore(dst, int(*d.UID), int(*d.GID)); err != nil {
				log.Debug("restore: chown dir", "path", dst, "err", err)
			}
		}
	}

	type job struct {
		ent manifest.FileEntry
	}
	jobs := make(chan job, workers*2)
	var wg sync.WaitGroup
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

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if j.ent.Type == "symlink" {
					if j.ent.SymlinkTarget == nil {
						setErr(fmt.Errorf("restore: symlink missing target %s", j.ent.Path))
						return
					}
					dst := filepath.Join(targetRoot, filepath.FromSlash(j.ent.Path))
					if err := removeRestore(dst); err != nil && !os.IsNotExist(err) {
						setErr(fmt.Errorf("restore: rm %s: %w", dst, err))
						return
					}
					if err := mkdirAllRestore(filepath.Dir(dst), 0o750); err != nil {
						setErr(err)
						return
					}
					if err := symlinkRestore(*j.ent.SymlinkTarget, dst); err != nil {
						log.Debug("restore: symlink failed", "path", dst, "err", err)
					}
					continue
				}
				if j.ent.Type != "regular" {
					continue
				}
				dst := filepath.Join(targetRoot, filepath.FromSlash(j.ent.Path))
				if err := mkdirAllRestore(filepath.Dir(dst), 0o750); err != nil {
					setErr(err)
					return
				}
				key := paths.ObjectKey(cfg.HostID, snapshotID, j.ent.ObjectID)
				objRC, err := st.GetObject(ctx2, key)
				if err != nil {
					setErr(fmt.Errorf("restore: get %s: %w", key, err))
					return
				}
				pr, err := envelope.DecryptReader(objRC, identities)
				if err != nil {
					_ = objRC.Close()
					setErr(err)
					return
				}
				tmp := dst + ".partial"
				// #nosec G304 -- temp file next to destination path during restore
				f, err := createRestore(tmp)
				if err != nil {
					_ = pr.Close()
					_ = objRC.Close()
					setErr(err)
					return
				}
				h := sha256.New()
				_, copyErr := copyRestore(f, io.TeeReader(pr, h))
				_ = f.Close()
				_ = pr.Close()
				_ = objRC.Close()
				if copyErr != nil {
					_ = removeRestore(tmp)
					setErr(copyErr)
					return
				}
				sum := hex.EncodeToString(h.Sum(nil))
				if !strings.EqualFold(sum, j.ent.SHA256Plain) {
					_ = removeRestore(tmp)
					setErr(fmt.Errorf("restore: hash mismatch %s", j.ent.Path))
					return
				}
				if err := chmodRestore(tmp, filePerm(&j.ent)); err != nil {
					log.Debug("restore: chmod", "path", tmp, "err", err)
				}
				if goosRestore() != "windows" && euidRestore() == 0 && j.ent.UID != nil && j.ent.GID != nil {
					_ = chownRestore(tmp, int(*j.ent.UID), int(*j.ent.GID))
				}
				if err := renameRestore(tmp, dst); err != nil {
					_ = removeRestore(tmp)
					setErr(fmt.Errorf("restore: rename %s: %w", dst, err))
					return
				}
				if err := chtimesRestore(dst, timeFromNs(j.ent.MtimeNs), timeFromNs(j.ent.MtimeNs)); err != nil {
					log.Debug("restore: chtimes", "path", dst, "err", err)
				}
			}
		}()
	}
	for _, fe := range m.Files {
		jobs <- job{ent: fe}
	}
	close(jobs)
	wg.Wait()
	errMu.Lock()
	defer errMu.Unlock()
	if firstErr != nil {
		return firstErr
	}
	log.Info("restore complete", "snapshot_id", snapshotID, "files", len(m.Files))
	return nil
}

func timeFromNs(ns int64) time.Time {
	return time.Unix(0, ns).UTC()
}

func depth(p string) int {
	return strings.Count(p, "/")
}

func dirPerm(d manifest.DirEntry) fs.FileMode {
	if d.Mode != nil {
		return fs.FileMode(*d.Mode)
	}
	return 0o755
}

func filePerm(f *manifest.FileEntry) fs.FileMode {
	if f.Mode != nil {
		return fs.FileMode(*f.Mode)
	}
	return 0o644
}

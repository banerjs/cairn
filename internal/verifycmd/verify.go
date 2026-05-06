package verifycmd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/banerjs/cairn/internal/config"
	"github.com/banerjs/cairn/internal/envelope"
	"github.com/banerjs/cairn/internal/manifest"
	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3store"

	"filippo.io/age"
)

// VerifyStore is the S3 read surface verify needs (implemented by *s3store.Store and test fakes).
type VerifyStore interface {
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	ListPrefix(ctx context.Context, prefix string) ([]s3store.ListedObject, error)
}

// Run verifies a snapshot: decrypt manifest, optional random sample of files, list coverage for object existence.
func Run(ctx context.Context, cfg *config.Config, st VerifyStore, identities []age.Identity, snapshotID string, sample int, log *slog.Logger) error {
	rc, err := st.GetObject(ctx, paths.ManifestKey(cfg.HostID, snapshotID))
	if err != nil {
		return fmt.Errorf("verify: manifest: %w", err)
	}
	cipher, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		return fmt.Errorf("verify: read manifest: %w", err)
	}
	plain, err := envelope.Decrypt(cipher, identities)
	if err != nil {
		return fmt.Errorf("verify: decrypt manifest: %w", err)
	}
	m, err := manifest.UnmarshalManifestJSON(plain)
	if err != nil {
		return fmt.Errorf("verify: manifest json: %w", err)
	}

	objPrefix := paths.SnapshotPrefix(cfg.HostID, snapshotID) + "objects/"
	listed, err := st.ListPrefix(ctx, objPrefix)
	if err != nil {
		return fmt.Errorf("verify: list objects: %w", err)
	}
	listSet := make(map[string]struct{})
	for _, o := range listed {
		base := o.Key[strings.LastIndex(o.Key, "/")+1:]
		if base != "" {
			listSet[base] = struct{}{}
		}
	}
	for _, fe := range m.Files {
		if fe.Type != "regular" || fe.ObjectID == "" {
			continue
		}
		if _, ok := listSet[fe.ObjectID]; !ok {
			return fmt.Errorf("verify: missing object %s for path %s", fe.ObjectID, fe.Path)
		}
	}

	var regular []manifest.FileEntry
	for _, fe := range m.Files {
		if fe.Type == "regular" && fe.ObjectID != "" {
			regular = append(regular, fe)
		}
	}
	want := sample
	if want == 0 {
		want = len(regular)
	}
	if want > len(regular) {
		want = len(regular)
	}
	idxs := pickRandomIndices(len(regular), want)
	for _, i := range idxs {
		fe := regular[i]
		key := paths.ObjectKey(cfg.HostID, snapshotID, fe.ObjectID)
		or, err := st.GetObject(ctx, key)
		if err != nil {
			return fmt.Errorf("verify: get %s: %w", fe.Path, err)
		}
		pr, err := envelope.DecryptReader(or, identities)
		if err != nil {
			_ = or.Close()
			return fmt.Errorf("verify: decrypt %s: %w", fe.Path, err)
		}
		h := sha256.New()
		n, err := io.Copy(h, pr)
		_ = pr.Close()
		_ = or.Close()
		if err != nil {
			return fmt.Errorf("verify: read %s: %w", fe.Path, err)
		}
		if n != fe.SizePlain {
			return fmt.Errorf("verify: size mismatch %s", fe.Path)
		}
		sum := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(sum, fe.SHA256Plain) {
			return fmt.Errorf("verify: hash mismatch %s", fe.Path)
		}
		log.Debug("verify ok", "path", fe.Path)
	}
	log.Info("verify complete", "snapshot_id", snapshotID, "sampled", len(idxs))
	return nil
}

func pickRandomIndices(n, k int) []int {
	if k <= 0 || n <= 0 {
		return nil
	}
	if k >= n {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	for i := 0; i < k; i++ {
		var rb [8]byte
		_, _ = rand.Read(rb[:])
		span := n - i
		u := binary.BigEndian.Uint64(rb[:])
		// #nosec G115 -- u % uint64(span) is strictly less than span <= n <= len(idxs), fits int on supported platforms.
		j := i + int(u%uint64(span))
		idx[i], idx[j] = idx[j], idx[i]
	}
	return idx[:k]
}

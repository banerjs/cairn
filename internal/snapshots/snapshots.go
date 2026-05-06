package snapshots

import (
	"context"
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

// SnapshotLister is the S3 read surface List needs (*s3store.Store implements this).
type SnapshotLister interface {
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	ListPrefix(ctx context.Context, prefix string) ([]s3store.ListedObject, error)
}

// List prints snapshot IDs, preferring the encrypted index when decryptable.
func List(ctx context.Context, cfg *config.Config, st SnapshotLister, identities []age.Identity, filterHost string, log *slog.Logger) error {
	tryHost := cfg.HostID
	if filterHost != "" {
		tryHost = filterHost
	}
	if len(identities) > 0 {
		rc, err := st.GetObject(ctx, paths.IndexKey(tryHost))
		if err == nil {
			b, _ := io.ReadAll(rc)
			_ = rc.Close()
			plain, err := envelope.Decrypt(b, identities)
			if err == nil {
				ix, err := manifest.UnmarshalIndexJSON(plain)
				if err == nil {
					for _, s := range ix.Snapshots {
						log.Info("snapshot", "host_id", ix.HostID, "snapshot_id", s.SnapshotID, "created_at", s.CreatedAt)
					}
					return nil
				}
			}
		}
	}

	objs, err := st.ListPrefix(ctx, paths.HostsRootPrefix())
	if err != nil {
		return err
	}
	seen := make(map[string]map[string]struct{}) // host -> snap set
	for _, o := range objs {
		if !strings.HasSuffix(o.Key, "manifest.age") {
			continue
		}
		host := paths.ParseHostIDFromHostsPath(o.Key)
		if host == "" {
			continue
		}
		if filterHost != "" && host != filterHost {
			continue
		}
		sid := paths.SnapshotIDFromKey(o.Key)
		if sid == "" {
			continue
		}
		if seen[host] == nil {
			seen[host] = make(map[string]struct{})
		}
		if _, ok := seen[host][sid]; ok {
			continue
		}
		seen[host][sid] = struct{}{}
		log.Info("snapshot", "host_id", host, "snapshot_id", sid)
	}
	if len(seen) == 0 {
		log.Warn("no snapshots found")
	}
	return nil
}

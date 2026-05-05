package backup

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3store"
)

// partialStore is the minimal S3 surface needed for partial snapshot GC.
type partialStore interface {
	ListPrefix(ctx context.Context, prefix string) ([]s3store.ListedObject, error)
	DeleteObject(ctx context.Context, key string) error
}

// CleanupPartialSnapshots deletes uncommitted snapshot prefixes older than grace.
//
// It lists objects under hosts/<host>/snapshots/, groups by snapshot ID, and removes
// trees that lack manifest.age when the embedded snapshot timestamp is older than grace.
func CleanupPartialSnapshots(ctx context.Context, st partialStore, hostID string, grace time.Duration, log *slog.Logger) error {
	prefix := paths.SnapshotsListPrefix(hostID)
	objs, err := st.ListPrefix(ctx, prefix)
	if err != nil {
		return err
	}
	type snap struct {
		hasManifest bool
		keys        []string
	}
	groups := make(map[string]*snap)
	for _, o := range objs {
		sid := paths.SnapshotIDFromKey(o.Key)
		if sid == "" {
			continue
		}
		g := groups[sid]
		if g == nil {
			g = &snap{}
			groups[sid] = g
		}
		g.keys = append(g.keys, o.Key)
		if strings.HasSuffix(o.Key, "/manifest.age") || strings.HasSuffix(o.Key, "manifest.age") {
			g.hasManifest = true
		}
	}
	now := time.Now().UTC()
	for sid, g := range groups {
		if g.hasManifest {
			continue
		}
		ts, err := paths.ParseSnapshotTime(sid)
		if err != nil {
			log.Warn("partial gc: skip unparsable snapshot id", "id", sid, "err", err)
			continue
		}
		if now.Sub(ts) <= grace {
			continue
		}
		log.Info("partial gc: deleting stale partial snapshot", "snapshot_id", sid, "keys", len(g.keys))
		for _, k := range g.keys {
			if err := st.DeleteObject(ctx, k); err != nil {
				return fmt.Errorf("partial gc delete %s: %w", k, err)
			}
		}
	}
	return nil
}

package prune

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3store"
)

// PruneStore lists and deletes objects under snapshot prefixes (implemented by *s3store.Store).
type PruneStore interface {
	ListPrefix(ctx context.Context, prefix string) ([]s3store.ListedObject, error)
	DeleteObject(ctx context.Context, key string) error
}

// Run deletes snapshot trees. If removeIDs is non-empty, only those committed
// snapshots are removed (keepLast and keepMonthly are ignored). Otherwise,
// retention rules apply to all committed snapshots.
func Run(ctx context.Context, st PruneStore, hostID string, removeIDs []string, keepLast, keepMonthly int, dryRun bool, log *slog.Logger) error {
	prefix := paths.SnapshotsListPrefix(hostID)
	objs, err := st.ListPrefix(ctx, prefix)
	if err != nil {
		return err
	}
	committed := make(map[string]struct{})
	for _, o := range objs {
		if strings.HasSuffix(o.Key, "manifest.age") {
			sid := paths.SnapshotIDFromKey(o.Key)
			if sid != "" {
				committed[sid] = struct{}{}
			}
		}
	}

	if len(removeIDs) > 0 {
		seen := make(map[string]struct{})
		for _, sid := range removeIDs {
			if _, dup := seen[sid]; dup {
				continue
			}
			seen[sid] = struct{}{}
			if _, ok := committed[sid]; !ok {
				return fmt.Errorf("prune: no committed snapshot %q for host %q", sid, hostID)
			}
			if err := deleteSnapshotTree(ctx, st, hostID, sid, dryRun, log); err != nil {
				return err
			}
		}
		return nil
	}

	var ids []string
	for sid := range committed {
		ids = append(ids, sid)
	}
	keep := SelectSnapshotsToKeep(ids, keepLast, keepMonthly)
	sort.Strings(ids)
	for _, sid := range ids {
		if _, ok := keep[sid]; ok {
			continue
		}
		if err := deleteSnapshotTree(ctx, st, hostID, sid, dryRun, log); err != nil {
			return err
		}
	}
	return nil
}

func deleteSnapshotTree(ctx context.Context, st PruneStore, hostID, sid string, dryRun bool, log *slog.Logger) error {
	snapPrefix := paths.SnapshotPrefix(hostID, sid)
	keys, err := st.ListPrefix(ctx, snapPrefix)
	if err != nil {
		return fmt.Errorf("prune list %s: %w", sid, err)
	}
	log.Info("prune: deleting snapshot", "snapshot_id", sid, "objects", len(keys), "dry_run", dryRun)
	if dryRun {
		return nil
	}
	for _, ko := range keys {
		if err := st.DeleteObject(ctx, ko.Key); err != nil {
			return fmt.Errorf("prune delete %s: %w", ko.Key, err)
		}
	}
	return nil
}

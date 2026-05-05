package prune

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3store"
)

// Run deletes snapshot trees not selected by retention rules.
func Run(ctx context.Context, st *s3store.Store, hostID string, keepLast, keepMonthly int, dryRun bool, log *slog.Logger) error {
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
	var ids []string
	for sid := range committed {
		ids = append(ids, sid)
	}
	keep := SelectSnapshotsToKeep(ids, keepLast, keepMonthly)
	for _, sid := range ids {
		if _, ok := keep[sid]; ok {
			continue
		}
		snapPrefix := paths.SnapshotPrefix(hostID, sid)
		keys, err := st.ListPrefix(ctx, snapPrefix)
		if err != nil {
			return fmt.Errorf("prune list %s: %w", sid, err)
		}
		log.Info("prune: deleting snapshot", "snapshot_id", sid, "objects", len(keys), "dry_run", dryRun)
		if dryRun {
			continue
		}
		for _, ko := range keys {
			if err := st.DeleteObject(ctx, ko.Key); err != nil {
				return fmt.Errorf("prune delete %s: %w", ko.Key, err)
			}
		}
	}
	return nil
}

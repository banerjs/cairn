package backup

import (
	"bytes"
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/banerjs/cairn/internal/envelope"
	"github.com/banerjs/cairn/internal/manifest"
	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3store"

	"filippo.io/age"
)

// RebuildIndex lists committed snapshots (manifest.age present) and overwrites hosts/<host>/index.age.
//
// Per-object manifest stats are left zero except snapshot_id and created_at parsed from the id;
// authoritative details remain inside each manifest.
func RebuildIndex(ctx context.Context, st *s3store.Store, hostID string, recipients []age.Recipient, storageClass string, log *slog.Logger) error {
	prefix := paths.SnapshotsListPrefix(hostID)
	objs, err := st.ListPrefix(ctx, prefix)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{})
	var snaps []manifest.IndexSnap
	for _, o := range objs {
		if !strings.HasSuffix(o.Key, "manifest.age") {
			continue
		}
		sid := paths.SnapshotIDFromKey(o.Key)
		if sid == "" {
			continue
		}
		if _, ok := seen[sid]; ok {
			continue
		}
		seen[sid] = struct{}{}
		ts, err := paths.ParseSnapshotTime(sid)
		if err != nil {
			log.Warn("index: skip bad snapshot id", "id", sid)
			continue
		}
		snaps = append(snaps, manifest.IndexSnap{
			SnapshotID:       sid,
			CreatedAt:        ts.UTC().Format(time.RFC3339),
			FilesTotal:       0,
			BytesObjectTotal: 0,
			StorageClass:     storageClass,
		})
	}
	sort.Slice(snaps, func(i, j int) bool {
		return snaps[i].SnapshotID > snaps[j].SnapshotID
	})
	ix := &manifest.Index{
		Schema:    manifest.IndexSchemaV1,
		HostID:    hostID,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Snapshots: snaps,
	}
	raw, err := manifest.MarshalIndexJSON(ix)
	if err != nil {
		return err
	}
	blob, err := envelope.Encrypt(raw, recipients)
	if err != nil {
		return err
	}
	return st.PutObject(ctx, paths.IndexKey(hostID), bytes.NewReader(blob), "")
}

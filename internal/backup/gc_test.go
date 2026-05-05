package backup

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3store"
)

type memPartial struct {
	keys []string
}

func (m *memPartial) ListPrefix(ctx context.Context, prefix string) ([]s3store.ListedObject, error) {
	var out []s3store.ListedObject
	for _, k := range m.keys {
		if strings.HasPrefix(k, prefix) {
			out = append(out, s3store.ListedObject{Key: k})
		}
	}
	return out, nil
}

func (m *memPartial) DeleteObject(ctx context.Context, key string) error {
	var next []string
	for _, k := range m.keys {
		if k != key {
			next = append(next, k)
		}
	}
	m.keys = next
	return nil
}

func TestCleanupPartialSnapshots_RespectsGrace(t *testing.T) {
	ctx := context.Background()
	host := "h1"
	oldSID := "20000101T000000Z-deadbeef"
	newSID := time.Now().UTC().Format("20060102T150405Z") + "-00000001"

	oldPrefix := paths.SnapshotPrefix(host, oldSID)
	newPrefix := paths.SnapshotPrefix(host, newSID)
	m := &memPartial{keys: []string{
		oldPrefix + "objects/u1",
		newPrefix + "objects/u2",
	}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := CleanupPartialSnapshots(ctx, m, host, 24*time.Hour, log); err != nil {
		t.Fatal(err)
	}
	if len(m.keys) != 1 {
		t.Fatalf("expected 1 key left, got %v", m.keys)
	}
	if m.keys[0] != newPrefix+"objects/u2" {
		t.Fatalf("wrong key retained: %v", m.keys)
	}
}

func TestCleanupPartialSnapshots_DeletesStalePartial(t *testing.T) {
	ctx := context.Background()
	host := "h1"
	oldSID := "20000101T000000Z-deadbeef"
	oldPrefix := paths.SnapshotPrefix(host, oldSID)
	m := &memPartial{keys: []string{oldPrefix + "objects/u1"}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := CleanupPartialSnapshots(ctx, m, host, time.Hour, log); err != nil {
		t.Fatal(err)
	}
	if len(m.keys) != 0 {
		t.Fatalf("expected no keys, got %v", m.keys)
	}
}

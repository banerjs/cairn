package prune

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3test"
)

func TestRun_DryRun(t *testing.T) {
	st, cleanup := s3test.NewStore(t, "prune-bucket")
	defer cleanup()
	ctx := context.Background()
	host := "ph"
	old := "20200101T000000Z-aaaaaaaa"
	newer := "20250101T000000Z-bbbbbbbb"
	for _, sid := range []string{old, newer} {
		k := paths.ManifestKey(host, sid)
		if err := st.PutObject(ctx, k, strings.NewReader("x"), ""); err != nil {
			t.Fatal(err)
		}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(ctx, st, host, nil, 1, 0, true, log); err != nil {
		t.Fatal(err)
	}
}

func TestRun_DeleteOldSnapshot(t *testing.T) {
	st, cleanup := s3test.NewStore(t, "prune-real")
	defer cleanup()
	ctx := context.Background()
	host := "ph2"
	old := "20200101T000000Z-deadbeef"
	newer := "20250101T000000Z-cafebabe"
	for _, sid := range []string{old, newer} {
		pre := paths.SnapshotPrefix(host, sid)
		if err := st.PutObject(ctx, pre+"objects/o1", strings.NewReader("payload"), ""); err != nil {
			t.Fatal(err)
		}
		if err := st.PutObject(ctx, paths.ManifestKey(host, sid), strings.NewReader("manifest"), ""); err != nil {
			t.Fatal(err)
		}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(ctx, st, host, nil, 1, 0, false, log); err != nil {
		t.Fatal(err)
	}
	after, err := st.ListPrefix(ctx, paths.SnapshotsListPrefix(host))
	if err != nil {
		t.Fatal(err)
	}
	foundOld, foundNew := false, false
	for _, o := range after {
		if strings.Contains(o.Key, "/"+old+"/") {
			foundOld = true
		}
		if strings.Contains(o.Key, "/"+newer+"/") {
			foundNew = true
		}
	}
	if foundOld || !foundNew {
		t.Fatalf("prune retained wrong snapshots: old=%v new=%v keys=%v", foundOld, foundNew, len(after))
	}
}

func TestRun_AllKept_NoDeletes(t *testing.T) {
	st, cleanup := s3test.NewStore(t, "prune-allkeep")
	defer cleanup()
	ctx := context.Background()
	host := "ph3"
	sid := "20200101T000000Z-11111111"
	if err := st.PutObject(ctx, paths.ManifestKey(host, sid), strings.NewReader("m"), ""); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(ctx, st, host, nil, 10, 0, false, log); err != nil {
		t.Fatal(err)
	}
	key := paths.ManifestKey(host, sid)
	if _, err := st.GetObject(ctx, key); err != nil {
		t.Fatal(err)
	}
}

func TestRun_ExplicitRemoveIDs(t *testing.T) {
	st, cleanup := s3test.NewStore(t, "prune-explicit")
	defer cleanup()
	ctx := context.Background()
	host := "ph4"
	a := "20200101T000000Z-aaaaaaaa"
	b := "20250101T000000Z-bbbbbbbb"
	for _, sid := range []string{a, b} {
		pre := paths.SnapshotPrefix(host, sid)
		if err := st.PutObject(ctx, pre+"objects/o1", strings.NewReader("p"), ""); err != nil {
			t.Fatal(err)
		}
		if err := st.PutObject(ctx, paths.ManifestKey(host, sid), strings.NewReader("m"), ""); err != nil {
			t.Fatal(err)
		}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(ctx, st, host, []string{a}, 0, 0, false, log); err != nil {
		t.Fatal(err)
	}
	after, err := st.ListPrefix(ctx, paths.SnapshotsListPrefix(host))
	if err != nil {
		t.Fatal(err)
	}
	var hasA, hasB bool
	for _, o := range after {
		if strings.Contains(o.Key, "/"+a+"/") {
			hasA = true
		}
		if strings.Contains(o.Key, "/"+b+"/") {
			hasB = true
		}
	}
	if hasA || !hasB {
		t.Fatalf("explicit prune removed wrong snapshots: hasA=%v hasB=%v keys=%d", hasA, hasB, len(after))
	}
}

func TestRun_ExplicitDuplicateSnapshotIDsSkipped(t *testing.T) {
	st, cleanup := s3test.NewStore(t, "prune-dup")
	defer cleanup()
	ctx := context.Background()
	host := "ph-dup"
	sid := "20200101T000000Z-dupdupdu"
	pre := paths.SnapshotPrefix(host, sid)
	if err := st.PutObject(ctx, pre+"objects/o1", strings.NewReader("p"), ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutObject(ctx, paths.ManifestKey(host, sid), strings.NewReader("m"), ""); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(ctx, st, host, []string{sid, sid}, 0, 0, false, log); err != nil {
		t.Fatal(err)
	}
	after, err := st.ListPrefix(ctx, paths.SnapshotsListPrefix(host))
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("expected snapshot removed once, got keys: %v", after)
	}
}

func TestRun_ExplicitUnknownID(t *testing.T) {
	st, cleanup := s3test.NewStore(t, "prune-unknown")
	defer cleanup()
	ctx := context.Background()
	host := "ph5"
	sid := "20200101T000000Z-11111111"
	if err := st.PutObject(ctx, paths.ManifestKey(host, sid), strings.NewReader("m"), ""); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(ctx, st, host, []string{"20290101T000000Z-nosuchsnap"}, 0, 0, false, log); err == nil {
		t.Fatal("expected error for unknown snapshot id")
	}
}

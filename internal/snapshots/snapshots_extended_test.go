package snapshots

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	appcfg "github.com/banerjs/cairn/internal/config"
	"github.com/banerjs/cairn/internal/envelope"
	"github.com/banerjs/cairn/internal/manifest"
	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3store"
	"github.com/banerjs/cairn/internal/s3test"

	"filippo.io/age"
)

func TestList_IndexDecryptPath(t *testing.T) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	recStr := id.Recipient().String()
	rec, err := age.ParseHybridRecipient(recStr)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := age.ParseIdentities(strings.NewReader(id.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	st, cleanup := s3test.NewStore(t, "snap-ix")
	defer cleanup()
	ctx := context.Background()
	host := "indexhost"

	ix := &manifest.Index{
		Schema:    manifest.IndexSchemaV1,
		HostID:    host,
		UpdatedAt: "2026-01-02T00:00:00Z",
		Snapshots: []manifest.IndexSnap{
			{SnapshotID: "20260102T010203Z-aaaaaaaa", CreatedAt: "2026-01-02T01:02:03Z"},
		},
	}
	raw, err := manifest.MarshalIndexJSON(ix)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := envelope.Encrypt(raw, []age.Recipient{rec})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutObject(ctx, paths.IndexKey(host), bytes.NewReader(blob), ""); err != nil {
		t.Fatal(err)
	}
	cfg := &appcfg.Config{HostID: host}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := List(ctx, cfg, st, ids, "", log); err != nil {
		t.Fatal(err)
	}
}

func TestList_WithIdentitiesNoIndexObjectUsesManifestListing(t *testing.T) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	ids, err := age.ParseIdentities(strings.NewReader(id.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	st, cleanup := s3test.NewStore(t, "snap-noix")
	defer cleanup()
	ctx := context.Background()
	host := "noidx"
	sid := "20200303T030303Z-cccccccc"
	if err := st.PutObject(ctx, paths.ManifestKey(host, sid), strings.NewReader("m"), ""); err != nil {
		t.Fatal(err)
	}
	cfg := &appcfg.Config{HostID: host}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := List(ctx, cfg, st, ids, "", log); err != nil {
		t.Fatal(err)
	}
}

func TestList_FilterHostUsesIndexHost(t *testing.T) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	recStr := id.Recipient().String()
	rec, err := age.ParseHybridRecipient(recStr)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := age.ParseIdentities(strings.NewReader(id.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	st, cleanup := s3test.NewStore(t, "snap-ixf")
	defer cleanup()
	ctx := context.Background()

	ixHost := "other"
	ix := &manifest.Index{Schema: manifest.IndexSchemaV1, HostID: ixHost, UpdatedAt: "2026-01-01", Snapshots: nil}
	raw, err := manifest.MarshalIndexJSON(ix)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := envelope.Encrypt(raw, []age.Recipient{rec})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutObject(ctx, paths.IndexKey(ixHost), bytes.NewReader(blob), ""); err != nil {
		t.Fatal(err)
	}
	cfg := &appcfg.Config{HostID: "default"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := List(ctx, cfg, st, ids, ixHost, log); err != nil {
		t.Fatal(err)
	}
}

func TestList_IndexGarbageFallback(t *testing.T) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	recStr := id.Recipient().String()
	rec, err := age.ParseHybridRecipient(recStr)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := age.ParseIdentities(strings.NewReader(id.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	st, cleanup := s3test.NewStore(t, "snap-fb")
	defer cleanup()
	ctx := context.Background()
	host := "fallbackh"
	blobBad, err := envelope.Encrypt([]byte("not-json"), []age.Recipient{rec})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutObject(ctx, paths.IndexKey(host), bytes.NewReader(blobBad), ""); err != nil {
		t.Fatal(err)
	}
	sid := "20200101T000000Z-bcbcbcbc"
	k := paths.ManifestKey(host, sid)
	if err := st.PutObject(ctx, k, strings.NewReader("m"), ""); err != nil {
		t.Fatal(err)
	}
	cfg := &appcfg.Config{HostID: host}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := List(ctx, cfg, st, ids, "", log); err != nil {
		t.Fatal(err)
	}
}

func TestList_IndexWrongSchemaFallback(t *testing.T) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	recStr := id.Recipient().String()
	rec, err := age.ParseHybridRecipient(recStr)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := age.ParseIdentities(strings.NewReader(id.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	st, cleanup := s3test.NewStore(t, "snap-sch")
	defer cleanup()
	ctx := context.Background()
	host := "badschema"
	wrong := `{"schema":"x","host_id":"h","updated_at":"t","snapshots":[]}`
	blob, err := envelope.Encrypt([]byte(wrong), []age.Recipient{rec})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutObject(ctx, paths.IndexKey(host), bytes.NewReader(blob), ""); err != nil {
		t.Fatal(err)
	}
	sid := "20200202T000000Z-abababab"
	if err := st.PutObject(ctx, paths.ManifestKey(host, sid), strings.NewReader("m"), ""); err != nil {
		t.Fatal(err)
	}
	cfg := &appcfg.Config{HostID: host}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := List(ctx, cfg, st, ids, "", log); err != nil {
		t.Fatal(err)
	}
}

func TestList_NoSnapshotsWarn(t *testing.T) {
	st, cleanup := s3test.NewStore(t, "snap-empty")
	defer cleanup()
	ctx := context.Background()
	cfg := &appcfg.Config{HostID: "empty"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := List(ctx, cfg, st, nil, "", log); err != nil {
		t.Fatal(err)
	}
}

func TestList_FilterHostManifests(t *testing.T) {
	st, cleanup := s3test.NewStore(t, "snap-filt")
	defer cleanup()
	ctx := context.Background()
	for _, host := range []string{"aa", "bb"} {
		sid := "20200101T000000Z-11111111"
		if host == "bb" {
			sid = "20200102T000000Z-22222222"
		}
		if err := st.PutObject(ctx, paths.ManifestKey(host, sid), strings.NewReader("m"), ""); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &appcfg.Config{HostID: "aa"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := List(ctx, cfg, st, nil, "bb", log); err != nil {
		t.Fatal(err)
	}
}

// dupListStub yields two listings that normalize to the same snapshot id under one host (covers seen dedupe).
type dupListStub struct{}

func (dupListStub) GetObject(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("no index")
}

func (dupListStub) ListPrefix(context.Context, string) ([]s3store.ListedObject, error) {
	return []s3store.ListedObject{
		{Key: "cairn/v1/hosts/foo/snapshots/20260101T000000Z-dddddddd/manifest.age"},
		{Key: "cairn/v1/hosts/foo/snapshots/20260101T000000Z-dddddddd/other/manifest.age"},
	}, nil
}

func TestList_DedupSeen(t *testing.T) {
	cfg := &appcfg.Config{HostID: "foo"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := List(context.Background(), cfg, dupListStub{}, nil, "", log); err != nil {
		t.Fatal(err)
	}
}

type errListStub struct{}

func (errListStub) GetObject(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("no")
}

func (errListStub) ListPrefix(context.Context, string) ([]s3store.ListedObject, error) {
	return nil, errors.New("list failed")
}

type badHostStub struct{}

func (badHostStub) GetObject(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("no index")
}

func (badHostStub) ListPrefix(context.Context, string) ([]s3store.ListedObject, error) {
	return []s3store.ListedObject{
		{Key: "manifest.age"}, // ParseHostIDFromHostsPath fails
		{Key: "cairn/v1/hosts/gh/snapshots/20260404T040404Z-eeeeeeee/other"}, // suffix not manifest.age
		{Key: "cairn/v1/hosts/gh/snapshots//manifest.age"},                   // SnapshotIDFromKey → ""
	}, nil
}

func TestList_NonManifestAndBadKeysSkipped(t *testing.T) {
	cfg := &appcfg.Config{HostID: "gh"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := List(context.Background(), cfg, badHostStub{}, nil, "", log); err != nil {
		t.Fatal(err)
	}
}

func TestList_ListRootError(t *testing.T) {
	cfg := &appcfg.Config{HostID: "h"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := List(context.Background(), cfg, errListStub{}, nil, "", log); err == nil {
		t.Fatal("expected error")
	}
}

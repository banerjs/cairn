package verifycmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appcfg "github.com/banerjs/cairn/internal/config"
	"github.com/banerjs/cairn/internal/envelope"
	"github.com/banerjs/cairn/internal/manifest"
	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3store"

	"filippo.io/age"
)

type errRC struct{}

func (errRC) Read([]byte) (int, error) { return 0, errors.New("read fail") }
func (errRC) Close() error             { return nil }

type stubStore struct {
	get  func(key string) (io.ReadCloser, error)
	list func(prefix string) ([]s3store.ListedObject, error)
}

func (s *stubStore) GetObject(_ context.Context, key string) (io.ReadCloser, error) {
	return s.get(key)
}
func (s *stubStore) ListPrefix(_ context.Context, prefix string) ([]s3store.ListedObject, error) {
	return s.list(prefix)
}

func verifyCfg(t *testing.T, rec string) *appcfg.Config {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "c.toml")
	body := fmt.Sprintf(`
host_id = "h"
[s3]
bucket = "b"
region = "us-east-1"
[encryption]
recipients = [%q]
[backup]
source_roots = ["/tmp"]
`, rec)
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := appcfg.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func encManifest(t *testing.T, m *manifest.Manifest, rec age.Recipient) []byte {
	t.Helper()
	raw, err := manifest.MarshalJSONForManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := envelope.Encrypt(raw, []age.Recipient{rec})
	if err != nil {
		t.Fatal(err)
	}
	return blob
}

func TestRun_ErrorBranches(t *testing.T) {
	id, _ := age.GenerateHybridIdentity()
	rec, _ := age.ParseHybridRecipient(id.Recipient().String())
	ids, _ := age.ParseIdentities(strings.NewReader(id.String() + "\n"))
	cfg := verifyCfg(t, id.Recipient().String())
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	snap := "20260103T000000Z-eeeeeeee"
	oid := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	manKey := paths.ManifestKey(cfg.HostID, snap)
	objPrefix := paths.SnapshotPrefix(cfg.HostID, snap) + "objects/"

	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: cfg.HostID, HostOS: "linux",
		CreatedAt: "t", CompletedAt: "t", Tool: manifest.ToolInfo{Name: "c", Version: "v"},
		Files: []manifest.FileEntry{
			{Path: "skip", Type: "symlink", MtimeNs: 1},
			{Path: "empty-id", Type: "regular", MtimeNs: 1},
			{Path: "f", Type: "regular", ObjectID: oid, SizePlain: 5, SHA256Plain: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", MtimeNs: 1},
		},
	}
	manBlob := encManifest(t, m, rec)
	mSize := *m
	mSize.Files = []manifest.FileEntry{
		{Path: "f", Type: "regular", ObjectID: oid, SizePlain: 9, SHA256Plain: m.Files[2].SHA256Plain, MtimeNs: 1},
	}
	manSizeBlob := encManifest(t, &mSize, rec)
	objBlob, _ := envelope.Encrypt([]byte("hello"), []age.Recipient{rec})

	// get manifest fails
	if err := Run(ctx, cfg, &stubStore{
		get:  func(string) (io.ReadCloser, error) { return nil, errors.New("no manifest") },
		list: func(string) ([]s3store.ListedObject, error) { return nil, nil },
	}, ids, snap, 1, log); err == nil {
		t.Fatal("expected manifest get error")
	}

	// read manifest fails
	if err := Run(ctx, cfg, &stubStore{
		get: func(key string) (io.ReadCloser, error) {
			if key == manKey {
				return errRC{}, nil
			}
			return nil, errors.New("unused")
		},
		list: func(string) ([]s3store.ListedObject, error) { return nil, nil },
	}, ids, snap, 1, log); err == nil || !strings.Contains(err.Error(), "read manifest") {
		t.Fatalf("err=%v", err)
	}

	// decrypt manifest fails (wrong identities)
	id2, _ := age.GenerateHybridIdentity()
	otherIDs, _ := age.ParseIdentities(strings.NewReader(id2.String() + "\n"))
	if err := Run(ctx, cfg, &stubStore{
		get:  func(key string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(string(manBlob))), nil },
		list: func(string) ([]s3store.ListedObject, error) { return nil, nil },
	}, otherIDs, snap, 1, log); err == nil || !strings.Contains(err.Error(), "decrypt manifest") {
		t.Fatalf("err=%v", err)
	}

	// manifest json fails
	badJSON, _ := envelope.Encrypt([]byte("{"), []age.Recipient{rec})
	if err := Run(ctx, cfg, &stubStore{
		get:  func(key string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(string(badJSON))), nil },
		list: func(string) ([]s3store.ListedObject, error) { return nil, nil },
	}, ids, snap, 1, log); err == nil || !strings.Contains(err.Error(), "manifest json") {
		t.Fatalf("err=%v", err)
	}

	// list fails
	if err := Run(ctx, cfg, &stubStore{
		get:  func(key string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(string(manBlob))), nil },
		list: func(string) ([]s3store.ListedObject, error) { return nil, errors.New("list fail") },
	}, ids, snap, 1, log); err == nil || !strings.Contains(err.Error(), "list objects") {
		t.Fatalf("err=%v", err)
	}

	// sample get fails + want > len(regular) path
	if err := Run(ctx, cfg, &stubStore{
		get: func(key string) (io.ReadCloser, error) {
			if key == manKey {
				return io.NopCloser(strings.NewReader(string(manBlob))), nil
			}
			return nil, errors.New("obj missing")
		},
		list: func(prefix string) ([]s3store.ListedObject, error) {
			if prefix == objPrefix {
				return []s3store.ListedObject{{Key: objPrefix + oid}}, nil
			}
			return nil, nil
		},
	}, ids, snap, 99, log); err == nil || !strings.Contains(err.Error(), "verify: get") {
		t.Fatalf("err=%v", err)
	}

	// object decrypt fails
	if err := Run(ctx, cfg, &stubStore{
		get: func(key string) (io.ReadCloser, error) {
			if key == manKey {
				return io.NopCloser(strings.NewReader(string(manBlob))), nil
			}
			return io.NopCloser(strings.NewReader("not-age")), nil
		},
		list: func(prefix string) ([]s3store.ListedObject, error) {
			return []s3store.ListedObject{{Key: objPrefix + oid}}, nil
		},
	}, ids, snap, 1, log); err == nil || !strings.Contains(err.Error(), "decrypt") {
		t.Fatalf("err=%v", err)
	}

	// copy/read fails via hook
	prevCopy := verifyCopy
	verifyCopy = func(io.Writer, io.Reader) (int64, error) { return 0, errors.New("copy fail") }
	defer func() { verifyCopy = prevCopy }()
	if err := Run(ctx, cfg, &stubStore{
		get: func(key string) (io.ReadCloser, error) {
			if key == manKey {
				return io.NopCloser(strings.NewReader(string(manBlob))), nil
			}
			return io.NopCloser(strings.NewReader(string(objBlob))), nil
		},
		list: func(prefix string) ([]s3store.ListedObject, error) {
			return []s3store.ListedObject{{Key: objPrefix + oid}}, nil
		},
	}, ids, snap, 1, log); err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("err=%v", err)
	}
	verifyCopy = prevCopy

	// size mismatch
	if err := Run(ctx, cfg, &stubStore{
		get: func(key string) (io.ReadCloser, error) {
			if key == manKey {
				return io.NopCloser(strings.NewReader(string(manSizeBlob))), nil
			}
			return io.NopCloser(strings.NewReader(string(objBlob))), nil
		},
		list: func(prefix string) ([]s3store.ListedObject, error) {
			return []s3store.ListedObject{{Key: objPrefix + oid}}, nil
		},
	}, ids, snap, 1, log); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("err=%v", err)
	}
}

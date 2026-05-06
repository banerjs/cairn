package verifycmd

import (
	"bytes"
	"context"
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

type mapStore struct {
	objs map[string][]byte
}

func (m *mapStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	b, ok := m.objs[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (m *mapStore) ListPrefix(ctx context.Context, prefix string) ([]s3store.ListedObject, error) {
	var out []s3store.ListedObject
	for k, v := range m.objs {
		if strings.HasPrefix(k, prefix) {
			out = append(out, s3store.ListedObject{Key: k, Size: int64(len(v))})
		}
	}
	return out, nil
}

func TestVerifyHappyPathAndTamper(t *testing.T) {
	ctx := context.Background()
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	recStr := id.Recipient().String()
	ids, err := age.ParseIdentities(strings.NewReader(id.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	body := fmt.Sprintf(`
host_id = "test-host"
cleanup_grace = "1h"

[s3]
bucket = "b"
region = "us-east-1"
storage_class = "STANDARD"

[encryption]
recipients = [%q]
identity_file = "/dev/null"

[backup]
source_roots = [%q]
`, recStr, filepath.Join(dir, "src"))
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := appcfg.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	rec, err := age.ParseHybridRecipient(recStr)
	if err != nil {
		t.Fatal(err)
	}
	recipients := []age.Recipient{rec}

	const snapID = "20260101T000000Z-aaaaaaaa"
	const objectID = "11111111-1111-4111-8111-111111111111"
	plain := []byte("hello")
	const wantSHA = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

	objCipher, err := envelope.Encrypt(plain, recipients)
	if err != nil {
		t.Fatal(err)
	}
	badCipher, err := envelope.Encrypt([]byte("hallo"), recipients)
	if err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		Schema:       manifest.SchemaV1,
		SnapshotID:   snapID,
		HostID:       cfg.HostID,
		HostOS:       "linux",
		CreatedAt:    "2026-01-01T00:00:00Z",
		CompletedAt:  "2026-01-01T00:00:01Z",
		Tool:         manifest.ToolInfo{Name: "cairn", Version: "test"},
		Compression:  manifest.CompressionInfo{Algorithm: "zstd", Level: 3},
		Encryption:   manifest.EncryptionInfo{Algorithm: "age", RecipientType: "mlkem768x25519", Recipients: cfg.Encryption.Recipients},
		SourceRoots:  cfg.Backup.SourceRoots,
		StorageClass: "STANDARD",
		Files: []manifest.FileEntry{
			{
				Path: "doc.txt", Type: "regular", ObjectID: objectID,
				SizePlain: int64(len(plain)), SizeObject: int64(len(objCipher)),
				SHA256Plain: wantSHA, MtimeNs: 1,
			},
		},
		Stats: manifest.Stats{FilesTotal: 1, BytesPlainTotal: int64(len(plain)), BytesObjectTotal: int64(len(objCipher))},
	}
	raw, err := manifest.MarshalJSONForManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	manCipher, err := envelope.Encrypt(raw, recipients)
	if err != nil {
		t.Fatal(err)
	}

	store := &mapStore{objs: map[string][]byte{
		paths.ManifestKey(cfg.HostID, snapID):           manCipher,
		paths.ObjectKey(cfg.HostID, snapID, objectID):   objCipher,
		paths.ObjectKey(cfg.HostID, snapID, "deadbeef"): []byte("noise"),
	}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := Run(ctx, cfg, store, ids, snapID, 0, log); err != nil {
		t.Fatalf("verify: %v", err)
	}

	store.objs[paths.ObjectKey(cfg.HostID, snapID, objectID)] = badCipher
	if err := Run(ctx, cfg, store, ids, snapID, 0, log); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected hash mismatch, got %v", err)
	}
}

func TestPickRandomIndices(t *testing.T) {
	if s := pickRandomIndices(0, 5); len(s) != 0 {
		t.Fatalf("got %#v", s)
	}
	if s := pickRandomIndices(5, 0); len(s) != 0 {
		t.Fatalf("got %#v", s)
	}
	if s := pickRandomIndices(3, 10); len(s) != 3 {
		t.Fatalf("got %#v", s)
	}
	if s := pickRandomIndices(10, 3); len(s) != 3 {
		t.Fatalf("got %#v", s)
	}
}

func TestVerifyMissingObject(t *testing.T) {
	ctx := context.Background()
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	recStr := id.Recipient().String()
	ids, err := age.ParseIdentities(strings.NewReader(id.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	body := fmt.Sprintf(`
host_id = "test-host"
cleanup_grace = "1h"
[s3]
bucket = "b"
region = "us-east-1"
storage_class = "STANDARD"
[encryption]
recipients = [%q]
identity_file = "/dev/null"
[backup]
source_roots = [%q]
`, recStr, filepath.Join(dir, "src"))
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := appcfg.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := age.ParseHybridRecipient(recStr)
	if err != nil {
		t.Fatal(err)
	}
	recipients := []age.Recipient{rec}

	const snapID = "20260102T000000Z-bbbbbbbb"
	const objectID = "22222222-2222-4222-8222-222222222222"
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snapID, HostID: cfg.HostID, HostOS: "linux",
		CreatedAt: "2026-01-02T00:00:00Z", CompletedAt: "2026-01-02T00:00:01Z",
		Tool:        manifest.ToolInfo{Name: "cairn", Version: "test"},
		Compression: manifest.CompressionInfo{Algorithm: "zstd", Level: 3},
		Encryption:  manifest.EncryptionInfo{Algorithm: "age", RecipientType: "mlkem768x25519", Recipients: cfg.Encryption.Recipients},
		Files: []manifest.FileEntry{
			{Path: "a", Type: "regular", ObjectID: objectID, SizePlain: 1, SHA256Plain: "00", MtimeNs: 1},
		},
	}
	raw, err := manifest.MarshalJSONForManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	manCipher, err := envelope.Encrypt(raw, recipients)
	if err != nil {
		t.Fatal(err)
	}

	store := &mapStore{objs: map[string][]byte{
		paths.ManifestKey(cfg.HostID, snapID): manCipher,
	}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(ctx, cfg, store, ids, snapID, 0, log); err == nil || !strings.Contains(err.Error(), "missing object") {
		t.Fatalf("expected missing object error, got %v", err)
	}
}

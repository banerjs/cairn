package restore

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
	"github.com/banerjs/cairn/internal/s3test"

	"filippo.io/age"
)

func TestRun_OneFile(t *testing.T) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	recStr := id.Recipient().String()
	rec, err := age.ParseHybridRecipient(recStr)
	if err != nil {
		t.Fatal(err)
	}
	recipients := []age.Recipient{rec}
	ids, err := age.ParseIdentities(strings.NewReader(id.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	const plain = "hello-restore"
	if err := os.WriteFile(filepath.Join(src, "f.txt"), []byte(plain), 0o644); err != nil {
		t.Fatal(err)
	}

	bucket := "restore-test"
	st, cleanup := s3test.NewStore(t, bucket)
	defer cleanup()
	ctx := context.Background()

	cfgPath := filepath.Join(tmp, "cfg.toml")
	body := fmt.Sprintf(`
host_id = "rh"
cleanup_grace = "1h"

[s3]
bucket = %q
region = "us-east-1"
storage_class = "STANDARD"

[encryption]
recipients = [%q]
identity_file = "/dev/null"

[backup]
source_roots = [%q]
`, bucket, recStr, src)
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := appcfg.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	const snapID = "20260206T120000Z-f00df00d"
	objID := "33333333-3333-4333-8333-333333333333"
	plainBytes := []byte(plain)
	objCipher, err := envelope.Encrypt(plainBytes, recipients)
	if err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snapID, HostID: cfg.HostID, HostOS: "linux",
		CreatedAt: "2026-01-01T00:00:00Z", CompletedAt: "2026-01-01T00:00:01Z",
		Tool:        manifest.ToolInfo{Name: "cairn", Version: "t"},
		Compression: manifest.CompressionInfo{Algorithm: "zstd", Level: 3},
		Encryption: manifest.EncryptionInfo{
			Algorithm: "age", RecipientType: "mlkem768x25519", Recipients: cfg.Encryption.Recipients,
		},
		SourceRoots: []string{src}, StorageClass: "STANDARD",
		Files: []manifest.FileEntry{{
			Path: "f.txt", Type: "regular", ObjectID: objID,
			SizePlain: int64(len(plainBytes)), SizeObject: int64(len(objCipher)),
			SHA256Plain: "5b95a02686eb36c9e66160582ec9dc6e27f1c79839869634d9e3fc34c545e3e0",
			MtimeNs:     1,
		}},
		Stats: manifest.Stats{FilesTotal: 1, BytesPlainTotal: int64(len(plainBytes)), BytesObjectTotal: int64(len(objCipher))},
	}
	raw, err := manifest.MarshalJSONForManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	manCipher, err := envelope.Encrypt(raw, recipients)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutObject(ctx, paths.ManifestKey(cfg.HostID, snapID), bytes.NewReader(manCipher), ""); err != nil {
		t.Fatal(err)
	}
	if err := st.PutObject(ctx, paths.ObjectKey(cfg.HostID, snapID, objID), bytes.NewReader(objCipher), ""); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tmp, "out")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(ctx, cfg, st, ids, snapID, outDir, 1, log); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(outDir, "f.txt"))
	if err != nil || string(got) != plain {
		t.Fatalf("restore file: %v %q", err, got)
	}
}

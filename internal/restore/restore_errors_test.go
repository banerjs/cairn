package restore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	appcfg "github.com/banerjs/cairn/internal/config"
	"github.com/banerjs/cairn/internal/envelope"
	"github.com/banerjs/cairn/internal/manifest"
	"github.com/banerjs/cairn/internal/paths"

	"filippo.io/age"
)

type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) { return 0, errors.New("read err") }
func (errReadCloser) Close() error             { return nil }

type manifestFlakyRead struct{}

func (manifestFlakyRead) GetObject(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(errReadCloser{}), nil
}

func TestRun_ReadManifestFails(t *testing.T) {
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 2}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), cfg, manifestFlakyRead{}, nil, "s", t.TempDir(), 1, log); err == nil {
		t.Fatal("expected error")
	}
}

type failGet struct{}

func (failGet) GetObject(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("no object")
}

func TestRun_ManifestGetFails(t *testing.T) {
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 2}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), cfg, failGet{}, nil, "s", t.TempDir(), 1, log); err == nil {
		t.Fatal("expected error")
	}
}

type staticGet struct{ r []byte }

func (g *staticGet) GetObject(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(g.r)), nil
}

func TestRun_DecryptManifestWrongIdentity(t *testing.T) {
	idA, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	recA, err := age.ParseHybridRecipient(idA.Recipient().String())
	if err != nil {
		t.Fatal(err)
	}
	blob, err := envelope.Encrypt([]byte(`{"schema":"cairn.manifest.v1"}`), []age.Recipient{recA})
	if err != nil {
		t.Fatal(err)
	}
	idB, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	identsB, err := age.ParseIdentities(strings.NewReader(idB.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 2}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := staticGet{r: blob}
	if err := Run(context.Background(), cfg, &g, identsB, "s", t.TempDir(), 1, log); err == nil {
		t.Fatal("expected decrypt error")
	}
}

func TestRun_ManifestJSONInvalid(t *testing.T) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	rec, err := age.ParseHybridRecipient(id.Recipient().String())
	if err != nil {
		t.Fatal(err)
	}
	blob, err := envelope.Encrypt([]byte("{"), []age.Recipient{rec})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := age.ParseIdentities(strings.NewReader(id.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 2}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := staticGet{r: blob}
	if err := Run(context.Background(), cfg, &g, ids, "s", t.TempDir(), 1, log); err == nil {
		t.Fatal("expected error")
	}
}

type dualGet struct {
	man []byte
	obj map[string][]byte
}

func (d *dualGet) GetObject(_ context.Context, key string) (io.ReadCloser, error) {
	if strings.HasSuffix(key, "manifest.age") {
		return io.NopCloser(bytes.NewReader(d.man)), nil
	}
	if d.obj != nil {
		if b, ok := d.obj[key]; ok {
			return io.NopCloser(bytes.NewReader(b)), nil
		}
	}
	return nil, errors.New("missing key")
}

func encryptedManifest(t *testing.T, m *manifest.Manifest) ([]byte, []age.Identity, []age.Recipient) {
	t.Helper()
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	rec, err := age.ParseHybridRecipient(id.Recipient().String())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := manifest.MarshalJSONForManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := envelope.Encrypt(raw, []age.Recipient{rec})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := age.ParseIdentities(strings.NewReader(id.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return blob, ids, []age.Recipient{rec}
}

func TestRun_SymlinkMissingTarget(t *testing.T) {
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: "20100101T000000Z-aaaaaaaa", HostID: "h",
		HostOS: "linux", CreatedAt: "2010-01-01T00:00:00Z", CompletedAt: "2010-01-01T00:00:01Z",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "badlink", Type: "symlink", MtimeNs: 1},
		},
	}
	manBlob, idents, _ := encryptedManifest(t, m)
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 2}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &dualGet{man: manBlob}
	if err := Run(context.Background(), cfg, g, idents, m.SnapshotID, t.TempDir(), 1, log); err == nil {
		t.Fatal("expected symlink error")
	}
}

func TestRun_ObjectGetFails(t *testing.T) {
	const snap = "20100202T000000Z-bbbbbbbb"
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "f", Type: "regular", ObjectID: "00000000-0000-4000-8000-000000000001",
				SizePlain: 1, SHA256Plain: "ab", MtimeNs: 1},
		},
	}
	manBlob, idents, _ := encryptedManifest(t, m)
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 2}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &dualGet{man: manBlob, obj: map[string][]byte{}}
	if err := Run(context.Background(), cfg, g, idents, snap, t.TempDir(), 1, log); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_HashMismatch(t *testing.T) {
	const snap = "20100303T000000Z-cccccccc"
	plain := []byte("x")
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "f", Type: "regular", ObjectID: "11111111-1111-4111-8111-111111111111",
				SizePlain: int64(len(plain)), SHA256Plain: "00", MtimeNs: 1},
		},
	}
	manBlob, idents, recs := encryptedManifest(t, m)
	objCipher, err := envelope.Encrypt(plain, recs)
	if err != nil {
		t.Fatal(err)
	}
	objKey := paths.ObjectKey(m.HostID, snap, "11111111-1111-4111-8111-111111111111")

	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 2}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	g := &dualGet{
		man: manBlob,
		obj: map[string][]byte{objKey: objCipher},
	}
	if err := Run(context.Background(), cfg, g, idents, snap, t.TempDir(), 2, log); err == nil {
		t.Fatal("expected hash mismatch")
	}
	if err != nil && !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected hash mismatch error, got: %v", err)
	}
}

func TestRun_DecryptReaderFailsForObject(t *testing.T) {
	const snap = "20100404T000000Z-dddddddd"
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "f", Type: "regular", ObjectID: "22222222-2222-4222-8222-222222222222",
				SizePlain: 1, SHA256Plain: "ab", MtimeNs: 1},
		},
	}
	manBlob, idents, _ := encryptedManifest(t, m)
	objKey := paths.ObjectKey(m.HostID, snap, "22222222-2222-4222-8222-222222222222")
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 2}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &dualGet{man: manBlob, obj: map[string][]byte{objKey: []byte("not-age-ciphertext")}}
	if err := Run(context.Background(), cfg, g, idents, snap, t.TempDir(), 1, log); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_WorkersDefaultFromConfig(t *testing.T) {
	const snap = "20100505T000000Z-eeeeeeee"
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool:        manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files:       []manifest.FileEntry{},
		Stats:       manifest.Stats{},
		Directories: []manifest.DirEntry{{Path: "d", MtimeNs: 1}},
	}
	manBlob, idents, _ := encryptedManifest(t, m)
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 3}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &dualGet{man: manBlob}
	out := t.TempDir()
	if err := Run(context.Background(), cfg, g, idents, snap, out, 0, log); err != nil {
		t.Fatal(err)
	}
}

func u32(v uint32) *uint32 { return &v }

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestRun_TargetRootNotDirectory(t *testing.T) {
	base := t.TempDir()
	blocked := filepath.Join(base, "file")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: "20100909T000000Z-99999999", HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
	}
	manBlob, idents, _ := encryptedManifest(t, m)
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 2}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &dualGet{man: manBlob}
	if err := Run(context.Background(), cfg, g, idents, m.SnapshotID, blocked, 1, log); err == nil {
		t.Fatal("expected mkdir target error")
	}
}

func TestRun_DepthSortAndDirMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory mode")
	}
	const snap = "20101010T101010Z-aaaaaaaa"
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Directories: []manifest.DirEntry{
			{Path: "shallow", MtimeNs: 1},
			{Path: "deep/nested", Mode: u32(0o750), MtimeNs: 2},
		},
	}
	manBlob, idents, _ := encryptedManifest(t, m)
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 2}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), cfg, &dualGet{man: manBlob}, idents, snap, t.TempDir(), 1, log); err != nil {
		t.Fatal(err)
	}
}

func TestRun_SymlinkCreated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink")
	}
	const snap = "20111111T111111Z-bbbbbbbb"
	tgt := "target"
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2011", CompletedAt: "2011",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "link", Type: "symlink", SymlinkTarget: &tgt, MtimeNs: 1},
		},
	}
	manBlob, idents, _ := encryptedManifest(t, m)
	root := filepath.Join(t.TempDir(), "out")
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 2}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), cfg, &dualGet{man: manBlob}, idents, snap, root, 1, log); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(filepath.Join(root, "link")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink %+v err %v", fi, err)
	}
}

func TestRun_FileEntryWithMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode")
	}
	const snap = "20121212T121212Z-cccccccc"
	plain := []byte("modefile")
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	rec, err := age.ParseHybridRecipient(id.Recipient().String())
	if err != nil {
		t.Fatal(err)
	}
	objCipher, err := envelope.Encrypt(plain, []age.Recipient{rec})
	if err != nil {
		t.Fatal(err)
	}
	idents, err := age.ParseIdentities(strings.NewReader(id.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2012", CompletedAt: "2012",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "mf", Type: "regular", ObjectID: "44444444-4444-4444-8444-444444444444",
				SizePlain: int64(len(plain)), SHA256Plain: sha256Hex(plain),
				Mode: u32(0o640), MtimeNs: 1},
		},
	}
	raw, err := manifest.MarshalJSONForManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	manBlob, err := envelope.Encrypt(raw, []age.Recipient{rec})
	if err != nil {
		t.Fatal(err)
	}
	objKey := paths.ObjectKey("h", snap, "44444444-4444-4444-8444-444444444444")
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 2}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &dualGet{man: manBlob, obj: map[string][]byte{objKey: objCipher}}
	if err := Run(context.Background(), cfg, g, idents, snap, t.TempDir(), 2, log); err != nil {
		t.Fatal(err)
	}
}

func TestRun_SkipsNonRegularFileEntries(t *testing.T) {
	const snap = "20100606T000000Z-ffffffff"
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "skipme", Type: "device", MtimeNs: 1},
		},
	}
	manBlob, idents, _ := encryptedManifest(t, m)
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 2}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &dualGet{man: manBlob}
	if err := Run(context.Background(), cfg, g, idents, snap, t.TempDir(), 1, log); err != nil {
		t.Fatal(err)
	}
}

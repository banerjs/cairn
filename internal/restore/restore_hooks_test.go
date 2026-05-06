package restore

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	appcfg "github.com/banerjs/cairn/internal/config"
	"github.com/banerjs/cairn/internal/envelope"
	"github.com/banerjs/cairn/internal/manifest"
	"github.com/banerjs/cairn/internal/paths"
)

func patchMkdirAllNeedle(t *testing.T, needle string, ferr error) {
	t.Helper()
	prev := mkdirAllRestore
	mkdirAllRestore = func(path string, perm fs.FileMode) error {
		if strings.Contains(path, needle) {
			return ferr
		}
		return prev(path, perm)
	}
	t.Cleanup(func() { mkdirAllRestore = prev })
}

func patchRemoveNeedle(t *testing.T, needle string, ferr error) {
	t.Helper()
	prev := removeRestore
	removeRestore = func(name string) error {
		if strings.Contains(name, needle) {
			return ferr
		}
		return prev(name)
	}
	t.Cleanup(func() { removeRestore = prev })
}

func patchCreateNeedle(t *testing.T, needle string, ferr error) {
	t.Helper()
	prev := createRestore
	createRestore = func(name string) (*os.File, error) {
		if strings.Contains(name, needle) {
			return nil, ferr
		}
		return prev(name)
	}
	t.Cleanup(func() { createRestore = prev })
}

func patchRenameNeedle(t *testing.T, needle string, ferr error) {
	t.Helper()
	prev := renameRestore
	renameRestore = func(oldpath, newpath string) error {
		if strings.Contains(newpath, needle) {
			return ferr
		}
		return prev(oldpath, newpath)
	}
	t.Cleanup(func() { renameRestore = prev })
}

func patchChmodNeedle(t *testing.T, needle string, ferr error) {
	t.Helper()
	prev := chmodRestore
	chmodRestore = func(name string, mode fs.FileMode) error {
		if strings.Contains(name, needle) {
			return ferr
		}
		return prev(name, mode)
	}
	t.Cleanup(func() { chmodRestore = prev })
}

func patchChtimesNeedle(t *testing.T, needle string, ferr error) {
	t.Helper()
	prev := chtimesRestore
	chtimesRestore = func(name string, atime time.Time, mtime time.Time) error {
		if strings.Contains(name, needle) {
			return ferr
		}
		return prev(name, atime, mtime)
	}
	t.Cleanup(func() { chtimesRestore = prev })
}

func patchSymlink(t *testing.T, stub func(oldname, newname string) error) {
	t.Helper()
	prev := symlinkRestore
	symlinkRestore = stub
	t.Cleanup(func() { symlinkRestore = prev })
}

func patchChownNeedle(t *testing.T, needle string, ferr error) {
	t.Helper()
	prev := chownRestore
	chownRestore = func(name string, uid, gid int) error {
		if strings.Contains(name, needle) {
			return ferr
		}
		return prev(name, uid, gid)
	}
	t.Cleanup(func() { chownRestore = prev })
}

func patchEuid(t *testing.T, u int) {
	t.Helper()
	prev := euidRestore
	euidRestore = func() int { return u }
	t.Cleanup(func() { euidRestore = prev })
}

func patchGoos(t *testing.T, ostype string) {
	t.Helper()
	prev := goosRestore
	goosRestore = func() string { return ostype }
	t.Cleanup(func() { goosRestore = prev })
}

func i64(v int64) *int64 { return &v }

func TestRun_HookMkdirRestoreDirFails(t *testing.T) {
	const snap = "20170101T000000Z-hookdir"
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool:        manifest.ToolInfo{Name: "cairn", Version: "t"},
		Directories: []manifest.DirEntry{{Path: "dir-mkdir-fail/me", MtimeNs: 1}},
	}
	manBlob, idents, _ := encryptedManifest(t, m)
	patchMkdirAllNeedle(t, "dir-mkdir-fail", errors.New("mkdir boom"))
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 1}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), cfg, &dualGet{man: manBlob}, idents, snap, t.TempDir(), 1, log); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_HookChtimesDirDebug(t *testing.T) {
	const snap = "20180101T000000Z-chtdir"
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool:        manifest.ToolInfo{Name: "cairn", Version: "t"},
		Directories: []manifest.DirEntry{{Path: "chtimes-dir-marker", MtimeNs: 1}},
	}
	manBlob, idents, _ := encryptedManifest(t, m)
	patchChtimesNeedle(t, "chtimes-dir-marker", errors.New("no chtimes"))
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 1}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), cfg, &dualGet{man: manBlob}, idents, snap, t.TempDir(), 1, log); err != nil {
		t.Fatal(err)
	}
}

func TestRun_HookChownDirDebug(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix chown")
	}
	const snap = "20190101T000000Z-chowndir"
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool:        manifest.ToolInfo{Name: "cairn", Version: "t"},
		Directories: []manifest.DirEntry{{Path: "d-chown-err", MtimeNs: 1, UID: i64(12), GID: i64(34)}},
	}
	manBlob, idents, _ := encryptedManifest(t, m)
	patchGoos(t, "linux")
	patchEuid(t, 0)
	patchChownNeedle(t, "d-chown-err", errors.New("chown dir"))
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 1}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), cfg, &dualGet{man: manBlob}, idents, snap, t.TempDir(), 1, log); err != nil {
		t.Fatal(err)
	}
}

func TestRun_HookSymlinkRemoveFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink")
	}
	tgt := "t"
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: "20200101T000000Z-symlrm", HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "symlink-rm-fail", Type: "symlink", SymlinkTarget: &tgt, MtimeNs: 1},
		},
	}
	manBlob, idents, _ := encryptedManifest(t, m)
	root := t.TempDir()
	bad := filepath.Join(root, "symlink-rm-fail")
	if err := os.WriteFile(bad, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	patchRemoveNeedle(t, "symlink-rm-fail", errors.New("rm blocked"))
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 1}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), cfg, &dualGet{man: manBlob}, idents, m.SnapshotID, root, 1, log); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_HookSymlinkMkdirParentFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink")
	}
	tgt := "t"
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: "20210101T000000Z-symkm", HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "sym-parent-mkdir-fail/x", Type: "symlink", SymlinkTarget: &tgt, MtimeNs: 1},
		},
	}
	manBlob, idents, _ := encryptedManifest(t, m)
	patchMkdirAllNeedle(t, "sym-parent-mkdir-fail", errors.New("mkdir parent"))
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 1}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), cfg, &dualGet{man: manBlob}, idents, m.SnapshotID, t.TempDir(), 1, log); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_HookSymlinkSyscallDebug(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink")
	}
	tgt := "t"
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: "20220101T000000Z-symln", HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "symlink-syscall-fail", Type: "symlink", SymlinkTarget: &tgt, MtimeNs: 1},
		},
	}
	manBlob, idents, _ := encryptedManifest(t, m)
	patchSymlink(t, func(_, _ string) error { return errors.New("symlink nope") })
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 1}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), cfg, &dualGet{man: manBlob}, idents, m.SnapshotID, t.TempDir(), 1, log); err != nil {
		t.Fatal(err)
	}
}

func TestRun_HookFileMkdirParentFails(t *testing.T) {
	const snap = "20230101T000000Z-fmkd"
	plain := []byte("p")
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "file-mkdir-fail/x", Type: "regular", ObjectID: "55555555-5555-4555-8555-555555555555",
				SizePlain: int64(len(plain)), SHA256Plain: sha256Hex(plain), MtimeNs: 1},
		},
	}
	manBlob, idents, recs := encryptedManifest(t, m)
	objCipher, err := envelope.Encrypt(plain, recs)
	if err != nil {
		t.Fatal(err)
	}
	patchMkdirAllNeedle(t, "file-mkdir-fail", errors.New("mkdir"))
	objKey := paths.ObjectKey("h", snap, "55555555-5555-4555-8555-555555555555")
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 1}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &dualGet{man: manBlob, obj: map[string][]byte{objKey: objCipher}}
	if err := Run(context.Background(), cfg, g, idents, snap, t.TempDir(), 1, log); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_HookFileCreateFails(t *testing.T) {
	const snap = "20240101T000000Z-fcrt"
	plain := []byte("c")
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "create-fail.bin", Type: "regular", ObjectID: "66666666-6666-4666-8666-666666666666",
				SizePlain: int64(len(plain)), SHA256Plain: sha256Hex(plain), MtimeNs: 1},
		},
	}
	manBlob, idents, recs := encryptedManifest(t, m)
	objCipher, err := envelope.Encrypt(plain, recs)
	if err != nil {
		t.Fatal(err)
	}
	patchCreateNeedle(t, ".partial", errors.New("cannot create"))
	objKey := paths.ObjectKey("h", snap, "66666666-6666-4666-8666-666666666666")
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 1}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &dualGet{man: manBlob, obj: map[string][]byte{objKey: objCipher}}
	if err := Run(context.Background(), cfg, g, idents, snap, t.TempDir(), 1, log); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_HookFileCopyFails(t *testing.T) {
	const snap = "20250101T000000Z-fcpy"
	plain := []byte("q")
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "copy-fail.txt", Type: "regular", ObjectID: "77777777-7777-4777-8777-777777777777",
				SizePlain: int64(len(plain)), SHA256Plain: sha256Hex(plain), MtimeNs: 1},
		},
	}
	manBlob, idents, recs := encryptedManifest(t, m)
	objCipher, err := envelope.Encrypt(plain, recs)
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("copy stopped")
	prev := copyRestore
	copyRestore = func(dst io.Writer, src io.Reader) (int64, error) {
		_, _ = io.Copy(dst, src)
		return 0, boom
	}
	t.Cleanup(func() { copyRestore = prev })
	objKey := paths.ObjectKey("h", snap, "77777777-7777-4777-8777-777777777777")
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 1}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &dualGet{man: manBlob, obj: map[string][]byte{objKey: objCipher}}
	runErr := Run(context.Background(), cfg, g, idents, snap, t.TempDir(), 1, log)
	if !errors.Is(runErr, boom) {
		t.Fatalf("expected copy error, got %v", runErr)
	}
}

func TestRun_HookFileChmodDebug(t *testing.T) {
	const snap = "20260101T000000Z-chm"
	plain := []byte(" chmod ")
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "chmod-marker/y", Type: "regular", ObjectID: "88888888-8888-4888-8888-888888888888",
				SizePlain: int64(len(plain)), SHA256Plain: sha256Hex(plain), MtimeNs: 1},
		},
	}
	manBlob, idents, recs := encryptedManifest(t, m)
	objCipher, err := envelope.Encrypt(plain, recs)
	if err != nil {
		t.Fatal(err)
	}
	patchChmodNeedle(t, filepath.Join("chmod-marker", "y.partial"), errors.New("chmod err"))
	objKey := paths.ObjectKey("h", snap, "88888888-8888-4888-8888-888888888888")
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 1}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &dualGet{man: manBlob, obj: map[string][]byte{objKey: objCipher}}
	if err := Run(context.Background(), cfg, g, idents, snap, t.TempDir(), 1, log); err != nil {
		t.Fatal(err)
	}
}

func TestRun_HookFileRenameFails(t *testing.T) {
	const snap = "20270101T000000Z-rnm"
	plain := []byte("r")
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "rename-marker/z", Type: "regular", ObjectID: "99999999-9999-4999-8999-999999999999",
				SizePlain: int64(len(plain)), SHA256Plain: sha256Hex(plain), MtimeNs: 1},
		},
	}
	manBlob, idents, recs := encryptedManifest(t, m)
	objCipher, err := envelope.Encrypt(plain, recs)
	if err != nil {
		t.Fatal(err)
	}
	patchRenameNeedle(t, filepath.Join("rename-marker", "z"), errors.New("rename err"))
	objKey := paths.ObjectKey("h", snap, "99999999-9999-4999-8999-999999999999")
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 1}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &dualGet{man: manBlob, obj: map[string][]byte{objKey: objCipher}}
	if err := Run(context.Background(), cfg, g, idents, snap, t.TempDir(), 1, log); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_HookFileChtimesDebug(t *testing.T) {
	const snap = "20280101T000000Z-cht"
	plain := []byte("ct")
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "chtimes-file-marker/f", Type: "regular", ObjectID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
				SizePlain: int64(len(plain)), SHA256Plain: sha256Hex(plain), MtimeNs: 1},
		},
	}
	manBlob, idents, recs := encryptedManifest(t, m)
	objCipher, err := envelope.Encrypt(plain, recs)
	if err != nil {
		t.Fatal(err)
	}
	patchChtimesNeedle(t, filepath.Join("chtimes-file-marker", "f"), errors.New("chtimes file"))
	objKey := paths.ObjectKey("h", snap, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 1}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &dualGet{man: manBlob, obj: map[string][]byte{objKey: objCipher}}
	if err := Run(context.Background(), cfg, g, idents, snap, t.TempDir(), 1, log); err != nil {
		t.Fatal(err)
	}
}

func TestRun_HookFileChownWhenEuidZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix chown")
	}
	const snap = "20290101T000000Z-sown"
	plain := []byte("own")
	m := &manifest.Manifest{
		Schema: manifest.SchemaV1, SnapshotID: snap, HostID: "h",
		HostOS: "linux", CreatedAt: "2010", CompletedAt: "2010",
		Tool: manifest.ToolInfo{Name: "cairn", Version: "t"},
		Files: []manifest.FileEntry{
			{Path: "chown-file/x", Type: "regular", ObjectID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
				SizePlain: int64(len(plain)), SHA256Plain: sha256Hex(plain), MtimeNs: 1, UID: i64(7), GID: i64(8)},
		},
	}
	manBlob, idents, recs := encryptedManifest(t, m)
	objCipher, err := envelope.Encrypt(plain, recs)
	if err != nil {
		t.Fatal(err)
	}
	patchGoos(t, "linux")
	patchEuid(t, 0)
	objKey := paths.ObjectKey("h", snap, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	cfg := &appcfg.Config{HostID: "h", Backup: appcfg.BackupConfig{Parallelism: 1}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := &dualGet{man: manBlob, obj: map[string][]byte{objKey: objCipher}}
	if err := Run(context.Background(), cfg, g, idents, snap, t.TempDir(), 1, log); err != nil {
		t.Fatal(err)
	}
}

package backup

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	appcfg "github.com/banerjs/cairn/internal/config"
	"github.com/banerjs/cairn/internal/envelope"
	"github.com/banerjs/cairn/internal/ignore"
	"github.com/banerjs/cairn/internal/manifest"
	"github.com/banerjs/cairn/internal/s3store"
	"github.com/banerjs/cairn/internal/s3test"
	"github.com/banerjs/cairn/internal/snapshotid"

	"filippo.io/age"
)

func backupCfg(t *testing.T, src string) (*appcfg.Config, []age.Recipient) {
	t.Helper()
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	rec, err := age.ParseHybridRecipient(id.Recipient().String())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &appcfg.Config{
		HostID: "h",
		S3: appcfg.S3Config{
			Bucket:       "b",
			Region:       "us-east-1",
			StorageClass: "STANDARD",
		},
		Encryption: appcfg.EncryptionConfig{Recipients: []string{id.Recipient().String()}},
		Backup:     appcfg.BackupConfig{SourceRoots: []string{src}, Parallelism: 1},
	}
	return cfg, []age.Recipient{rec}
}

func resetRunHooks() {
	cleanupPartialSnapshotsFn = CleanupPartialSnapshots
	snapshotIDNewFn = snapshotid.New
	pathAbsFn = filepath.Abs
	relPathFn = filepath.Rel
	ignoreCompileFn = ignore.Compile
	walkDirFn = filepath.WalkDir
	readlinkFn = os.Readlink
	statFn = os.Stat
	runtimeGOOSFn = defaultRuntimeGOOS
	nowUTCFn = defaultNowUTC
	marshalManifestFn = manifest.MarshalJSONForManifest
	encryptManifestFn = envelope.Encrypt
	rebuildIndexFn = RebuildIndex
	openFileFn = os.Open
	encryptReaderFn = envelope.EncryptReader
	newUUIDFn = defaultNewUUID
}

func TestRun_EarlyHookErrors(t *testing.T) {
	t.Cleanup(resetRunHooks)
	src := t.TempDir()
	cfg, recips := backupCfg(t, src)
	st, cleanup := s3test.NewStore(t, "bk-run-hooks")
	defer cleanup()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	cleanupPartialSnapshotsFn = func(context.Context, partialStore, string, time.Duration, *slog.Logger) error {
		return errors.New("gc")
	}
	if err := Run(ctx, cfg, st, recips, "", 0, log); err == nil || !strings.Contains(err.Error(), "partial gc") {
		t.Fatalf("err=%v", err)
	}
	cleanupPartialSnapshotsFn = CleanupPartialSnapshots

	snapshotIDNewFn = func() (string, error) { return "", errors.New("sid") }
	if err := Run(ctx, cfg, st, recips, "", 0, log); err == nil || !strings.Contains(err.Error(), "sid") {
		t.Fatalf("err=%v", err)
	}
	snapshotIDNewFn = snapshotid.New

	pathAbsFn = func(string) (string, error) { return "", errors.New("abs") }
	if err := Run(ctx, cfg, st, recips, "", 0, log); err == nil || !strings.Contains(err.Error(), "root") {
		t.Fatalf("err=%v", err)
	}
	pathAbsFn = filepath.Abs

	ignoreCompileFn = func(string, []string, []string) (*ignore.Matcher, error) { return nil, errors.New("ign") }
	if err := Run(ctx, cfg, st, recips, "", 0, log); err == nil || !strings.Contains(err.Error(), "ignore") {
		t.Fatalf("err=%v", err)
	}
	ignoreCompileFn = ignore.Compile

	walkDirFn = func(string, fs.WalkDirFunc) error { return errors.New("walk") }
	if err := Run(ctx, cfg, st, recips, "", 0, log); err == nil || !strings.Contains(err.Error(), "walk") {
		t.Fatalf("err=%v", err)
	}
}

func TestRun_ManifestAndIndexHookErrorsAndHostOSFallback(t *testing.T) {
	t.Cleanup(resetRunHooks)
	src := t.TempDir()
	cfg, recips := backupCfg(t, src)
	st, cleanup := s3test.NewStore(t, "bk-run-hooks-2")
	defer cleanup()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	marshalManifestFn = func(*manifest.Manifest) ([]byte, error) { return nil, errors.New("marshal") }
	if err := Run(ctx, cfg, st, recips, "", 0, log); err == nil || !strings.Contains(err.Error(), "marshal manifest") {
		t.Fatalf("err=%v", err)
	}
	marshalManifestFn = manifest.MarshalJSONForManifest

	encryptManifestFn = func([]byte, []age.Recipient) ([]byte, error) { return nil, errors.New("enc") }
	if err := Run(ctx, cfg, st, recips, "", 0, log); err == nil || !strings.Contains(err.Error(), "encrypt manifest") {
		t.Fatalf("err=%v", err)
	}
	encryptManifestFn = envelope.Encrypt

	rebuildIndexFn = func(context.Context, *s3store.Store, string, []age.Recipient, string, *slog.Logger) error {
		return errors.New("idx")
	}
	if err := Run(ctx, cfg, st, recips, "", 0, log); err == nil || !strings.Contains(err.Error(), "index") {
		t.Fatalf("err=%v", err)
	}
	rebuildIndexFn = RebuildIndex

	runtimeGOOSFn = func() string { return "plan9" }
	marshalManifestFn = func(m *manifest.Manifest) ([]byte, error) {
		if m.HostOS != "linux" {
			t.Fatalf("host os = %q", m.HostOS)
		}
		return nil, errors.New("stop")
	}
	if err := Run(ctx, cfg, st, recips, "", 0, log); err == nil || !strings.Contains(err.Error(), "marshal manifest") {
		t.Fatalf("err=%v", err)
	}
}

func TestUploadRegular_Errors(t *testing.T) {
	t.Cleanup(resetRunHooks)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()
	j := walkJob{relSlash: "a.txt", fullPath: filepath.Join(t.TempDir(), "missing")}
	st, cleanup := s3test.NewStore(t, "bk-upload-errors")
	defer cleanup()

	if _, err := uploadRegular(ctx, st, "h", "s", j, nil, "STANDARD", log); err == nil || !strings.Contains(err.Error(), "open") {
		t.Fatalf("err=%v", err)
	}

	src := filepath.Join(t.TempDir(), "x.txt")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	j = walkJob{relSlash: "x.txt", fullPath: src, info: fi}
	if _, err := uploadRegular(ctx, st, "h", "s", j, nil, "STANDARD", log); err == nil {
		t.Fatal("expected encrypt reader error")
	}

	cfg := aws.Config{
		Region: "us-east-1",
		HTTPClient: &http.Client{
			Transport: rtFunc(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded }),
		},
	}
	cl := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://127.0.0.1:2")
		o.UsePathStyle = true
	})
	badStore := s3store.NewWithClient(cl, "b")
	id, _ := age.GenerateHybridIdentity()
	rec, _ := age.ParseHybridRecipient(id.Recipient().String())
	if _, err := uploadRegular(ctx, badStore, "h", "s", j, []age.Recipient{rec}, "STANDARD", log); err == nil {
		t.Fatal("expected put error")
	}
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRun_SymlinkAndFilterBranches(t *testing.T) {
	t.Cleanup(resetRunHooks)
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs")
	}
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "excluded"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "excluded", "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "dir-target"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "regular.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("regular.txt", filepath.Join(src, "s-file")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("dir-target", filepath.Join(src, "s-dir")); err != nil {
		t.Fatal(err)
	}

	cfg, recips := backupCfg(t, src)
	cfg.Backup.Excludes = []string{"excluded/"}
	cfg.Backup.FollowSymlinks = ptrBool(false)

	st, cleanup := s3test.NewStore(t, "bk-symlink-branches")
	defer cleanup()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), cfg, st, recips, "", 2, log); err != nil {
		t.Fatal(err)
	}

	cfg2, recips2 := backupCfg(t, src)
	cfg2.Backup.FollowSymlinks = ptrBool(true)
	st2, cleanup2 := s3test.NewStore(t, "bk-symlink-follow")
	defer cleanup2()
	if err := Run(context.Background(), cfg2, st2, recips2, "STANDARD_IA", 1, log); err != nil {
		t.Fatal(err)
	}
}

func TestRun_ReadlinkAndStatSymlinkErrors(t *testing.T) {
	t.Cleanup(resetRunHooks)
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs")
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("f.txt", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	cfg, recips := backupCfg(t, src)
	cfg.Backup.FollowSymlinks = ptrBool(false)
	st, cleanup := s3test.NewStore(t, "bk-symlink-errs")
	defer cleanup()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	readlinkFn = func(string) (string, error) { return "", errors.New("readlink") }
	if err := Run(context.Background(), cfg, st, recips, "", 1, log); err != nil {
		t.Fatal(err)
	}

	readlinkFn = os.Readlink
	statFn = func(string) (os.FileInfo, error) { return nil, errors.New("stat") }
	if err := Run(context.Background(), cfg, st, recips, "", 1, log); err != nil {
		t.Fatal(err)
	}
}

func ptrBool(v bool) *bool { return &v }

type stubDirEnt struct {
	isDir bool
	info  fs.FileInfo
	iErr  error
}

func (s stubDirEnt) Name() string               { return "n" }
func (s stubDirEnt) IsDir() bool                { return s.isDir }
func (s stubDirEnt) Type() fs.FileMode          { return s.info.Mode().Type() }
func (s stubDirEnt) Info() (fs.FileInfo, error) { return s.info, s.iErr }

type stubInfo struct {
	mode fs.FileMode
	size int64
}

func (s stubInfo) Name() string       { return "x" }
func (s stubInfo) Size() int64        { return s.size }
func (s stubInfo) Mode() fs.FileMode  { return s.mode }
func (s stubInfo) ModTime() time.Time { return time.Now() }
func (s stubInfo) IsDir() bool        { return s.mode.IsDir() }
func (s stubInfo) Sys() any           { return nil }

func TestRun_WalkCallbackBranches(t *testing.T) {
	t.Cleanup(resetRunHooks)
	src := t.TempDir()
	cfg, recips := backupCfg(t, src)
	st, cleanup := s3test.NewStore(t, "bk-walk-callback")
	defer cleanup()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	walkDirFn = func(root string, fn fs.WalkDirFunc) error {
		_ = fn(filepath.Join(root, "x"), nil, errors.New("walk-cb"))
		return errors.New("walk-cb")
	}
	if err := Run(ctx, cfg, st, recips, "", 1, log); err == nil || !strings.Contains(err.Error(), "walk") {
		t.Fatalf("err=%v", err)
	}

	walkDirFn = func(root string, fn fs.WalkDirFunc) error {
		relPathFn = func(string, string) (string, error) { return "", errors.New("rel") }
		defer func() { relPathFn = filepath.Rel }()
		return fn(filepath.Join(root, "x"), stubDirEnt{isDir: true, info: stubInfo{mode: fs.ModeDir}}, nil)
	}
	if err := Run(ctx, cfg, st, recips, "", 1, log); err == nil || !strings.Contains(err.Error(), "walk") {
		t.Fatalf("err=%v", err)
	}

	walkDirFn = func(root string, fn fs.WalkDirFunc) error {
		return fn(filepath.Join(root, "d"), stubDirEnt{isDir: true, iErr: errors.New("info"), info: stubInfo{mode: fs.ModeDir}}, nil)
	}
	if err := Run(ctx, cfg, st, recips, "", 1, log); err == nil || !strings.Contains(err.Error(), "walk") {
		t.Fatalf("err=%v", err)
	}

	walkDirFn = func(root string, fn fs.WalkDirFunc) error {
		_ = fn(filepath.Join(root, "skip.me"), stubDirEnt{isDir: false, info: stubInfo{mode: 0}}, nil)
		return nil
	}
	cfg.Backup.Excludes = []string{"*.me"}
	if err := Run(ctx, cfg, st, recips, "", 1, log); err != nil {
		t.Fatal(err)
	}
	cfg.Backup.Excludes = nil

	walkDirFn = func(root string, fn fs.WalkDirFunc) error {
		return fn(filepath.Join(root, "f"), stubDirEnt{isDir: false, iErr: errors.New("info"), info: stubInfo{mode: 0}}, nil)
	}
	if err := Run(ctx, cfg, st, recips, "", 1, log); err == nil || !strings.Contains(err.Error(), "walk") {
		t.Fatalf("err=%v", err)
	}

	walkDirFn = func(root string, fn fs.WalkDirFunc) error {
		_ = fn(filepath.Join(root, "dev"), stubDirEnt{isDir: false, info: stubInfo{mode: fs.ModeDevice}}, nil)
		_ = fn(filepath.Join(root, "nonreg"), stubDirEnt{isDir: false, info: stubInfo{mode: fs.ModeDir}}, nil)
		return nil
	}
	if err := Run(ctx, cfg, st, recips, "", 1, log); err != nil {
		t.Fatal(err)
	}
}

func TestRun_UploadErrorAndPutManifestError(t *testing.T) {
	t.Cleanup(resetRunHooks)
	src := t.TempDir()
	file := filepath.Join(src, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, recips := backupCfg(t, src)
	st, cleanup := s3test.NewStore(t, "bk-upload-error")
	defer cleanup()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	encryptReaderFn = func(io.Reader, []age.Recipient, int) (io.Reader, error) { return nil, errors.New("enc-reader") }
	if err := Run(context.Background(), cfg, st, recips, "", 1, log); err == nil || !strings.Contains(err.Error(), "upload") {
		t.Fatalf("err=%v", err)
	}

	resetRunHooks()
	emptySrc := t.TempDir()
	cfg2, recips2 := backupCfg(t, emptySrc)
	badCfg := aws.Config{
		Region: "us-east-1",
		HTTPClient: &http.Client{
			Transport: rtFunc(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded }),
		},
	}
	badClient := s3.NewFromConfig(badCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://127.0.0.1:2")
		o.UsePathStyle = true
	})
	badStore := s3store.NewWithClient(badClient, "b")
	cleanupPartialSnapshotsFn = func(context.Context, partialStore, string, time.Duration, *slog.Logger) error { return nil }
	if err := Run(context.Background(), cfg2, badStore, recips2, "", 1, log); err == nil || !strings.Contains(err.Error(), "put manifest") {
		t.Fatalf("err=%v", err)
	}
}

func TestRun_HookFunctionsInvoked(t *testing.T) {
	t.Cleanup(resetRunHooks)
	resetRunHooks()
	if runtimeGOOSFn() == "" {
		t.Fatal("empty goos")
	}
	if nowUTCFn().IsZero() {
		t.Fatal("zero time")
	}
	if newUUIDFn() == "" {
		t.Fatal("empty uuid")
	}
}

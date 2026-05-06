package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"

	appcfg "github.com/banerjs/cairn/internal/config"
	"github.com/banerjs/cairn/internal/prune"
	"github.com/banerjs/cairn/internal/restore"
	"github.com/banerjs/cairn/internal/s3store"
	"github.com/banerjs/cairn/internal/snapshots"
	"github.com/banerjs/cairn/internal/status"
	"github.com/banerjs/cairn/internal/verifycmd"

	"filippo.io/age"
)

func runMainCaptured(t *testing.T, argv []string) int {
	t.Helper()
	oldArgs := append([]string(nil), os.Args...)
	os.Args = argv
	oldExit := exitHook
	var code int
	exitHook = func(c int) { code = c }
	t.Cleanup(func() {
		exitHook = oldExit
		os.Args = oldArgs
	})
	main()
	return code
}

func TestDefaultExitDelegatesToOsExit(t *testing.T) {
	var got int
	prev := osExit
	osExit = func(code int) { got = code }
	defer func() { osExit = prev }()
	defaultExit(42)
	if got != 42 {
		t.Fatalf("got %d", got)
	}
}

func TestMain_ExitViaHook(t *testing.T) {
	if c := runMainCaptured(t, []string{"cairn", "--help"}); c != 0 {
		t.Fatalf("code=%d want 0", c)
	}
}

func TestDefaultOpenStore_EmptyRegion(t *testing.T) {
	_, err := defaultOpenStore(context.Background(), &appcfg.Config{S3: appcfg.S3Config{Bucket: "b"}})
	if err == nil || !strings.Contains(err.Error(), "region") {
		t.Fatalf("err=%v", err)
	}
}

func TestDefaultOpenStore_EmptyBucket(t *testing.T) {
	prev := awsLoadForStore
	awsLoadForStore = func(ctx context.Context, region string) (aws.Config, error) {
		return aws.Config{}, nil
	}
	defer func() { awsLoadForStore = prev }()
	_, err := defaultOpenStore(context.Background(), &appcfg.Config{S3: appcfg.S3Config{Region: "us-east-1"}})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "bucket") {
		t.Fatalf("err=%v", err)
	}
}

func hybridRecipientLine(t *testing.T) string {
	t.Helper()
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return id.Recipient().String()
}

func mustWriteConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(strings.TrimPrefix(body, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustWriteIdentity(t *testing.T) string {
	t.Helper()
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "id.age")
	if err := os.WriteFile(p, []byte(id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRun_KeygenBadFlag_ContinuesError(t *testing.T) {
	if code := run([]string{"cairn", "keygen", "--output", filepath.Join(t.TempDir(), "k.age"), "--nope-flag"}); code != 1 {
		t.Fatalf("code=%d want 1", code)
	}
}

func TestRun_KeygenHookError(t *testing.T) {
	prev := keygenRunHook
	keygenRunHook = func(string) (string, error) { return "", errors.New("keygen wired") }
	defer func() { keygenRunHook = prev }()
	out := filepath.Join(t.TempDir(), "k.age")
	if code := run([]string{"cairn", "keygen", "--output", out}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_BackupBadFlag(t *testing.T) {
	rec := hybridRecipientLine(t)
	root := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := mustWriteConfig(t, fmt.Sprintf(`
host_id = "h"
[s3]
bucket = "b"
region = "us-east-1"
[encryption]
recipients = [%q]
[backup]
source_roots = [%q]
`, rec, root))
	if code := run([]string{"cairn", "backup", cfg, "--zzz"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Backup_ConfigLoadError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing.toml")
	if code := run([]string{"cairn", "backup", p}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Backup_RecipientsParseError(t *testing.T) {
	cfg := mustWriteConfig(t, `
host_id = "h"
[s3]
bucket = "b"
region = "us-east-1"
[encryption]
recipients = ["not-any-format"]
[backup]
source_roots = ["/"]
`)
	if code := run([]string{"cairn", "backup", cfg}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Backup_RunHookError(t *testing.T) {
	rec := hybridRecipientLine(t)
	root := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := mustWriteConfig(t, fmt.Sprintf(`
host_id = "h"
[s3]
bucket = "b"
region = "us-east-1"
[encryption]
recipients = [%q]
[backup]
source_roots = [%q]
`, rec, root))
	prevB := backupRunHook
	prevO := openStoreHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	backupRunHook = func(context.Context, *appcfg.Config, *s3store.Store, []age.Recipient, string, int, *slog.Logger) error {
		return errors.New("boom")
	}
	defer func() {
		backupRunHook = prevB
		openStoreHook = prevO
	}()
	if code := run([]string{"cairn", "backup", cfgPath}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Backup_Success_Stubs(t *testing.T) {
	rec := hybridRecipientLine(t)
	root := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := mustWriteConfig(t, fmt.Sprintf(`
host_id = "h"
[s3]
bucket = "b"
region = "us-east-1"
[encryption]
recipients = [%q]
[backup]
source_roots = [%q]
`, rec, root))
	prevB := backupRunHook
	prevO := openStoreHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	backupRunHook = func(context.Context, *appcfg.Config, *s3store.Store, []age.Recipient, string, int, *slog.Logger) error {
		return nil
	}
	defer func() {
		backupRunHook = prevB
		openStoreHook = prevO
	}()
	if code := run([]string{"cairn", "backup", cfgPath}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func baseCfgToml(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf(`
host_id = "h"
[s3]
bucket = "b"
region = "us-east-1"
[encryption]
recipients = [%q]
`, hybridRecipientLine(t))
}

func restoreCfgBody(t *testing.T, identityPath string) string {
	return baseCfgToml(t) + fmt.Sprintf(`
identity_file = %q
`, identityPath)
}

func TestRun_RestoreBadFlag(t *testing.T) {
	idPath := mustWriteIdentity(t)
	cfg := mustWriteConfig(t, restoreCfgBody(t, idPath))
	if code := run([]string{"cairn", "restore", "--config", cfg, "--target", t.TempDir(), "--badflag", "snap"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Restore_ConfigLoadMissing(t *testing.T) {
	badCfg := filepath.Join(t.TempDir(), "nope.toml")
	if code := run([]string{"cairn", "restore", "--config", badCfg, "--target", t.TempDir(), "snap1"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Restore_NoIdentityConfigured(t *testing.T) {
	cfg := mustWriteConfig(t, baseCfgToml(t))
	prev := getenv
	getenv = func(k string) string {
		if k == "CAIRN_IDENTITY_FILE" {
			return ""
		}
		return prev(k)
	}
	defer func() { getenv = prev }()
	if code := run([]string{"cairn", "restore", "--config", cfg, "--target", t.TempDir(), "snap"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Restore_BadIdentityFile(t *testing.T) {
	rec := hybridRecipientLine(t)
	badId := filepath.Join(t.TempDir(), "bad.age")
	if err := os.WriteFile(badId, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := mustWriteConfig(t, fmt.Sprintf(`
host_id = "h"
[s3]
bucket = "b"
region = "us-east-1"
[encryption]
recipients = [%q]
identity_file = %q
`, rec, badId))
	if code := run([]string{"cairn", "restore", "--config", cfg, "--target", t.TempDir(), "snap"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Restore_OpenStoreError(t *testing.T) {
	cfg := mustWriteConfig(t, restoreCfgBody(t, mustWriteIdentity(t)))
	prev := openStoreHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) {
		return nil, errors.New("open fail")
	}
	defer func() { openStoreHook = prev }()
	if code := run([]string{"cairn", "restore", "--config", cfg, "--target", t.TempDir(), "snap"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Restore_RunHookError(t *testing.T) {
	cfg := mustWriteConfig(t, restoreCfgBody(t, mustWriteIdentity(t)))
	prevO := openStoreHook
	prevR := restoreRunHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	restoreRunHook = func(context.Context, *appcfg.Config, restore.ObjectGetter, []age.Identity, string, string, int, *slog.Logger) error {
		return errors.New("rst")
	}
	defer func() {
		openStoreHook = prevO
		restoreRunHook = prevR
	}()
	if code := run([]string{"cairn", "restore", "--config", cfg, "--target", t.TempDir(), "snap"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Restore_Success_Stubs(t *testing.T) {
	cfg := mustWriteConfig(t, restoreCfgBody(t, mustWriteIdentity(t)))
	prevO := openStoreHook
	prevR := restoreRunHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	restoreRunHook = func(context.Context, *appcfg.Config, restore.ObjectGetter, []age.Identity, string, string, int, *slog.Logger) error {
		return nil
	}
	defer func() {
		openStoreHook = prevO
		restoreRunHook = prevR
	}()
	if code := run([]string{"cairn", "restore", "--config", cfg, "--target", t.TempDir(), "snap"}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Snapshots_BadFlag(t *testing.T) {
	cfg := mustWriteConfig(t, baseCfgToml(t))
	if code := run([]string{"cairn", "snapshots", "--config", cfg, "--bad"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Snapshots_ConfigLoadError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing.toml")
	if code := run([]string{"cairn", "snapshots", "--config", p}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Snapshots_OpenStoreError(t *testing.T) {
	cfg := mustWriteConfig(t, baseCfgToml(t))
	prev := openStoreHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) {
		return nil, errors.New("no")
	}
	defer func() { openStoreHook = prev }()
	if code := run([]string{"cairn", "snapshots", "--config", cfg}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Snapshots_ListHookError(t *testing.T) {
	cfg := mustWriteConfig(t, baseCfgToml(t))
	prevO := openStoreHook
	prevL := snapshotsListHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	snapshotsListHook = func(context.Context, *appcfg.Config, snapshots.SnapshotLister, []age.Identity, string, *slog.Logger) error {
		return errors.New("list")
	}
	defer func() {
		openStoreHook = prevO
		snapshotsListHook = prevL
	}()
	if code := run([]string{"cairn", "snapshots", "--config", cfg}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Snapshots_Success_Stubs(t *testing.T) {
	cfg := mustWriteConfig(t, baseCfgToml(t))
	prevO := openStoreHook
	prevL := snapshotsListHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	snapshotsListHook = func(context.Context, *appcfg.Config, snapshots.SnapshotLister, []age.Identity, string, *slog.Logger) error {
		return nil
	}
	defer func() {
		openStoreHook = prevO
		snapshotsListHook = prevL
	}()
	if code := run([]string{"cairn", "snapshots", "--config", cfg}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Snapshots_IdentityOptional_IgnoresLoadErr(t *testing.T) {
	cfg := mustWriteConfig(t, restoreCfgBody(t, filepath.Join(t.TempDir(), "missing.age")))
	prevO := openStoreHook
	prevL := snapshotsListHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	snapshotsListHook = func(context.Context, *appcfg.Config, snapshots.SnapshotLister, []age.Identity, string, *slog.Logger) error {
		return nil
	}
	defer func() {
		openStoreHook = prevO
		snapshotsListHook = prevL
	}()
	if code := run([]string{"cairn", "snapshots", "--config", cfg}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Verify_BadFlag(t *testing.T) {
	cfg := mustWriteConfig(t, restoreCfgBody(t, mustWriteIdentity(t)))
	if code := run([]string{"cairn", "verify", "--config", cfg, "--bad", "s"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Verify_ConfigMissing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope.toml")
	if code := run([]string{"cairn", "verify", "--config", p, "s"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Verify_NoIdentity(t *testing.T) {
	cfg := mustWriteConfig(t, baseCfgToml(t))
	prev := getenv
	getenv = func(k string) string {
		if k == "CAIRN_IDENTITY_FILE" {
			return ""
		}
		return prev(k)
	}
	defer func() { getenv = prev }()
	if code := run([]string{"cairn", "verify", "--config", cfg, "s"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Verify_BadIdentity(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "x.age")
	if err := os.WriteFile(bad, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := mustWriteConfig(t, restoreCfgBody(t, bad))
	if code := run([]string{"cairn", "verify", "--config", cfg, "s"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Verify_OpenStoreError(t *testing.T) {
	cfg := mustWriteConfig(t, restoreCfgBody(t, mustWriteIdentity(t)))
	prev := openStoreHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) {
		return nil, errors.New("x")
	}
	defer func() { openStoreHook = prev }()
	if code := run([]string{"cairn", "verify", "--config", cfg, "s"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Verify_RunHookError(t *testing.T) {
	cfg := mustWriteConfig(t, restoreCfgBody(t, mustWriteIdentity(t)))
	prevO := openStoreHook
	prevV := verifyRunHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	verifyRunHook = func(context.Context, *appcfg.Config, verifycmd.VerifyStore, []age.Identity, string, int, *slog.Logger) error {
		return errors.New("v")
	}
	defer func() {
		openStoreHook = prevO
		verifyRunHook = prevV
	}()
	if code := run([]string{"cairn", "verify", "--config", cfg, "s"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Verify_Success_Stubs(t *testing.T) {
	cfg := mustWriteConfig(t, restoreCfgBody(t, mustWriteIdentity(t)))
	prevO := openStoreHook
	prevV := verifyRunHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	verifyRunHook = func(context.Context, *appcfg.Config, verifycmd.VerifyStore, []age.Identity, string, int, *slog.Logger) error {
		return nil
	}
	defer func() {
		openStoreHook = prevO
		verifyRunHook = prevV
	}()
	if code := run([]string{"cairn", "verify", "--config", cfg, "s"}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Prune_BadFlag(t *testing.T) {
	cfg := mustWriteConfig(t, baseCfgToml(t))
	if code := run([]string{"cairn", "prune", "--config", cfg, "--keep-last", "2", "--nope"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Prune_ConfigMissing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope.toml")
	if code := run([]string{"cairn", "prune", "--config", p, "--keep-last", "2"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Prune_OpenStoreError(t *testing.T) {
	cfg := mustWriteConfig(t, baseCfgToml(t))
	prev := openStoreHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) {
		return nil, errors.New("p")
	}
	defer func() { openStoreHook = prev }()
	if code := run([]string{"cairn", "prune", "--config", cfg, "--keep-last", "3"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Prune_RunHookError(t *testing.T) {
	cfg := mustWriteConfig(t, baseCfgToml(t))
	prevO := openStoreHook
	prevP := pruneRunHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	pruneRunHook = func(context.Context, prune.PruneStore, string, int, int, bool, *slog.Logger) error {
		return errors.New("prune")
	}
	defer func() {
		openStoreHook = prevO
		pruneRunHook = prevP
	}()
	if code := run([]string{"cairn", "prune", "--config", cfg, "--keep-last", "3"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Prune_Success_Stubs(t *testing.T) {
	cfg := mustWriteConfig(t, baseCfgToml(t))
	prevO := openStoreHook
	prevP := pruneRunHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	pruneRunHook = func(context.Context, prune.PruneStore, string, int, int, bool, *slog.Logger) error {
		return nil
	}
	defer func() {
		openStoreHook = prevO
		pruneRunHook = prevP
	}()
	if code := run([]string{"cairn", "prune", "--config", cfg, "--keep-last", "4"}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Export_BadFlag(t *testing.T) {
	out := t.TempDir()
	if code := run([]string{"cairn", "export-recovery-kit", "--output", out, "--nope"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Export_ConfigLoadError(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "missing.toml")
	out := filepath.Join(t.TempDir(), "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"cairn", "export-recovery-kit", "--output", out, "--config", bad}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Export_HookError(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	prev := exportRecoveryKitHook
	exportRecoveryKitHook = func(string, *appcfg.Config) error { return errors.New("exp") }
	defer func() { exportRecoveryKitHook = prev }()
	if code := run([]string{"cairn", "export-recovery-kit", "--output", out}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Export_Success_WithConfig(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := mustWriteConfig(t, baseCfgToml(t))
	if code := run([]string{"cairn", "export-recovery-kit", "--output", out, "--config", cfg}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Status_BadFlag(t *testing.T) {
	cfg := mustWriteConfig(t, baseCfgToml(t))
	if code := run([]string{"cairn", "status", "--config", cfg, "--bad"}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Status_ConfigMissing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope.toml")
	if code := run([]string{"cairn", "status", "--config", p}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Status_OpenStoreError(t *testing.T) {
	cfg := mustWriteConfig(t, baseCfgToml(t))
	prev := openStoreHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) {
		return nil, errors.New("st")
	}
	defer func() { openStoreHook = prev }()
	if code := run([]string{"cairn", "status", "--config", cfg}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Status_RunHookError(t *testing.T) {
	cfg := mustWriteConfig(t, baseCfgToml(t))
	prevO := openStoreHook
	prevS := statusRunHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	statusRunHook = func(context.Context, status.PrefixLister, string, bool, *slog.Logger) error {
		return errors.New("status")
	}
	defer func() {
		openStoreHook = prevO
		statusRunHook = prevS
	}()
	if code := run([]string{"cairn", "status", "--config", cfg}); code != 1 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Status_Success_Stubs(t *testing.T) {
	cfg := mustWriteConfig(t, baseCfgToml(t))
	prevO := openStoreHook
	prevS := statusRunHook
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	statusRunHook = func(context.Context, status.PrefixLister, string, bool, *slog.Logger) error {
		return nil
	}
	defer func() {
		openStoreHook = prevO
		statusRunHook = prevS
	}()
	if code := run([]string{"cairn", "status", "--config", cfg, "--show-cost"}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Restore_DefaultConfigPath(t *testing.T) {
	raw := restoreCfgBody(t, mustWriteIdentity(t))
	cfgPath := filepath.Join(t.TempDir(), "fallback.toml")
	if err := os.WriteFile(cfgPath, []byte(strings.TrimPrefix(raw, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	prevP := defaultConfigPath
	prevO := openStoreHook
	prevR := restoreRunHook
	defaultConfigPath = func() string { return cfgPath }
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	restoreRunHook = func(context.Context, *appcfg.Config, restore.ObjectGetter, []age.Identity, string, string, int, *slog.Logger) error {
		return nil
	}
	defer func() {
		defaultConfigPath = prevP
		openStoreHook = prevO
		restoreRunHook = prevR
	}()
	if code := run([]string{"cairn", "restore", "--target", t.TempDir(), "snap"}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Snapshots_DefaultConfigPath(t *testing.T) {
	raw := baseCfgToml(t)
	cfgPath := filepath.Join(t.TempDir(), "fb.toml")
	if err := os.WriteFile(cfgPath, []byte(strings.TrimPrefix(raw, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	prevP := defaultConfigPath
	prevO := openStoreHook
	prevL := snapshotsListHook
	defaultConfigPath = func() string { return cfgPath }
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	snapshotsListHook = func(context.Context, *appcfg.Config, snapshots.SnapshotLister, []age.Identity, string, *slog.Logger) error {
		return nil
	}
	defer func() {
		defaultConfigPath = prevP
		openStoreHook = prevO
		snapshotsListHook = prevL
	}()
	if code := run([]string{"cairn", "snapshots"}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Verify_DefaultConfigPath(t *testing.T) {
	raw := restoreCfgBody(t, mustWriteIdentity(t))
	cfgPath := filepath.Join(t.TempDir(), "fv.toml")
	if err := os.WriteFile(cfgPath, []byte(strings.TrimPrefix(raw, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	prevP := defaultConfigPath
	prevO := openStoreHook
	prevV := verifyRunHook
	defaultConfigPath = func() string { return cfgPath }
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	verifyRunHook = func(context.Context, *appcfg.Config, verifycmd.VerifyStore, []age.Identity, string, int, *slog.Logger) error {
		return nil
	}
	defer func() {
		defaultConfigPath = prevP
		openStoreHook = prevO
		verifyRunHook = prevV
	}()
	if code := run([]string{"cairn", "verify", "mysnap"}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Prune_DefaultConfigPath(t *testing.T) {
	raw := baseCfgToml(t)
	cfgPath := filepath.Join(t.TempDir(), "fp.toml")
	if err := os.WriteFile(cfgPath, []byte(strings.TrimPrefix(raw, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	prevP := defaultConfigPath
	prevO := openStoreHook
	prevN := pruneRunHook
	defaultConfigPath = func() string { return cfgPath }
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	pruneRunHook = func(context.Context, prune.PruneStore, string, int, int, bool, *slog.Logger) error { return nil }
	defer func() {
		defaultConfigPath = prevP
		openStoreHook = prevO
		pruneRunHook = prevN
	}()
	if code := run([]string{"cairn", "prune", "--keep-last", "2"}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestRun_Status_DefaultConfigPath(t *testing.T) {
	raw := baseCfgToml(t)
	cfgPath := filepath.Join(t.TempDir(), "fs.toml")
	if err := os.WriteFile(cfgPath, []byte(strings.TrimPrefix(raw, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	prevP := defaultConfigPath
	prevO := openStoreHook
	prevS := statusRunHook
	defaultConfigPath = func() string { return cfgPath }
	openStoreHook = func(context.Context, *appcfg.Config) (*s3store.Store, error) { return nil, nil }
	statusRunHook = func(context.Context, status.PrefixLister, string, bool, *slog.Logger) error {
		return nil
	}
	defer func() {
		defaultConfigPath = prevP
		openStoreHook = prevO
		statusRunHook = prevS
	}()
	if code := run([]string{"cairn", "status"}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

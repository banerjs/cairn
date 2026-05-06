package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appcfg "github.com/banerjs/cairn/internal/config"
	"github.com/banerjs/cairn/internal/s3store"

	"filippo.io/age"
)

func TestRun_NoArgs(t *testing.T) {
	if code := run([]string{"cairn"}); code != 1 {
		t.Fatalf("code %d", code)
	}
}

func TestRun_Help(t *testing.T) {
	for _, a := range [][]string{{"cairn", "-h"}, {"cairn", "--help"}, {"cairn", "help"}} {
		if code := run(a); code != 0 {
			t.Fatalf("%v code %d", a, code)
		}
	}
}

func TestRun_Version(t *testing.T) {
	if code := run([]string{"cairn", "version"}); code != 0 {
		t.Fatalf("code %d", code)
	}
}

func TestRun_Unknown(t *testing.T) {
	if code := run([]string{"cairn", "nope"}); code != 1 {
		t.Fatalf("code %d", code)
	}
}

func TestRun_KeygenRequiresOutput(t *testing.T) {
	if code := run([]string{"cairn", "keygen"}); code != 1 {
		t.Fatalf("code %d", code)
	}
}

func TestRun_BackupRequiresConfigPath(t *testing.T) {
	if code := run([]string{"cairn", "backup"}); code != 1 {
		t.Fatalf("code %d", code)
	}
}

func TestRun_RestoreRequiresTarget(t *testing.T) {
	if code := run([]string{"cairn", "restore", "snap123"}); code != 1 {
		t.Fatalf("code %d", code)
	}
}

func TestRun_VerifyRequiresSnapshot(t *testing.T) {
	if code := run([]string{"cairn", "verify"}); code != 1 {
		t.Fatalf("code %d", code)
	}
}

func TestRun_PruneRequiresKeepLast(t *testing.T) {
	if code := run([]string{"cairn", "prune"}); code != 1 {
		t.Fatalf("code %d", code)
	}
}

func TestRun_ExportRecoveryRequiresOutput(t *testing.T) {
	if code := run([]string{"cairn", "export-recovery-kit"}); code != 1 {
		t.Fatalf("code %d", code)
	}
}

func TestRun_OpenStoreError(t *testing.T) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	rec := id.Recipient().String()
	prev := openStoreHook
	defer func() { openStoreHook = prev }()
	openStoreHook = func(ctx context.Context, cfg *appcfg.Config) (*s3store.Store, error) {
		return nil, errors.New("no aws")
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.toml")
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
	if code := run([]string{"cairn", "backup", cfgPath}); code != 1 {
		t.Fatalf("code %d", code)
	}
}

func TestLoggerFromVerbosity(t *testing.T) {
	_ = loggerFromVerbosity(false, false)
	_ = loggerFromVerbosity(true, false)
	_ = loggerFromVerbosity(false, true)
}

func TestRun_ExportRecoveryKitSuccess(t *testing.T) {
	out := filepath.Join(t.TempDir(), "kit")
	if code := run([]string{"cairn", "export-recovery-kit", "--output", out}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if _, err := os.Stat(filepath.Join(out, "FORMAT.md")); err != nil {
		t.Fatal(err)
	}
}

func TestRun_KeygenSuccess(t *testing.T) {
	out := filepath.Join(t.TempDir(), "k.age")
	if code := run([]string{"cairn", "keygen", "--output", out}); code != 0 {
		t.Fatalf("code %d", code)
	}
	b, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(b), "AGE-SECRET-KEY") {
		t.Fatalf("key file: %v %s", err, b)
	}
}

func TestRun_PruneKeepLastInvalid(t *testing.T) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "c.toml")
	body := fmt.Sprintf(`
host_id = "h"

[s3]
bucket = "b"
region = "us-east-1"

[encryption]
recipients = [%q]

[backup]
source_roots = ["/tmp"]
`, id.Recipient().String())
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"cairn", "prune", "--config", cfgPath, "--keep-last", "0"}); code != 1 {
		t.Fatalf("code %d", code)
	}
}

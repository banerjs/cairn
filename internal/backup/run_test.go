package backup

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appcfg "github.com/banerjs/cairn/internal/config"
	"github.com/banerjs/cairn/internal/s3test"

	"filippo.io/age"
)

func TestRun_SingleFile(t *testing.T) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	recStr := id.Recipient().String()
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	idPath := filepath.Join(tmp, "id.age")
	if err := os.WriteFile(idPath, []byte(id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bucket := "cairn-backup-test"
	st, cleanup := s3test.NewStore(t, bucket)
	defer cleanup()

	cfgPath := filepath.Join(tmp, "cairn.toml")
	cfgBody := fmt.Sprintf(`
host_id = "test-host"
cleanup_grace = "1ns"

[s3]
bucket = %q
region = "us-east-1"
storage_class = "STANDARD"

[encryption]
recipients = [%q]
identity_file = %q

[backup]
source_roots = [%q]
parallelism = 2
`, bucket, recStr, idPath, src)
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0o644); err != nil {
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
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	if err := Run(ctx, cfg, st, recipients, "", 0, log); err != nil {
		t.Fatal(err)
	}
}

func TestCountingReader(t *testing.T) {
	cr := &countingReader{r: strings.NewReader("abcd")}
	var buf [4]byte
	n, err := cr.Read(buf[:])
	if err != nil || n != 4 || cr.n != 4 {
		t.Fatalf("n=%d err=%v count=%d", n, err, cr.n)
	}
}

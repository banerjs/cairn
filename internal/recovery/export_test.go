package recovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	appcfg "github.com/banerjs/cairn/internal/config"
)

func TestExportRecoveryKit_Errors(t *testing.T) {
	if err := ExportRecoveryKit("", nil); err == nil {
		t.Fatal("expected empty dir error")
	}
	if err := ExportRecoveryKit("   ", nil); err == nil {
		t.Fatal("expected empty dir error")
	}
}

func TestExportRecoveryKit_NoConfig(t *testing.T) {
	dir := t.TempDir()
	if err := ExportRecoveryKit(dir, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "FORMAT.md"))
	if err != nil || len(b) < 50 {
		t.Fatalf("FORMAT.md: %v len %d", err, len(b))
	}
	r, err := os.ReadFile(filepath.Join(dir, "RESTORE.txt"))
	if err != nil || !strings.Contains(string(r), "Cairn recovery kit") {
		t.Fatal("RESTORE.txt")
	}
}

func TestExportRecoveryKit_WithConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &appcfg.Config{
		HostID: "h1",
		S3:     appcfg.S3Config{Bucket: "my-bucket", Region: "us-west-2"},
		Encryption: appcfg.EncryptionConfig{
			Recipients: []string{"age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"},
		},
	}
	if err := ExportRecoveryKit(dir, cfg); err != nil {
		t.Fatal(err)
	}
	pub, err := os.ReadFile(filepath.Join(dir, "recipients_public.txt"))
	if err != nil || len(pub) < 10 {
		t.Fatalf("recipients_public.txt: %v", err)
	}
}

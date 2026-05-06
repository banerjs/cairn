package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_MinimalValid(t *testing.T) {
	p := writeCfg(t, `
host_id = "my-host"
cleanup_grace = "48h"

[s3]
bucket = "my-bucket"
region = "us-east-1"
storage_class = "STANDARD"

[encryption]
recipients = ["age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"]

[backup]
source_roots = ["/tmp"]
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.HostID != "my-host" {
		t.Fatal(c.HostID)
	}
	if c.CleanupGraceDuration() <= 0 {
		t.Fatal("cleanup duration")
	}
	if c.S3.StorageClass != "STANDARD" {
		t.Fatal(c.S3.StorageClass)
	}
	if c.Backup.Parallelism < 1 {
		t.Fatal(c.Backup.Parallelism)
	}
	if !c.FollowSymlinksEffective() {
		t.Fatal("follow symlinks default")
	}
	f := false
	c2 := *c
	c2.Backup.FollowSymlinks = &f
	if c2.FollowSymlinksEffective() {
		t.Fatal("expected false")
	}
}

func TestLoad_DefaultStorageClass(t *testing.T) {
	p := writeCfg(t, `
host_id = "defhost"

[s3]
bucket = "b"
region = "us-east-1"

[encryption]
recipients = ["age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"]

[backup]
source_roots = ["/tmp"]
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.HostID != "defhost" {
		t.Fatalf("host_id %q", c.HostID)
	}
	if c.S3.StorageClass != "STANDARD_IA" {
		t.Fatalf("got %q", c.S3.StorageClass)
	}
}

func TestLoad_ExpandEnv(t *testing.T) {
	t.Setenv("MYBUCKET", "from-env")
	p := writeCfg(t, `
host_id = "h"

[s3]
bucket = "${MYBUCKET}"
region = "us-east-1"

[encryption]
recipients = ["age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"]

[backup]
source_roots = ["/tmp"]
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.S3.Bucket != "from-env" {
		t.Fatalf("got %q", c.S3.Bucket)
	}
}

func TestLoad_Errors(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.toml")); err == nil {
		t.Fatal("expected missing file error")
	}
	badToml := writeCfg(t, `not toml {{{`)
	if _, err := Load(badToml); err == nil {
		t.Fatal("expected decode error")
	}
	unknown := writeCfg(t, `
host_id = "h"
extra_field = 1

[s3]
bucket = "b"
region = "us-east-1"

[encryption]
recipients = ["age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"]

[backup]
source_roots = ["/tmp"]
`)
	if _, err := Load(unknown); err == nil || !strings.Contains(err.Error(), "unknown keys") {
		t.Fatalf("got %v", err)
	}
	badGrace := writeCfg(t, `
host_id = "h"
cleanup_grace = "not-a-duration"

[s3]
bucket = "b"
region = "us-east-1"

[encryption]
recipients = ["age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"]

[backup]
source_roots = ["/tmp"]
`)
	if _, err := Load(badGrace); err == nil {
		t.Fatal("expected cleanup_grace error")
	}
}

func TestValidate_BadHost(t *testing.T) {
	cfg := Config{
		HostID: "BAD HOST!!!",
		S3:     S3Config{Bucket: "b", Region: "us-east-1", StorageClass: "STANDARD"},
		Encryption: EncryptionConfig{
			Recipients: []string{"age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"},
		},
	}
	if err := cfg.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidate_NoBucket(t *testing.T) {
	cfg := Config{
		HostID: "h",
		S3:     S3Config{Region: "us-east-1", StorageClass: "STANDARD"},
		Encryption: EncryptionConfig{
			Recipients: []string{"age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"},
		},
	}
	if err := cfg.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidate_NoRegion(t *testing.T) {
	cfg := Config{
		HostID: "h",
		S3:     S3Config{Bucket: "b", StorageClass: "STANDARD"},
		Encryption: EncryptionConfig{
			Recipients: []string{"age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"},
		},
	}
	if err := cfg.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidate_BadStorageClass(t *testing.T) {
	cfg := Config{
		HostID: "h",
		S3:     S3Config{Bucket: "b", Region: "us-east-1", StorageClass: "INVALID"},
		Encryption: EncryptionConfig{
			Recipients: []string{"age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"},
		},
	}
	if err := cfg.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidate_NoRecipients(t *testing.T) {
	cfg := Config{
		HostID:     "h",
		S3:         S3Config{Bucket: "b", Region: "us-east-1", StorageClass: "STANDARD"},
		Encryption: EncryptionConfig{},
	}
	if err := cfg.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDefaultConfigPath_ReturnsPath(t *testing.T) {
	p := DefaultConfigPath()
	if p == "" {
		t.Fatal("empty path")
	}
	if runtime.GOOS == "windows" && os.Getenv("APPDATA") == "" {
		return
	}
	if !strings.Contains(p, "cairn") {
		t.Fatalf("unexpected path %q", p)
	}
}

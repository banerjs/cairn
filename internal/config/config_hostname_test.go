package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyDefaults_HostnameError(t *testing.T) {
	prev := hostnameForConfig
	hostnameForConfig = func() (string, error) { return "", errors.New("no host") }
	defer func() { hostnameForConfig = prev }()
	c := &Config{
		S3:         S3Config{Bucket: "b", Region: "r", StorageClass: "STANDARD"},
		Encryption: EncryptionConfig{Recipients: []string{"age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"}},
	}
	if err := c.applyDefaults(); err == nil || !strings.Contains(err.Error(), "hostname") {
		t.Fatalf("got %v", err)
	}
}

func TestLoad_ReadErrorContainsPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing.toml")
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("got %v", err)
	}
}

func TestLoad_ValidateErrorFromBadHost(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.toml")
	body := `
host_id = "BAD HOST"

[s3]
bucket = "b"
region = "us-east-1"

[encryption]
recipients = ["age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"]

[backup]
source_roots = ["/tmp"]
`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "host_id") {
		t.Fatalf("got %v", err)
	}
}

func TestApplyDefaults_HostnameSuccessSetsHostID(t *testing.T) {
	prev := hostnameForConfig
	hostnameForConfig = func() (string, error) { return "from-host", nil }
	defer func() { hostnameForConfig = prev }()
	c := &Config{
		S3:         S3Config{Bucket: "b", Region: "r", StorageClass: "STANDARD"},
		Encryption: EncryptionConfig{Recipients: []string{"age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"}},
	}
	if err := c.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	if c.HostID != "from-host" {
		t.Fatalf("got %q", c.HostID)
	}
}

func TestDefaultConfigPath_OSVariants(t *testing.T) {
	prevOS := goosForConfig
	prevHome := userHomeDirForConfig
	prevGetenv := getenvForConfig
	defer func() {
		goosForConfig = prevOS
		userHomeDirForConfig = prevHome
		getenvForConfig = prevGetenv
	}()

	userHomeDirForConfig = func() (string, error) { return "/home/u", nil }

	goosForConfig = func() string { return "darwin" }
	if p := DefaultConfigPath(); !strings.Contains(p, "Application Support") {
		t.Fatalf("darwin path %q", p)
	}

	goosForConfig = func() string { return "windows" }
	getenvForConfig = func(k string) string {
		if k == "APPDATA" {
			return "C:\\AppData"
		}
		return ""
	}
	if p := DefaultConfigPath(); !strings.Contains(strings.ToLower(p), "appdata") {
		t.Fatalf("windows path %q", p)
	}

	goosForConfig = func() string { return "linux" }
	getenvForConfig = func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return ""
		}
		return ""
	}
	if p := DefaultConfigPath(); !strings.Contains(p, ".config") {
		t.Fatalf("default xdg path %q", p)
	}
}

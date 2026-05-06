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

func TestApplyDefaults_HostnameSlugifiesMixedCaseAndPunct(t *testing.T) {
	prev := hostnameForConfig
	hostnameForConfig = func() (string, error) {
		return "\tJohns MacBook-Pro@LAN.local!", nil
	}
	defer func() { hostnameForConfig = prev }()
	c := &Config{
		S3:         S3Config{Bucket: "b", Region: "r", StorageClass: "STANDARD"},
		Encryption: EncryptionConfig{Recipients: []string{"age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"}},
	}
	if err := c.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	want := "johns-macbook-pro-lan.local"
	if c.HostID != want {
		t.Fatalf("got %q want %q", c.HostID, want)
	}
}

func TestApplyDefaults_HostnameSlugifyErrorsEmptyAfterTrim(t *testing.T) {
	prev := hostnameForConfig
	hostnameForConfig = func() (string, error) { return "  \t  ", nil }
	defer func() { hostnameForConfig = prev }()
	c := &Config{
		S3:         S3Config{Bucket: "b", Region: "r", StorageClass: "STANDARD"},
		Encryption: EncryptionConfig{Recipients: []string{"age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"}},
	}
	if err := c.applyDefaults(); err == nil || !strings.Contains(err.Error(), "host_id from hostname") {
		t.Fatalf("got %v", err)
	}
}

func TestApplyDefaults_HostnameSlugifyErrorsNoUsableChars(t *testing.T) {
	prev := hostnameForConfig
	hostnameForConfig = func() (string, error) { return "@@@", nil }
	defer func() { hostnameForConfig = prev }()
	c := &Config{
		S3:         S3Config{Bucket: "b", Region: "r", StorageClass: "STANDARD"},
		Encryption: EncryptionConfig{Recipients: []string{"age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"}},
	}
	if err := c.applyDefaults(); err == nil || !strings.Contains(err.Error(), "usable") {
		t.Fatalf("got %v", err)
	}
}

func TestApplyDefaults_HostnameSlugifyTruncatesTo64Runes(t *testing.T) {
	prev := hostnameForConfig
	long := strings.Repeat("x", 90)
	hostnameForConfig = func() (string, error) { return long, nil }
	defer func() { hostnameForConfig = prev }()
	c := &Config{
		S3:         S3Config{Bucket: "b", Region: "r", StorageClass: "STANDARD"},
		Encryption: EncryptionConfig{Recipients: []string{"age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"}},
	}
	if err := c.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	if got, want := len([]rune(c.HostID)), 64; got != want {
		t.Fatalf("len %d want %d", got, want)
	}
}

func TestApplyDefaults_HostnameSlugifyNonASCIISeparator(t *testing.T) {
	prev := hostnameForConfig
	hostnameForConfig = func() (string, error) { return "café-host", nil }
	defer func() { hostnameForConfig = prev }()
	c := &Config{
		S3:         S3Config{Bucket: "b", Region: "r", StorageClass: "STANDARD"},
		Encryption: EncryptionConfig{Recipients: []string{"age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"}},
	}
	if err := c.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	if c.HostID != "caf-host" {
		t.Fatalf("got %q", c.HostID)
	}
}

func TestApplyDefaults_HostnameSlugifyPreservesDigitsAndUnderscore(t *testing.T) {
	prev := hostnameForConfig
	hostnameForConfig = func() (string, error) { return "Box_42.TMP", nil }
	defer func() { hostnameForConfig = prev }()
	c := &Config{
		S3:         S3Config{Bucket: "b", Region: "r", StorageClass: "STANDARD"},
		Encryption: EncryptionConfig{Recipients: []string{"age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"}},
	}
	if err := c.applyDefaults(); err != nil {
		t.Fatal(err)
	}
	if c.HostID != "box_42.tmp" {
		t.Fatalf("got %q", c.HostID)
	}
}

func TestLoad_OmittedHostIDUsesSlugifiedHostname(t *testing.T) {
	prev := hostnameForConfig
	hostnameForConfig = func() (string, error) { return "My-Host", nil }
	defer func() { hostnameForConfig = prev }()
	p := writeCfg(t, `
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
	if c.HostID != "my-host" {
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

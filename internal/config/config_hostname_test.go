package config

import (
	"errors"
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

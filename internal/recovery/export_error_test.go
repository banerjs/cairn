package recovery

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appcfg "github.com/banerjs/cairn/internal/config"
)

func TestExportRecoveryKit_MkdirError(t *testing.T) {
	prev := mkdirAllRecovery
	mkdirAllRecovery = func(string, os.FileMode) error { return errors.New("mkdir") }
	defer func() { mkdirAllRecovery = prev }()
	if err := ExportRecoveryKit(os.TempDir(), nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestExportRecoveryKit_FormatWriteError(t *testing.T) {
	prev := mkdirAllRecovery
	mkdirAllRecovery = func(string, os.FileMode) error { return nil }
	defer func() { mkdirAllRecovery = prev }()
	prevW := writeFileRecovery
	writeFileRecovery = func(name string, b []byte, m os.FileMode) error {
		if strings.HasSuffix(name, string(filepath.Separator)+"FORMAT.md") {
			return errors.New("FORMAT fail")
		}
		return prevW(name, b, m)
	}
	defer func() { writeFileRecovery = prevW }()

	dir := t.TempDir()
	if err := ExportRecoveryKit(dir, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestExportRecoveryKit_NoRecipientsWritten(t *testing.T) {
	dir := t.TempDir()
	cfg := &appcfg.Config{
		HostID:     "x",
		S3:         appcfg.S3Config{Bucket: "b", Region: "r"},
		Encryption: appcfg.EncryptionConfig{Recipients: []string{"  ", "\t"}},
	}
	if err := ExportRecoveryKit(dir, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "recipients_public.txt")); err == nil {
		t.Fatal("expected no recipients file")
	}
}

func TestExportRecoveryKit_RecipientsWriteError(t *testing.T) {
	prevW := writeFileRecovery
	writeFileRecovery = func(name string, b []byte, m os.FileMode) error {
		if strings.Contains(name, "recipients_public.txt") {
			return errors.New("recipients_public fail")
		}
		return prevW(name, b, m)
	}
	defer func() { writeFileRecovery = prevW }()
	dir := t.TempDir()
	cfg := &appcfg.Config{
		HostID:     "x",
		S3:         appcfg.S3Config{Bucket: "b", Region: "r"},
		Encryption: appcfg.EncryptionConfig{Recipients: []string{"age1pq1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"}},
	}
	if err := ExportRecoveryKit(dir, cfg); err == nil {
		t.Fatal("expected error")
	}
}

func TestExportRecoveryKit_RestoreTxtWriteError(t *testing.T) {
	prevW := writeFileRecovery
	writeFileRecovery = func(name string, b []byte, m os.FileMode) error {
		if strings.Contains(name, "RESTORE.txt") {
			return errors.New("RESTORE fail")
		}
		return prevW(name, b, m)
	}
	defer func() { writeFileRecovery = prevW }()
	if err := ExportRecoveryKit(t.TempDir(), nil); err == nil {
		t.Fatal("expected error")
	}
}

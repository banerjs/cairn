package ageutil

import (
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

func TestParsePQRecipients_HybridSkipsBlank(t *testing.T) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	out, err := ParsePQRecipients([]string{"  ", id.Recipient().String()})
	if err != nil || len(out) != 1 {
		t.Fatalf("got %v len %d", err, len(out))
	}
}

func TestParsePQRecipients_Errors(t *testing.T) {
	if _, err := ParsePQRecipients(nil); err == nil {
		t.Fatal("expected no recipients")
	}
	if _, err := ParsePQRecipients([]string{"age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsqqqqqqqqqqqqqqqqqp"}); err == nil {
		t.Fatal("expected classical rejection")
	}
	if _, err := ParsePQRecipients([]string{"not-a-key"}); err == nil {
		t.Fatal("expected unknown format")
	}
}

func TestIdentityPath(t *testing.T) {
	if _, err := IdentityPath("", ""); err == nil {
		t.Fatal("expected error")
	}
	p, err := IdentityPath(" /tmp/x ", "")
	if err != nil || p != "/tmp/x" {
		t.Fatalf("got %q %v", p, err)
	}
	p2, err := IdentityPath("", "/cfg/id.age")
	if err != nil || p2 != "/cfg/id.age" {
		t.Fatalf("got %q %v", p2, err)
	}
}

func TestGeneratePQIdentity(t *testing.T) {
	sec, pub, err := GeneratePQIdentity()
	if err != nil || sec == "" || pub == "" {
		t.Fatalf("%v", err)
	}
}

func TestLoadIdentities_InvalidPath(t *testing.T) {
	if _, err := LoadIdentities("/nonexistent/cairn/id.age"); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadIdentities_InvalidContent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.age")
	if err := os.WriteFile(p, []byte("not-an-age-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIdentities(p); err == nil {
		t.Fatal("expected parse error")
	}
}

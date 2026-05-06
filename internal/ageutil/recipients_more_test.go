package ageutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"filippo.io/age/tag"
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

func TestParsePQRecipients_TaggedRecipient(t *testing.T) {
	prev := parseTagRecipient
	parseTagRecipient = func(string) (*tag.Recipient, error) { return nil, nil }
	defer func() { parseTagRecipient = prev }()
	out, err := ParsePQRecipients([]string{"age1tagpq1synthetic"})
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
	if _, err := ParsePQRecipients([]string{"age1pq1bad"}); err == nil {
		t.Fatal("expected malformed hybrid rejection")
	}
	if _, err := ParsePQRecipients([]string{"age1tagpq1bad"}); err == nil {
		t.Fatal("expected malformed tagged rejection")
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

func TestLoadIdentities_Success(t *testing.T) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "ok.age")
	if err := os.WriteFile(p, []byte(id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ids, err := LoadIdentities(p)
	if err != nil || len(ids) == 0 {
		t.Fatalf("ids=%d err=%v", len(ids), err)
	}
}

func TestGeneratePQIdentity_Error(t *testing.T) {
	boom := errors.New("rng")
	prev := generateHybridIdentity
	generateHybridIdentity = func() (*age.HybridIdentity, error) {
		return nil, boom
	}
	defer func() { generateHybridIdentity = prev }()
	if _, _, err := GeneratePQIdentity(); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
}

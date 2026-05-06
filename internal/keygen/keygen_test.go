package keygen

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

func TestWriteNewHybridIdentity(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "id.age")
	rec, err := WriteNewHybridIdentity(p)
	if err != nil {
		t.Fatal(err)
	}
	if rec == "" {
		t.Fatal("empty recipient")
	}
	b, err := os.ReadFile(p)
	if err != nil || len(b) < 20 {
		t.Fatalf("file: %v len %d", err, len(b))
	}
	if _, err := WriteNewHybridIdentity(p); err == nil {
		t.Fatal("expected exists error")
	}
}

func TestWriteNewHybridIdentity_StatError(t *testing.T) {
	boom := errors.New("boom")
	prev := osStat
	osStat = func(string) (os.FileInfo, error) {
		return nil, boom
	}
	defer func() { osStat = prev }()
	if _, err := WriteNewHybridIdentity(filepath.Join(t.TempDir(), "n.age")); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
}

func TestWriteNewHybridIdentity_GenerateFails(t *testing.T) {
	boom := errors.New("rng")
	prev := generateHybridIdentity
	generateHybridIdentity = func() (*age.HybridIdentity, error) { return nil, boom }
	defer func() { generateHybridIdentity = prev }()
	if _, err := WriteNewHybridIdentity(filepath.Join(t.TempDir(), "g.age")); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
}

func TestWriteNewHybridIdentity_WriteError(t *testing.T) {
	boom := errors.New("cannot write")
	prev := osWriteFile
	osWriteFile = func(string, []byte, os.FileMode) error { return boom }
	defer func() { osWriteFile = prev }()
	if _, err := WriteNewHybridIdentity(filepath.Join(t.TempDir(), "w.age")); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
}

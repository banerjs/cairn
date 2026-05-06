package keygen

import (
	"os"
	"path/filepath"
	"testing"
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

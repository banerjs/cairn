//go:build !windows

package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileMeta_regularFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	mode, uid, gid := fileMeta(fi)
	if mode == nil || uid == nil || gid == nil {
		t.Fatal("expected unix metadata")
	}
}

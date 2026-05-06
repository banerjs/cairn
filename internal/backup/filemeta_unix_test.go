//go:build !windows

package backup

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
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

type fakeInfo struct{}

func (fakeInfo) Name() string       { return "x" }
func (fakeInfo) Size() int64        { return 1 }
func (fakeInfo) Mode() fs.FileMode  { return 0o644 }
func (fakeInfo) ModTime() time.Time { return time.Now() }
func (fakeInfo) IsDir() bool        { return false }
func (fakeInfo) Sys() any           { return "not-stat" }

func TestFileMeta_NonStatSys(t *testing.T) {
	mode, uid, gid := fileMeta(fakeInfo{})
	if mode != nil || uid != nil || gid != nil {
		t.Fatal("expected nil metadata for non-stat sys payload")
	}
}

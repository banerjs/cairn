package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompile_BasicExclude(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skip.dat"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Compile(root, []string{"*.dat"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Matches("skip.dat", false) {
		t.Fatal("expected skip.dat excluded")
	}
	if m.Matches("keep.txt", false) {
		t.Fatal("keep.txt should not match exclude")
	}
}

func TestCompile_GlobalIncludes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Compile(root, nil, []string{"*.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Matches("a.txt", false) {
		t.Fatal("included file should not be excluded")
	}
}

func TestCompile_CairnIgnoreFile(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, ".cairnignore"), []byte("ignored.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "ignored.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Compile(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Matches("sub/ignored.log", false) {
		t.Fatal("expected nested ignore rule")
	}
}

func TestMatcher_Nil(t *testing.T) {
	var m *Matcher
	if m.Matches("x", false) {
		t.Fatal("nil matcher should not match")
	}
}

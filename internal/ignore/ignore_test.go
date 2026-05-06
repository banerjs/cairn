package ignore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestCompile_MissingRootError(t *testing.T) {
	if _, err := Compile(filepath.Join(t.TempDir(), "missing"), nil, nil); err == nil {
		t.Fatal("expected walk error")
	}
}

func TestCompile_GlobalIncludesVariants(t *testing.T) {
	root := t.TempDir()
	m, err := Compile(root, []string{"   "}, []string{"  ", "!already-negated", "plain"})
	if err != nil {
		t.Fatal(err)
	}
	if m.Matches("plain", false) {
		t.Fatal("plain should be included by synthesized !plain")
	}
	if m.Matches("already-negated", false) {
		t.Fatal("negated include should remain include")
	}
}

func TestCollectCairnIgnoreLines_ScannerError(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, ".cairnignore")
	// Scanner token too long -> ErrTooLong path.
	longLine := strings.Repeat("a", 70*1024)
	if err := os.WriteFile(p, []byte(longLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := collectCairnIgnoreLines(root); err == nil {
		t.Fatal("expected scanner error")
	}
}

func TestPrefixPatternCases(t *testing.T) {
	if got := prefixPattern("", "/x"); got != "x" {
		t.Fatalf("got %q", got)
	}
	if got := prefixPattern("a/b", "/x"); got != "a/b/x" {
		t.Fatalf("got %q", got)
	}
	if got := prefixPattern("a", "!x"); got != "!a/x" {
		t.Fatalf("got %q", got)
	}
	if got := prefixPattern("", "x"); got != "x" {
		t.Fatalf("got %q", got)
	}
	if got := prefixPattern("", "   "); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestMatcher_DirSuffixAndNilGI(t *testing.T) {
	m := &Matcher{}
	if m.Matches("dir", true) {
		t.Fatal("nil gi should not match")
	}
	root := t.TempDir()
	real, err := Compile(root, []string{"dir/"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !real.Matches("dir", true) {
		t.Fatal("dir should match with slash suffix")
	}
}

func TestCollectCairnIgnore_RelAndOpenErrors(t *testing.T) {
	root := t.TempDir()
	ig := filepath.Join(root, ".cairnignore")
	if err := os.WriteFile(ig, []byte("x\n#comment\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prevRel := relForIgnore
	relForIgnore = func(string, string) (string, error) { return "", errors.New("rel") }
	if _, err := collectCairnIgnoreLines(root); err == nil || !strings.Contains(err.Error(), "ignore rel") {
		t.Fatalf("err=%v", err)
	}
	relForIgnore = prevRel

	prevOpen := openForIgnore
	openForIgnore = func(string) (*os.File, error) { return nil, errors.New("open") }
	defer func() { openForIgnore = prevOpen }()
	if _, err := collectCairnIgnoreLines(root); err == nil || !strings.Contains(err.Error(), "ignore open") {
		t.Fatalf("err=%v", err)
	}
}

func TestCollectCairnIgnore_CommentLineIgnored(t *testing.T) {
	root := t.TempDir()
	ig := filepath.Join(root, ".cairnignore")
	if err := os.WriteFile(ig, []byte("# only comment\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lines, err := collectCairnIgnoreLines(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected no patterns, got %v", lines)
	}
}

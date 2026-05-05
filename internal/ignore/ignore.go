// Package ignore combines config excludes/includes with hierarchical .cairnignore files.
package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"
)

// Matcher answers whether a path relative to a source root (forward slashes) should be backed up.
type Matcher struct {
	gi *gitignore.GitIgnore
}

// Compile builds a gitignore-style matcher for one source root tree.
//
// relPath passed to Matches must use '/' and be relative to that root (no leading slash).
func Compile(rootAbs string, globalExcludes, globalIncludes []string) (*Matcher, error) {
	extra, err := collectCairnIgnoreLines(rootAbs)
	if err != nil {
		return nil, err
	}
	var lines []string
	if len(globalIncludes) > 0 {
		lines = append(lines, "*")
		for _, inc := range globalIncludes {
			inc = strings.TrimSpace(inc)
			if inc == "" {
				continue
			}
			if strings.HasPrefix(inc, "!") {
				lines = append(lines, inc)
			} else {
				lines = append(lines, "!"+inc)
			}
		}
	}
	for _, ex := range globalExcludes {
		ex = strings.TrimSpace(ex)
		if ex != "" {
			lines = append(lines, ex)
		}
	}
	lines = append(lines, extra...)
	gi := gitignore.CompileIgnoreLines(lines...)
	return &Matcher{gi: gi}, nil
}

func collectCairnIgnoreLines(rootAbs string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(rootAbs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != ".cairnignore" {
			return nil
		}
		relDir, err := filepath.Rel(rootAbs, filepath.Dir(path))
		if err != nil {
			return err
		}
		relDir = filepath.ToSlash(relDir)
		if relDir == "." {
			relDir = ""
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			pref := prefixPattern(relDir, line)
			if pref != "" {
				out = append(out, pref)
			}
		}
		_ = f.Close()
		return sc.Err()
	})
	return out, err
}

func prefixPattern(relDir, line string) string {
	neg := strings.HasPrefix(line, "!")
	if neg {
		line = strings.TrimPrefix(line, "!")
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	var pat string
	switch {
	case strings.HasPrefix(line, "/"):
		line = strings.TrimPrefix(line, "/")
		if relDir == "" {
			pat = line
		} else {
			pat = relDir + "/" + line
		}
	default:
		if relDir == "" {
			pat = line
		} else {
			pat = relDir + "/" + line
		}
	}
	pat = filepath.ToSlash(pat)
	if neg {
		return "!" + pat
	}
	return pat
}

// Matches returns true if relSlash should be excluded from backup (SkipDir / skip file).
func (m *Matcher) Matches(relSlash string, isDir bool) bool {
	if m == nil || m.gi == nil {
		return false
	}
	p := relSlash
	if isDir && !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return m.gi.MatchesPath(p)
}

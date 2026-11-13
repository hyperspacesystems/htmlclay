// Package pathrel compares folders the way the volume does: by path, and past the
// path by file identity, so a symlink, another letter case on a case-insensitive
// volume, or a Windows junction that reaches the same folder counts as the same.
package pathrel

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Within reports whether path is dir or lies below it.
func Within(path, dir string) bool {
	if path == dir {
		return true
	}
	prefix := dir
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(path, prefix)
}

// SameOrBelow reports whether path is dir or lies below it. Past the string
// comparison it compares file identity for each existing ancestor of path, so a
// spelling the volume treats as the same folder, such as another letter case on
// a case-insensitive Mac volume, is caught too.
func SameOrBelow(path, dir string) bool {
	if Within(path, dir) {
		return true
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		return false
	}
	for current := path; ; current = filepath.Dir(current) {
		if info, err := os.Stat(current); err == nil && os.SameFile(info, dirInfo) {
			return true
		}
		if filepath.Dir(current) == current {
			return false
		}
	}
}

// RealPath resolves symlinks in the nearest existing ancestor of path and
// rejoins the segments that do not exist yet, so a path that is missing still
// resolves through symlinks the way it will once it is created.
func RealPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(abs)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", err
		}
		missing = append(missing, filepath.Base(abs))
		abs = parent
	}
}

// Relation is how folder a stands to folder b.
type Relation int

const (
	Unrelated Relation = iota
	Same
	Inside   // a lies below b
	Contains // b lies below a
)

// Relate compares two folders with SameOrBelow in both directions.
func Relate(a, b string) Relation {
	below, above := SameOrBelow(a, b), SameOrBelow(b, a)
	switch {
	case below && above:
		return Same
	case below:
		return Inside
	case above:
		return Contains
	}
	return Unrelated
}

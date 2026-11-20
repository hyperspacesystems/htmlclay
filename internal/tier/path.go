package tier

import (
	"os"
	"path/filepath"
)

// canonicalPath is one spelling of a document's path, so every record answers
// for the same file however the caller spells it. The file's links are
// resolved while it exists; a file that is gone keeps its cleaned absolute
// path.
func canonicalPath(path string) string {
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	path = filepath.Clean(path)
	if _, err := os.Lstat(path); err == nil {
		if real, err := filepath.EvalSymlinks(path); err == nil {
			path = real
		}
	}
	return foldCase(path)
}

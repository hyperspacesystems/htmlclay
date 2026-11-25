//go:build darwin

package tier

import "strings"

// foldCase lowercases a path on a disk that ignores letter case, so Doc.html
// and doc.html are one document.
func foldCase(path string) string { return strings.ToLower(path) }

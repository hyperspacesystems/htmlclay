//go:build managedtrial

package managedtrial

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// publishRecoveredCopy is Local's publishRecoveredCopy (utils/backup.js:193-205):
// a copy of a document's unsaved edits under .hyperclay/recovered/<name without
// its extension>/, named and written the way a version is, through a directory
// chain checked free of symlinks before and after it is made.
func publishRecoveredCopy(baseDir, rel string, content []byte) (PublishedVersion, error) {
	invalid := errors.New("Invalid recovered document path")
	if !pageExtension.MatchString(rel) || filepath.IsAbs(rel) || strings.Contains(rel, `\`) {
		return PublishedVersion{}, invalid
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") {
			return PublishedVersion{}, invalid
		}
	}
	dir := filepath.Join(baseDir, filepath.FromSlash(recoveredDirName), filepath.FromSlash(pageExtension.ReplaceAllString(rel, "")))
	if err := AssertVersionsChain(baseDir, dir); err != nil {
		return PublishedVersion{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return PublishedVersion{}, err
	}
	if err := AssertVersionsChain(baseDir, dir); err != nil {
		return PublishedVersion{}, err
	}
	return PublishVersion(dir, ".html", content)
}

//go:build managedtrial

// Package managedtrial is the managed Go trial host for Hyperclay Local
// documents. Every file in it carries the managedtrial build tag, so release
// builds cannot contain it.
package managedtrial

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// RefuseLiveRoot fails when root is, resolves inside, or contains the live
// Hyperclay Local apps folder. That folder syncs to hyperclay.com within
// seconds, so a trial host writing there would change production, and a root
// above it serves its documents as ordinary children. It answers from the
// paths alone, so it never depends on the folder existing.
func RefuseLiveRoot(root, liveRoot string) error {
	resolved, err := realPath(root)
	if err != nil {
		return err
	}
	live, err := realPath(liveRoot)
	if err != nil {
		return err
	}
	if sameOrBelow(resolved, live) {
		return errors.New("--root " + root + " is inside " + live + " (LOCAL_APPS), which syncs to hyperclay.com; refusing to serve it")
	}
	if sameOrBelow(live, resolved) {
		return errors.New("--root " + root + " contains " + live + " (LOCAL_APPS), which syncs to hyperclay.com; refusing to serve it")
	}
	return nil
}

// within reports whether path is dir or lies below it.
func within(path, dir string) bool {
	if path == dir {
		return true
	}
	prefix := dir
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(path, prefix)
}

// sameOrBelow reports whether path is dir or lies below it. Past the string
// comparison it compares file identity for each existing ancestor of path, so a
// spelling the volume treats as the same folder, such as another letter case on
// a case-insensitive Mac volume, is caught too.
func sameOrBelow(path, dir string) bool {
	if within(path, dir) {
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

// reachesLiveRoot reports whether a symlink target is, lies inside, or contains
// the live folder. When the live folder cannot be named it answers true, so the
// link is refused.
func reachesLiveRoot(target string) bool {
	live, err := liveRoot()
	if err != nil {
		return true
	}
	return RefuseLiveRoot(target, live) != nil
}

// DefaultLiveRoot is the live Hyperclay Local apps folder for this user.
func DefaultLiveRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return realPath(filepath.Join(home, "Documents", "GitHub", "hyper", "LOCAL_APPS"))
}

// realPath resolves symlinks in the nearest existing ancestor of path and
// rejoins the segments that do not exist yet, so a path that is missing still
// resolves through symlinks the way it will once it is created.
func realPath(path string) (string, error) {
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

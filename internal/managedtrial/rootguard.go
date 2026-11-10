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

// RefuseLiveRoot fails when root is, or resolves inside, the live Hyperclay
// Local apps folder. That folder syncs to hyperclay.com within seconds, so a
// trial host writing there would change production. It answers from the paths
// alone, so it never depends on the folder existing.
func RefuseLiveRoot(root, liveRoot string) error {
	resolved, err := realPath(root)
	if err != nil {
		return err
	}
	live, err := realPath(liveRoot)
	if err != nil {
		return err
	}
	if resolved == live || strings.HasPrefix(resolved, live+string(filepath.Separator)) {
		return errors.New("--root " + root + " is inside " + live + " (LOCAL_APPS), which syncs to hyperclay.com; refusing to serve it")
	}
	return nil
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

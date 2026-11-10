//go:build managedtrial

package managedtrial

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// In-app symlink containment for the destructive filesystem paths of the
// version store, ported from Local's src/main/utils/real-dir-chain.js.
//
// Immediately before a destructive op, lstat every directory component from
// just below the served folder down to the operating directory and require
// each to be a real directory, never a symlink. A directory symlink planted
// under .hyperclay can then no longer redirect an unlink or a write out of
// tree. The residual race the Node module documents (a same-user process
// swapping a real directory for a symlink between the lstat and the following
// syscall) is the same here.

// canonicalizeBase resolves the served folder to its canonical, symlink-free
// form once per entry point. On macOS the served folder commonly sits under
// /var -> /private/var, so a lexical base would never line up with a realpath'd
// child.
func canonicalizeBase(baseDir string) (string, error) {
	resolved, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		return "", err
	}
	return filepath.Abs(resolved)
}

// rebaseOntoCanonical takes a path built by lexically joining onto baseDir,
// cancels baseDir's own symlinked prefix against the lexical base, and re-roots
// the remainder onto the canonical base so the chain walk lstats real,
// symlink-free prefixes.
func rebaseOntoCanonical(canonicalBase, baseDir, dir string) (string, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absBase, absDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(canonicalBase, rel), nil
}

// assertRealDirChain asserts every directory component from just below
// canonicalBase down to dir is a real directory, never a symlink. dir must
// already sit under canonicalBase. A component that does not exist yet stops
// the walk: it will be freshly mkdir'd as a real directory, or the op it guards
// is a no-op.
func assertRealDirChain(canonicalBase, dir string) error {
	target, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if target == canonicalBase {
		return nil
	}

	rel, err := filepath.Rel(canonicalBase, target)
	if err != nil {
		return err
	}
	if rel == "" || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return errors.New("Path chain escapes the served folder: " + dir)
	}

	prefix := canonicalBase
	for _, segment := range strings.Split(rel, string(filepath.Separator)) {
		if segment == "" {
			continue
		}
		prefix = filepath.Join(prefix, segment)
		info, err := os.Lstat(prefix)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !info.IsDir() {
			return errors.New("Refusing filesystem op: '" + prefix + "' is a symlink or not a directory")
		}
	}
	return nil
}

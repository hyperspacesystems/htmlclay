// Package served is the managed Go trial host for Hyperclay Local
// documents. Its fault hooks carry the managedtrial build tag, so a release
// build compiles them to no-ops.
package served

import (
	"errors"

	"github.com/panphora/htmlclay/internal/pathrel"
)

// RefuseLiveRoot fails when root is, resolves inside, or contains the live
// Hyperclay Local apps folder. That folder syncs to hyperclay.com within
// seconds, so a trial host writing there would change production, and a root
// above it serves its documents as ordinary children. It answers from the
// paths alone, so it never depends on the folder existing.
func RefuseLiveRoot(root, liveRoot string) error {
	resolved, err := pathrel.RealPath(root)
	if err != nil {
		return err
	}
	live, err := pathrel.RealPath(liveRoot)
	if err != nil {
		return err
	}
	if pathrel.SameOrBelow(resolved, live) {
		return errors.New("--root " + root + " is inside " + live + " (LOCAL_APPS), which syncs to hyperclay.com; refusing to serve it")
	}
	if pathrel.SameOrBelow(live, resolved) {
		return errors.New("--root " + root + " contains " + live + " (LOCAL_APPS), which syncs to hyperclay.com; refusing to serve it")
	}
	return nil
}

// reachesLiveRoot reports whether a symlink target is, lies inside, or contains
// the live folder. When the live folder cannot be named it answers true, so the
// link is refused.
func reachesLiveRoot(target string) bool {
	if liveRoot == nil {
		return false
	}
	live, err := liveRoot()
	if err != nil {
		return true
	}
	return RefuseLiveRoot(target, live) != nil
}

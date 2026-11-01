package platform

import (
	"os"
	"time"
)

// Downloaded reports whether the operating system marked path as having come
// from the internet: macOS's com.apple.quarantine, Windows' Zone.Identifier
// with an internet zone, Linux's user.xdg.origin.url. HTML Clay never offers
// such a file as a program suggestion. Not com.apple.provenance: macOS puts
// that on every file an app touches, downloaded or not.
func Downloaded(path string) bool {
	return downloaded(path)
}

// ChangedWithin reports whether path was written or created within d of now,
// or its timestamps lie in the future. It reads the later of the modification
// time and the change (or creation) time, because unzipping keeps an archive's
// old modification time but cannot backdate the other one. A file it cannot
// read counts as changed, so an error never makes a file look settled.
func ChangedWithin(path string, d time.Duration, now time.Time) bool {
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	latest := info.ModTime()
	if other, ok := changeTime(info); ok && other.After(latest) {
		latest = other
	}
	return latest.After(now.Add(-d))
}

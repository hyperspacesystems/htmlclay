//go:build windows

package folderlock

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// lockFile takes the exclusive non-blocking byte-range lock on f. A lock already
// held by another open file answers ErrHeld.
func lockFile(f *os.File) error {
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0,
		&windows.Overlapped{},
	)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrHeld
	}
	return err
}

//go:build !windows

package folderlock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// lockFile takes the exclusive non-blocking flock on f. A lock already held by
// another open file answers ErrHeld.
func lockFile(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrHeld
	}
	return err
}

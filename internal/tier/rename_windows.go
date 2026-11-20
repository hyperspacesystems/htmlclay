//go:build windows

package tier

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
	"time"
)

// renameTries and renameWait are the Windows publish retry: a virus scanner or
// the search indexer can hold a file open for a moment.
const (
	renameTries = 10
	renameWait  = 20 * time.Millisecond
)

// renameWithRetry publishes a record, trying again while Windows holds either
// name open.
func renameWithRetry(from, to string) error {
	var err error
	for i := 0; i < renameTries; i++ {
		if err = os.Rename(from, to); err == nil || !renameHeld(err) {
			return err
		}
		time.Sleep(renameWait)
	}
	return err
}

// renameHeld reports a sharing violation, a lock violation or access denied:
// ERROR_SHARING_VIOLATION, ERROR_LOCK_VIOLATION and ERROR_ACCESS_DENIED.
func renameHeld(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) && (errno == 5 || errno == 32 || errno == 33) {
		return true
	}
	return errors.Is(err, fs.ErrPermission)
}

// Package folderlock is the exclusive OS lock a process holds on a folder while it
// serves it. The lock lives on a file in a directory shared by every HTML Clay
// profile on this account, named by the folder's identity, so another profile, a
// candidate build, or a later Hyperclay Local release asking for the same folder
// is refused. The OS drops the lock when the process ends, even on a crash.
package folderlock

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"github.com/panphora/htmlclay/internal/platform"
)

// ErrHeld is what Acquire answers when another holder has the folder's lock.
var ErrHeld = errors.New("another process is serving this folder")

// Lock is one held folder lock. Release drops it.
type Lock struct{ f *os.File }

// DefaultDir is $HTMLCLAY_FOLDER_LOCKS when set, else <os.UserConfigDir()>/htmlclay-folder-locks.
// It deliberately ignores HTMLCLAY_CONFIG_BASE: profiles must see each other's locks.
func DefaultDir() (string, error) {
	if dir := os.Getenv("HTMLCLAY_FOLDER_LOCKS"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "htmlclay-folder-locks"), nil
}

// Acquire takes the lock for folder (resolved path) without waiting. It answers
// ErrHeld when another holder has it. The lock file is named
// hex(sha256(identity))[:32] + ".lock", where identity is platform.DirIdentity(folder),
// or the folder path itself when that is empty. The directory is created 0700.
// After locking, the file is truncated and holds "<pid>\n<folder>\n", for people
// reading it; nothing reads it back.
func Acquire(dir, folder string) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, lockName(folder)), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, err
	}
	writeOwner(f, folder)
	return &Lock{f: f}, nil
}

// Release drops the lock by closing the file. The file is never deleted: deleting
// a lock file another process is opening would let two processes lock two files.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	f := l.f
	l.f = nil
	return f.Close()
}

func lockName(folder string) string {
	identity := platform.DirIdentity(folder)
	if identity == "" {
		identity = folder
	}
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])[:32] + ".lock"
}

// writeOwner leaves the pid and the folder in the lock file for people reading it.
// It writes with WriteAt at offset 0 because the locked byte range is one byte, and
// a failed write is not a reason to refuse the lock.
func writeOwner(f *os.File, folder string) {
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"+folder+"\n"), 0)
}

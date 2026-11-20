package served

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/panphora/htmlclay/internal/platform"
)

// Shared atomic publication for every writer that touches a served file,
// ported from Local's src/main/utils/write-queue.js. The serialize-the-whole-
// read-modify-write half of that module (withFileLock, withDocumentWrite,
// withStructureChange and the boundary assertions) belongs to the commit core,
// so only the publication primitives are here.

const (
	// atomicTempMode is what a temp file is created with, so a partially
	// written temp is never readable by anyone else. It is re-moded to the
	// target's permissions just before publication.
	atomicTempMode = os.FileMode(0o600)

	// atomicDefaultMode is what a brand-new file gets, at Local's 0o644.
	atomicDefaultMode = os.FileMode(0o644)

	// atomicRenameAttempts is the Windows-only publish retry budget: 10ms
	// doubling to 160ms, about 310ms across six attempts.
	atomicRenameAttempts = 6

	// atomicTempCounterMod bounds the process-wide temp counter.
	atomicTempCounterMod = 1000000
)

// atomicRenameRetryCodes are the rename failures Windows retries, while
// Defender or the Search indexer holds the file for a moment: Local's list, by
// Node's names. Only Windows retries (renameWithRetry): POSIX renames over
// open files without complaint.
var atomicRenameRetryCodes = []syscall.Errno{syscall.EPERM, syscall.EBUSY, syscall.EACCES}

// atomicRenameRetryWin32 is the same list as Go sees it: os.Rename on Windows
// answers Win32 codes, which Go's EPERM, EBUSY and EACCES never equal there.
// Node maps these three onto the names above.
var atomicRenameRetryWin32 = []syscall.Errno{
	5,  // ERROR_ACCESS_DENIED
	32, // ERROR_SHARING_VIOLATION
	33, // ERROR_LOCK_VIOLATION
}

// atomicDirSyncUnsupported lists the codes a directory fsync gives where the
// platform cannot sync a directory at all (Windows, some network and FUSE
// filesystems): not a durability failure, just no promise to keep.
var atomicDirSyncUnsupportedCodes = []syscall.Errno{
	syscall.EISDIR,
	syscall.EPERM,
	syscall.EINVAL,
	syscall.ENOTSUP,
	syscall.EBADF,
	syscall.EACCES,
}

var (
	atomicTempMu      sync.Mutex
	atomicTempCounter int
)

// AfterRenameError marks a failure that landed after the rename: the bytes are
// in place but their directory entry may not survive a crash.
type AfterRenameError struct {
	Err error
}

func (e *AfterRenameError) Error() string { return e.Err.Error() }

func (e *AfterRenameError) Unwrap() error { return e.Err }

// nextAtomicTempPath is Local's `.{base}.{pid}.{n}.tmp` beside the target. The
// counter is process-wide, so two writes of the same file never pick the same
// temp name.
func nextAtomicTempPath(dir, base string) string {
	atomicTempMu.Lock()
	atomicTempCounter = (atomicTempCounter + 1) % atomicTempCounterMod
	n := atomicTempCounter
	atomicTempMu.Unlock()
	return filepath.Join(dir, "."+tempStem(base)+"."+strconv.Itoa(os.Getpid())+"."+strconv.Itoa(n)+".tmp")
}

// atomicTempStemMax keeps a temp name within the 255-byte name limit of every
// supported file system: the dot, pid, counter and suffix add at most 30 bytes.
const atomicTempStemMax = 200

// tempStem is base cut to atomicTempStemMax bytes at a character boundary, so
// a document with a long name can still be saved through its temp.
func tempStem(base string) string {
	if len(base) <= atomicTempStemMax {
		return base
	}
	cut := atomicTempStemMax
	for cut > 0 && !utf8.RuneStart(base[cut]) {
		cut--
	}
	return base[:cut]
}

// syncDirectory fsyncs a directory so a rename into it survives a crash.
func syncDirectory(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = handle.Sync()
	if closeErr := handle.Close(); err == nil {
		err = closeErr
	}
	return err
}

// atomicDirSync is the directory fsync atomicWriteFile runs after the rename. It
// is a variable so a test can make it fail and prove that a write whose bytes
// landed, but whose directory entry may not survive, is reported as
// indeterminate rather than committed.
var atomicDirSync = syncDirectory

// renameWithRetry publishes tmpPath at filePath, running beforeRename first so a
// caller can re-check the target once the temp is durable.
func renameWithRetry(tmpPath, filePath string, beforeRename func() error) error {
	if runtime.GOOS != "windows" {
		if beforeRename != nil {
			if err := beforeRename(); err != nil {
				return err
			}
		}
		return os.Rename(tmpPath, filePath)
	}

	delay := 10 * time.Millisecond
	for attempt := 1; ; attempt++ {
		if beforeRename != nil {
			if err := beforeRename(); err != nil {
				return err
			}
		}
		err := os.Rename(tmpPath, filePath)
		if err == nil {
			return nil
		}
		if attempt == atomicRenameAttempts || !(hasErrno(err, atomicRenameRetryCodes) || hasErrno(err, atomicRenameRetryWin32)) {
			return err
		}
		time.Sleep(delay)
		delay *= 2
	}
}

// atomicWriteFile writes content via a same-directory temp file and a rename, so
// a crash, a full disk or a killed process can never leave partial bytes at
// filePath. The rename replaces the target rather than following it, which is
// why callers must pass an already-canonicalized path. The target's permissions
// are inherited, so a save never silently re-modes a file the person chose to
// make group or world readable.
//
// durable rejects when the directory sync after the rename fails for a reason
// that means the rename itself may not survive a crash.
func atomicWriteFile(filePath string, content []byte, beforeRename func() error, durable bool) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return err
	}

	tmpPath := nextAtomicTempPath(dir, filepath.Base(filePath))

	mode := atomicDefaultMode
	if info, err := os.Stat(filePath); err == nil {
		mode = info.Mode() & os.ModePerm
	}

	handle, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, atomicTempMode)
	if err != nil {
		return err
	}
	// Any failure up to the rename leaves only the temp to undo.
	if err := fillAtomicTemp(handle, content, mode); err != nil {
		os.Remove(tmpPath)
		return err
	}
	_ = platform.KeepDownloadMark(filePath, tmpPath)
	if takeFault(faultKillBeforeRename, filePath) {
		killSelf()
	}
	if err := renameWithRetry(tmpPath, filePath, beforeRename); err != nil {
		os.Remove(tmpPath)
		return err
	}

	syncErr := atomicDirSync(dir)
	if syncErr == nil && durable && takeFault(faultDurableFail, filePath) {
		syncErr = &os.PathError{Op: "fsync", Path: dir, Err: syscall.EIO}
	}
	if syncErr != nil {
		if durable && !dirSyncUnsupported(syncErr, runtime.GOOS) {
			return &AfterRenameError{Err: syncErr}
		}
	}
	if takeFault(faultKillAfterRename, filePath) {
		killSelf()
	}
	return nil
}

// dirSyncUnsupported reports a directory fsync failure that means the platform
// cannot sync a directory, which Local ignores through DIR_SYNC_UNSUPPORTED. On
// Windows, libuv maps every such failure into that set, but Go's syscall.EACCES
// there is not ERROR_ACCESS_DENIED, so a permission-class error is matched by
// kind and any Windows directory sync failure counts as unsupported.
func dirSyncUnsupported(err error, goos string) bool {
	return goos == "windows" || hasErrno(err, atomicDirSyncUnsupportedCodes) || errors.Is(err, fs.ErrPermission)
}

// fillAtomicTemp writes, syncs, remodes and closes the temp file, so the caller
// only has to publish it or delete it.
func fillAtomicTemp(handle *os.File, content []byte, mode os.FileMode) error {
	_, err := handle.Write(content)
	if err == nil {
		err = handle.Sync()
	}
	if err == nil {
		err = handle.Chmod(mode)
	}
	if closeErr := handle.Close(); err == nil {
		err = closeErr
	}
	return err
}

// hasErrno reports whether err carries one of codes.
func hasErrno(err error, codes []syscall.Errno) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	for _, code := range codes {
		if errno == code {
			return true
		}
	}
	return false
}

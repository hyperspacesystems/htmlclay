//go:build managedtrial

package managedtrial

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/panphora/htmlclay/internal/dataapi"
)

// Local's workspace structural writes: a move, a trash move, a duplicate and
// the two sync spellings of the first two. A structure change touches whole
// trees, so Local runs it inside one exclusive boundary that no document write
// can enter while it runs; every path it names is locked here instead, and in
// an order that cannot deadlock against another structure change.

// structurePolicy is one row of Local's STRUCTURE_KINDS.
type structurePolicy struct {
	retire      bool
	copy        bool
	crossDevice bool
}

var structureKinds = map[string]structurePolicy{
	"move":       {crossDevice: true},
	"trash":      {retire: true, crossDevice: true},
	"duplicate":  {copy: true},
	"sync-move":  {crossDevice: true},
	"sync-trash": {retire: true, crossDevice: true},
}

// structureEntry is one resolved step: the canonical path a name is at now and
// the canonical path it is going to.
type structureEntry struct {
	FromPath string
	ToPath   string
	IsDir    bool
}

// structureResult mirrors Local's structure answer. Err carries the failure
// itself, which Local keeps as a non-enumerable `error` on the answer.
type structureResult struct {
	Outcome   string
	Completed int
	Reason    string
	Err       error
}

// metadataDirName is Local's METADATA_DIR, the one folder a duplicate copies
// whole without walking it.
const metadataDirName = ".hyperclay"

// structureLockPaths is every canonical path a structure change names, sorted
// and deduplicated so two changes that touch the same two files always take
// their locks in the same order.
func structureLockPaths(entries []structureEntry) []string {
	seen := make(map[string]bool, len(entries)*2)
	paths := make([]string, 0, len(entries)*2)
	for _, entry := range entries {
		for _, path := range []string{entry.FromPath, entry.ToPath} {
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

// restructure is Local's restructure. There is no authority to plan against
// without rooms, so the outcome is the moves themselves.
func (h *commitHost) restructure(kind string, entries []structureEntry) (structureResult, error) {
	policy, ok := structureKinds[kind]
	if !ok {
		return structureResult{}, fmt.Errorf("Unknown structure kind: %s", kind)
	}
	unlocks := make([]func(), 0, 2*len(entries))
	for _, path := range structureLockPaths(entries) {
		unlocks = append(unlocks, h.locks.lock(path))
	}
	defer func() {
		for _, unlock := range unlocks {
			unlock()
		}
	}()

	completed := 0
	var failure error
	if policy.copy {
		for index, entry := range entries {
			if err := os.MkdirAll(upathDir(entry.ToPath), 0o777); err != nil {
				failure = err
				break
			}
			if err := copyStructure(entry.FromPath, entry.ToPath); err != nil {
				failure = err
				break
			}
			completed = index + 1
		}
		return structureAnswer(completed, failure), nil
	}

	for index, entry := range entries {
		if _, err := moveEntry(entry.FromPath, entry.ToPath, policy.crossDevice); err != nil {
			failure = err
			break
		}
		completed = index + 1
	}
	return structureAnswer(completed, failure), nil
}

// structureAnswer is Local's outcome/completed/reason triple: a failure after
// one step is partial, a failure before any is failed.
func structureAnswer(completed int, failure error) structureResult {
	result := structureResult{Outcome: "committed", Completed: completed, Err: failure}
	if failure == nil {
		return result
	}
	result.Outcome = "failed"
	if completed > 0 {
		result.Outcome = "partial"
	}
	result.Reason = structureErrorCode(failure)
	return result
}

// structureErrorCode is the `error.code` Local reports as the reason, or its
// `failed` when the failure carries none.
func structureErrorCode(err error) string {
	for _, code := range []struct {
		errno syscall.Errno
		name  string
	}{
		{syscall.ENOENT, "ENOENT"},
		{syscall.EEXIST, "EEXIST"},
		{syscall.ENOTDIR, "ENOTDIR"},
		{syscall.EISDIR, "EISDIR"},
		{syscall.EACCES, "EACCES"},
		{syscall.EPERM, "EPERM"},
		{syscall.EXDEV, "EXDEV"},
		{syscall.ENOSPC, "ENOSPC"},
		{syscall.EIO, "EIO"},
	} {
		if errors.Is(err, code.errno) {
			return code.name
		}
	}
	return "failed"
}

// moveEntry is Local's moveEntry: a rename, and on a cross-device rename a copy
// and an unlink. It reports whether the entry was copied.
func moveEntry(from, to string, crossDevice bool) (bool, error) {
	if err := os.MkdirAll(upathDir(to), 0o777); err != nil {
		return false, err
	}
	err := os.Rename(from, to)
	if err == nil {
		return false, nil
	}
	if !crossDevice || !errors.Is(err, syscall.EXDEV) {
		return false, err
	}
	if err := copyFileOverwriting(from, to); err != nil {
		return false, err
	}
	if err := os.Remove(from); err != nil {
		return false, err
	}
	return true, nil
}

// copyFileOverwriting is fs.copyFile's default: the destination is replaced.
func copyFileOverwriting(from, to string) error {
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	target, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		return err
	}
	return target.Close()
}

// copyStructure is Local's copyStructure: a duplicate, with nothing raw ever
// written first for a watcher to see. Documents lose their copy-only sections,
// every entry is created exclusively so an occupied destination is never
// overwritten, and the source is only ever read.
func copyStructure(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()

	if info.IsDir() {
		if upathBase(from) == metadataDirName {
			return copyTreeExclusive(from, to)
		}
		if err := os.Mkdir(to, mode); err != nil {
			return err
		}
		if err := os.Chmod(to, mode); err != nil {
			return err
		}
		names, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		sort.Slice(names, func(i, j int) bool { return names[i].Name() < names[j].Name() })
		for _, entry := range names {
			if err := copyStructure(upathJoin(from, entry.Name()), upathJoin(to, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return copySymlinkExclusive(from, to)
	}

	if isDocumentName(upathBase(from)) {
		bytes, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		if err := writeExclusive(to, stripCopyBytes(bytes), mode); err != nil {
			return err
		}
		return os.Chmod(to, mode)
	}

	bytes, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return writeExclusive(to, bytes, mode)
}

// stripCopyBytes is Local's stripCopyBytes: a copy carries no copy-only
// analytics or promos, and a document with nothing marked keeps its exact
// bytes. Where the strip cannot locate a marked section it leaves the bytes
// alone, so a duplicate never fails on markup it could not parse.
func stripCopyBytes(bytes []byte) []byte {
	cleaned, err := dataapi.StripCopyContent(bytes)
	if err != nil {
		return bytes
	}
	return cleaned
}

// writeExclusive is fs.writeFile's `wx` flag: a destination that exists is an
// error, never an overwrite.
func writeExclusive(to string, content []byte, mode os.FileMode) error {
	handle, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := handle.Write(content); err != nil {
		handle.Close()
		return err
	}
	return handle.Close()
}

// copySymlinkExclusive recreates a link where the destination is free.
func copySymlinkExclusive(from, to string) error {
	if _, err := os.Lstat(to); err == nil {
		return &os.LinkError{Op: "copy", Old: from, New: to, Err: syscall.EEXIST}
	}
	target, err := os.Readlink(from)
	if err != nil {
		return err
	}
	return os.Symlink(target, to)
}

// copyTreeExclusive is fs.cp with `recursive` and `errorOnExist` and without
// `force`, the one call a duplicate makes for the metadata folder.
func copyTreeExclusive(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return copySymlinkExclusive(from, to)
	}
	if !info.IsDir() {
		bytes, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		return writeExclusive(to, bytes, info.Mode().Perm())
	}
	if err := os.Mkdir(to, info.Mode().Perm()); err != nil {
		return err
	}
	names, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	sort.Slice(names, func(i, j int) bool { return names[i].Name() < names[j].Name() })
	for _, entry := range names {
		if err := copyTreeExclusive(filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

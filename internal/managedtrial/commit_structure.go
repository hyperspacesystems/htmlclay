//go:build managedtrial

package managedtrial

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
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

// structureEntry is one resolved step: the canonical path a name is at now, the
// canonical path it is going to, the root-relative names the step gave, and the
// page-state work Local's workspace command does after the step.
type structureEntry struct {
	FromPath  string
	ToPath    string
	IsDir     bool
	FromName  string
	ToName    string
	PageState string
}

// structureResult mirrors Local's structure answer. Err carries the failure
// itself, which Local keeps as a non-enumerable `error` on the answer.
type structureResult struct {
	Outcome      string
	Completed    int
	Reason       string
	Err          error
	PageStateErr error
	// Refusal is a refusal answered to the client as it was decided.
	Refusal *serviceRefusal
}

// metadataDirName is Local's METADATA_DIR, the one folder a duplicate copies
// whole without walking it.
const metadataDirName = ".hyperclay"

// restructure is Local's restructure. There is no authority to plan against
// without rooms, so the outcome is the moves themselves.
func (h *commitHost) restructure(kind string, entries []structureEntry) (structureResult, error) {
	return h.restructureOp(kind, entries, "", nil)
}

// restructureOp is restructure with the op a service-attached change is
// consulted under.
func (h *commitHost) restructureOp(kind string, entries []structureEntry, opID string, opHandle *string) (structureResult, error) {
	unlock := h.locks.lockStructure()
	defer unlock()
	switch kind {
	case "folder":
		return h.makeFolders(entries), nil
	case "empty-trash":
		return h.emptyTrash(), nil
	}
	policy, ok := structureKinds[kind]
	if !ok {
		return structureResult{}, fmt.Errorf("Unknown structure kind: %s", kind)
	}

	var consult *consultation
	var holdID string
	if h.service != nil {
		// The full-host stage: the service plans the rooms' move before the
		// first file moves and settles it with what completed (protocol v1,
		// structure/prepare and structure/settle).
		if opID == "" {
			opID = newOpID()
		}
		consult = h.service.begin(opID, opHandle, h.rootID, h.baseDir, "", "", "structure", kind)
		answer, err := consult.prepareStructure(h.structureSteps(kind, entries))
		if err != nil {
			consult.end(err)
			refusal := retryableRefusal("service-unavailable")
			return structureResult{Outcome: "refused", Reason: refusal.Code, Refusal: &refusal}, nil
		}
		if answer.Decision == "refuse" {
			h.service.abort(consult.ID, "refused")
			refusal := serviceRefusal{Status: answer.Status, Code: answer.Code, Body: answer.Body, Headers: answer.Headers}
			return structureResult{Outcome: "refused", Reason: answer.Code, Refusal: &refusal}, nil
		}
		holdID = *answer.HoldID
	}

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
		result := structureAnswer(completed, failure)
		if consult != nil {
			copied := make([]int, completed)
			for index := range copied {
				copied[index] = index
			}
			if err := consult.settleStructure(holdID, completed, copied, result.Outcome); err != nil {
				log.Printf("structure %s: settle: %v", kind, err)
			}
		}
		return result, nil
	}

	var copiedSteps []int
	for index, entry := range entries {
		copied, err := moveEntry(entry.FromPath, entry.ToPath, policy.crossDevice)
		if err != nil {
			failure = err
			break
		}
		if copied {
			copiedSteps = append(copiedSteps, index)
		}
		completed = index + 1
	}
	if consult != nil {
		if err := consult.settleStructure(holdID, completed, copiedSteps, structureAnswer(completed, failure).Outcome); err != nil {
			log.Printf("structure %s: settle: %v", kind, err)
		}
	}
	for _, entry := range entries[:completed] {
		var err error
		switch {
		case entry.PageState == "move" && entry.IsDir:
			err = movePageStateDir(h.baseDir, entry.FromName, entry.ToName)
		case entry.PageState == "move":
			err = movePageState(h.baseDir, entry.FromName, entry.ToName)
		case entry.PageState == "drop" && entry.IsDir:
			err = dropPageStateDir(h.baseDir, entry.FromName)
		case entry.PageState == "drop":
			err = dropPageState(h.baseDir, entry.FromName)
		}
		if err != nil {
			result := structureAnswer(completed, failure)
			result.PageStateErr = err
			return result, nil
		}
	}
	return structureAnswer(completed, failure), nil
}

// structureSteps is what the service plans from: each step's root-relative
// names, their locations under the root, and the canonical paths resolved here.
func (h *commitHost) structureSteps(kind string, entries []structureEntry) []structureStep {
	steps := make([]structureStep, 0, len(entries))
	for _, entry := range entries {
		step := structureStep{Kind: kind}
		if entry.FromName != "" {
			location := filepath.Join(h.baseDir, filepath.FromSlash(entry.FromName))
			isDir := entry.IsDir
			isSymlink := false
			if info, err := os.Lstat(location); err == nil {
				isSymlink = info.Mode()&os.ModeSymlink != 0
			}
			step.From = &structureSide{Name: entry.FromName, Location: location, CanonicalPath: entry.FromPath, IsDir: &isDir, IsSymlink: &isSymlink}
		}
		if entry.ToName != "" {
			step.To = &structureSide{Name: entry.ToName, Location: filepath.Join(h.baseDir, filepath.FromSlash(entry.ToName)), CanonicalPath: entry.ToPath}
		}
		steps = append(steps, step)
	}
	return steps
}

// makeFolders is the workspace's folder command: each step's destination made
// as one new directory, never its parents, stopping at the first failure.
func (h *commitHost) makeFolders(entries []structureEntry) structureResult {
	completed := 0
	var failure error
	for index, entry := range entries {
		if err := os.Mkdir(entry.ToPath, 0o777); err != nil {
			failure = err
			break
		}
		completed = index + 1
	}
	return structureAnswer(completed, failure)
}

// emptyTrash is the workspace's empty-trash command: everything inside the
// root's `.trash/` removed, the folder itself kept. A missing Trash is already
// empty; a `.trash` that is not a directory is refused as Local's
// assertTrashDirectory refuses it.
func (h *commitHost) emptyTrash() structureResult {
	trash := filepath.Join(h.baseDir, ".trash")
	info, err := os.Lstat(trash)
	if errors.Is(err, fs.ErrNotExist) {
		return structureAnswer(0, nil)
	}
	if err != nil {
		return structureAnswer(0, err)
	}
	if !info.IsDir() {
		return structureAnswer(0, &os.PathError{Op: "lstat", Path: trash, Err: syscall.ENOTDIR})
	}
	names, err := os.ReadDir(trash)
	if err != nil {
		return structureAnswer(0, err)
	}
	completed := 0
	for _, entry := range names {
		if err := os.RemoveAll(filepath.Join(trash, entry.Name())); err != nil {
			return structureAnswer(completed, err)
		}
		completed++
	}
	return structureAnswer(completed, nil)
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

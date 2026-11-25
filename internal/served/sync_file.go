package served

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// The files sync keeps in a served folder for itself: the marker that names the
// folder's sync session, and the copies conflicts keep. The service writes them
// through Go like every other write (08-sync.md, C4), and only them: the
// versions folder and everything else under .hyperclay/ stay Go's own.
const (
	syncMarkerRel       = ".hyperclay/sync-root.json"
	syncConflictsPrefix = ".hyperclay/conflicts/"
)

// controlSyncFileBody is one POST /v1/sync/file.
type controlSyncFileBody struct {
	OpID  string            `json:"opId"`
	Root  string            `json:"root"`
	Op    string            `json:"op"`
	Rel   string            `json:"rel"`
	Bytes *controlBytesBody `json:"bytes"`
}

// syncFileAllowed reports whether rel is one of sync's own files for op.
func syncFileAllowed(op, rel string) bool {
	if rel == syncMarkerRel {
		return op == "put"
	}
	rest, ok := strings.CutPrefix(rel, syncConflictsPrefix)
	if !ok || rest == "" {
		return false
	}
	for _, segment := range strings.Split(rest, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.HasPrefix(segment, ".") ||
			strings.ContainsAny(segment, "\\\x00") || jsStringLength(segment) > 255 {
			return false
		}
	}
	return true
}

// containedAfterLinks reports whether dir, or its deepest folder that exists,
// is inside baseReal once links are followed.
func containedAfterLinks(baseReal, dir string) bool {
	for {
		if _, err := os.Lstat(dir); err == nil {
			real, err := filepath.EvalSymlinks(dir)
			return err == nil && isContained(baseReal, filepath.ToSlash(real))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// syncFileErrno names a failed write the way C4 reports it.
func syncFileErrno(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ENOSPC:
			return "ENOSPC"
		case syscall.EACCES:
			return "EACCES"
		case syscall.EPERM:
			return "EPERM"
		case syscall.EROFS:
			return "EROFS"
		case syscall.ENOTDIR:
			return "ENOTDIR"
		case syscall.EISDIR:
			return "EISDIR"
		}
	}
	return "EIO"
}

// serveSyncFile is POST /v1/sync/file: a put or remove of one of sync's own
// files, under the file's lock, through a temporary file and a rename.
func (a *controlAPI) serveSyncFile(w http.ResponseWriter, r *http.Request) {
	var body controlSyncFileBody
	if !decodeControlJSON(w, r, &body) {
		return
	}
	root, ok := a.root(w, body.Root)
	if !ok {
		return
	}
	if body.Op != "put" && body.Op != "remove" {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "op must be put or remove"})
		return
	}
	var content []byte
	if body.Op == "put" {
		if body.Bytes == nil {
			writeControlJSON(w, http.StatusBadRequest, controlError{Error: "a put needs bytes"})
			return
		}
		decoded, _, err := body.Bytes.decode()
		if err != nil {
			writeControlJSON(w, http.StatusBadRequest, controlError{Error: err.Error()})
			return
		}
		content = decoded
	}
	opID, ok := claimControlOp(w, root.events, body.OpID)
	if !ok {
		return
	}
	refuse := func(reason string) {
		root.events.finishOp(opID, "refused", "", "")
		writeControlJSON(w, http.StatusOK, orderedObject{{Key: "outcome", Value: "refused"}, {Key: "reason", Value: reason}, {Key: "opId", Value: opID}})
	}
	if !syncFileAllowed(body.Op, body.Rel) {
		refuse("not-allowed")
		return
	}
	target := filepath.Join(root.baseDir, filepath.FromSlash(body.Rel))
	dir := filepath.Dir(target)
	if !containedAfterLinks(root.baseDir, dir) {
		refuse("not-allowed")
		return
	}
	unlock := root.commit.locks.lock(filepath.ToSlash(target))
	defer unlock()

	answer := orderedObject{{Key: "outcome", Value: "committed"}}
	if body.Op == "remove" {
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			refuse(syncFileErrno(err))
			return
		}
	} else {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			refuse(syncFileErrno(err))
			return
		}
		// A folder made just now could only be a link if something raced it in.
		if !containedAfterLinks(root.baseDir, dir) {
			refuse("not-allowed")
			return
		}
		if err := atomicWriteFile(target, content, nil, true); err != nil {
			refuse(syncFileErrno(err))
			return
		}
		sum := sha256.Sum256(content)
		answer = append(answer, kv{Key: "sha256", Value: hex.EncodeToString(sum[:])})
	}
	root.events.finishOp(opID, "committed", "", "")
	writeControlJSON(w, http.StatusOK, append(answer, kv{Key: "opId", Value: opID}))
}

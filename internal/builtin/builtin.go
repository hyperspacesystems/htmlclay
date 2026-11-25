// Package builtin is HTML Clay's built-in programs: file search, file
// conversion and folder watching. Each runs inside the app for a document the
// person allowed, and only inside the trusted or served folder that holds it.
package builtin

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	FileSearch  = "file-search"
	FileConvert = "file-convert"
	FolderWatch = "folder-watch"
)

// IDs is every built-in program, in the order the Programs page lists them.
var IDs = []string{FileSearch, FileConvert, FolderWatch}

// Is reports whether name is a built-in program's name.
func Is(name string) bool { return slices.Contains(IDs, name) }

// Title is the name a person sees for a built-in program, or the ID itself
// for a name that is not built in.
func Title(id string) string {
	switch id {
	case FileSearch:
		return "File search"
	case FileConvert:
		return "Image converter"
	case FolderWatch:
		return "Folder watcher"
	}
	return id
}

// Access states: the policy's answer for one document and one program.
const (
	Ready     = "ready"
	Off       = "off"
	Untrusted = "untrusted"
	Ask       = "ask"
	Removed   = "removed"
)

// Access is whether a document may run a program now, and the folder it works
// in when it may.
type Access struct {
	State string
	Root  string
}

// Error is a refusal the page shows. Code is one of invalid_request, not_found,
// too_large, unsupported, exists, busy, failed.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

func invalid(field string) *Error {
	message := "HTML Clay could not read that request."
	switch field {
	case "query":
		message = "Enter something to search for."
	case "path":
		message = "Choose a file or folder inside this folder."
	case "to":
		message = "Choose PNG, JPEG, GIF, BMP, TIFF or WebP."
	case "maxWidth", "maxHeight":
		message = "Enter a size up to 10,000 pixels."
	case "quality":
		message = "Enter a quality from 1 to 100."
	}
	return &Error{Code: "invalid_request", Message: message}
}

// failed is an error the page shows in plain words; the cause stays out of the
// page because a system error names paths and calls a person cannot act on.
func failed(error) *Error {
	return &Error{Code: "failed", Message: "That did not work. Try again."}
}

// RevealTarget checks that rel names a file or folder inside root and returns
// its absolute path, for file-search's reveal.
func RevealTarget(root, rel string) (string, error) {
	p, perr := relPath(rel)
	if perr != nil {
		return "", perr
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return "", failed(err)
	}
	defer r.Close()
	if _, err := r.Stat(p); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", &Error{Code: "not_found", Message: "That file does not exist."}
		}
		return "", failed(err)
	}
	return filepath.Join(root, p), nil
}

// relPath checks a slash-separated path relative to the program's folder and
// returns it in this system's form. "" is the folder itself. An absolute path,
// "..", ".", an empty segment, a backslash and a hidden name are refused.
func relPath(p string) (string, *Error) {
	if p == "" || p == "." {
		return ".", nil
	}
	if len(p) > 1024 || strings.Contains(p, "\\") || strings.HasPrefix(p, "/") || strings.Contains(p, ":") {
		return "", invalid("path")
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") {
			return "", invalid("path")
		}
	}
	return filepath.FromSlash(p), nil
}

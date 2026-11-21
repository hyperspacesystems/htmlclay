// Package carried runs the JavaScript programs a document carries in
// <script type="text/x-clay-program"> tags. A program runs in goja inside a
// separate HTML Clay worker process started through runner.Start with only
// CleanEnv: it starts no process and reaches files, git and the network only
// through host calls this package answers in the app. The deadline, a cancel
// and the memory watch kill the worker's whole process group, and a worker
// that crashes ends only itself.
package carried

import (
	"fmt"
	"strings"
)

// HelperName is the wire helper a page's clay.programs sends to.
const HelperName = "programs"

// Level is what a program's tag asks for in its can attribute.
type Level string

const (
	// LevelRead reads files in the chosen folder and reads its git history.
	LevelRead Level = "read"
	// LevelChange also writes files in the chosen folder.
	LevelChange Level = "change"
	// LevelInternet fetches public addresses and reaches no file.
	LevelInternet Level = "internet"
)

// Calls are the host calls one run may make.
type Calls struct {
	Files bool
	Git   bool
	Write bool
	Fetch bool
}

// ParseLevel reads a program tag's can attribute: "read" and "git" (the
// default when it is empty), "change", or "internet" on its own. Internet
// never comes with files, so a tag that asks for both is refused, and so is a
// word HTML Clay does not know.
func ParseLevel(can string) (Level, error) {
	level := LevelRead
	internet, files := false, false
	for _, word := range strings.Fields(strings.ToLower(can)) {
		switch word {
		case "read", "git":
			files = true
		case "change":
			files = true
			level = LevelChange
		case "internet":
			internet = true
		default:
			return "", &Error{Code: CodeProgramInvalid, Message: fmt.Sprintf("This program asks for %q, which HTML Clay does not know. A program may ask for read, git, change or internet.", clip(word, 32))}
		}
	}
	if internet && files {
		return "", &Error{Code: CodeProgramInvalid, Message: "This program asks for the internet and for files. HTML Clay never gives a program both: split it into two programs."}
	}
	if internet {
		return LevelInternet, nil
	}
	return level, nil
}

// NeedsFolder reports whether a program at this level works in a folder.
func (l Level) NeedsFolder() bool { return l == LevelRead || l == LevelChange }

// Policy is the one place a document's tier becomes host calls. tier is
// "mine", "ecosystem", "team" or "unknown", and the caller has already
// checked the document is ready (vouched for or allowed, and not refused).
// Every tier may read and use git in its folder, and fetch public addresses
// on its own. Only a document the person or a signed release vouches for
// changes files: for any other, the change level answers false, and the
// program is refused before anyone is asked for a folder.
func Policy(tier string, level Level) (Calls, bool) {
	switch level {
	case LevelInternet:
		return Calls{Fetch: true}, true
	case LevelChange:
		if tier != "mine" && tier != "ecosystem" {
			return Calls{}, false
		}
		return Calls{Files: true, Git: true, Write: true}, true
	}
	return Calls{Files: true, Git: true}, true
}

// Error codes a run answers with. Each reaches the page as error.code.
const (
	CodeProgramMissing   = "program_missing"
	CodeProgramNotRead   = "program_not_read"
	CodeProgramInvalid   = "program_invalid"
	CodeLevelUnavailable = "level_unavailable"
	CodeProgramChanged   = "program_changed"
	CodeProgramFailed    = "program_failed"
	CodeProgramTimeout   = "program_timeout"
	CodeProgramMemory    = "program_memory"
	CodeProgramCrashed   = "program_crashed"
	CodeResultTooLarge   = "result_too_large"
	CodeUnavailable      = "programs_unavailable"
	CodeCallRefused      = "call_refused"
	CodeCallInvalid      = "call_invalid"
	CodeNotFound         = "not_found"
	CodeNotAFile         = "not_a_file"
	CodeNotText          = "not_text"
	CodeTooLarge         = "too_large"
	CodeLimit            = "limit"
	CodeGitFailed        = "git_failed"
	CodeFetchRefused     = "fetch_refused"
	CodeFetchFailed      = "fetch_failed"
	CodeOutputRefused    = "output_refused"
	CodeStoreFailed      = "store_failed"
	CodeWorkerFailed     = "worker_failed"
	CodeFolderUnreadable = "folder_unreadable"
)

// Error is a refusal with a code the page can act on and a message a person
// can read.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

func fail(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

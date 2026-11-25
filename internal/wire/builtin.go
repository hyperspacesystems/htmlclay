package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/aiedit"
	"github.com/panphora/htmlclay/internal/builtin"
)

// The built-in programs (internal/builtin) are answered here, never by a
// handler, and need the document's token, as ai-edit does. Unlike ai-edit each
// has its own switch, runs only for a document the person allowed, and only
// inside the trusted or served folder that holds it. One request per document
// per program runs at a time.

const (
	builtinBudget        = 5 * time.Minute
	builtinProgressEvery = 250 * time.Millisecond
	builtinOffText       = "This program is turned off in HTML Clay. Turn it on in Settings and try again."
	builtinUntrustedText = "Move this document into a trusted or served folder in HTML Clay before using this program."
	builtinAskText       = "HTML Clay asked whether this document may use this program. Answer in HTML Clay, then try again."
	builtinRemovedText   = "Permission for this document was removed. Allow it again in HTML Clay Settings to use this program."
)

type builtinRegistry struct {
	mu      sync.Mutex
	running map[string]AIEditRun // program + "\x00" + document path
}

func builtinKey(program, file string) string { return program + "\x00" + file }

// searchedProgress is the search's progress line, singular for one file.
func searchedProgress(files int) string {
	if files == 1 {
		return "Searched 1 file"
	}
	return fmt.Sprintf("Searched %d files", files)
}

// changesProgress is the folder watcher's progress line, singular for one change.
func changesProgress(changes int) string {
	if changes == 1 {
		return "1 change"
	}
	return fmt.Sprintf("%d changes", changes)
}

// builtinMeta lists the built-in programs in a document's discovery: ready when
// it may run them or may ask, unavailable when a program is off or the document
// is not in a trusted or served folder.
func (s *Core) builtinMeta(file string) []HelperMeta {
	if !aiedit.IsDocument(file) {
		return nil
	}
	meta := make([]HelperMeta, 0, len(builtin.IDs))
	for _, id := range builtin.IDs {
		state := "unavailable"
		if access := s.policy.BuiltinFor(file, id); access.State == builtin.Ready || access.State == builtin.Ask {
			state = "ready"
		}
		meta = append(meta, HelperMeta{Name: id, State: state})
	}
	return meta
}

// startBuiltin checks the program may run, registers the request and runs it
// in the background. The caller has already checked the document's token.
func (s *Core) startBuiltin(doc Document, request Envelope) {
	program := request.Helper
	if request.Type == "wire/describe" {
		s.refuseAIEdit(request, "host", "invalid_type", "This built-in program does not support that request.")
		return
	}
	access := s.policy.BuiltinFor(doc.Path, program)
	switch access.State {
	case builtin.Off:
		s.refuseAIEdit(request, "host", "helper_off", builtinOffText)
		return
	case builtin.Untrusted:
		s.refuseAIEdit(request, "host", "helper_not_granted", builtinUntrustedText)
		return
	case builtin.Removed:
		s.refuseAIEdit(request, "host", "helper_not_granted", builtinRemovedText)
		return
	case builtin.Ask:
		s.policy.AskBuiltin(doc.Path, program)
		s.refuseAIEdit(request, "host", "helper_not_granted", builtinAskText)
		return
	default:
		if access.State != builtin.Ready {
			s.refuseAIEdit(request, "host", "helper_off", builtinOffText)
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), builtinBudget)
	key := builtinKey(program, doc.Path)
	s.Builtin.mu.Lock()
	if s.Builtin.running == nil {
		s.Builtin.running = make(map[string]AIEditRun)
	}
	if run, busy := s.Builtin.running[key]; busy {
		s.Builtin.mu.Unlock()
		cancel()
		if run.ID == request.ID {
			s.refuseAIEdit(request, "host", "duplicate_request", "That request is already running.")
			s.Hub.forgetTerminal(request.File, request.ID)
		} else {
			s.refuseAIEdit(request, "host", "helper_busy", builtin.Title(program)+" is already running in this document.")
		}
		return
	}
	s.Builtin.running[key] = AIEditRun{ID: request.ID, Cancel: cancel}
	s.Builtin.mu.Unlock()
	s.Hub.forgetTerminal(request.File, request.ID)
	s.publishAIEdit(request, "wire/ack", "", map[string]any{"mode": "jsonl", "budgetMs": builtinBudget.Milliseconds()})
	go s.runBuiltin(ctx, cancel, key, access.Root, doc.Path, request)
}

func (s *Core) runBuiltin(ctx context.Context, cancel context.CancelFunc, key, root, file string, request Envelope) {
	var result any
	var err error
	switch request.Helper {
	case builtin.FileSearch:
		var req builtin.SearchRequest
		if err = json.Unmarshal(request.Payload, &req); err == nil && req.Reveal != "" {
			var abs string
			if abs, err = builtin.RevealTarget(root, req.Reveal); err == nil {
				if err = s.policy.RevealFile(abs); err == nil {
					result = map[string]string{"revealed": req.Reveal}
				}
			}
		} else if err == nil {
			var last time.Time
			result, err = builtin.Search(ctx, root, req, func(files int) {
				if now := time.Now(); now.Sub(last) >= builtinProgressEvery {
					last = now
					s.publishAIEdit(request, "wire/status", searchedProgress(files),
						map[string]any{"progress": map[string]any{"completed": files, "unit": "files"}})
				}
			})
		}
	case builtin.FileConvert:
		var req builtin.ConvertRequest
		if err = json.Unmarshal(request.Payload, &req); err == nil {
			result, err = builtin.Convert(root, req)
		}
	case builtin.FolderWatch:
		var req builtin.WatchRequest
		if err = json.Unmarshal(request.Payload, &req); err == nil {
			err = builtin.Watch(ctx, root, file+"\x00"+req.Path, req, builtin.WatchInterval, func(changes []builtin.Change) {
				s.publishAIEdit(request, "wire/status", changesProgress(len(changes)), map[string]any{"changes": changes})
			})
			if err == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
				result = map[string]string{"ended": "time"}
			}
		}
	}
	ctxErr := ctx.Err()
	// The slot is released before the terminal frame, so a page that asks again
	// on seeing it is never refused as busy.
	cancel()
	s.Builtin.mu.Lock()
	if run, ok := s.Builtin.running[key]; ok && run.ID == request.ID {
		delete(s.Builtin.running, key)
	}
	s.Builtin.mu.Unlock()

	var coded *builtin.Error
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case err == nil && result != nil:
		s.publishAIEdit(request, "wire/done", "", result)
	case errors.Is(ctxErr, context.DeadlineExceeded):
		s.refuseAIEdit(request, "host", "helper_timeout", builtin.Title(request.Helper)+" did not finish within 5 minutes.")
	case ctxErr != nil:
		s.refuseAIEdit(request, "host", "helper_cancelled", "Request stopped.")
	case errors.As(err, &coded):
		s.refuseAIEdit(request, "application", coded.Code, coded.Message)
	case errors.As(err, &syntaxErr), errors.As(err, &typeErr):
		s.refuseAIEdit(request, "application", "invalid_request", "HTML Clay could not read that request.")
	default:
		s.refuseAIEdit(request, "application", "failed", fmt.Sprint(err))
	}
}

// cancelBuiltin cancels a document's built-in request whose id matches, and
// reports whether it did.
func (s *Core) cancelBuiltin(file, id string) bool {
	s.Builtin.mu.Lock()
	defer s.Builtin.mu.Unlock()
	for _, program := range builtin.IDs {
		if run, ok := s.Builtin.running[builtinKey(program, file)]; ok && run.ID == id {
			run.Cancel()
			return true
		}
	}
	return false
}

// CancelBuiltins stops every running request of program, or of every built-in
// program when program is "": its switch went off, or the server is closing.
func (s *Core) CancelBuiltins(program string) {
	s.Builtin.mu.Lock()
	defer s.Builtin.mu.Unlock()
	for key, run := range s.Builtin.running {
		if program == "" || len(key) > len(program) && key[:len(program)+1] == program+"\x00" {
			run.Cancel()
		}
	}
}

// CancelBuiltinFor stops program's request on one document: the person took
// the program away from it.
func (s *Core) CancelBuiltinFor(program, file string) {
	s.Builtin.mu.Lock()
	defer s.Builtin.mu.Unlock()
	if run, ok := s.Builtin.running[builtinKey(program, file)]; ok {
		run.Cancel()
	}
}

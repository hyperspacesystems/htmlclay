package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/aiedit"
)

// The built-in ai-edit helper. ClayJS sends a named request with helper
// "ai-edit"; this server answers it itself, from internal/aiedit, with no
// declaration, no approval and without the document's handler slot. Only the
// AI Editing switch gates it, and one request runs per document at a time.

const (
	aiEditBudget        = 5 * time.Minute
	aiEditProgressEvery = 250 * time.Millisecond
	maxAIEditEnvelope   = 1 << 20
	aiEditOffText       = "AI Editing is off. Turn it on in HTML Clay's menu."
	aiEditAskText       = "AI editing cannot be used until you allow this document in HTML Clay."
)

type AIEditRun struct {
	ID     string
	Cancel context.CancelFunc
}

type aiEditRegistry struct {
	Mu      sync.Mutex
	Running map[string]AIEditRun // document path -> its one request in flight
}

func (s *Core) aiEditOffered() bool {
	return s.policy.AIEditOffered()
}

// aiEditMeta is the discovery entry for a document, when this server offers AI
// editing for it.
func (s *Core) aiEditMeta(file string) (HelperMeta, bool) {
	if !s.aiEditOffered() || !aiedit.IsDocument(file) {
		return HelperMeta{}, false
	}
	state := "unavailable"
	if on, ask := s.policy.AIEditFor(file); on || ask {
		state = "ready"
	}
	return HelperMeta{Name: aiedit.HelperName, State: state}, true
}

func (s *Core) publishAIEdit(request Envelope, typ, text string, payload any) {
	env := Envelope{
		V:      1,
		Type:   typ,
		ID:     request.ID,
		From:   "process",
		File:   request.File,
		Helper: request.Helper,
		Text:   text,
	}
	if payload != nil {
		env.Payload = encodeHelperJSON(payload)
	}
	if len(encodeHelperJSON(env)) > maxAIEditEnvelope {
		env.Type = "wire/error"
		env.Text = "the rewrite does not fit in a wire envelope"
		env.Payload = encodeHelperJSON(map[string]string{"source": "host", "code": "helper_result_too_large"})
	}
	s.Hub.Publish(request.File, env)
}

func (s *Core) refuseAIEdit(request Envelope, source, code, text string) {
	s.publishAIEdit(request, "wire/error", text, map[string]string{"source": source, "code": code})
}

// startAIEdit registers the request and runs it in the background. The caller
// has already checked the document's token.
func (s *Core) startAIEdit(doc Document, request Envelope) {
	if on, ask := s.policy.AIEditFor(doc.Path); !on {
		if ask {
			s.policy.AskAIEdit(doc.Path)
			s.refuseAIEdit(request, "host", "helper_not_granted", aiEditAskText)
			return
		}
		s.refuseAIEdit(request, "host", "helper_not_granted", aiEditOffText)
		return
	}
	if request.Type == "wire/describe" {
		s.refuseAIEdit(request, "host", "invalid_type", "ai-edit does not support wire/describe")
		return
	}
	var payload aiedit.Payload
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		s.refuseAIEdit(request, "application", "invalid_request", "malformed ai-edit request")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), aiEditBudget)
	s.AIEdit.Mu.Lock()
	if s.AIEdit.Running == nil {
		s.AIEdit.Running = make(map[string]AIEditRun)
	}
	if run, busy := s.AIEdit.Running[doc.Path]; busy {
		s.AIEdit.Mu.Unlock()
		cancel()
		if run.ID == request.ID {
			s.refuseAIEdit(request, "host", "duplicate_request", "a request with this id is already running")
			// This refusal is about the second request. Left retained, it would win
			// the id's one remembered outcome and a reconnect would replay it
			// instead of the running request's real answer.
			s.Hub.forgetTerminal(request.File, request.ID)
		} else {
			s.refuseAIEdit(request, "host", "helper_busy", "an AI edit is already running on this document")
		}
		return
	}
	s.AIEdit.Running[doc.Path] = AIEditRun{ID: request.ID, Cancel: cancel}
	s.AIEdit.Mu.Unlock()
	// An accepted request starts with no stale outcome under its id.
	s.Hub.forgetTerminal(request.File, request.ID)

	s.publishAIEdit(request, "wire/ack", "", map[string]any{"mode": "jsonl", "budgetMs": aiEditBudget.Milliseconds()})
	go s.RunAIEdit(ctx, cancel, doc.Path, request, payload)
}

func (s *Core) RunAIEdit(ctx context.Context, cancel context.CancelFunc, file string, request Envelope, payload aiedit.Payload) {
	// The slot is released before the terminal frame goes out, so a page that
	// sends its next request on seeing that frame is never refused as busy.
	release := func() {
		cancel()
		s.AIEdit.Mu.Lock()
		if run, ok := s.AIEdit.Running[file]; ok && run.ID == request.ID {
			delete(s.AIEdit.Running, file)
		}
		s.AIEdit.Mu.Unlock()
	}

	var def string
	var engines map[string][]string
	def, engines = s.policy.AIEditEngines()
	opts := aiedit.Options{
		File:     file,
		BaseDir:  filepath.Dir(file),
		Default:  def,
		Engines:  engines,
		Internal: s.docs.Internal,
		Mock:     os.Getenv("MOCK_MODEL") == "1",
		Status: func(text string) {
			s.publishAIEdit(request, "wire/status", text, nil)
		},
	}

	var progressMu sync.Mutex
	var completed int
	var last time.Time
	progress := func(text string) {
		progressMu.Lock()
		defer progressMu.Unlock()
		completed += len(text)
		now := time.Now()
		if !last.IsZero() && now.Sub(last) < aiEditProgressEvery {
			return
		}
		last = now
		s.publishAIEdit(request, "wire/status", fmt.Sprintf("Writing, %.1f KB", float64(completed)/1024),
			map[string]any{"progress": map[string]any{"completed": completed, "unit": "bytes"}})
	}

	result, err := aiedit.Run(ctx, payload, opts, progress)
	ctxErr := ctx.Err() // read before release, whose cancel() would set it
	release()
	switch {
	case err == nil:
		s.publishAIEdit(request, "wire/done", "", result)
	case errors.Is(ctxErr, context.DeadlineExceeded):
		s.refuseAIEdit(request, "host", "helper_timeout", "the AI edit did not finish within five minutes")
	case ctxErr != nil:
		s.refuseAIEdit(request, "host", "helper_cancelled", "helper cancelled")
	default:
		var coded *aiedit.Error
		if errors.As(err, &coded) {
			s.refuseAIEdit(request, "application", coded.Code, coded.Message)
		} else {
			s.refuseAIEdit(request, "application", "engine_failed", err.Error())
		}
	}
}

// cancelAIEdit cancels the running AI edit on a document when its id matches,
// and reports whether it did.
func (s *Core) cancelAIEdit(file, id string) bool {
	s.AIEdit.Mu.Lock()
	defer s.AIEdit.Mu.Unlock()
	run, ok := s.AIEdit.Running[file]
	if !ok || run.ID != id {
		return false
	}
	run.Cancel()
	return true
}

// CancelAIEditFor cancels the AI request running on one document: the person
// took AI editing away from it.
func (s *Core) CancelAIEditFor(file string) {
	s.AIEdit.Mu.Lock()
	defer s.AIEdit.Mu.Unlock()
	if run, ok := s.AIEdit.Running[file]; ok {
		run.Cancel()
	}
}

// CancelAIEdits cancels every running AI edit: the switch was turned off, or the
// server is shutting down.
func (s *Core) CancelAIEdits() {
	s.AIEdit.Mu.Lock()
	defer s.AIEdit.Mu.Unlock()
	for _, run := range s.AIEdit.Running {
		run.Cancel()
	}
}

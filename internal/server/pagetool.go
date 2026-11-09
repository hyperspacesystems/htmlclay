package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/panphora/htmlclay/internal/aiedit"
	"github.com/panphora/htmlclay/internal/clayruntime"
	"github.com/panphora/htmlclay/internal/session"
)

// The built-in page-tool helper. ClayJS sends a named request with helper
// "page-tool"; this server answers it itself, from internal/aiedit, exactly as
// it answers ai-edit: no declaration, no approval, only the AI Editing switch,
// and the same one-request-per-document slot so a tool and an edit can never
// race on one document.

const pageToolOffText = "AI Editing is off. Turn it on in HTML Clay's menu."

// pageToolBudget is a variable so a test can shorten it.
var pageToolBudget = 5 * time.Minute

// pageToolPrompts reads this helper's system prompt: tool-generation/system.md
// and context.md from the runtime vendored into this binary, at the selected
// version. A test replaces it with a fixture, because the vendored runtime does
// not carry the files until ClayJS ships them.
var pageToolPrompts = readPageToolPrompts

func readPageToolPrompts() (string, bool) {
	_, system, ok := clayruntime.Asset(clayruntime.Selected, "tool-generation/system.md")
	if !ok {
		return "", false
	}
	_, context, ok := clayruntime.Asset(clayruntime.Selected, "tool-generation/context.md")
	if !ok {
		return "", false
	}
	head := bytes.TrimSpace(system)
	tail := bytes.TrimSpace(context)
	if len(head) == 0 || len(tail) == 0 {
		return "", false
	}
	return string(head) + "\n\n" + string(tail) + "\n", true
}

// pageToolOffered reports whether this server can answer page-tool at all: the
// AI Editing hook, and the prompt files it needs.
func (s *Server) pageToolOffered() bool {
	if !s.aiEditOffered() {
		return false
	}
	_, ok := pageToolPrompts()
	return ok
}

// pageToolMeta is the discovery entry for a document, when this server offers
// the page-tool helper for it.
func (s *Server) pageToolMeta(file string) (helperMeta, bool) {
	if !s.pageToolOffered() || !aiedit.IsDocument(file) {
		return helperMeta{}, false
	}
	state := "unavailable"
	if s.hooks.AIEditEnabled() {
		state = "ready"
	}
	return helperMeta{Name: aiedit.PageToolHelperName, State: state}, true
}

type pageToolDone struct {
	RequestID string          `json:"requestId"`
	Artifact  json.RawMessage `json:"artifact"`
	State     json.RawMessage `json:"state"`
	Model     string          `json:"model"`
}

// startPageTool registers the request and runs it in the background. The caller
// has already checked the document's token.
func (s *Server) startPageTool(f *session.File, request wireEnvelope) {
	if !s.hooks.AIEditEnabled() {
		s.refuseAIEdit(request, "host", "helper_not_granted", pageToolOffText)
		return
	}
	systemPrompt, ok := pageToolPrompts()
	if !ok {
		s.refuseAIEdit(request, "host", "helper_not_granted", "this build has no tool prompt files")
		return
	}
	if request.Type == "wire/describe" {
		s.refuseAIEdit(request, "host", "invalid_type", "page-tool does not support wire/describe")
		return
	}
	toolRequest, requestErr := aiedit.ValidateToolRequest(request.Payload)
	if requestErr != nil {
		s.refuseAIEdit(request, "application", requestErr.Code, requestErr.Message)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), pageToolBudget)
	s.aiEdit.mu.Lock()
	if s.aiEdit.running == nil {
		s.aiEdit.running = make(map[string]aiEditRun)
	}
	if run, busy := s.aiEdit.running[f.AbsPath]; busy {
		s.aiEdit.mu.Unlock()
		cancel()
		if run.id == request.ID {
			s.refuseAIEdit(request, "host", "duplicate_request", "a request with this id is already running")
			s.wire.forgetTerminal(request.File, request.ID)
		} else {
			s.refuseAIEdit(request, "host", "helper_busy", "an AI request is already running on this document")
		}
		return
	}
	s.aiEdit.running[f.AbsPath] = aiEditRun{id: request.ID, cancel: cancel}
	s.aiEdit.mu.Unlock()
	s.wire.forgetTerminal(request.File, request.ID)

	s.publishAIEdit(request, "wire/ack", "", map[string]any{"mode": "jsonl", "budgetMs": pageToolBudget.Milliseconds()})
	s.publishAIEdit(request, "wire/status", "Making your tool…", nil)
	go s.runPageTool(ctx, cancel, f.AbsPath, request, toolRequest, systemPrompt)
}

func (s *Server) runPageTool(ctx context.Context, cancel context.CancelFunc, file string, request wireEnvelope, toolRequest aiedit.ToolRequest, systemPrompt string) {
	release := func() {
		cancel()
		s.aiEdit.mu.Lock()
		if run, ok := s.aiEdit.running[file]; ok && run.id == request.ID {
			delete(s.aiEdit.running, file)
		}
		s.aiEdit.mu.Unlock()
	}

	var def string
	var engines map[string][]string
	if s.hooks.AIEditEngines != nil {
		def, engines = s.hooks.AIEditEngines()
	}
	opts := aiedit.ToolOptions{
		BaseDir:      filepath.Dir(file),
		Default:      def,
		Engines:      engines,
		Mock:         os.Getenv("MOCK_MODEL") == "1",
		SystemPrompt: systemPrompt,
		Status: func(text string) {
			s.publishAIEdit(request, "wire/status", text, nil)
		},
	}

	reply, err := aiedit.RunTool(ctx, toolRequest, opts)
	ctxErr := ctx.Err() // read before release, whose cancel() would set it
	release()
	switch {
	case err == nil:
		s.publishAIEdit(request, "wire/done", "", pageToolDone{
			RequestID: request.ID,
			Artifact:  reply.Artifact,
			State:     reply.State,
			Model:     reply.Model,
		})
	case errors.Is(ctxErr, context.DeadlineExceeded):
		s.refuseAIEdit(request, "host", "helper_timeout", "the tool did not finish within five minutes")
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

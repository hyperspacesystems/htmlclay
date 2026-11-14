package wire

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
)

// The built-in blockeditor-tool helper. ClayJS sends a named request with helper
// "blockeditor-tool"; this server answers it itself, from internal/aiedit, exactly as
// it answers ai-edit: no declaration, no approval, only the AI Editing switch,
// and the same one-request-per-document slot so a tool and an edit can never
// race on one document.

const pageToolOffText = "AI Editing is off. Turn it on in HTML Clay's menu."

// PageToolBudget is a variable so a test can shorten it.
var PageToolBudget = 5 * time.Minute

// PageToolPrompts reads this helper's system prompt: tool-generation/system.md
// and context.md from the runtime vendored into this binary, at the selected
// version. A test replaces it with a fixture, because the vendored runtime does
// not carry the files until ClayJS ships them.
var PageToolPrompts = ReadPageToolPrompts

func ReadPageToolPrompts() (string, bool) {
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

// pageToolOffered reports whether this server can answer blockeditor-tool at all: the
// AI Editing hook, and the prompt files it needs.
func (s *Core) pageToolOffered() bool {
	if !s.aiEditOffered() {
		return false
	}
	_, ok := PageToolPrompts()
	return ok
}

// pageToolMeta is the discovery entry for a document, when this server offers
// the blockeditor-tool helper for it.
func (s *Core) pageToolMeta(file string) (HelperMeta, bool) {
	if !s.pageToolOffered() || !aiedit.IsDocument(file) {
		return HelperMeta{}, false
	}
	state := "unavailable"
	if on, ask := s.policy.AIEditFor(file); on || ask {
		state = "ready"
	}
	return HelperMeta{Name: aiedit.PageToolHelperName, State: state}, true
}

type pageToolDone struct {
	RequestID string          `json:"requestId"`
	Artifact  json.RawMessage `json:"artifact"`
	State     json.RawMessage `json:"state"`
	Model     string          `json:"model"`
}

// startPageTool registers the request and runs it in the background. The caller
// has already checked the document's token.
func (s *Core) startPageTool(doc Document, request Envelope) {
	if on, ask := s.policy.AIEditFor(doc.Path); !on {
		if ask {
			s.policy.AskAIEdit(doc.Path)
			s.refuseAIEdit(request, "host", "helper_not_granted", aiEditAskText)
			return
		}
		s.refuseAIEdit(request, "host", "helper_not_granted", pageToolOffText)
		return
	}
	systemPrompt, ok := PageToolPrompts()
	if !ok {
		s.refuseAIEdit(request, "host", "helper_not_granted", "this build has no tool prompt files")
		return
	}
	if request.Type == "wire/describe" {
		s.refuseAIEdit(request, "host", "invalid_type", "blockeditor-tool does not support wire/describe")
		return
	}
	toolRequest, requestErr := aiedit.ValidateToolRequest(request.Payload)
	if requestErr != nil {
		s.refuseAIEdit(request, "application", requestErr.Code, requestErr.Message)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), PageToolBudget)
	s.AIEdit.Mu.Lock()
	if s.AIEdit.Running == nil {
		s.AIEdit.Running = make(map[string]AIEditRun)
	}
	if run, busy := s.AIEdit.Running[doc.Path]; busy {
		s.AIEdit.Mu.Unlock()
		cancel()
		if run.ID == request.ID {
			s.refuseAIEdit(request, "host", "duplicate_request", "a request with this id is already running")
			s.Hub.forgetTerminal(request.File, request.ID)
		} else {
			s.refuseAIEdit(request, "host", "helper_busy", "an AI request is already running on this document")
		}
		return
	}
	s.AIEdit.Running[doc.Path] = AIEditRun{ID: request.ID, Cancel: cancel}
	s.AIEdit.Mu.Unlock()
	s.Hub.forgetTerminal(request.File, request.ID)

	s.publishAIEdit(request, "wire/ack", "", map[string]any{"mode": "jsonl", "budgetMs": PageToolBudget.Milliseconds()})
	s.publishAIEdit(request, "wire/status", "Making your tool…", nil)
	go s.runPageTool(ctx, cancel, doc.Path, request, toolRequest, systemPrompt)
}

func (s *Core) runPageTool(ctx context.Context, cancel context.CancelFunc, file string, request Envelope, toolRequest aiedit.ToolRequest, systemPrompt string) {
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

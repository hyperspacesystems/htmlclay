package wire

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/aiedit"
	"github.com/panphora/htmlclay/internal/aitools"
	"github.com/panphora/htmlclay/internal/pathrel"
)

// The built-in ai-tools helper. ClayJS's clay.ai.ask sends a named request
// with helper "ai-tools"; this server answers it itself, from internal/aitools:
// it checks the document's trust, asks the person for a folder when the
// document has none, and runs the person's own agent with read tools there.
// The request is parked while the person is asked (up to five minutes, the
// first ack's 20 minute budget covers that and the run) and gets a second ack
// with the run's own 15 minute budget once the folder is known.

var (
	aiToolsParkBudget = 20 * time.Minute
	aiToolsAskLimit   = 5 * time.Minute
	aiToolsRunBudget  = 15 * time.Minute
)

const aiToolsOffText = "AI is off in HTML Clay. Turn on AI Editing in HTML Clay's menu, then try again."

// AIToolsHost is the app's side of ai-tools.
type AIToolsHost interface {
	// Enabled reports the person's AI switch.
	Enabled() bool
	Service() *aitools.Service
	// Preferred is the agent the person chose in HTML Clay, and the model for
	// each agent.
	Preferred() (agent string, models map[string]string)
	// Folder is the folder document's agent reads: the one the person chose
	// for it before, or a new choice when choose is true or there is none.
	// asking is called when it raises a question; it waits for the answer
	// until ctx ends. A refusal is an *aitools.Error.
	Folder(ctx context.Context, document string, choose bool, level string, asking func()) (folder string, remembered bool, err error)
	// HasFolder reports a document the person gave a folder.
	HasFolder(document string) bool
}

type aiToolsRegistry struct {
	Mu      sync.Mutex
	Running map[string]AIEditRun // document path -> its one request in flight
	Asked   map[string]bool      // documents that sent a request to this Core
}

// CancelAIUnder cancels every AI edit, ai-tools request and program for a
// document in dir.
func (s *Core) CancelAIUnder(dir string) {
	s.cancelProgramsUnder(dir)
	s.AITools.Mu.Lock()
	for path, run := range s.AITools.Running {
		if pathrel.SameOrBelow(path, dir) {
			run.Cancel()
		}
	}
	s.AITools.Mu.Unlock()
	s.AIEdit.Mu.Lock()
	for path, run := range s.AIEdit.Running {
		if pathrel.SameOrBelow(path, dir) {
			run.Cancel()
		}
	}
	s.AIEdit.Mu.Unlock()
}

func (s *Core) aiToolsHost() AIToolsHost {
	if s.policy == nil {
		return nil
	}
	return s.policy.AITools()
}

// Trust is a document's trust as the host sees it.
type Trust struct {
	// Tier is "mine", "ecosystem", "team" or "unknown".
	Tier string
	// Ready is whether an agent may reach the document's folder now: its code
	// is vouched for or allowed, and the person did not refuse it.
	Ready bool
	// Denied is whether the person chose Don't allow for the document.
	Denied bool
}

// aiToolsMeta is the discovery entry for a document, when this host offers
// AI with tools.
func (s *Core) aiToolsMeta(file string) (HelperMeta, bool) {
	host := s.aiToolsHost()
	if host == nil || !aiedit.IsDocument(file) {
		return HelperMeta{}, false
	}
	state := "unavailable"
	switch {
	case !host.Enabled():
	case s.policy.DocumentTrust(file).Denied:
		state = "denied"
	default:
		state = "ready"
	}
	return HelperMeta{Name: aitools.HelperName, State: state}, true
}

// refuseRun ends an ai-tools or programs request with code. Codes of the
// wire itself come from the host; every other code from the application.
func (s *Core) refuseRun(request Envelope, code, text string) {
	source := "application"
	if strings.HasPrefix(code, "helper_") || code == "duplicate_request" || code == "invalid_type" {
		source = "host"
	}
	s.publishAIEdit(request, "wire/error", text, map[string]string{"source": source, "code": code})
}

func (s *Core) runStatus(request Envelope, phase, text string) {
	s.publishAIEdit(request, "wire/status", text, map[string]any{"phase": phase})
}

// startAITools checks the request and the document's trust, takes the
// document's one ai-tools slot and parks the request. The caller has already
// checked the document's token.
func (s *Core) startAITools(doc Document, request Envelope) {
	host := s.aiToolsHost()
	switch {
	case host == nil || !host.Enabled():
		s.refuseRun(request, "helper_off", aiToolsOffText)
		return
	case request.Type != "wire/request":
		s.refuseRun(request, "invalid_type", "ai-tools does not support "+request.Type)
		return
	}
	payload, perr := aitools.Parse(request.Payload)
	if perr != nil {
		s.refuseRun(request, perr.Code, perr.Message)
		return
	}
	trust := s.policy.DocumentTrust(doc.Path)
	if !trust.Ready {
		s.policy.AskDocument(doc.Path, aitools.HelperName)
		s.refuseRun(request, "helper_not_granted", aiEditAskText)
		return
	}
	level := payload.Tools
	switch {
	case level == aitools.LevelInternet:
		s.refuseRun(request, aitools.CodeLevelUnavailable, "HTML Clay does not let an agent use the internet for a page.")
		return
	case level == aitools.LevelChange && trust.Tier != "mine" && trust.Tier != "ecosystem":
		level = aitools.LevelRead
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.AITools.Mu.Lock()
	if s.AITools.Running == nil {
		s.AITools.Running = make(map[string]AIEditRun)
	}
	if run, busy := s.AITools.Running[doc.Path]; busy {
		s.AITools.Mu.Unlock()
		cancel()
		if run.ID == request.ID {
			s.refuseRun(request, "duplicate_request", "a request with this id is already running")
			s.Hub.forgetTerminal(request.File, request.ID)
		} else {
			s.refuseRun(request, "helper_busy", "This page already has an AI request running. Wait for it to finish, or stop it.")
		}
		return
	}
	s.AITools.Running[doc.Path] = AIEditRun{ID: request.ID, Cancel: cancel}
	if s.AITools.Asked == nil {
		s.AITools.Asked = make(map[string]bool)
	}
	s.AITools.Asked[doc.Path] = true
	s.AITools.Mu.Unlock()
	s.Hub.forgetTerminal(request.File, request.ID)

	s.publishAIEdit(request, "wire/ack", "", map[string]any{"mode": "jsonl", "budgetMs": aiToolsParkBudget.Milliseconds()})
	go s.runAITools(ctx, cancel, doc, request, payload, level, host)
}

func (s *Core) runAITools(ctx context.Context, cancel context.CancelFunc, doc Document, request Envelope, payload aitools.Request, level string, host AIToolsHost) {
	run := NewRunID()
	release := func() {
		cancel()
		s.AITools.Mu.Lock()
		if r, ok := s.AITools.Running[doc.Path]; ok && r.ID == request.ID {
			delete(s.AITools.Running, doc.Path)
		}
		s.AITools.Mu.Unlock()
	}

	askCtx, askCancel := context.WithTimeout(ctx, aiToolsAskLimit)
	folder, remembered, err := host.Folder(askCtx, doc.Path, payload.Folder == aitools.FolderChoose, level, func() {
		s.runStatus(request, "asking", "Choose a folder for this page in HTML Clay")
	})
	askErr := askCtx.Err()
	askCancel()
	if err != nil {
		cancelled := ctx.Err() != nil
		release()
		switch {
		case cancelled:
			s.refuseRun(request, "helper_cancelled", "helper cancelled")
		case errors.Is(askErr, context.DeadlineExceeded):
			s.refuseRun(request, "helper_timeout", "Nobody chose a folder in HTML Clay within five minutes.")
		default:
			s.refuseAIToolsErr(request, err)
		}
		return
	}

	s.publishAIEdit(request, "wire/ack", "", map[string]any{"mode": "jsonl", "budgetMs": aiToolsRunBudget.Milliseconds()})
	runCtx, runCancel := context.WithTimeout(ctx, aiToolsRunBudget)
	agent, models := host.Preferred()
	result, err := host.Service().Run(runCtx, aitools.Job{
		Document:  doc.Path,
		Folder:    folder,
		Level:     level,
		Request:   payload,
		Preferred: agent,
		Models:    models,
		Status:    func(phase, text string) { s.runStatus(request, phase, text) },
		Put: func(id string, body []byte) (string, string, error) {
			ref, err := s.RunFiles.Put(doc.Path, run, id, body)
			if errors.Is(err, ErrRunFileTooLarge) || errors.Is(err, ErrRunTooLarge) {
				return "", "", &aitools.Error{Code: aitools.CodeSourceTooLarge, Message: "The code this answer is about is too large to send to the page. Ask about a smaller part of the folder."}
			}
			return ref.URL, ref.SHA256, err
		},
	})
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	runCancel()
	cancelled := ctx.Err() != nil
	release()
	switch {
	case err == nil:
		s.RunFiles.Finish(run)
		var model any
		if result.Model != "" {
			model = result.Model
		}
		s.publishAIEdit(request, "wire/done", "", map[string]any{
			"data":    result.Data,
			"sources": result.Sources,
			"folder":  map[string]string{"name": filepath.Base(folder)},
			"grant":   map[string]any{"level": level, "remembered": remembered},
			"engine":  result.Engine,
			"model":   model,
		})
		return
	case cancelled:
		s.RunFiles.Drop(run)
		s.refuseRun(request, "helper_cancelled", "helper cancelled")
	case timedOut:
		s.RunFiles.Drop(run)
		s.refuseRun(request, "helper_timeout", "The agent did not finish within 15 minutes.")
	default:
		s.RunFiles.Drop(run)
		s.refuseAIToolsErr(request, err)
	}
}

func (s *Core) refuseAIToolsErr(request Envelope, err error) {
	var coded *aitools.Error
	if errors.As(err, &coded) {
		s.refuseRun(request, coded.Code, coded.Message)
		return
	}
	s.refuseRun(request, aitools.CodeEngineFailed, err.Error())
}

// cancelAITools cancels the running ai-tools request on a document when its
// id matches, and reports whether it did.
func (s *Core) cancelAITools(file, id string) bool {
	s.AITools.Mu.Lock()
	defer s.AITools.Mu.Unlock()
	run, ok := s.AITools.Running[file]
	if !ok || run.ID != id {
		return false
	}
	run.Cancel()
	return true
}

// CancelAIToolsFor cancels the ai-tools request running on one document.
func (s *Core) CancelAIToolsFor(file string) {
	s.AITools.Mu.Lock()
	defer s.AITools.Mu.Unlock()
	if run, ok := s.AITools.Running[file]; ok {
		run.Cancel()
	}
}

// CancelAIFor cancels every AI run on one document, an AI edit, an ai-tools
// request and a program: the document was refused, forgotten or lost its
// access.
func (s *Core) CancelAIFor(file string) {
	s.CancelAIEditFor(file)
	s.CancelAIToolsFor(file)
	s.CancelProgramsFor(file)
}

// CancelAI cancels every AI run and every program: the AI Editing switch was
// turned off, or the host is shutting down.
func (s *Core) CancelAI() {
	s.CancelAIEdits()
	s.CancelAITools()
	s.CancelPrograms()
}

// CancelAITools cancels every running ai-tools request.
func (s *Core) CancelAITools() {
	s.AITools.Mu.Lock()
	defer s.AITools.Mu.Unlock()
	for _, run := range s.AITools.Running {
		run.Cancel()
	}
}

// RelayBlockedText is live sync's refusal for a document RelayBlocked reports.
const RelayBlockedText = "live sync is off for a document that runs programs or AI with tools"

// RelayBlocked reports a document live sync must not push a snapshot into:
// one with programs bound, or one that can run AI with tools. A snapshot from
// another tab of the same origin would otherwise run its scripts with that
// document's abilities. A document can run AI with tools when it asked this
// Core, holds a remembered folder, or loads ClayJS's ai plugin or carries
// instructions on disk; Block Editor pages load none of these.
func (s *Core) RelayBlocked(path string) bool {
	return s.HelpersBound(path) || s.aiToolsCapable(path) || s.programsCapable(path)
}

// WhileRelayAllowed runs fn while path may take a relayed snapshot, holding
// the helper bindings' lock across fn as WhileUnbound does, and reports
// whether it ran.
func (s *Core) WhileRelayAllowed(path string, fn func()) bool {
	if s.aiToolsCapable(path) || s.programsCapable(path) {
		return false
	}
	ran := false
	s.WhileUnbound(path, func() {
		if !s.aiToolsAsked(path) && !s.programsAsked(path) {
			fn()
			ran = true
		}
	})
	return ran
}

func (s *Core) aiToolsAsked(path string) bool {
	s.AITools.Mu.Lock()
	defer s.AITools.Mu.Unlock()
	return s.AITools.Asked[path]
}

func (s *Core) aiToolsCapable(path string) bool {
	if s.aiToolsAsked(path) {
		return true
	}
	if host := s.aiToolsHost(); host != nil && host.HasFolder(path) {
		return true
	}
	return aitools.DocumentCarriesAI(path)
}

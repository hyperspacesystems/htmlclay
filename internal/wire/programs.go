package wire

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/panphora/htmlclay/internal/aiedit"
	"github.com/panphora/htmlclay/internal/aitools"
	"github.com/panphora/htmlclay/internal/carried"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/htmlutil"
	"github.com/panphora/htmlclay/internal/pathrel"
)

// The built-in programs helper. ClayJS's clay.programs.run sends a named
// request with helper "programs" and the name of one of the document's
// <script type="text/x-clay-program"> tags. The page never sends program
// text: the host reads the tag from the file on disk, checks the trust of
// those exact bytes, asks for a folder when the program needs one, reads the
// file again and runs the program only if its text is unchanged, from the
// stored copy, in a worker process (internal/carried).

var (
	programsParkBudget = 20 * time.Minute
	programsAskLimit   = 5 * time.Minute
	programsRunBudget  = carried.DefaultTimeout + 30*time.Second
	programsLogWait    = 100 * time.Millisecond
)

const (
	programsOffText    = "Programs are off in HTML Clay. Turn on AI Editing in HTML Clay's menu, then try again."
	programsChangeText = "This page can read files, not change them. Only documents you made or trusted can change files. Run htmlclay trust on this file, then run the program again."
	maxProgramInput    = 256 << 10
	maxLogFrame        = 16 << 10
	maxLogPiece        = 64 << 10
	maxDoneLog         = 16 << 10
	programsFolderText = "Choose a folder for this page's program in HTML Clay"
)

// ProgramsHost is the app's side of the programs helper.
type ProgramsHost interface {
	// Enabled reports whether carried programs may run now. They run under
	// the AI Editing switch, as AI with tools does.
	Enabled() bool
	// Load reads document once and returns its program tags with the trust
	// of exactly the bytes they came from.
	Load(document string) (ProgramSet, error)
	// Folder is the folder document's programs read: the one the person
	// chose for the page before (shared with ai-tools), or a new choice. The
	// question names program. A refusal is an *aitools.Error.
	Folder(ctx context.Context, document, program string, choose bool, level string, asking func()) (folder string, remembered bool, err error)
	// HasFolder reports a document the person gave a folder.
	HasFolder(document string) bool
	// Run stores the job's modules, runs the stored copies in a worker and
	// answers the outcome.
	Run(ctx context.Context, job carried.Job) (carried.Result, error)
}

// ProgramSet is a document's program tags and the trust of the bytes they
// were read from.
type ProgramSet struct {
	Trust Trust
	Scan  htmlutil.ProgramScan
}

type programsRequest struct {
	Program string          `json:"program"`
	Input   json.RawMessage `json:"input"`
	Folder  string          `json:"folder"`
}

func (s *Core) programsHost() ProgramsHost {
	if s.policy == nil {
		return nil
	}
	return s.policy.Programs()
}

// programsMeta is the discovery entry for a document, when this host runs
// carried programs.
func (s *Core) programsMeta(file string) (HelperMeta, bool) {
	host := s.programsHost()
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
	return HelperMeta{Name: carried.HelperName, State: state}, true
}

func (s *Core) refuseProgramsErr(request Envelope, err error) {
	var coded *carried.Error
	if errors.As(err, &coded) {
		s.refuseRun(request, coded.Code, coded.Message)
		return
	}
	var tools *aitools.Error
	if errors.As(err, &tools) {
		s.refuseRun(request, tools.Code, tools.Message)
		return
	}
	s.refuseRun(request, carried.CodeProgramFailed, "The program could not run.")
}

func parseProgramsRequest(raw json.RawMessage) (programsRequest, *carried.Error) {
	var p programsRequest
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, &carried.Error{Code: "invalid_request", Message: "A programs request is an object with a program name and its input."}
	}
	if !config.ValidHelperName(p.Program) || config.IsReservedHelperName(p.Program) {
		return p, &carried.Error{Code: "invalid_request", Message: "A program name is 1 to 32 lowercase letters, digits and hyphens, and not one HTML Clay uses itself."}
	}
	if len(p.Input) > maxProgramInput {
		return p, &carried.Error{Code: "invalid_request", Message: "A program's input is larger than 256 KB."}
	}
	if p.Folder != "" && p.Folder != "remembered" && p.Folder != "choose" {
		return p, &carried.Error{Code: "invalid_request", Message: `folder must be "remembered" or "choose".`}
	}
	return p, nil
}

// pickProgram finds the named program in set and reads its level.
func pickProgram(set ProgramSet, name string) (htmlutil.Program, carried.Level, *carried.Error) {
	prog, ok := set.Scan.Program(name)
	if !ok {
		if reason, seen := set.Scan.Reason(name); seen {
			code := carried.CodeProgramNotRead
			if reason == htmlutil.NotReadDuplicate {
				code = carried.CodeProgramInvalid
			}
			return prog, "", &carried.Error{Code: code, Message: notReadText(name, reason)}
		}
		return prog, "", &carried.Error{Code: carried.CodeProgramMissing, Message: "This page has no program named " + name + " in its saved file."}
	}
	level, err := carried.ParseLevel(prog.Can)
	if err != nil {
		var coded *carried.Error
		errors.As(err, &coded)
		return prog, "", coded
	}
	return prog, level, nil
}

func notReadText(name, reason string) string {
	switch reason {
	case htmlutil.NotReadPastLimit, htmlutil.NotReadCut:
		return "HTML Clay only reads programs inside the first 2.5 MB of a file. Move the program " + name + " closer to the top of the file."
	case htmlutil.NotReadTooLarge:
		return "The program " + name + " is larger than 256 KB, so HTML Clay did not read it."
	case htmlutil.NotReadTooMany:
		return "A file can carry at most 8 programs, and the program " + name + " comes after the first 8."
	case htmlutil.NotReadDuplicate:
		return "More than one program in this file is named " + name + ", so HTML Clay did not run it. Give each program its own name."
	}
	return "HTML Clay did not read the program " + name + "."
}

// startPrograms checks the request, the document's trust and the program,
// takes the document's one programs slot and parks the request. The caller
// has already checked the document's token.
func (s *Core) startPrograms(doc Document, request Envelope) {
	host := s.programsHost()
	switch {
	case host == nil || !host.Enabled():
		s.refuseRun(request, "helper_off", programsOffText)
		return
	case request.Type != "wire/request":
		s.refuseRun(request, "invalid_type", "programs does not support "+request.Type)
		return
	}
	payload, perr := parseProgramsRequest(request.Payload)
	if perr != nil {
		s.refuseRun(request, perr.Code, perr.Message)
		return
	}
	set, err := host.Load(doc.Path)
	if err != nil {
		s.refuseProgramsErr(request, err)
		return
	}
	if !set.Trust.Ready {
		s.policy.AskDocument(doc.Path, payload.Program)
		s.refuseRun(request, "helper_not_granted", aiEditAskText)
		return
	}
	prog, level, cerr := pickProgram(set, payload.Program)
	if cerr != nil {
		s.refuseRun(request, cerr.Code, cerr.Message)
		return
	}
	calls, allowed := carried.Policy(set.Trust.Tier, level)
	if !allowed {
		s.refuseRun(request, carried.CodeLevelUnavailable, programsChangeText)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.Programs.Mu.Lock()
	if s.Programs.Running == nil {
		s.Programs.Running = make(map[string]AIEditRun)
	}
	if run, busy := s.Programs.Running[doc.Path]; busy {
		s.Programs.Mu.Unlock()
		cancel()
		if run.ID == request.ID {
			s.refuseRun(request, "duplicate_request", "a request with this id is already running")
			s.Hub.forgetTerminal(request.File, request.ID)
		} else {
			s.refuseRun(request, "helper_busy", "This page already has a program running. Wait for it to finish, or stop it.")
		}
		return
	}
	s.Programs.Running[doc.Path] = AIEditRun{ID: request.ID, Cancel: cancel}
	if s.Programs.Asked == nil {
		s.Programs.Asked = make(map[string]bool)
	}
	s.Programs.Asked[doc.Path] = true
	s.Programs.Mu.Unlock()
	s.Hub.forgetTerminal(request.File, request.ID)

	s.publishAIEdit(request, "wire/ack", "", map[string]any{"mode": "jsonl", "budgetMs": programsParkBudget.Milliseconds()})
	go s.runProgram(ctx, cancel, doc, request, payload, set.Trust.Tier, prog, level, calls, set.Scan.Programs, host)
}

// runProgram asks for the folder, reads the file again and runs the program.
// calls hold for the whole run: the re-read refuses a changed tier.
func (s *Core) runProgram(ctx context.Context, cancel context.CancelFunc, doc Document, request Envelope, payload programsRequest, tier string, prog htmlutil.Program, level carried.Level, calls carried.Calls, initialPrograms []htmlutil.Program, host ProgramsHost) {
	release := func() {
		cancel()
		s.Programs.Mu.Lock()
		if r, ok := s.Programs.Running[doc.Path]; ok && r.ID == request.ID {
			delete(s.Programs.Running, doc.Path)
		}
		s.Programs.Mu.Unlock()
	}
	folder, remembered := "", false
	if level.NeedsFolder() {
		askCtx, askCancel := context.WithTimeout(ctx, programsAskLimit)
		var err error
		folder, remembered, err = host.Folder(askCtx, doc.Path, prog.Name, payload.Folder == "choose", string(level), func() {
			s.runStatus(request, "asking", programsFolderText)
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
				s.refuseProgramsErr(request, err)
			}
			return
		}
	}

	// The question may have taken minutes. Read the file again: the program
	// runs only if the bytes are still trusted and its text is the one
	// checked, so a page cannot swap what runs while the person answers.
	set, err := host.Load(doc.Path)
	if err != nil {
		release()
		s.refuseProgramsErr(request, err)
		return
	}
	if !set.Trust.Ready {
		release()
		s.policy.AskDocument(doc.Path, payload.Program)
		s.refuseRun(request, "helper_not_granted", aiEditAskText)
		return
	}
	again, _, cerr := pickProgram(set, prog.Name)
	if cerr != nil || again.Text != prog.Text || again.Can != prog.Can || set.Trust.Tier != tier || !slices.Equal(set.Scan.Programs, initialPrograms) {
		release()
		s.refuseRun(request, carried.CodeProgramChanged, "The program "+prog.Name+" changed while HTML Clay was getting it ready. Run it again.")
		return
	}
	modules := make(map[string]string, len(set.Scan.Programs))
	for _, p := range set.Scan.Programs {
		modules[p.Name] = p.Text
	}

	s.publishAIEdit(request, "wire/ack", "", map[string]any{"mode": "jsonl", "budgetMs": programsRunBudget.Milliseconds()})
	s.runStatus(request, "starting", "Starting "+prog.Name)
	run := NewRunID()
	pump := &logPump{send: func(text string) { s.runStatus(request, "log", text) }}
	sp := &statusPump{send: func(text string) { s.runStatus(request, "working", text) }}
	result, err := host.Run(ctx, carried.Job{
		Entry:   prog.Name,
		Modules: modules,
		Input:   payload.Input,
		Level:   level,
		Calls:   calls,
		Folder:  folder,
		Status:  sp.set,
		Log:     pump.add,
		Put: func(name string, body []byte) (string, error) {
			ref, err := s.RunFiles.Put(doc.Path, run, name, body)
			if errors.Is(err, ErrRunFileTooLarge) || errors.Is(err, ErrRunTooLarge) {
				return "", &carried.Error{Code: carried.CodeTooLarge, Message: "This output is larger than a run may keep (4 MB a file, 8 MB a run)."}
			}
			return ref.URL, err
		},
	})
	pump.stop()
	sp.stop()
	cancelled := ctx.Err() != nil
	release()
	switch {
	case err == nil:
		s.RunFiles.Finish(run)
		var folderInfo any
		if folder != "" {
			folderInfo = map[string]string{"name": filepath.Base(folder)}
		}
		data := result.Data
		if len(data) == 0 {
			data = json.RawMessage("null")
		}
		outputs := result.Outputs
		if outputs == nil {
			outputs = []carried.Output{}
		}
		s.publishAIEdit(request, "wire/done", "", map[string]any{
			"program": prog.Name,
			"data":    data,
			"log":     lastBytes(result.Log, maxDoneLog),
			"logCut":  result.LogCut || len(result.Log) > maxDoneLog,
			"outputs": outputs,
			"folder":  folderInfo,
			"grant":   map[string]any{"level": string(level), "remembered": remembered},
		})
	case cancelled:
		s.RunFiles.Drop(run)
		s.refuseRun(request, "helper_cancelled", "helper cancelled")
	default:
		s.RunFiles.Drop(run)
		s.refuseProgramsErr(request, err)
	}
}

// lastBytes is the end of text, at most n bytes, starting on a whole
// character.
func lastBytes(text string, n int) string {
	if len(text) <= n {
		return text
	}
	text = text[len(text)-n:]
	for len(text) > 0 && !utf8.RuneStart(text[0]) {
		text = text[1:]
	}
	return text
}

// logPump turns a program's console log into status frames: at most one
// frame each 100 ms, of at most 16 KB, so a program that prints in a loop
// cannot flood the page's stream.
type logPump struct {
	mu      sync.Mutex
	buf     []byte
	timer   *time.Timer
	stopped bool
	send    func(text string)
}

func (p *logPump) add(text string) {
	if len(text) > maxLogPiece {
		text = strings.ToValidUTF8(text[:maxLogPiece], "")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return
	}
	p.buf = append(p.buf, text...)
	if p.timer == nil {
		p.timer = time.AfterFunc(programsLogWait, p.tick)
	}
}

func (p *logPump) tick() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.timer = nil
	if p.stopped || len(p.buf) == 0 {
		return
	}
	n := min(len(p.buf), maxLogFrame)
	for n < len(p.buf) && n > 0 && !utf8.RuneStart(p.buf[n]) {
		n--
	}
	p.send(string(p.buf[:n]))
	p.buf = p.buf[n:]
	if len(p.buf) > 0 {
		p.timer = time.AfterFunc(programsLogWait, p.tick)
	} else {
		p.buf = nil
	}
}

// stop sends at most one last frame and sends nothing after, so no log frame
// follows the run's outcome.
func (p *logPump) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	if !p.stopped && len(p.buf) > 0 {
		p.send(lastBytes(string(p.buf), maxLogFrame))
		p.buf = nil
	}
	p.stopped = true
}

// statusPump turns clay.status into at most one frame each 100 ms, the latest
// text winning, so a program that reports in a loop cannot flood the page's
// stream and evict its own observer.
type statusPump struct {
	mu         sync.Mutex
	pending    string
	hasPending bool
	timer      *time.Timer
	stopped    bool
	send       func(text string)
}

func (p *statusPump) set(text string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return
	}
	p.pending = text
	p.hasPending = true
	if p.timer == nil {
		p.timer = time.AfterFunc(programsLogWait, p.tick)
	}
}

func (p *statusPump) tick() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.timer = nil
	if p.stopped || !p.hasPending {
		return
	}
	text := p.pending
	p.pending = ""
	p.hasPending = false
	p.send(text)
}

func (p *statusPump) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	if !p.stopped && p.hasPending {
		p.send(p.pending)
		p.pending = ""
		p.hasPending = false
	}
	p.stopped = true
}

// cancelProgram cancels the running program on a document when its id
// matches, and reports whether it did.
func (s *Core) cancelProgram(file, id string) bool {
	s.Programs.Mu.Lock()
	defer s.Programs.Mu.Unlock()
	run, ok := s.Programs.Running[file]
	if !ok || run.ID != id {
		return false
	}
	run.Cancel()
	return true
}

// CancelProgramsFor cancels the program running on one document.
func (s *Core) CancelProgramsFor(file string) {
	s.Programs.Mu.Lock()
	defer s.Programs.Mu.Unlock()
	if run, ok := s.Programs.Running[file]; ok {
		run.Cancel()
	}
}

// CancelPrograms cancels every running program.
func (s *Core) CancelPrograms() {
	s.Programs.Mu.Lock()
	defer s.Programs.Mu.Unlock()
	for _, run := range s.Programs.Running {
		run.Cancel()
	}
}

// cancelProgramsUnder cancels every program of a document in dir.
func (s *Core) cancelProgramsUnder(dir string) {
	s.Programs.Mu.Lock()
	defer s.Programs.Mu.Unlock()
	for path, run := range s.Programs.Running {
		if pathrel.SameOrBelow(path, dir) {
			run.Cancel()
		}
	}
}

func (s *Core) programsAsked(path string) bool {
	s.Programs.Mu.Lock()
	defer s.Programs.Mu.Unlock()
	return s.Programs.Asked[path]
}

func (s *Core) programsCapable(path string) bool {
	if s.programsAsked(path) {
		return true
	}
	host := s.programsHost()
	return host != nil && host.HasFolder(path)
}

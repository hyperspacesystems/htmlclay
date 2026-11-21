package aitools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// perAgent is how many runs of one agent go at once; more wait.
	perAgent      = 2
	selfTestLimit = 6 * time.Minute
)

// Phases the page sees in wire/status, after the host's own "asking".
const (
	PhaseStarting   = "starting"
	PhaseWorking    = "working"
	PhaseCollecting = "collecting"
)

// Config is what a Service needs from the app.
type Config struct {
	Store *Store
	// Scratch is where each run's scratch folders go.
	Scratch string
	// Home is the person's home folder, where Codex and agy keep their sign-in.
	Home string
	// LoginEnv is the person's login environment, for finding agents.
	LoginEnv func() []string
	// ClaudeToken reads Claude Code's sign-in; nil means ReadClaudeToken.
	ClaudeToken func(ctx context.Context) (Token, error)
}

// Service runs ai-tools requests. One Service serves every document.
type Service struct {
	cfg  Config
	gate Gate

	life context.Context
	stop context.CancelFunc

	mu       sync.Mutex
	slots    map[string]chan struct{}
	flights  map[string]*flight
	versions map[string]versionSeen
}

type flight struct {
	done     chan struct{}
	finished bool
	waiters  int
	cancel   context.CancelFunc
	leaked   []string
	err      error
}

type versionSeen struct {
	stamp   string
	version string
}

func NewService(cfg Config) *Service {
	// A run cut off by a quit leaves its scratch behind, with a copy of an
	// agent's sign-in in Codex's case.
	if cfg.Scratch != "" {
		_ = os.RemoveAll(cfg.Scratch)
	}
	if cfg.ClaudeToken == nil {
		home := cfg.Home
		cfg.ClaudeToken = func(ctx context.Context) (Token, error) { return ReadClaudeToken(ctx, home) }
	}
	life, stop := context.WithCancel(context.Background())
	return &Service{cfg: cfg, life: life, stop: stop, slots: map[string]chan struct{}{}, flights: map[string]*flight{}, versions: map[string]versionSeen{}}
}

// Close stops every self-test and renewal still running.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stop()
	s.life, s.stop = context.WithCancel(context.Background())
}

// Store is the service's store, for the Programs page.
func (s *Service) Store() *Store { return s.cfg.Store }

// Gate is the service's git gate, which carried programs share so that git
// reads stay one per folder and two in all.
func (s *Service) Gate() *Gate { return &s.gate }

// Job is one request, after the host checked the document and found its folder.
type Job struct {
	Document string
	Folder   string
	Level    string
	Request  Request
	// Preferred is the agent the person chose in HTML Clay, used when the page
	// names none.
	Preferred string
	// Models is the model the person chose for each agent.
	Models map[string]string
	// Status reports progress to the page.
	Status func(phase, text string)
	// Started is called once the agent itself is running.
	Started func()
	Put     Put
}

// Result is a run's answer, as the wire sends it.
type Result struct {
	Data    json.RawMessage
	Sources []Source
	Engine  string
	Model   string
}

func (s *Service) loginEnv() []string {
	if s.cfg.LoginEnv == nil {
		return os.Environ()
	}
	return s.cfg.LoginEnv()
}

func (s *Service) scratch(prefix string) (string, error) {
	if err := os.MkdirAll(s.cfg.Scratch, 0o700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(s.cfg.Scratch, prefix+"-")
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	return dir, nil
}

// Pick finds the agent for a request: the one the page named, else the
// person's choice, else the first installed of Agents.
func (s *Service) Pick(ctx context.Context, engine, preferred string) (Installed, error) {
	env := s.loginEnv()
	names := Agents
	switch {
	case engine != "" && !Supported(engine):
		return Installed{}, fail(CodeEngineUnsupported, fmt.Sprintf("HTML Clay cannot run the agent %q. It runs Claude Code, Codex or Gemini (agy).", engine))
	case engine != "":
		names = []string{engine}
	case Supported(preferred):
		names = append([]string{preferred}, Agents...)
	}
	for _, name := range names {
		program, ok := find(name, env)
		if !ok {
			continue
		}
		v, err := s.version(ctx, program, env)
		if err != nil {
			return Installed{}, fail(CodeEngineFailed, fmt.Sprintf("%s did not start: %v", Label(name), err))
		}
		return Installed{Name: name, Program: program, Version: v}, nil
	}
	if engine != "" {
		if scriptOnly(engine, env) {
			return Installed{}, fail(CodeEngineUnsupported, Label(engine)+" is installed through npm, which HTML Clay cannot run safely on Windows. Install it with its own Windows installer, then try again.")
		}
		return Installed{}, fail(CodeEngineUnsupported, Label(engine)+" is not installed on this computer.")
	}
	return Installed{}, fail(CodeEngineUnsupported, "No AI agent is installed on this computer. Install Claude Code, Codex or Gemini (agy) and sign in to it, then try again.")
}

// version asks once per program file, again when the file changed.
func (s *Service) version(ctx context.Context, program string, env []string) (string, error) {
	info, err := os.Stat(program)
	if err != nil {
		return "", err
	}
	stamp := fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
	s.mu.Lock()
	seen, ok := s.versions[program]
	s.mu.Unlock()
	if ok && seen.stamp == stamp {
		return seen.version, nil
	}
	dir, err := s.scratch("version")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	v, err := version(ctx, program, env, dir)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.versions[program] = versionSeen{stamp: stamp, version: v}
	s.mu.Unlock()
	return v, nil
}

// Run runs one job to its answer.
func (s *Service) Run(ctx context.Context, job Job) (Result, error) {
	status := job.Status
	if status == nil {
		status = func(string, string) {}
	}
	inst, err := s.Pick(ctx, job.Request.Engine, job.Preferred)
	if err != nil {
		return Result{}, err
	}
	switch {
	case job.Level == LevelChange && !CanChange(inst.Name):
		return Result{}, fail(CodeLevelUnavailable, Label(inst.Name)+" can only read a folder for a page. Choose Claude Code in HTML Clay to let a page change files.")
	case job.Level != LevelRead && job.Level != LevelChange:
		return Result{}, fail(CodeLevelUnavailable, "HTML Clay lets an agent read a folder, or change files in it, but nothing more.")
	}
	folder, err := filepath.EvalSymlinks(job.Folder)
	if err != nil {
		return Result{}, folderMissing(job.Folder)
	}
	if info, err := os.Stat(folder); err != nil || !info.IsDir() {
		return Result{}, folderMissing(job.Folder)
	}
	release, err := s.acquire(ctx, inst.Name, status)
	if err != nil {
		return Result{}, err
	}
	defer release()
	if err := s.check(ctx, inst, job.Level, status); err != nil {
		return Result{}, err
	}
	status(PhaseStarting, "Reading the folder's git history")
	diffBytes := PromptDiffBytes
	if inst.Name == AgentAgy {
		diffBytes = agyMaxPrompt / 2
	}
	git, err := GitContext(ctx, &s.gate, folder, diffBytes)
	if err != nil {
		return Result{}, err
	}
	schema := Wrap(job.Request.Schema, job.Request.Sources)
	dir, err := s.scratch("run")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(dir)
	status(PhaseWorking, fmt.Sprintf("%s is reading %s", Label(inst.Name), baseName(folder)))
	model := job.Models[inst.Name]
	r, err := s.call(ctx, inst, call{Folder: folder, Prompt: Prompt(job.Request, job.Level, baseName(folder), git), Schema: schema, Level: job.Level, Model: model, Scratch: dir, Started: job.Started}, status)
	if err != nil {
		return Result{}, err
	}
	if !job.Request.Sources {
		r.Answer = withoutSources(r.Answer)
	}
	if err := Validate(schema, r.Answer); err != nil {
		return Result{}, fail(CodeResultInvalid, fmt.Sprintf("%s's answer did not match what the page asked for (%v). Try again.", Label(inst.Name), err))
	}
	var answer Answer
	if err := json.Unmarshal(r.Answer, &answer); err != nil {
		return Result{}, fail(CodeResultInvalid, Label(inst.Name)+"'s answer was not the JSON the page asked for.")
	}
	status(PhaseCollecting, "Collecting the code the answer is about")
	sources, err := Expand(ctx, &s.gate, folder, answer.Sources, job.Put)
	if err != nil {
		return Result{}, err
	}
	if r.Model != "" {
		model = r.Model
	}
	return Result{Data: answer.Data, Sources: sources, Engine: inst.Name, Model: model}, nil
}

// withoutSources drops a sources list the page did not ask for, which an
// agent sometimes adds on its own.
func withoutSources(answer json.RawMessage) json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(answer, &fields) != nil {
		return answer
	}
	if _, ok := fields["sources"]; !ok {
		return answer
	}
	delete(fields, "sources")
	out, err := json.Marshal(fields)
	if err != nil {
		return answer
	}
	return out
}

// acquire takes one of the agent's run slots, waiting while it is busy.
func (s *Service) acquire(ctx context.Context, agent string, status func(phase, text string)) (func(), error) {
	s.mu.Lock()
	slot := s.slots[agent]
	if slot == nil {
		slot = make(chan struct{}, perAgent)
		s.slots[agent] = slot
	}
	s.mu.Unlock()
	select {
	case slot <- struct{}{}:
	default:
		status(PhaseStarting, Label(agent)+" is busy with other requests. This one starts when one of them finishes.")
		select {
		case slot <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return func() { <-slot }, nil
}

// call runs one agent call. Claude Code gets its token from its own sign-in
// record, and a refused token is renewed by Claude itself once.
func (s *Service) call(ctx context.Context, inst Installed, c call, status func(phase, text string)) (reply, error) {
	env := s.loginEnv()
	switch inst.Name {
	case AgentCodex:
		return runCodex(ctx, inst.Program, env, s.cfg.Home, c)
	case AgentAgy:
		return runAgy(ctx, inst.Program, env, s.cfg.Home, c)
	}
	renewed := false
	for attempt := 0; ; attempt++ {
		token, didRenew, err := s.claudeToken(ctx, inst, attempt > 0, renewed, status)
		if err != nil {
			return reply{}, err
		}
		renewed = renewed || didRenew
		r, err := runClaude(ctx, inst.Program, env, token.Access, c)
		if !errors.Is(err, errSignIn) {
			return r, err
		}
		if attempt > 0 {
			return r, fail(CodeEngineFailed, "Claude Code's sign-in has expired. Open Terminal, run claude and sign in again, then try again.")
		}
	}
}

// claudeToken is Claude Code's sign-in for one call. A token Claude refused, or
// one close to expiring, is renewed first, but only once per request: renewed
// says this request already did.
func (s *Service) claudeToken(ctx context.Context, inst Installed, refused, renewed bool, status func(phase, text string)) (Token, bool, error) {
	token, err := s.cfg.ClaudeToken(ctx)
	if err != nil {
		return Token{}, false, fail(CodeEngineFailed, "Claude Code is not signed in on this computer. Open Terminal, run claude and sign in, then try again.")
	}
	if renewed || !refused && (token.Expires.IsZero() || time.Until(token.Expires) > tokenRenewAhead) {
		return token, false, nil
	}
	status(PhaseStarting, "Waiting for Claude Code to renew its sign-in")
	if _, err := s.once(ctx, "renew\x00"+inst.Program, func(ctx context.Context) ([]string, error) {
		dir, err := s.scratch("renew")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		return nil, renewClaude(ctx, inst.Program, s.loginEnv(), s.cfg.Home, dir)
	}); err != nil && ctx.Err() != nil {
		return Token{}, false, ctx.Err()
	}
	token, err = s.cfg.ClaudeToken(ctx)
	if err != nil {
		return Token{}, false, fail(CodeEngineFailed, "Claude Code is not signed in on this computer. Open Terminal, run claude and sign in, then try again.")
	}
	return token, true, nil
}

// check makes sure the agent's version passed the canary self-test at level,
// running the test when this version never ran it.
func (s *Service) check(ctx context.Context, inst Installed, level string, status func(phase, text string)) error {
	store := s.cfg.Store
	if rec, ok := store.Check(inst.Name, inst.Version, level); ok {
		return checkError(inst, rec)
	}
	if inst.Name == AgentClaude {
		dir, err := s.scratch("help")
		if err != nil {
			return err
		}
		ok, err := claudeHasRestricted(ctx, inst.Program, s.loginEnv(), dir)
		os.RemoveAll(dir)
		if err != nil {
			return fail(CodeEngineFailed, fmt.Sprintf("Claude Code did not start: %v", err))
		}
		if !ok {
			return fail(CodeEngineUnsupported, fmt.Sprintf("This Claude Code (%s) has no --restricted option, so HTML Clay cannot keep it inside the folder. Update Claude Code (run claude update in Terminal), then try again.", inst.Version))
		}
	}
	status(PhaseStarting, fmt.Sprintf("Checking that %s stays inside the folder. This runs once for each version.", Label(inst.Name)))
	leaked, err := s.once(ctx, "check\x00"+inst.Name+"\x00"+inst.Version+"\x00"+level, func(ctx context.Context) ([]string, error) {
		root, err := s.scratch("selftest")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(root)
		return selfTest(ctx, root, level, selfTestProof(inst.Name), claudeTools(level), func(ctx context.Context, c call) (reply, error) {
			c.Model = selfTestModel(inst.Name)
			return s.call(ctx, inst, c, func(string, string) {})
		})
	})
	var untried *untriedError
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.As(err, &untried):
		return fail(CodeEngineFailed, fmt.Sprintf("HTML Clay could not check that %s stays inside the folder: in its safety check it did not try %s, even when asked twice. Nothing was recorded, so the next request checks again.", Label(inst.Name), strings.Join(untried.names, ", ")))
	case errors.Is(err, errInconclusive), errors.Is(err, context.DeadlineExceeded):
		return fail(CodeEngineFailed, fmt.Sprintf("HTML Clay could not check that %s stays inside the folder: it did not read the test file. Make sure %s works in Terminal, then try again.", Label(inst.Name), Label(inst.Name)))
	case err != nil:
		var coded *Error
		if errors.As(err, &coded) {
			return coded
		}
		return fail(CodeEngineFailed, fmt.Sprintf("HTML Clay could not check %s: %v", Label(inst.Name), err))
	}
	rec := CheckRecord{Agent: inst.Name, Version: inst.Version, Level: level, Program: inst.Program, Passed: len(leaked) == 0, Leaked: leaked, At: time.Now().UTC()}
	if err := store.SetCheck(rec); err != nil {
		return fail(CodeEngineFailed, fmt.Sprintf("HTML Clay could not record the check of %s: %v", Label(inst.Name), err))
	}
	return checkError(inst, rec)
}

func checkError(inst Installed, rec CheckRecord) error {
	if rec.Passed {
		return nil
	}
	return fail(CodeEngineUnsupported, fmt.Sprintf("%s %s reached files outside its folder in HTML Clay's safety check (%s), so HTML Clay will not run it. Update %s or choose another agent in HTML Clay.", Label(inst.Name), inst.Version, strings.Join(rec.Leaked, ", "), Label(inst.Name)))
}

func selfTestProof(agent string) proof {
	switch agent {
	case AgentClaude:
		return proofToolCalls
	case AgentAgy:
		return proofRefusal
	}
	return proofNone
}

func selfTestModel(agent string) string {
	switch agent {
	case AgentClaude:
		return ClaudeSelfTestModel
	case AgentAgy:
		return AgySelfTestModel
	}
	return ""
}

// once runs work for key one time however many requests wait on it. It runs
// on its own clock, so one page cancelling does not stop it for the others.
func (s *Service) once(ctx context.Context, key string, work func(context.Context) ([]string, error)) ([]string, error) {
	s.mu.Lock()
	f := s.flights[key]
	if f == nil {
		f = &flight{done: make(chan struct{})}
		s.flights[key] = f
		wctx, cancel := context.WithTimeout(s.life, selfTestLimit)
		f.cancel = cancel
		go func() {
			defer cancel()
			f.leaked, f.err = work(wctx)
			s.mu.Lock()
			f.finished = true
			if s.flights[key] == f {
				delete(s.flights, key)
			}
			s.mu.Unlock()
			close(f.done)
		}()
	}
	f.waiters++
	s.mu.Unlock()
	select {
	case <-f.done:
		return f.leaked, f.err
	case <-ctx.Done():
	}
	s.mu.Lock()
	f.waiters--
	if f.waiters == 0 && !f.finished {
		// Nobody waits for it now, so it stops, and the next request starts
		// a fresh one instead of joining one that is ending.
		f.cancel()
		if s.flights[key] == f {
			delete(s.flights, key)
		}
	}
	s.mu.Unlock()
	return nil, ctx.Err()
}

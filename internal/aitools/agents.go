package aitools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/runner"
)

// Agent names a page may ask for.
const (
	AgentClaude = "claude"
	AgentCodex  = "codex"
	AgentAgy    = "agy"
)

// Agents is the order HTML Clay looks for an agent when the person picked none.
var Agents = []string{AgentClaude, AgentCodex, AgentAgy}

// Label is how the person knows an agent.
func Label(name string) string {
	switch name {
	case AgentClaude:
		return "Claude Code"
	case AgentCodex:
		return "Codex"
	case AgentAgy:
		return "Gemini (agy)"
	}
	return name
}

// Supported reports whether name is an agent ai-tools can run.
func Supported(name string) bool {
	return name == AgentClaude || name == AgentCodex || name == AgentAgy
}

// CanChange reports whether an agent can run at the change level.
func CanChange(name string) bool { return name == AgentClaude }

// Installed is one agent found on this computer.
type Installed struct {
	Name string
	// Program is the absolute path of the agent with its links resolved.
	Program string
	Version string
}

const (
	versionTimeout = 15 * time.Second
	maxOutput      = 8 << 20
)

// call is one run of an agent.
type call struct {
	Folder string
	Prompt string
	Schema json.RawMessage
	Level  string
	Model  string
	// Scratch is a fresh folder for this call, deleted after it.
	Scratch string
	// Started is called once the agent process is running.
	Started func()
}

// reply is what one run produced.
type reply struct {
	Answer json.RawMessage
	Model  string
	// Output is everything the agent printed, bounded, which the self-test
	// searches for canaries.
	Output []byte
	// Tools are the tool calls the agent made, for an agent whose output
	// lists them (Claude Code).
	Tools []toolCall
}

// toolCall is one tool call an agent made: the tool's name and its input.
type toolCall struct {
	Name  string
	Input json.RawMessage
}

var errSignIn = errors.New("aitools: the agent's sign-in was refused")

// proc is one child process.
type proc struct {
	Argv  []string
	Dir   string
	Env   []string
	Stdin string
	// Line sees each line of standard output as it arrives. Returning false
	// kills the run.
	Line func(line []byte) bool
	// Started is called once the process is running.
	Started func()
}

type procResult struct {
	Stdout, Stderr []byte
	Err            error
	// Stopped is true when Line ended the run.
	Stopped bool
}

// run starts p through internal/runner, so the agent gets only the
// environment built for it and its whole process tree ends with it.
func run(ctx context.Context, p proc) procResult {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := &lineWriter{limit: maxOutput}
	var stopped bool
	if p.Line != nil {
		out.line = func(b []byte) {
			if !stopped && !p.Line(b) {
				stopped = true
				cancel()
			}
		}
	}
	errs := &capped{limit: 64 << 10}
	process, err := runner.Start(ctx, runner.Spec{Argv: p.Argv, Dir: p.Dir, Env: p.Env, Stdin: strings.NewReader(p.Stdin), Stdout: out, Stderr: errs})
	if err != nil {
		return procResult{Err: err}
	}
	if p.Started != nil {
		p.Started()
	}
	err = process.Wait()
	out.flush()
	return procResult{Stdout: out.all.Bytes(), Stderr: errs.Bytes(), Err: err, Stopped: stopped}
}

// lineWriter keeps what a child prints, up to limit, and hands each complete
// line to line.
type lineWriter struct {
	mu      sync.Mutex
	limit   int
	all     bytes.Buffer
	pending []byte
	line    func([]byte)
}

func (w *lineWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if room := w.limit - w.all.Len(); room > 0 {
		w.all.Write(b[:min(len(b), room)])
	}
	if w.line == nil {
		return len(b), nil
	}
	w.pending = append(w.pending, b...)
	for {
		i := bytes.IndexByte(w.pending, '\n')
		if i < 0 {
			break
		}
		w.line(w.pending[:i])
		w.pending = w.pending[i+1:]
	}
	if len(w.pending) > maxOutput {
		w.pending = nil
	}
	return len(b), nil
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.line != nil && len(w.pending) > 0 {
		w.line(w.pending)
		w.pending = nil
	}
}

// capped keeps the first limit bytes written to it.
type capped struct {
	mu    sync.Mutex
	limit int
	buf   bytes.Buffer
}

func (c *capped) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if room := c.limit - c.buf.Len(); room > 0 {
		c.buf.Write(b[:min(len(b), room)])
	}
	return len(b), nil
}

func (c *capped) Bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.buf.Bytes()...)
}

// find looks for an agent on the person's login PATH and resolves its links,
// so the run and the self-test start the same program.
func find(name string, loginEnv []string) (string, bool) {
	exts := []string{""}
	if runtime.GOOS == "windows" {
		exts = []string{".exe"}
	}
	for _, dir := range filepath.SplitList(envValue(loginEnv, "PATH")) {
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		for _, ext := range exts {
			candidate := filepath.Join(dir, name+ext)
			real, err := filepath.EvalSymlinks(candidate)
			if err != nil {
				continue
			}
			if info, err := os.Stat(real); err == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode().Perm()&0o111 != 0) {
				return real, true
			}
		}
	}
	return "", false
}

// scriptOnly reports an agent installed on Windows only as a .cmd or .bat
// script, such as an npm install, which HTML Clay does not start: cmd.exe would
// read the page's text in its arguments as commands.
func scriptOnly(name string, loginEnv []string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	for _, dir := range filepath.SplitList(envValue(loginEnv, "PATH")) {
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		for _, ext := range []string{".cmd", ".bat"} {
			if info, err := os.Stat(filepath.Join(dir, name+ext)); err == nil && info.Mode().IsRegular() {
				return true
			}
		}
	}
	return false
}

// agentPath is the PATH an agent runs with: its own folder, the folder of the
// node a script install needs, and the system folders.
func agentPath(program string, loginEnv []string) string {
	dirs := []string{filepath.Dir(program)}
	if node, ok := find("node", loginEnv); ok {
		dirs = append(dirs, filepath.Dir(node))
	}
	if runtime.GOOS == "windows" {
		if root := os.Getenv("SystemRoot"); root != "" {
			dirs = append(dirs, filepath.Join(root, "System32"), root)
		}
	} else {
		dirs = append(dirs, "/usr/bin", "/bin")
	}
	return strings.Join(dirs, string(filepath.ListSeparator))
}

// version asks the agent for its version, in a scratch folder with a clean
// environment.
func version(ctx context.Context, program string, loginEnv []string, scratch string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	res := run(ctx, proc{Argv: []string{program, "--version"}, Dir: scratch, Env: runner.CleanEnv(scratch, agentPath(program, loginEnv))})
	if res.Err != nil {
		return "", fmt.Errorf("%s --version: %w", filepath.Base(program), res.Err)
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(res.Stdout)), "\n")
	line = strings.TrimSpace(line)
	if line == "" || len(line) > 200 {
		return "", fmt.Errorf("%s --version printed no version", filepath.Base(program))
	}
	return line, nil
}

func envValue(env []string, key string) string {
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if ok && (k == key || runtime.GOOS == "windows" && strings.EqualFold(k, key)) {
			return v
		}
	}
	return ""
}

// jsonObject finds the JSON object an agent printed as text: the whole text,
// a fenced block, or the outermost braces.
func jsonObject(text string) (json.RawMessage, bool) {
	text = strings.TrimSpace(text)
	candidates := []string{text}
	if i := strings.Index(text, "```"); i >= 0 {
		rest := text[i+3:]
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[nl+1:]
		}
		if j := strings.Index(rest, "```"); j >= 0 {
			candidates = append(candidates, strings.TrimSpace(rest[:j]))
		}
	}
	if i, j := strings.IndexByte(text, '{'), strings.LastIndexByte(text, '}'); i >= 0 && j > i {
		candidates = append(candidates, text[i:j+1])
	}
	for _, c := range candidates {
		var v map[string]any
		if json.Unmarshal([]byte(c), &v) == nil {
			return json.RawMessage(c), true
		}
	}
	return nil, false
}

func tail(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		s = "…" + s[len(s)-n:]
	}
	return s
}

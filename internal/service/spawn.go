package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Spec says how to run the service.
type Spec struct {
	// Command is the program and its arguments, without --data-dir.
	Command []string
	// DataDir is passed as --data-dir; the only place the service keeps files.
	DataDir string
	// Env is added to the process's environment (os.Environ() plus these).
	Env []string
	// Log receives the service's stderr, line by line. Nil discards it.
	Log func(line string)
}

// Ready is the service's READY line.
type Ready struct {
	URL   string `json:"url"`
	Epoch string `json:"epoch"`
	PID   int    `json:"pid"`
}

// Root is one served folder as the service knows it.
type Root struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	URL  string `json:"url"` // the folder's public origin, http://<name>.htmlclay.localhost:<port>
}

// Host is Go's control API as the service reaches it: the host line's body.
type Host struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
	Epoch  string `json:"epoch"`
	Roots  []Root `json:"roots"`
}

const (
	// stderrTail is how many stderr lines an error carries.
	stderrTail = 20
	// stdoutQueue holds lines read before the caller is listening.
	stdoutQueue = 128
	// scannerMax is the longest stdout or stderr line that is still one line.
	scannerMax = 1024 * 1024
)

// Process is one running service.
type Process struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	writeMu sync.Mutex
	// exchangeMu holds one stdin line and the stdout answer it waits for, so a
	// sync key line sent from an action never reads the runner's ATTACHED.
	exchangeMu sync.Mutex
	// lines carries stdout lines to Start and Attach; a reader never blocks on
	// it, so a full queue makes lines go to Log instead.
	lines   chan string
	outDone chan struct{}
	errDone chan struct{}
	done    chan struct{}
	cleanup func()
	log     func(line string)
	logMu   sync.Mutex

	mu   sync.Mutex
	code int
	tail []string
}

// Start runs the service and waits for READY. secret is 64 lowercase hex.
// On any failure the process is killed and the error includes the last
// 20 stderr lines; the process is still returned so its exit can be awaited.
func Start(ctx context.Context, spec Spec, secret string, readyTimeout time.Duration) (*Process, Ready, error) {
	if len(spec.Command) == 0 {
		return nil, Ready{}, errors.New("spawn: no service command")
	}
	args := append(append([]string{}, spec.Command[1:]...), "--data-dir", spec.DataDir)
	cmd := exec.Command(spec.Command[0], args...)
	cmd.Env = append(os.Environ(), spec.Env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, Ready{}, fmt.Errorf("spawn: stdin pipe: %w", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		stdin.Close()
		return nil, Ready{}, fmt.Errorf("spawn: stdout pipe: %w", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		stdin.Close()
		outR.Close()
		outW.Close()
		return nil, Ready{}, fmt.Errorf("spawn: stderr pipe: %w", err)
	}
	cmd.Stdout = outW
	cmd.Stderr = errW
	if err := cmd.Start(); err != nil {
		stdin.Close()
		outR.Close()
		outW.Close()
		errR.Close()
		errW.Close()
		return nil, Ready{}, fmt.Errorf("spawn: %w", err)
	}
	// The parent's write ends close here, so the readers see end of file as soon
	// as the service is gone. cmd.Wait does not own these pipes.
	outW.Close()
	errW.Close()
	cleanup, err := afterStart(cmd)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		stdin.Close()
		outR.Close()
		errR.Close()
		return nil, Ready{}, fmt.Errorf("spawn: %w", err)
	}
	p := &Process{
		cmd:     cmd,
		stdin:   stdin,
		lines:   make(chan string, stdoutQueue),
		outDone: make(chan struct{}),
		errDone: make(chan struct{}),
		done:    make(chan struct{}),
		cleanup: cleanup,
		log:     spec.Log,
		code:    -1,
	}
	go p.readStdout(outR)
	go p.readStderr(errR)
	go p.wait()
	if err := p.writeLine(struct {
		V      int    `json:"v"`
		Secret string `json:"secret"`
	}{V: 1, Secret: secret}); err != nil {
		p.Kill()
		return p, Ready{}, fmt.Errorf("spawn: writing the bootstrap line: %w; %s", err, p.stderrText())
	}
	timer := time.NewTimer(readyTimeout)
	defer timer.Stop()
	for {
		select {
		case line := <-p.lines:
			rest, ok := strings.CutPrefix(line, "READY ")
			if !ok {
				p.logLine("stdout: " + line)
				continue
			}
			var ready Ready
			if err := json.Unmarshal([]byte(strings.TrimSpace(rest)), &ready); err != nil {
				p.Kill()
				return p, Ready{}, fmt.Errorf("spawn: malformed READY line %q: %w; %s", line, err, p.stderrText())
			}
			return p, ready, nil
		case <-p.done:
			return p, Ready{}, p.diedError("READY")
		case <-p.outDone:
			return p, Ready{}, p.diedError("READY")
		case <-timer.C:
			p.Kill()
			return p, Ready{}, fmt.Errorf("spawn: timeout: the service printed no READY line within %v; %s", readyTimeout, p.stderrText())
		case <-ctx.Done():
			p.Kill()
			return p, Ready{}, fmt.Errorf("spawn: %w; %s", ctx.Err(), p.stderrText())
		}
	}
}

// Attach writes {"host": host} and waits for the ATTACHED line.
func (p *Process) Attach(host Host, timeout time.Duration) error {
	if host.Roots == nil {
		// The service iterates roots, so it needs a list, not null.
		host.Roots = []Root{}
	}
	_, err := p.exchange(struct {
		Host Host `json:"host"`
	}{Host: host}, "attach", "the host line", timeout, "ATTACHED")
	return err
}

// ErrSyncKeyRefused is the service's SYNC KEY REFUSED answer: the line did not
// carry a key and server it accepts.
var ErrSyncKeyRefused = errors.New("the service refused the sync key line")

// syncLine is C2's stdin line. It is marshalled only inside writeLine and never
// logged; no error built here carries any part of it.
type syncLine struct {
	Sync struct {
		V         int     `json:"v"`
		ServerURL string  `json:"serverUrl,omitempty"`
		Key       *string `json:"key"`
	} `json:"sync"`
}

// SendSyncKey hands the service the sync key and waits for SYNC KEY HELD.
func (p *Process) SendSyncKey(serverURL, key string, timeout time.Duration) error {
	var line syncLine
	line.Sync.V = 1
	line.Sync.ServerURL = serverURL
	line.Sync.Key = &key
	answer, err := p.exchange(line, "sync key", "the sync key line", timeout, "SYNC KEY HELD", "SYNC KEY REFUSED")
	if err != nil {
		return err
	}
	if answer == "SYNC KEY REFUSED" {
		return ErrSyncKeyRefused
	}
	return nil
}

// ClearSyncKey tells the service to drop the key and waits for SYNC KEY CLEARED,
// which it prints after every session has stopped.
func (p *Process) ClearSyncKey(timeout time.Duration) error {
	var line syncLine
	line.Sync.V = 1
	_, err := p.exchange(line, "sync key", "the sync key line", timeout, "SYNC KEY CLEARED")
	return err
}

// exchange writes one line and waits for one of the answers. Other stdout lines
// go to the log.
func (p *Process) exchange(v any, what, line string, timeout time.Duration, answers ...string) (string, error) {
	p.exchangeMu.Lock()
	defer p.exchangeMu.Unlock()
	if err := p.writeLine(v); err != nil {
		return "", fmt.Errorf("%s: writing %s: %w; %s", what, line, err, p.stderrText())
	}
	want := strings.Join(answers, " or ")
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case out := <-p.lines:
			trimmed := strings.TrimSpace(out)
			for _, answer := range answers {
				if trimmed == answer {
					return answer, nil
				}
			}
			p.logLine("stdout: " + out)
		case <-p.done:
			return "", p.diedError(want)
		case <-p.outDone:
			return "", p.diedError(want)
		case <-timer.C:
			return "", fmt.Errorf("%s: timeout: the service printed no %s line within %v; %s", what, want, timeout, p.stderrText())
		}
	}
}

// Close ends stdin, which makes the service flush nothing more and exit; if it
// has not exited after grace, it is killed. It returns once the process is gone.
func (p *Process) Close(grace time.Duration) error {
	p.writeMu.Lock()
	err := p.stdin.Close()
	p.writeMu.Unlock()
	if err != nil && !errors.Is(err, os.ErrClosed) && !errors.Is(err, io.ErrClosedPipe) {
		return err
	}
	select {
	case <-p.done:
	case <-time.After(grace):
		p.Kill()
	}
	return nil
}

// Kill ends the process now and returns once it is gone.
func (p *Process) Kill() {
	if p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
	<-p.done
}

// Done is closed when the process has exited, for any reason.
func (p *Process) Done() <-chan struct{} {
	return p.done
}

// ExitCode is the exit code after Done (-1 when killed by a signal).
func (p *Process) ExitCode() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.code
}

// CommandFromEnv answers HTMLCLAY_SERVICE_CMD split on spaces, or nil when it
// is unset. Development and tests run the service from source with it, e.g.
// "node /abs/path/service/main.cjs".
func CommandFromEnv() []string {
	cmd := strings.Fields(os.Getenv("HTMLCLAY_SERVICE_CMD"))
	if len(cmd) == 0 {
		return nil
	}
	return cmd
}

// wait reaps the process: exactly one goroutine calls cmd.Wait.
func (p *Process) wait() {
	p.cmd.Wait()
	code := -1
	if state := p.cmd.ProcessState; state != nil {
		code = state.ExitCode()
	}
	p.mu.Lock()
	p.code = code
	p.mu.Unlock()
	if p.cleanup != nil {
		p.cleanup()
	}
	close(p.done)
}

func (p *Process) readStdout(r io.ReadCloser) {
	defer close(p.outDone)
	defer r.Close()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), scannerMax)
	for sc.Scan() {
		line := sc.Text()
		select {
		case p.lines <- line:
		default:
			p.logLine("stdout: " + line)
		}
	}
}

func (p *Process) readStderr(r io.ReadCloser) {
	defer close(p.errDone)
	defer r.Close()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), scannerMax)
	for sc.Scan() {
		line := sc.Text()
		p.mu.Lock()
		p.tail = append(p.tail, line)
		if len(p.tail) > stderrTail {
			p.tail = p.tail[len(p.tail)-stderrTail:]
		}
		p.mu.Unlock()
		p.logLine(line)
	}
}

func (p *Process) writeLine(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	_, err = p.stdin.Write(append(body, '\n'))
	return err
}

func (p *Process) logLine(line string) {
	if p.log == nil {
		return
	}
	p.logMu.Lock()
	defer p.logMu.Unlock()
	p.log(line)
}

// diedError describes an exit that came before the line Start or Attach waits
// for.
func (p *Process) diedError(want string) error {
	waitFor(p.done, 5*time.Second)
	waitFor(p.errDone, 2*time.Second)
	return fmt.Errorf("spawn: the service exited before %s (exit %d); %s", want, p.ExitCode(), p.stderrText())
}

func (p *Process) stderrText() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.tail) == 0 {
		return "no stderr output"
	}
	return "stderr: " + strings.Join(p.tail, " | ")
}

func waitFor(c <-chan struct{}, d time.Duration) {
	select {
	case <-c:
	case <-time.After(d):
	}
}

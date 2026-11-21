package carried

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panphora/htmlclay/internal/aitools"
	"github.com/panphora/htmlclay/internal/runner"
)

const (
	// DefaultMemory is a worker's memory cap.
	DefaultMemory = 256 << 20
	// DefaultTimeout is how long a program may run.
	DefaultTimeout = 5 * time.Minute

	memoryEvery = 20 * time.Millisecond
	maxLogKept  = 64 << 10
	maxLogSent  = 1 << 20
)

// Job is one run of a carried program. Modules are the stored copies of
// every program tag of the document, by name; Entry names the one whose main
// runs. Calls come from Policy and are checked on every host call.
type Job struct {
	// Worker is the absolute path of the HTML Clay binary.
	Worker  string
	Entry   string
	Modules map[string]string
	Input   json.RawMessage
	Level   Level
	Calls   Calls
	// Folder is the chosen folder for file and git calls; empty at the
	// internet level.
	Folder string
	// Gate is shared with ai-tools, so git reads stay one per folder and two
	// in all.
	Gate    *aitools.Gate
	Memory  uint64
	Timeout time.Duration
	// Status hears clay.status, Log each piece of console output.
	Status func(text string)
	Log    func(text string)
	// Put stores clay.output's text as a run file and returns its address.
	Put func(name string, body []byte) (url string, err error)

	dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

// Output is one clay.output of a run, kept as a run file.
type Output struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Bytes int    `json:"bytes"`
}

// Result is a finished run: what main returned, as JSON, the end of its
// console log, and the outputs it kept with clay.output.
type Result struct {
	Data    json.RawMessage
	Log     string
	LogCut  bool
	Outputs []Output
}

// Run starts a worker, runs the program, answers its host calls and returns
// its outcome. A cancelled ctx kills the worker and answers ctx's error; every
// other failure is an *Error.
func Run(ctx context.Context, job Job) (Result, error) {
	if !memoryWatched {
		return Result{}, fail(CodeUnavailable, "HTML Clay cannot hold a program's memory on this system, so it runs no program here.")
	}
	if !filepath.IsAbs(job.Worker) {
		return Result{}, fail(CodeUnavailable, "HTML Clay could not find itself to start the program.")
	}
	memory := job.Memory
	if memory == 0 {
		memory = DefaultMemory
	}
	timeout := job.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	scratch, err := os.MkdirTemp("", "htmlclay-program-")
	if err != nil {
		return Result{}, fail(CodeWorkerFailed, "HTML Clay could not make a folder to start the program in.")
	}
	defer os.RemoveAll(scratch)
	toWorkerR, toWorkerW, err := os.Pipe()
	if err != nil {
		return Result{}, fail(CodeWorkerFailed, "HTML Clay could not start the program.")
	}
	fromWorkerR, fromWorkerW, err := os.Pipe()
	if err != nil {
		toWorkerR.Close()
		toWorkerW.Close()
		return Result{}, fail(CodeWorkerFailed, "HTML Clay could not start the program.")
	}
	stderr := &tail{max: 16 << 10}
	proc, err := runner.Start(ctx, runner.Spec{
		Argv:        []string{job.Worker, WorkerArg},
		Dir:         scratch,
		Env:         runner.CleanEnv(scratch, ""),
		Stdin:       toWorkerR,
		Stdout:      fromWorkerW,
		Stderr:      stderr,
		// Match Linux's hardLimit(memory*2): the OS hard limit stops sudden
		// bursts past 2x while leaving room for watchMemory to observe usage
		// crossing memory and kill cleanly instead of letting Go's GC thrash
		// at a 1x commit wall until the run timeout.
		MemoryLimit: memory * 2,
	})
	toWorkerR.Close()
	fromWorkerW.Close()
	if err != nil {
		toWorkerW.Close()
		fromWorkerR.Close()
		return Result{}, fail(CodeWorkerFailed, "HTML Clay could not start the program.")
	}
	exited := make(chan struct{})
	go func() {
		proc.Wait()
		close(exited)
	}()
	var overMemory atomic.Bool
	go watchMemory(proc, memory, &overMemory, exited)
	defer func() {
		proc.Kill()
		toWorkerW.Close()
		<-exited
		fromWorkerR.Close()
	}()

	h := newHostCalls(&job)
	defer h.close()
	out := &lineWriter{w: toWorkerW}
	_ = out.send(startMessage{Entry: job.Entry, Modules: job.Modules, Input: job.Input, Memory: memory})
	r := bufio.NewReaderSize(fromWorkerR, 64<<10)
	for {
		line, err := readLine(r, maxWorkerLine)
		if err != nil {
			if errors.Is(err, errLineTooLong) {
				proc.Kill()
				return h.result(), fail(CodeProgramInvalid, "The program sent HTML Clay a message larger than 8 MB, so HTML Clay stopped it.")
			}
			return h.result(), ended(ctx, proc, exited, &overMemory, stderr, memory, timeout)
		}
		var msg workerMessage
		if json.Unmarshal(line, &msg) != nil {
			proc.Kill()
			return h.result(), fail(CodeWorkerFailed, "The program's worker sent HTML Clay something it could not read, so HTML Clay stopped it.")
		}
		switch msg.T {
		case "call":
			value, cerr := h.handle(ctx, msg.Name, msg.Args)
			_ = out.send(replyMessage{T: "reply", ID: msg.ID, Value: value, Error: cerr})
		case "log":
			h.log(msg.Text)
		case "status":
			if job.Status != nil {
				job.Status(clip(msg.Text, maxStatusText))
			}
		case "done":
			res := h.result()
			if len(msg.Value) > maxResultBytes || !json.Valid(msg.Value) {
				return res, fail(CodeResultTooLarge, "The program's result is larger than 768 KB. Write large output with clay.output instead.")
			}
			res.Data = msg.Value
			return res, nil
		case "error":
			code := msg.Code
			if !callCodes[code] && code != CodeProgramInvalid && code != CodeResultTooLarge {
				code = CodeProgramFailed
			}
			return h.result(), &Error{Code: code, Message: clip(msg.Message, maxErrorText)}
		}
	}
}

// ended explains a worker that stopped without an outcome.
func ended(ctx context.Context, proc runner.Process, exited <-chan struct{}, overMemory *atomic.Bool, stderr *tail, memory uint64, timeout time.Duration) error {
	proc.Kill()
	select {
	case <-exited:
	case <-time.After(2 * time.Second):
	}
	switch {
	case overMemory.Load() || stderr.isOOM():
		return fail(CodeProgramMemory, "The program used more than %d MB of memory, so HTML Clay stopped it.", memory>>20)
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fail(CodeProgramTimeout, "The program did not finish within %s, so HTML Clay stopped it.", timeout)
	case ctx.Err() != nil:
		return ctx.Err()
	}
	return fail(CodeProgramCrashed, "The program's worker stopped before the program finished. HTML Clay keeps running.")
}

// watchMemory samples the worker's memory from outside and kills its group
// past the cap. The worker cannot see or stop this watch.
func watchMemory(proc runner.Process, limit uint64, over *atomic.Bool, exited <-chan struct{}) {
	t := time.NewTicker(memoryEvery)
	defer t.Stop()
	pid := proc.Pid()
	for {
		select {
		case <-exited:
			return
		case <-t.C:
			if n, ok := memoryOf(pid); ok && n > limit {
				over.Store(true)
				proc.Kill()
				return
			}
		}
	}
}

// tail keeps the last max bytes written to it and remembers whether stderr
// ever reported a runtime out-of-memory error (which Go prints at the top of
// a stack dump that can exceed max).
type tail struct {
	mu   sync.Mutex
	max  int
	buf  []byte
	prev []byte
	oom  bool
}

var oomMarkers = [][]byte{
	[]byte("out of memory"),
	[]byte("cannot allocate memory"),
	[]byte("cannot map pages in arena"),
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.oom {
		window := bytes.ToLower(append(append([]byte(nil), t.prev...), p...))
		for _, m := range oomMarkers {
			if bytes.Contains(window, m) {
				t.oom = true
				break
			}
		}
		const overlap = 48
		if len(window) > overlap {
			t.prev = append([]byte(nil), window[len(window)-overlap:]...)
		} else {
			t.prev = window
		}
	}
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-t.max:]...)
	}
	return len(p), nil
}

func (t *tail) isOOM() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.oom
}

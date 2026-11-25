// Package runner starts the child processes HTML Clay runs for a page, such as
// an agent. A child gets only the environment its caller builds (CleanEnv),
// never HTML Clay's own, and it runs in its own process group (a job object on
// Windows), so Kill, a cancelled context and the child's own exit all end
// every program it started. Nothing here confines what a child can read: the
// caller picks a program whose own rules do that.
package runner

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

type Spec struct {
	// Argv[0] is an absolute path.
	Argv []string
	// Dir is the absolute working folder.
	Dir string
	// Env is the whole environment. A nil Env is an empty one: nothing is
	// inherited from HTML Clay.
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// MemoryLimit, when not zero, is the most memory in bytes the program may
	// commit. Windows enforces it in the job object; elsewhere the caller
	// watches the process itself.
	MemoryLimit uint64
}

type Process interface {
	Wait() error
	// Kill ends the whole process tree.
	Kill() error
	// Pid is the first program's process id.
	Pid() int
}

var (
	ErrRelativeProgram = errors.New("runner: the program must be an absolute path")
	ErrRelativeDir     = errors.New("runner: the working folder must be an absolute path")
)

// Start runs s.Argv in its own group. Cancelling ctx kills the group, and so
// does the first program's exit: a child it left running in the background
// does not outlive it.
func Start(ctx context.Context, s Spec) (Process, error) {
	if len(s.Argv) == 0 || !filepath.IsAbs(s.Argv[0]) {
		return nil, ErrRelativeProgram
	}
	if !filepath.IsAbs(s.Dir) {
		return nil, ErrRelativeDir
	}
	cmd := exec.Command(s.Argv[0], s.Argv[1:]...)
	cmd.Dir = s.Dir
	cmd.Env = append([]string{}, s.Env...)
	in, stdin, err := input(s.Stdin)
	if err != nil {
		return nil, err
	}
	cmd.Stdin = stdin
	stdout, out, err := output(s.Stdout)
	if err != nil {
		if in != nil {
			in.close()
		}
		return nil, err
	}
	var stderr *os.File
	var errs *pipe
	if s.Stdout != nil && s.Stderr != nil && interfaceEqual(s.Stdout, s.Stderr) {
		stderr = stdout
	} else {
		stderr, errs, err = output(s.Stderr)
		if err != nil {
			closePipes([]*pipe{out})
			if in != nil {
				in.close()
			}
			return nil, err
		}
	}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	pipes := []*pipe{}
	for _, p := range []*pipe{out, errs} {
		if p != nil {
			pipes = append(pipes, p)
		}
	}
	return startGroup(ctx, cmd, pipes, in, s.MemoryLimit)
}

// interfaceEqual compares two writers as os/exec does: only when both dynamic
// types are comparable.
func interfaceEqual(a, b any) bool {
	defer func() { recover() }()
	return a == b
}

// input is a pipe runner makes for a caller's io.Reader. exec.Cmd copies a
// non-file Stdin in its own goroutine and its Wait waits for that copy, so a
// background program that holds stdin, or a reader that never ends, keeps Wait
// from ever returning. Runner's own pipe lets the leader's exit end it at once.
type stdinPipe struct {
	r, w *os.File
	in   io.Reader
}

func input(in io.Reader) (*stdinPipe, *os.File, error) {
	if in == nil {
		return nil, nil, nil
	}
	if f, ok := in.(*os.File); ok {
		return nil, f, nil
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	return &stdinPipe{r: r, w: w, in: in}, r, nil
}

// start closes the parent's copy of the read end and copies the caller's
// reader into the write end, closing it when the copy ends.
func (s *stdinPipe) start() {
	s.r.Close()
	go func() {
		io.Copy(s.w, s.in)
		s.w.Close()
	}()
}

// closeWrite ends the copy at once: a copy blocked on a full pipe returns with
// an error, which is ignored.
func (s *stdinPipe) closeWrite() {
	s.w.Close()
}

func (s *stdinPipe) close() {
	s.r.Close()
	s.w.Close()
}

// pipe is a pipe runner makes for a caller's io.Writer. exec.Cmd makes its
// own for a writer that is not an *os.File, and its Wait then waits for it
// until WaitDelay, so a background program that inherited the write end turns
// a clean exit into exec.ErrWaitDelay. Runner's own pipe lets the leader's
// exit end the group at once.
type pipe struct {
	r, w *os.File
	out  io.Writer
	n    atomic.Int64
}

func output(w io.Writer) (*os.File, *pipe, error) {
	if w == nil {
		return nil, nil, nil
	}
	if f, ok := w.(*os.File); ok {
		return f, nil, nil
	}
	r, wf, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	return wf, &pipe{r: r, w: wf, out: w}, nil
}

// startCopies closes the parent's copy of each write end and copies the read
// end into the caller's writer. The channel closes when every copy is done.
func startCopies(pipes []*pipe) <-chan struct{} {
	done := make(chan struct{})
	var copying sync.WaitGroup
	for _, p := range pipes {
		p.w.Close()
		copying.Add(1)
		go func(p *pipe) {
			defer copying.Done()
			io.Copy(countWriter{w: p.out, n: &p.n}, p.r)
		}(p)
	}
	go func() {
		copying.Wait()
		close(done)
	}()
	return done
}

type countWriter struct {
	w io.Writer
	n *atomic.Int64
}

func (c countWriter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	c.n.Add(int64(n))
	return n, err
}

// finishCopies waits for every copy to reach end of file. If no new bytes
// arrive for 2 seconds while a copy is still open, only a process outside the
// group can hold the pipe, so it closes the read ends and still waits for
// every copy to return before Wait does.
func finishCopies(done <-chan struct{}, pipes []*pipe) {
	last := totalBytes(pipes)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			now := totalBytes(pipes)
			if now == last {
				for _, p := range pipes {
					p.r.Close()
				}
				<-done
				return
			}
			last = now
		}
	}
}

func totalBytes(pipes []*pipe) int64 {
	var total int64
	for _, p := range pipes {
		total += p.n.Load()
	}
	return total
}

func closePipes(pipes []*pipe) {
	for _, p := range pipes {
		if p != nil {
			p.w.Close()
			p.r.Close()
		}
	}
}

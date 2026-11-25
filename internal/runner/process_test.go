//go:build !windows

package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func alive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func readPid(t *testing.T, path string) int {
	t.Helper()
	var pid int
	eventually(t, "the background pid", func() bool {
		b, err := os.ReadFile(path)
		if err != nil || !bytes.HasSuffix(b, []byte("\n")) {
			return false
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(b)))
		return err == nil
	})
	return pid
}

func TestStartGivesTheChildOnlyItsOwnEnvironment(t *testing.T) {
	for _, c := range []struct {
		env  []string
		want string
	}{{nil, ""}, {[]string{"ONLY=1"}, "ONLY=1\n"}} {
		var out bytes.Buffer
		p, err := Start(context.Background(), Spec{Argv: []string{"/usr/bin/env"}, Dir: t.TempDir(), Env: c.env, Stdout: &out})
		if err != nil {
			t.Fatal(err)
		}
		p.Wait()
		if out.String() != c.want {
			t.Errorf("Env %q: the child saw %q", c.env, out.String())
		}
	}
}

func TestStartRefusesRelativePaths(t *testing.T) {
	if _, err := Start(context.Background(), Spec{Argv: []string{"env"}, Dir: t.TempDir()}); !errors.Is(err, ErrRelativeProgram) {
		t.Errorf("relative program: %v", err)
	}
	if _, err := Start(context.Background(), Spec{Argv: []string{"/usr/bin/env"}, Dir: "."}); !errors.Is(err, ErrRelativeDir) {
		t.Errorf("relative folder: %v", err)
	}
}

// A helper the child left in the background that kept the output pipe open
// must not turn a clean exit into an error, and must not outlive the run:
// only the exit status the child itself reported reaches the caller.
func TestCleanExitWithBackgroundHelper(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	var out bytes.Buffer
	start := time.Now()
	p, err := Start(context.Background(), Spec{Argv: []string{"/bin/sh", "-c", `echo ok; sleep 30 & echo $! > "$0"`, pidFile}, Dir: dir, Stdout: &out})
	if err != nil {
		t.Fatal(err)
	}
	pid := readPid(t, pidFile)
	waitErr := p.Wait()
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Wait took %v with a background helper holding the output", elapsed)
	}
	if waitErr != nil {
		t.Errorf("a clean exit returned %v, want nil", waitErr)
	}
	if out.String() != "ok\n" {
		t.Errorf("Stdout = %q, want %q", out.String(), "ok\n")
	}
	eventually(t, "the background helper to die after the child exited", func() bool { return !alive(pid) })
}

// A program the child started in the background dies with the child, whether
// the child exits on its own or its context is cancelled.
func TestNoChildOutlivesTheRun(t *testing.T) {
	t.Run("exit", func(t *testing.T) {
		dir := t.TempDir()
		pidFile := filepath.Join(dir, "pid")
		p, err := Start(context.Background(), Spec{Argv: []string{"/bin/sh", "-c", `sleep 30 & echo $! > "$0"`, pidFile}, Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		pid := readPid(t, pidFile)
		p.Wait()
		eventually(t, "the background sleep to die after the child exited", func() bool { return !alive(pid) })
	})
	t.Run("cancel", func(t *testing.T) {
		dir := t.TempDir()
		pidFile := filepath.Join(dir, "pid")
		ctx, cancel := context.WithCancel(context.Background())
		p, err := Start(ctx, Spec{Argv: []string{"/bin/sh", "-c", `sleep 30 & echo $! > "$0"; sleep 30`, pidFile}, Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		pid := readPid(t, pidFile)
		if !alive(pid) {
			t.Fatal("the background sleep is not running (the test cannot fail otherwise)")
		}
		cancel()
		done := make(chan struct{})
		go func() { p.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Wait did not return after the context was cancelled")
		}
		eventually(t, "the background sleep to die after the cancel", func() bool { return !alive(pid) })
	})
}

// A stdin the leader never reads, or a reader that never ends, must not keep
// Wait from returning: the copy runs on runner's own pipe.
func TestStdinDoesNotHoldWait(t *testing.T) {
	t.Run("reader never ends", func(t *testing.T) {
		pr, pw := io.Pipe()
		defer pw.Close()
		defer pr.Close()
		p, err := Start(context.Background(), Spec{Argv: []string{"/bin/sh", "-c", "exit 3"}, Dir: t.TempDir(), Stdin: pr})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- p.Wait() }()
		select {
		case err := <-done:
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 3 {
				t.Errorf("Wait = %v, want exit code 3", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Wait did not return while the stdin reader was still open")
		}
	})
	t.Run("leader does not read", func(t *testing.T) {
		dir := t.TempDir()
		pidFile := filepath.Join(dir, "pid")
		p, err := Start(context.Background(), Spec{
			Argv:  []string{"/bin/sh", "-c", `sleep 30 & echo $! > "$0"; exit 0`, pidFile},
			Dir:   dir,
			Stdin: strings.NewReader(strings.Repeat("x", 1<<20)),
		})
		if err != nil {
			t.Fatal(err)
		}
		pid := readPid(t, pidFile)
		start := time.Now()
		if err := p.Wait(); err != nil {
			t.Errorf("Wait = %v, want nil", err)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("Wait took %v with a full stdin pipe", elapsed)
		}
		eventually(t, "the background sleep to die", func() bool { return !alive(pid) })
	})
}

// The same writer for stdout and stderr gets one pipe and one copy goroutine,
// so the two streams never race and every line arrives whole.
func TestSharedStdoutAndStderrUseOnePipe(t *testing.T) {
	var out bytes.Buffer
	p, err := Start(context.Background(), Spec{
		Argv:   []string{"/bin/sh", "-c", `i=0; while [ $i -lt 1000 ]; do echo out$i; echo err$i >&2; i=$((i+1)); done`},
		Dir:    t.TempDir(),
		Stdout: &out,
		Stderr: &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2000 {
		t.Errorf("got %d lines, want 2000", len(lines))
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "out") && !strings.HasPrefix(line, "err") {
			t.Fatalf("a line was cut or interleaved: %q", line)
		}
	}
}

type slowWriter struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	delay time.Duration
}

func (w *slowWriter) Write(p []byte) (int, error) {
	time.Sleep(w.delay)
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

// A caller's writer that falls behind still receives every byte before Wait
// returns, while data keeps arriving inside the drain window.
func TestWaitReturnsAfterAllOutputIsWritten(t *testing.T) {
	w := &slowWriter{delay: time.Second}
	p, err := Start(context.Background(), Spec{
		Argv:   []string{"/bin/sh", "-c", `dd if=/dev/zero bs=1024 count=200 2>/dev/null | tr '\0' x`},
		Dir:    t.TempDir(),
		Stdout: w,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err != nil {
		t.Fatal(err)
	}
	w.mu.Lock()
	n := w.buf.Len()
	w.mu.Unlock()
	if n != 200*1024 {
		t.Errorf("the writer got %d bytes, want %d", n, 200*1024)
	}
}

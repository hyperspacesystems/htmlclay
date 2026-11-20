//go:build !windows

package gitread

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// A named pipe where a repository file could be must not block a read.
func TestNamedPipeDoesNotHang(t *testing.T) {
	r := newRepo(t, t.TempDir())
	r.write("a.txt", "a\n")
	r.commit("one")
	r.run("checkout", "-q", "-b", "feature")
	r.write("a.txt", "b\n")
	r.commit("two")
	mkfifo(t, filepath.Join(r.dir, ".gitignore"))
	mkfifo(t, filepath.Join(r.dir, ".git", "info", "exclude"))

	before := runtime.NumGoroutine()
	for _, c := range []struct {
		name string
		call func(context.Context) error
	}{
		{"Status", func(ctx context.Context) error { _, err := Status(ctx, r.dir); return err }},
		{"Diff", func(ctx context.Context) error {
			_, err := Diff(ctx, r.dir, DiffOptions{Base: "main", Head: "feature"})
			return err
		}},
	} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			c.call(context.Background())
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s did not return within 2 seconds with a named pipe in the repository", c.name)
		}
	}
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines are still running, %d before the reads", runtime.NumGoroutine(), before)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func mkfifo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
}

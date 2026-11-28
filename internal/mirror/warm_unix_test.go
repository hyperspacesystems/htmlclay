//go:build unix

package mirror_test

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/mirror"
)

// A FIFO named .html in a trusted folder must not block the warmer: reading it would
// wait forever and no later pass would run.
func TestWarmingSkipsAFIFONamedHTML(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe.html"), 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	store := openStore(t, mirror.DefaultLimits)
	w := &mirror.Warmer{Fetch: mirror.NewFetcher(store, nil), Folders: func() []string { return []string{dir} }}
	done := make(chan struct{})
	go func() { w.Pass(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the warmer is still blocked on a FIFO named pipe.html after 3 seconds")
	}
}

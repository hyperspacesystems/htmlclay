//go:build managedtrial

package managedtrial

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRestructureLocksEveryPath proves a structure change waits for a write on
// every path it names: a save holding the destination's lock delays a move onto
// it, and the move is judged against the state that save left behind.
func TestRestructureLocksEveryPath(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := []byte("<!doctype html>\n<html><body><h1>One</h1></body></html>\n")
	writeCommitFile(t, root, "a.html", first)

	previousClock := versionClock
	versionClock = func() int64 { return 1762000000000 }
	t.Cleanup(func() { versionClock = previousClock })

	locked := make(chan struct{})
	release := make(chan struct{})
	hooks := &blockingCommitHooks{after: func() {
		close(locked)
		<-release
	}}
	host := newCommitHost(root, "root-structure-lock", newConsentRegistry(root, nil), hooks, nil)

	saved := make(chan commitResult, 1)
	go func() {
		result, _ := host.commit(&commitRequest{
			Kind:  "browser-save",
			Name:  "b.html",
			Bytes: []byte("<!doctype html>\n<html><body><h1>Two</h1></body></html>\n"),
			Text:  true,
		})
		saved <- result
	}()
	<-locked

	moved := make(chan structureResult, 1)
	go func() {
		result, _ := host.restructure("move", []structureEntry{{
			FromPath: filepath.Join(root, "a.html"),
			ToPath:   filepath.Join(root, "b.html"),
		}})
		moved <- result
	}()

	select {
	case result := <-moved:
		t.Fatalf("the move did not wait for the save on its destination: %+v", result)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	if result := <-saved; result.Outcome != "committed" {
		t.Fatalf("the save got %q, want committed", result.Outcome)
	}
	result := <-moved
	if result.Outcome != "committed" || result.Completed != 1 {
		t.Fatalf("the move got %+v, want committed after the save", result)
	}
	if data, err := os.ReadFile(filepath.Join(root, "b.html")); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(data, first) {
		t.Errorf("the move did not replace the saved file: %q", data)
	}
}

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/platform"
)

// startingSI answers "starting" to the first n commands, then answer.
type startingSI struct {
	commandSI
	n int
}

func (s *startingSI) SendCommand(cmd string) (string, error) {
	s.sent = append(s.sent, cmd)
	if len(s.sent) <= s.n {
		return platform.CommandStarting, nil
	}
	return s.answer, nil
}

// TestInstanceFrameWhileStarting: a command sent while the app is still
// starting waits and is sent again, and one that outlasts the wait says so in
// words and exits as not running (08-sync.md step 8.26's pair run hit this).
func TestInstanceFrameWhileStarting(t *testing.T) {
	t.Run("a command sent during start-up waits and runs", func(t *testing.T) {
		si := &startingSI{commandSI: commandSI{answer: "done Sync is on"}, n: 3}
		var out, errOut bytes.Buffer
		if code := sendInstanceFrame(si, "sync-resume", "sync", &out, &errOut); code != serveExitOK {
			t.Fatalf("exit %d (%q %q), want %d", code, out.String(), errOut.String(), serveExitOK)
		}
		if len(si.sent) != 4 || out.String() != "Sync is on\n" {
			t.Fatalf("sent %d frames, printed %q; want 4 and the answer", len(si.sent), out.String())
		}
	})
	t.Run("an app that stays starting is reported in words", func(t *testing.T) {
		old := instanceStartingWait
		instanceStartingWait = 300 * time.Millisecond
		t.Cleanup(func() { instanceStartingWait = old })
		si := &startingSI{commandSI: commandSI{answer: "done x"}, n: 1 << 30}
		var out, errOut bytes.Buffer
		if code := sendInstanceFrame(si, "sync-resume", "sync", &out, &errOut); code != serveExitNotRunning {
			t.Fatalf("exit %d, want %d", code, serveExitNotRunning)
		}
		if !strings.Contains(errOut.String(), "still starting") || strings.Contains(out.String(), "starting") {
			t.Fatalf("stdout %q stderr %q, want the sentence on stderr only", out.String(), errOut.String())
		}
	})
}

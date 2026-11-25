package panel

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestMain doubles as the panel's child: the supervisor starts this test binary
// again with PANEL_FAKE set, and it answers the way the real child does, with
// no window.
func TestMain(m *testing.M) {
	mode := os.Getenv("PANEL_FAKE")
	switch mode {
	case "":
		os.Exit(m.Run())
	case "exit1":
		os.Exit(1)
	}
	say := func(e Event) {
		line, _ := json.Marshal(e)
		fmt.Println(string(line))
	}
	if mode == "no-runtime" {
		say(Event{Event: EventNoRuntime})
		os.Exit(3)
	}
	say(Event{Event: EventReady})
	lines := bufio.NewScanner(os.Stdin)
	for lines.Scan() {
		var c Command
		json.Unmarshal(lines.Bytes(), &c)
		say(Event{Event: "got", Detail: fmt.Sprintf("%s %s %d %d", c.Cmd, c.URL, c.X, c.Y)})
		if c.Cmd == CmdToggle && mode == "reenter" {
			say(Event{Event: EventReenter})
		}
		if c.Cmd == CmdToggle && mode == "idle" {
			os.Exit(0)
		}
	}
	os.Exit(0)
}

func fake(mode string) func() *exec.Cmd {
	return func() *exec.Cmd {
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), "PANEL_FAKE="+mode)
		return cmd
	}
}

type seen struct {
	mu  sync.Mutex
	got []string
}

func (s *seen) add(e Event) {
	if e.Event != "got" {
		return
	}
	s.mu.Lock()
	s.got = append(s.got, e.Detail)
	s.mu.Unlock()
}

func (s *seen) list() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.got...)
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func entries() func() (string, error) {
	var n atomic.Int32
	return func() (string, error) {
		return fmt.Sprintf("http://entry/%d", n.Add(1)), nil
	}
}

// The child is signed in before it is shown: the entry link comes first, then
// the click point.
func TestToggleStartsTheChildAndNavigatesFirst(t *testing.T) {
	var s seen
	allowed := 0
	sup := &Supervisor{Spawn: fake("ok"), Entry: entries(), OnEvent: s.add, Allow: func(int) { allowed++ }}
	defer sup.Close()
	if err := sup.Toggle(10, 20); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the navigate and the toggle", func() bool { return len(s.list()) == 2 })
	if got := s.list(); got[0] != "navigate http://entry/1 0 0" || got[1] != "toggle  10 20" {
		t.Fatalf("child received %q", got)
	}
	if allowed != 1 {
		t.Fatalf("the child was let take the foreground %d times for one click, want 1", allowed)
	}
}

// A child that dies before it is ready (go-webview2 exits 1 on any WebView2
// error) turns the panel off after two tries, and the host is told once, so
// left click shows the menu again instead of doing nothing.
func TestAChildThatDiesBeforeReadyTurnsThePanelOff(t *testing.T) {
	var told atomic.Int32
	sup := &Supervisor{Spawn: fake("exit1"), Entry: entries(), MaxFailedStarts: 2, OnUnavailable: func() { told.Add(1) }}
	defer sup.Close()
	eventually(t, "the panel to turn off", func() bool {
		return errors.Is(sup.Toggle(1, 1), ErrUnavailable)
	})
	eventually(t, "the host to be told", func() bool { return told.Load() > 0 })
	sup.Toggle(1, 1)
	if n := told.Load(); n != 1 {
		t.Fatalf("the host was told %d times, want once", n)
	}
}

// A child that finds no WebView2 runtime turns the panel off at once: trying
// again cannot install it.
func TestAChildWithNoRuntimeTurnsThePanelOffAtOnce(t *testing.T) {
	off := make(chan struct{})
	sup := &Supervisor{Spawn: fake("no-runtime"), Entry: entries(), MaxFailedStarts: 2, OnUnavailable: func() { close(off) }}
	defer sup.Close()
	if err := sup.Toggle(1, 1); err != nil && !errors.Is(err, ErrUnavailable) {
		t.Logf("first toggle: %v", err)
	}
	select {
	case <-off:
	case <-time.After(5 * time.Second):
		t.Fatal("one start with no runtime did not turn the panel off")
	}
	if err := sup.Toggle(1, 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("toggle after no runtime = %v, want ErrUnavailable", err)
	}
}

// A child binary that cannot start at all turns the panel off too, and the host
// is told: left click must not stay dead.
func TestAChildThatCannotStartTurnsThePanelOff(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-htmlclay")
	noDashboard := func() (string, error) { return "", errors.New("the dashboard is not running") }
	for _, c := range []struct {
		name  string
		spawn func() *exec.Cmd
		entry func() (string, error)
	}{
		{"the program is missing", func() *exec.Cmd { return exec.Command(missing) }, entries()},
		{"there is no entry link to sign in with", fake("ok"), noDashboard},
	} {
		t.Run(c.name, func(t *testing.T) {
			var told atomic.Int32
			sup := &Supervisor{Spawn: c.spawn, Entry: c.entry, MaxFailedStarts: 2, OnUnavailable: func() { told.Add(1) }}
			defer sup.Close()
			sup.Toggle(1, 1)
			sup.Toggle(1, 1)
			if err := sup.Toggle(1, 1); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("third toggle = %v, want ErrUnavailable", err)
			}
			if n := told.Load(); n != 1 {
				t.Fatalf("the host was told %d times, want once", n)
			}
		})
	}
}

// The child ends itself after a while hidden. The next click starts a new one,
// signed in with a new entry link, and that is not a failed start.
func TestAChildThatExitedIsStartedAgainOnTheNextClick(t *testing.T) {
	var s seen
	sup := &Supervisor{Spawn: fake("idle"), Entry: entries(), OnEvent: s.add, MaxFailedStarts: 1}
	defer sup.Close()
	if err := sup.Toggle(1, 1); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the first child to exit", func() bool {
		sup.mu.Lock()
		defer sup.mu.Unlock()
		return sup.child == nil
	})
	if err := sup.Toggle(2, 2); err != nil {
		t.Fatalf("the click after the child exited: %v", err)
	}
	eventually(t, "the second child's navigate", func() bool {
		for _, got := range s.list() {
			if got == "navigate http://entry/2 0 0" {
				return true
			}
		}
		return false
	})
}

// Closing the pipe is how the child learns the host is gone: it ends itself,
// with no kill.
func TestClosingThePipeEndsTheChildWithoutAKill(t *testing.T) {
	sup := &Supervisor{Spawn: fake("ok"), Entry: entries()}
	if err := sup.Toggle(1, 1); err != nil {
		t.Fatal(err)
	}
	sup.mu.Lock()
	c := sup.child
	sup.mu.Unlock()
	sup.Close()
	if code := c.cmd.ProcessState.ExitCode(); code != 0 {
		t.Fatalf("child exit code %d, want 0 (it ended on its own when the pipe closed)", code)
	}
}

// When the page's session ran out, the child asks again and gets a fresh entry
// link; the page never navigates by itself.
func TestReenterGetsAFreshEntryLink(t *testing.T) {
	var s seen
	sup := &Supervisor{Spawn: fake("reenter"), Entry: entries(), OnEvent: s.add}
	defer sup.Close()
	if err := sup.Toggle(1, 1); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the second navigate", func() bool { return len(s.list()) == 3 })
	if got := s.list()[2]; got != "navigate http://entry/2 0 0" {
		t.Fatalf("after reenter the child received %q", got)
	}
}

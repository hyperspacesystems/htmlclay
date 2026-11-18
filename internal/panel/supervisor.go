package panel

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"
)

// ErrUnavailable is a Toggle after the panel was turned off for this run.
var ErrUnavailable = errors.New("the panel is unavailable")

// closeWait is how long Close lets the child end on its own before killing it.
const closeWait = 2 * time.Second

// Supervisor starts the panel's child on the first Toggle, signs it in with a
// fresh entry link, and starts it again after it exits (it exits on its own
// after a while hidden). A child that exits before it is ready is a failed
// start; MaxFailedStarts of them in a row, or one child that finds no WebView2
// runtime, turn the panel off for this run and call OnUnavailable.
type Supervisor struct {
	Spawn           func() *exec.Cmd
	Entry           func() (string, error)
	Allow           func(pid int)
	OnUnavailable   func()
	OnEvent         func(Event)
	Logf            func(format string, args ...any)
	MaxFailedStarts int

	mu     sync.Mutex
	child  *child
	failed int
	off    bool
	told   bool
	closed bool
}

type child struct {
	cmd       *exec.Cmd
	in        io.WriteCloser
	ready     bool
	noRuntime bool
	gone      chan struct{}
}

// Toggle shows the panel at the click point (physical screen pixels), or hides
// it when it shows.
func (s *Supervisor) Toggle(x, y int) error {
	s.mu.Lock()
	err := s.showLocked(x, y)
	tell := s.tellLocked()
	s.mu.Unlock()
	if tell {
		s.OnUnavailable()
	}
	return err
}

func (s *Supervisor) showLocked(x, y int) error {
	if s.off {
		return ErrUnavailable
	}
	fresh := s.child == nil
	if fresh {
		if err := s.startLocked(); err != nil {
			return err
		}
	}
	err := s.toggleLocked(x, y)
	if err != nil && !fresh {
		s.child = nil
		if err := s.startLocked(); err != nil {
			return err
		}
		err = s.toggleLocked(x, y)
	}
	return err
}

// tellLocked reports, once, that the panel turned itself off, so the host can
// show the menu on a left click again.
func (s *Supervisor) tellLocked() bool {
	if !s.off || s.told || s.closed || s.OnUnavailable == nil {
		return false
	}
	s.told = true
	return true
}

func (s *Supervisor) toggleLocked(x, y int) error {
	if s.Allow != nil {
		s.Allow(s.child.cmd.Process.Pid)
	}
	return s.sendLocked(Command{Cmd: CmdToggle, X: x, Y: y})
}

// Close ends the child and keeps the panel off: the host is shutting down.
func (s *Supervisor) Close() {
	s.mu.Lock()
	c := s.child
	s.off, s.closed = true, true
	s.mu.Unlock()
	if c == nil {
		return
	}
	c.in.Close()
	select {
	case <-c.gone:
	case <-time.After(closeWait):
		c.cmd.Process.Kill()
		<-c.gone
	}
}

func (s *Supervisor) startLocked() error {
	url, err := s.Entry()
	if err != nil {
		s.logf("Could not sign the panel in: %v", err)
		s.failedStartLocked()
		return err
	}
	cmd := s.Spawn()
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		s.logf("Could not start the panel: %v", err)
		s.failedStartLocked()
		return err
	}
	c := &child{cmd: cmd, in: in, gone: make(chan struct{})}
	s.child = c
	go s.watch(c, out)
	return s.sendLocked(Command{Cmd: CmdNavigate, URL: url})
}

func (s *Supervisor) watch(c *child, out io.Reader) {
	lines := bufio.NewScanner(out)
	for lines.Scan() {
		var e Event
		if json.Unmarshal(lines.Bytes(), &e) != nil {
			continue
		}
		switch e.Event {
		case EventReady:
			s.mu.Lock()
			c.ready = true
			s.failed = 0
			s.mu.Unlock()
		case EventNoRuntime:
			s.mu.Lock()
			c.noRuntime = true
			s.mu.Unlock()
		case EventReenter:
			s.mu.Lock()
			if s.child == c {
				if url, err := s.Entry(); err == nil {
					s.sendLocked(Command{Cmd: CmdNavigate, URL: url})
				}
			}
			s.mu.Unlock()
		case EventExit:
			s.logf("The panel is closing: %s", e.Detail)
		}
		if s.OnEvent != nil {
			s.OnEvent(e)
		}
	}
	err := c.cmd.Wait()
	s.mu.Lock()
	if s.child == c {
		s.child = nil
	}
	if !s.off {
		switch {
		case c.noRuntime:
			s.logf("The panel found no WebView2 runtime; left click shows the menu")
			s.off = true
		case !c.ready:
			s.logf("The panel exited before it was ready: %v", err)
			s.failedStartLocked()
		}
	}
	tell := s.tellLocked()
	s.mu.Unlock()
	close(c.gone)
	if tell {
		s.OnUnavailable()
	}
}

func (s *Supervisor) failedStartLocked() {
	s.failed++
	if s.failed >= max(s.MaxFailedStarts, 1) {
		s.off = true
	}
}

func (s *Supervisor) sendLocked(c Command) error {
	line, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = s.child.in.Write(append(line, '\n'))
	return err
}

func (s *Supervisor) logf(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
	}
}

// Package service supervises HTML Clay's live-room service: the JavaScript
// process that keeps tabs on one document live and writes their unsaved edits
// back through Go. It runs only while a folder is served.
package service

import "time"

// State is where the supervisor stands. The snapshot shows it as host.service.
type State string

const (
	// Off: no folder is served, so no process runs.
	Off State = "off"
	// Starting: the process was spawned and has not attached yet.
	Starting State = "starting"
	// Running: the process attached; live rooms work.
	Running State = "running"
	// Restarting: the process exited and starts again after a backoff.
	Restarting State = "restarting"
	// Stopping: the last folder stopped being served; waiting for the exit.
	Stopping State = "stopping"
	// Failed: five exits within two minutes. Saves run Go's native lanes until a
	// person restarts the service or the app.
	Failed State = "failed"
)

const (
	// failWindow and failExits: this many exits within the window means failed.
	failWindow = 2 * time.Minute
	failExits  = 5
	// attachDeadline: a process that has not attached by then is killed, which
	// counts as an exit.
	attachDeadline = 20 * time.Second
)

// backoff is the wait before the nth restart within the window.
var backoff = []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}

// EventKind is something that happened to the supervisor.
type EventKind int

const (
	// Want says whether any folder is served now (Event.On).
	Want EventKind = iota + 1
	// Spawned: the process started.
	Spawned
	// Attached: the process printed ATTACHED after Go sent its host line.
	Attached
	// Exited: the process ended, for any reason.
	Exited
	// Tick: time passed; the supervisor checks its deadlines.
	Tick
	// Restart: a person asked to restart the service (service.restart).
	Restart
)

// Event is one input to Step.
type Event struct {
	Kind EventKind
	On   bool
}

// CommandKind is something the supervisor asks its runner to do.
type CommandKind int

const (
	// Spawn the process and run its bootstrap.
	Spawn CommandKind = iota + 1
	// Stop: flush, then close the process's stdin so it exits.
	Stop
	// Kill the process now (it missed its attach deadline).
	Kill
	// WakeAt: send a Tick at Command.At.
	WakeAt
	// Detach the service link from every root, so saves run Go's native lanes.
	Detach
	// Reattach the link and mark every page document unconfirmed until the
	// service reconciles it (Go wrote natively while detached).
	Reattach
)

// Command is one output of Step.
type Command struct {
	Kind CommandKind
	At   time.Time
}

// Machine is the supervisor's state. It does no I/O: Step takes an event and the
// time and answers what to do, so every rule is a table test.
type Machine struct {
	state    State
	want     bool
	exits    []time.Time
	retryAt  time.Time
	deadline time.Time
	// detached is true from failed until the service attaches again.
	detached bool
}

// State is where the machine stands now.
func (m *Machine) State() State {
	if m.state == "" {
		return Off
	}
	return m.state
}

// Step applies one event at now and answers the commands to run, in order.
func (m *Machine) Step(now time.Time, ev Event) []Command {
	if m.state == "" {
		m.state = Off
	}
	switch ev.Kind {
	case Want:
		return m.setWant(now, ev.On)
	case Spawned:
		return nil
	case Attached:
		if m.state != Starting {
			return nil
		}
		m.state = Running
		m.deadline = time.Time{}
		if m.detached {
			m.detached = false
			return []Command{{Kind: Reattach}}
		}
		return nil
	case Exited:
		return m.exited(now)
	case Tick:
		return m.tick(now)
	case Restart:
		if m.state != Failed {
			return nil
		}
		m.exits = nil
		if !m.want {
			m.state = Off
			return nil
		}
		return m.spawn(now)
	}
	return nil
}

func (m *Machine) setWant(now time.Time, on bool) []Command {
	m.want = on
	switch m.state {
	case Off:
		if on {
			return m.spawn(now)
		}
	case Starting, Running:
		if !on {
			m.state = Stopping
			m.deadline = time.Time{}
			return []Command{{Kind: Stop}}
		}
	case Restarting:
		if !on {
			m.state = Off
			m.retryAt = time.Time{}
		}
	case Failed:
		if !on {
			// Nothing is served, so nothing is detached any more.
			m.state = Off
			m.detached = false
		}
	}
	// Stopping keeps going; its exit decides whether to start again.
	return nil
}

func (m *Machine) exited(now time.Time) []Command {
	switch m.state {
	case Stopping:
		m.state = Off
		if m.want {
			return m.spawn(now)
		}
		return nil
	case Starting, Running:
	default:
		return nil
	}
	m.deadline = time.Time{}
	kept := m.exits[:0]
	for _, at := range m.exits {
		if now.Sub(at) < failWindow {
			kept = append(kept, at)
		}
	}
	m.exits = append(kept, now)
	if len(m.exits) >= failExits {
		m.state = Failed
		m.detached = true
		return []Command{{Kind: Detach}}
	}
	wait := backoff[min(len(m.exits)-1, len(backoff)-1)]
	m.state = Restarting
	m.retryAt = now.Add(wait)
	return []Command{{Kind: WakeAt, At: m.retryAt}}
}

func (m *Machine) tick(now time.Time) []Command {
	switch m.state {
	case Restarting:
		if !now.Before(m.retryAt) {
			m.retryAt = time.Time{}
			return m.spawn(now)
		}
	case Starting:
		if !m.deadline.IsZero() && !now.Before(m.deadline) {
			m.deadline = time.Time{}
			return []Command{{Kind: Kill}}
		}
	}
	return nil
}

func (m *Machine) spawn(now time.Time) []Command {
	m.state = Starting
	m.deadline = now.Add(attachDeadline)
	return []Command{{Kind: Spawn}, {Kind: WakeAt, At: m.deadline}}
}

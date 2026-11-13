package service

import (
	"reflect"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

type step struct {
	at    time.Duration
	ev    Event
	want  []CommandKind
	state State
}

func run(t *testing.T, steps []step) *Machine {
	t.Helper()
	var m Machine
	for i, s := range steps {
		got := m.Step(t0.Add(s.at), s.ev)
		kinds := []CommandKind{}
		for _, c := range got {
			kinds = append(kinds, c.Kind)
		}
		want := s.want
		if want == nil {
			want = []CommandKind{}
		}
		if !reflect.DeepEqual(kinds, want) {
			t.Fatalf("step %d (%+v at %v): commands %v, want %v", i, s.ev, s.at, kinds, want)
		}
		if m.State() != s.state {
			t.Fatalf("step %d (%+v at %v): state %s, want %s", i, s.ev, s.at, m.State(), s.state)
		}
	}
	return &m
}

var (
	on      = Event{Kind: Want, On: true}
	off     = Event{Kind: Want, On: false}
	spawned = Event{Kind: Spawned}
	attach  = Event{Kind: Attached}
	exit    = Event{Kind: Exited}
	tick    = Event{Kind: Tick}
	restart = Event{Kind: Restart}
)

func TestStateStartsOnlyWhenAFolderIsServed(t *testing.T) {
	run(t, []step{
		{0, off, nil, Off},
		{0, tick, nil, Off},
		{0, on, []CommandKind{Spawn, WakeAt}, Starting},
		{0, spawned, nil, Starting},
		{time.Second, attach, nil, Running},
		{time.Minute, off, []CommandKind{Stop}, Stopping},
		{time.Minute + time.Second, exit, nil, Off},
	})
}

func TestStateBackoffThenRunning(t *testing.T) {
	m := run(t, []step{
		{0, on, []CommandKind{Spawn, WakeAt}, Starting},
		{time.Second, attach, nil, Running},
		{10 * time.Second, exit, []CommandKind{WakeAt}, Restarting},
	})
	if want := t0.Add(10*time.Second + 500*time.Millisecond); !m.retryAt.Equal(want) {
		t.Fatalf("first retry at %v, want %v", m.retryAt, want)
	}
	steps := []step{
		{10*time.Second + 400*time.Millisecond, tick, nil, Restarting},
		{10*time.Second + 500*time.Millisecond, tick, []CommandKind{Spawn, WakeAt}, Starting},
		{11 * time.Second, exit, []CommandKind{WakeAt}, Restarting},
	}
	for i, s := range steps {
		got := m.Step(t0.Add(s.at), s.ev)
		if len(got) != len(s.want) || m.State() != s.state {
			t.Fatalf("step %d: %v in %s, want %v in %s", i, got, m.State(), s.want, s.state)
		}
	}
	if want := t0.Add(12 * time.Second); !m.retryAt.Equal(want) {
		t.Fatalf("second retry at %v, want %v (one second after the second exit)", m.retryAt, want)
	}
}

func TestStateBackoffDoubles(t *testing.T) {
	var m Machine
	now := t0
	m.Step(now, on)
	var waits []time.Duration
	for range 4 {
		now = now.Add(time.Second)
		cmds := m.Step(now, exit)
		waits = append(waits, cmds[0].At.Sub(now))
		now = cmds[0].At
		m.Step(now, tick)
	}
	want := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second}
	if !reflect.DeepEqual(waits, want) {
		t.Fatalf("waits %v, want %v", waits, want)
	}
}

func TestStateFiveExitsInTwoMinutesFails(t *testing.T) {
	var m Machine
	now := t0
	m.Step(now, on)
	for i := range 4 {
		now = now.Add(20 * time.Second)
		if cmds := m.Step(now, exit); cmds[0].Kind != WakeAt {
			t.Fatalf("exit %d: %v, want a retry", i+1, cmds)
		}
		m.Step(now.Add(10*time.Second), tick)
	}
	now = now.Add(20 * time.Second)
	if cmds := m.Step(now, exit); len(cmds) != 1 || cmds[0].Kind != Detach {
		t.Fatalf("fifth exit within two minutes: %v, want Detach", cmds)
	}
	if m.State() != Failed {
		t.Fatalf("state %s, want failed", m.State())
	}
	if cmds := m.Step(now.Add(time.Hour), tick); len(cmds) != 0 || m.State() != Failed {
		t.Fatalf("failed restarted on its own: %v in %s", cmds, m.State())
	}
}

func TestStateExitsSpreadOutNeverFail(t *testing.T) {
	var m Machine
	now := t0
	m.Step(now, on)
	for i := range 10 {
		now = now.Add(31 * time.Second)
		cmds := m.Step(now, exit)
		if m.State() == Failed {
			t.Fatalf("exit %d, 31 s apart, failed", i+1)
		}
		m.Step(cmds[0].At, tick)
		m.Step(cmds[0].At, attach)
	}
}

func TestStateRestartAfterFailedReattaches(t *testing.T) {
	var m Machine
	now := t0
	m.Step(now, on)
	for range 5 {
		now = now.Add(time.Second)
		if cmds := m.Step(now, exit); len(cmds) > 0 && cmds[0].Kind == WakeAt {
			m.Step(cmds[0].At, tick)
		}
	}
	if m.State() != Failed {
		t.Fatalf("state %s, want failed", m.State())
	}
	run2 := []step{
		{time.Hour, restart, []CommandKind{Spawn, WakeAt}, Starting},
		{time.Hour + time.Second, attach, []CommandKind{Reattach}, Running},
		{2 * time.Hour, exit, []CommandKind{WakeAt}, Restarting},
	}
	for i, s := range run2 {
		got := m.Step(t0.Add(s.at), s.ev)
		kinds := []CommandKind{}
		for _, c := range got {
			kinds = append(kinds, c.Kind)
		}
		if !reflect.DeepEqual(kinds, s.want) || m.State() != s.state {
			t.Fatalf("step %d: %v in %s, want %v in %s", i, kinds, m.State(), s.want, s.state)
		}
	}
	// A later attach after an ordinary restart does not reattach again.
	m.Step(t0.Add(2*time.Hour+time.Second), tick)
	if cmds := m.Step(t0.Add(2*time.Hour+2*time.Second), attach); len(cmds) != 0 {
		t.Fatalf("an ordinary restart reattached: %v", cmds)
	}
}

func TestStateStopWhileStarting(t *testing.T) {
	run(t, []step{
		{0, on, []CommandKind{Spawn, WakeAt}, Starting},
		{100 * time.Millisecond, off, []CommandKind{Stop}, Stopping},
		{200 * time.Millisecond, attach, nil, Stopping},
		{300 * time.Millisecond, exit, nil, Off},
	})
}

func TestStateServedAgainWhileStopping(t *testing.T) {
	run(t, []step{
		{0, on, []CommandKind{Spawn, WakeAt}, Starting},
		{time.Second, attach, nil, Running},
		{2 * time.Second, off, []CommandKind{Stop}, Stopping},
		{2*time.Second + 100*time.Millisecond, on, nil, Stopping},
		{3 * time.Second, exit, []CommandKind{Spawn, WakeAt}, Starting},
	})
}

func TestStateStopWhileRestarting(t *testing.T) {
	run(t, []step{
		{0, on, []CommandKind{Spawn, WakeAt}, Starting},
		{time.Second, exit, []CommandKind{WakeAt}, Restarting},
		{time.Second + 100*time.Millisecond, off, nil, Off},
		{10 * time.Second, tick, nil, Off},
	})
}

func TestStateAttachDeadlineKills(t *testing.T) {
	run(t, []step{
		{0, on, []CommandKind{Spawn, WakeAt}, Starting},
		{19 * time.Second, tick, nil, Starting},
		{20 * time.Second, tick, []CommandKind{Kill}, Starting},
		{20*time.Second + 50*time.Millisecond, exit, []CommandKind{WakeAt}, Restarting},
	})
}

func TestStateFailedThenNothingServed(t *testing.T) {
	var m Machine
	now := t0
	m.Step(now, on)
	for range 5 {
		now = now.Add(time.Second)
		if cmds := m.Step(now, exit); len(cmds) > 0 && cmds[0].Kind == WakeAt {
			m.Step(cmds[0].At, tick)
		}
	}
	m.Step(now, off)
	if m.State() != Off || m.detached {
		t.Fatalf("state %s detached %v, want off and attached", m.State(), m.detached)
	}
	if cmds := m.Step(now.Add(time.Hour), on); len(cmds) == 0 || cmds[0].Kind != Spawn {
		t.Fatalf("serving again: %v, want Spawn", cmds)
	}
	if cmds := m.Step(now.Add(time.Hour+time.Second), attach); len(cmds) != 0 {
		t.Fatalf("attach after nothing was detached: %v, want no reattach", cmds)
	}
}

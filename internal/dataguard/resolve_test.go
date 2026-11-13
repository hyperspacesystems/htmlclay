package dataguard_test

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/dataguard"
	"github.com/panphora/htmlclay/internal/versions"
)

// writeBack mirrors the server's save: the bytes land on disk and the test keeps
// a copy to judge.
func (h *harness) writeBack(written *[]byte) func([]byte) error {
	return func(body []byte) error {
		*written = append([]byte(nil), body...)
		return os.WriteFile(h.docPath, body, 0o644)
	}
}

func (h *harness) currentEvent(html []byte) *dataguard.ClientEvent {
	h.t.Helper()
	event, err := h.guard.CurrentEvent(h.key, h.docPath, func() ([]byte, error) { return html, nil })
	if err != nil {
		h.t.Fatalf("CurrentEvent: %v", err)
	}
	return event
}

// CurrentEvent seeds a baseline on first sight, so a file that was already open
// when the app started is covered, and it re-validates a pending event against
// the bytes now on disk.
func TestCurrentEvent(t *testing.T) {
	whole := boardHTML("Board", fourItems...)
	twoItemsGone := boardHTML("Board", fourItems[:2]...)

	t.Run("first sight seeds the baseline from the current island", func(t *testing.T) {
		h := newHarness(t)
		h.clock = 7000
		if event := h.currentEvent(whole); event != nil {
			t.Fatalf("a seed raised an event: %+v", event)
		}
		st := h.state()
		if st.Baseline == nil {
			t.Fatal("no baseline seeded")
		}
		if !bytes.Equal(st.Baseline.Data, islandJSON(t, whole)) {
			t.Fatalf("baseline = %s, want %s", st.Baseline.Data, islandJSON(t, whole))
		}
		if st.Baseline.At != 7000 {
			t.Fatalf("baseline.at = %d, want 7000", st.Baseline.At)
		}
		if st.LastSeenHash != versions.Hash(whole) {
			t.Fatalf("lastSeenHash = %q, want the hash of the bytes", st.LastSeenHash)
		}
		if st.UIWorkPending {
			t.Fatal("a seed left uiWorkPending set")
		}
	})

	t.Run("an empty api island still seeds once it is wrapped", func(t *testing.T) {
		h := newHarness(t)
		if event := h.currentEvent(boardWith("", "")); event != nil {
			t.Fatalf("an empty island raised an event: %+v", event)
		}
		if st := h.state(); st.Baseline == nil {
			t.Fatal("no baseline seeded from a wrapped empty island")
		}
	})

	t.Run("a document with no api tag does not seed", func(t *testing.T) {
		h := newHarness(t)
		if event := h.currentEvent([]byte(`<html><body><h1 class="title">Board</h1></body></html>`)); event != nil {
			t.Fatalf("a document with no island raised an event: %+v", event)
		}
		if st := h.state(); st.Baseline != nil {
			t.Fatalf("baseline seeded from a document with no island: %+v", st.Baseline)
		}
	})

	t.Run("pending event still losing is returned", func(t *testing.T) {
		h := newHarness(t)
		fired := h.observe(twoItemsGone, whole, dataguard.UIGestured)
		if fired == nil {
			t.Fatal("no event raised")
		}
		event := h.currentEvent(twoItemsGone)
		if event == nil {
			t.Fatal("the pending event was not returned")
		}
		if event.ID != fired.ID {
			t.Fatalf("event id = %q, want %q", event.ID, fired.ID)
		}
		if !event.Restorable || !event.CanRevert {
			t.Fatalf("event = %+v, want restorable and revertable", event)
		}
		if event.Status != "pending" {
			t.Fatalf("status = %q, want pending", event.Status)
		}
	})

	t.Run("event whose loss was undone elsewhere is cleared", func(t *testing.T) {
		h := newHarness(t)
		fired := h.observe(twoItemsGone, whole, dataguard.UIGestured)
		if fired == nil {
			t.Fatal("no event raised")
		}
		if event := h.currentEvent(whole); event != nil {
			t.Fatalf("a resolved event stayed pending: %+v", event)
		}
		st := h.state()
		if st.Event != nil {
			t.Fatalf("stale event kept: %+v", st.Event)
		}
		if st.Status != "restored" {
			t.Fatalf("status = %q, want restored", st.Status)
		}
		if st.Baseline == nil || !bytes.Equal(st.Baseline.Data, islandJSON(t, whole)) {
			t.Fatalf("baseline = %+v, want the current island", st.Baseline)
		}
	})

	t.Run("unversioned document fails open", func(t *testing.T) {
		h := newHarness(t)
		event, err := h.guard.CurrentEvent("", h.docPath, func() ([]byte, error) { return whole, nil })
		if err != nil || event != nil {
			t.Fatalf("CurrentEvent = %+v, %v; want nothing", event, err)
		}
	})
}

// Dismiss adopts the current page as the baseline even when the clobber emptied
// it, so the same write does not fire again.
func TestResolveDismiss(t *testing.T) {
	whole := boardHTML("Board", fourItems...)
	clobbered := boardWith("", "")

	h := newHarness(t)
	fired := h.observe(clobbered, whole, dataguard.UIGestured)
	if fired == nil {
		t.Fatal("no event raised")
	}

	res, err := h.guard.Resolve(h.key, h.docPath, fired.ID, "dismiss", clobbered, func([]byte) error {
		t.Fatal("dismiss wrote to the page")
		return nil
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Status != "dismissed" || res.Choice != "dismiss" || res.ID != fired.ID {
		t.Fatalf("result = %+v", res)
	}
	st := h.state()
	if st.Event != nil || st.Status != "dismissed" {
		t.Fatalf("state = %+v, want a dismissed event", st)
	}
	if st.Baseline == nil || !bytes.Equal(st.Baseline.Data, islandJSON(t, clobbered)) {
		t.Fatalf("baseline = %+v, want the empty current island", st.Baseline)
	}
	if st.LastSeenHash != versions.Hash(clobbered) {
		t.Fatalf("lastSeenHash = %q, want the hash of the current bytes", st.LastSeenHash)
	}

	if event := h.observe(clobbered, clobbered, dataguard.UIGestured); event != nil {
		t.Fatalf("a dismissed wipe re-fired: %+v", event)
	}
}

func TestResolveRestore(t *testing.T) {
	whole := boardHTML("Board", fourItems...)
	twoItemsGone := boardHTML("Board", fourItems[:2]...)

	h := newHarness(t)
	fired := h.observe(twoItemsGone, whole, dataguard.UIGestured)
	if fired == nil {
		t.Fatal("no event raised")
	}

	var written []byte
	res, err := h.guard.Resolve(h.key, h.docPath, fired.ID, "restore", twoItemsGone, h.writeBack(&written))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Status != "restored" || res.Choice != "restore" || res.ID != fired.ID {
		t.Fatalf("result = %+v", res)
	}
	if !bytes.Contains(written, []byte("fourth note")) {
		t.Fatalf("restored page does not carry the lost item:\n%s", written)
	}
	onDisk, err := os.ReadFile(h.docPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, written) {
		t.Fatal("the written bytes are not the ones on disk")
	}
	if got, want := islandJSON(t, onDisk), islandJSON(t, whole); !bytes.Equal(got, want) {
		t.Fatalf("island after restore = %s, want %s", got, want)
	}

	st := h.state()
	if st.Event != nil || st.Status != "restored" {
		t.Fatalf("state = %+v, want a restored event", st)
	}
	if st.LastSeenHash != versions.Hash(written) {
		t.Fatalf("lastSeenHash = %q, want the hash of the written bytes", st.LastSeenHash)
	}
	if st.Baseline == nil || !bytes.Equal(st.Baseline.Data, islandJSON(t, whole)) {
		t.Fatalf("baseline = %+v, want the restored island", st.Baseline)
	}
}

// Restore overwrites the page, so a write that cannot reproduce the data is
// refused: an emptied list has no row left to clone.
func TestResolveRestoreRefusesUnreproducibleData(t *testing.T) {
	whole := boardHTML("Board", fourItems...)
	emptiedList := boardHTML("Board")

	h := newHarness(t)
	fired := h.observe(emptiedList, whole, dataguard.UIGestured)
	if fired == nil {
		t.Fatal("no event raised")
	}
	if fired.Restorable {
		t.Fatal("a page with no row to clone offered a restore")
	}

	res, err := h.guard.Resolve(h.key, h.docPath, fired.ID, "restore", emptiedList, func([]byte) error {
		t.Fatal("an unreproducible restore wrote to the page")
		return nil
	})
	if !errors.Is(err, dataguard.ErrNotRestorable) {
		t.Fatalf("Resolve = %+v, %v; want ErrNotRestorable", res, err)
	}
	st := h.state()
	if st.Event == nil || st.Event.ID != fired.ID {
		t.Fatalf("the refused restore dropped the event: %+v", st.Event)
	}
	if st.Status != "pending" {
		t.Fatalf("status = %q, want pending", st.Status)
	}
}

func TestResolveRevert(t *testing.T) {
	whole := boardHTML("Board", fourItems...)
	twoItemsGone := boardHTML("Board", fourItems[:2]...)

	h := newHarness(t)
	fired := h.observe(twoItemsGone, whole, dataguard.UIGestured)
	if fired == nil {
		t.Fatal("no event raised")
	}

	var written []byte
	res, err := h.guard.Resolve(h.key, h.docPath, fired.ID, "revert", twoItemsGone, h.writeBack(&written))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Status != "reverted" || res.Choice != "revert" || res.ID != fired.ID {
		t.Fatalf("result = %+v", res)
	}
	if !bytes.Equal(written, whole) {
		t.Fatalf("reverted page is %d bytes, want the %d-byte recover copy", len(written), len(whole))
	}
	st := h.state()
	if st.Event != nil || st.Status != "reverted" {
		t.Fatalf("state = %+v, want a reverted event", st)
	}
	if st.LastSeenHash != versions.Hash(whole) {
		t.Fatalf("lastSeenHash = %q, want the hash of the recovered bytes", st.LastSeenHash)
	}
	if st.Baseline == nil || !bytes.Equal(st.Baseline.Data, islandJSON(t, whole)) {
		t.Fatalf("baseline = %+v, want the recovered island", st.Baseline)
	}
}

func TestResolveRefusals(t *testing.T) {
	whole := boardHTML("Board", fourItems...)
	twoItemsGone := boardHTML("Board", fourItems[:2]...)
	noop := func([]byte) error { return nil }

	t.Run("no pending event", func(t *testing.T) {
		h := newHarness(t)
		if _, err := h.guard.Resolve(h.key, h.docPath, "", "dismiss", whole, noop); !errors.Is(err, dataguard.ErrNoEvent) {
			t.Fatalf("Resolve err = %v, want ErrNoEvent", err)
		}
	})

	t.Run("mismatched id keeps the event", func(t *testing.T) {
		h := newHarness(t)
		fired := h.observe(twoItemsGone, whole, dataguard.UIGestured)
		if fired == nil {
			t.Fatal("no event raised")
		}
		if _, err := h.guard.Resolve(h.key, h.docPath, "not-this-event", "dismiss", twoItemsGone, noop); !errors.Is(err, dataguard.ErrEventMismatch) {
			t.Fatalf("Resolve err = %v, want ErrEventMismatch", err)
		}
		st := h.state()
		if st.Event == nil || st.Event.ID != fired.ID || st.Status != "pending" {
			t.Fatalf("state = %+v, want the event untouched", st)
		}
	})

	t.Run("unknown choice keeps the event", func(t *testing.T) {
		h := newHarness(t)
		if h.observe(twoItemsGone, whole, dataguard.UIGestured) == nil {
			t.Fatal("no event raised")
		}
		if _, err := h.guard.Resolve(h.key, h.docPath, "", "explode", twoItemsGone, noop); !errors.Is(err, dataguard.ErrUnknownChoice) {
			t.Fatalf("Resolve err = %v, want ErrUnknownChoice", err)
		}
		if h.state().Event == nil {
			t.Fatal("an unknown choice dropped the event")
		}
	})

	t.Run("revert with no recover copy", func(t *testing.T) {
		h := newHarness(t)
		h.observe(whole, nil, dataguard.External)
		event, err := h.guard.Observe(dataguard.Observation{
			Key:     h.key,
			AbsPath: h.docPath,
			NewHTML: twoItemsGone,
			Prov:    dataguard.UIGestured,
			Notify:  func(string, *dataguard.ClientEvent, string) {},
		})
		if err != nil {
			t.Fatalf("Observe: %v", err)
		}
		if event == nil || event.CanRevert {
			t.Fatalf("event = %+v, want one without a revert", event)
		}
		if _, err := h.guard.Resolve(h.key, h.docPath, event.ID, "revert", twoItemsGone, noop); !errors.Is(err, dataguard.ErrNoRecoverCopy) {
			t.Fatalf("Resolve err = %v, want ErrNoRecoverCopy", err)
		}
	})

	t.Run("failed write-back keeps the event", func(t *testing.T) {
		h := newHarness(t)
		fired := h.observe(twoItemsGone, whole, dataguard.UIGestured)
		if fired == nil {
			t.Fatal("no event raised")
		}
		boom := errors.New("disk full")
		if _, err := h.guard.Resolve(h.key, h.docPath, fired.ID, "revert", twoItemsGone, func([]byte) error { return boom }); !errors.Is(err, boom) {
			t.Fatalf("Resolve err = %v, want the write-back error", err)
		}
		if st := h.state(); st.Event == nil || st.Event.ID != fired.ID {
			t.Fatalf("a failed write dropped the event: %+v", st.Event)
		}
	})
}

// A resolve runs behind whatever the queue is already judging for the document,
// which is what keeps it from interleaving with an Observe.
func TestResolveWaitsForQueuedJob(t *testing.T) {
	whole := boardHTML("Board", fourItems...)
	twoItemsGone := boardHTML("Board", fourItems[:2]...)

	h := newHarness(t)
	fired := h.observe(twoItemsGone, whole, dataguard.UIGestured)
	if fired == nil {
		t.Fatal("no event raised")
	}

	var q dataguard.Queue
	h.guard.SetQueue(&q)

	release := make(chan struct{})
	started := make(chan struct{})
	q.Do(h.key, func() {
		close(started)
		<-release
	})
	<-started

	type outcome struct {
		res *dataguard.ResolveResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := h.guard.Resolve(h.key, h.docPath, fired.ID, "dismiss", twoItemsGone, func([]byte) error { return nil })
		done <- outcome{res, err}
	}()

	select {
	case got := <-done:
		t.Fatalf("resolve ran ahead of the queued job: %+v, %v", got.res, got.err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	got := <-done
	if got.err != nil {
		t.Fatalf("Resolve: %v", got.err)
	}
	if got.res.Status != "dismissed" {
		t.Fatalf("result = %+v", got.res)
	}
}

// CurrentEvent reads the document inside its own queue job, so an answer can never
// be computed from bytes older than a write already judged.
func TestCurrentEventReadsInsideTheQueue(t *testing.T) {
	h := newHarness(t)
	whole := boardHTML("Board", fourItems...)
	if event := h.currentEvent(whole); event != nil {
		t.Fatalf("first sight raised an event: %+v", event)
	}

	queue := &dataguard.Queue{}
	h.guard.SetQueue(queue)
	ran := make(chan string, 2)
	entered, release := make(chan struct{}), make(chan struct{})
	queue.Do(h.key, func() {
		close(entered)
		<-release
		ran <- "job"
	})
	<-entered

	type answer struct {
		event *dataguard.ClientEvent
		err   error
	}
	done := make(chan answer, 1)
	go func() {
		event, err := h.guard.CurrentEvent(h.key, h.docPath, func() ([]byte, error) {
			ran <- "read"
			return whole, nil
		})
		done <- answer{event: event, err: err}
	}()
	close(release)

	select {
	case got := <-done:
		if got.err != nil || got.event != nil {
			t.Fatalf("CurrentEvent = %+v, %v; want nothing", got.event, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CurrentEvent never answered")
	}
	first, second := <-ran, <-ran
	if first != "job" || second != "read" {
		t.Fatalf("the read ran %q then %q; want the already-queued job first, so the read happens inside CurrentEvent's own job", first, second)
	}
}

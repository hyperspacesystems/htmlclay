package dataguard_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dataguard"
	"github.com/panphora/htmlclay/internal/server"
)

// keepFailsHistory is a history whose copies cannot be stored: every other read
// goes through the real history, the way the watcher provides it.
type keepFailsHistory struct {
	*server.GuardHistory
}

func (keepFailsHistory) Keep([]byte) (string, error) {
	return "", errors.New("the version store refused the copy")
}

func (h *harness) observeWithHistory(newHTML, prevHTML []byte, prov dataguard.Provenance, history dataguard.HistoryReader) *dataguard.ClientEvent {
	h.t.Helper()
	event, err := h.guard.Observe(dataguard.Observation{
		Key:      h.key,
		AbsPath:  h.docPath,
		NewHTML:  newHTML,
		PrevHTML: prevHTML,
		Prov:     prov,
		History:  history,
		Notify: func(kind string, event *dataguard.ClientEvent, id string) {
			h.notes = append(h.notes, notification{kind: kind, id: id, event: event})
		},
	})
	if err != nil {
		h.t.Fatalf("Observe: %v", err)
	}
	return event
}

// A stale file that puts its own older answer back over one the person typed shows
// no sign of losing data, so the guard stays silent. The answer it replaced is
// still a copy the person made, so it is pinned.
func TestStaleOutsideWriteReplacingATypedValuePinsIt(t *testing.T) {
	stale := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", "old note"}, putBackItem{"6", "six", "six note"})
	typed := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", "my note"}, putBackItem{"6", "six", "six note"})

	h := newHarness(t)
	h.clock = 500
	if event := h.observe(stale, nil, dataguard.External); event != nil {
		t.Fatalf("cold seed fired: %+v", event)
	}
	h.clock = 1000
	if event := h.observe(typed, stale, dataguard.UIGestured); event != nil {
		t.Fatalf("the person's save fired: %+v", event)
	}
	h.backupForTest(typed)
	h.clock = 1000 + 2*60*60*1000
	h.backupForTest(stale)

	if event := h.observe(stale, nil, dataguard.External); event != nil {
		t.Fatalf("a stale file replacing a typed answer fired: %+v", event)
	}

	bodies := h.pinnedBodies()
	found := false
	for _, body := range bodies {
		if string(body) == string(typed) {
			found = true
		}
	}
	if !found {
		t.Fatalf("the version holding the typed answer was not pinned: %q", bodies)
	}
}

// When the version the guard replaced cannot be kept, silence would lose it: the
// guard speaks instead, with the verdict the judge gave, and leaves the typing
// record and the baseline where they were.
func TestUnkeepableReplacedVersionRaisesAnEvent(t *testing.T) {
	start := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", ""}, putBackItem{"6", "six", "six note"})
	typed := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", "my note"}, putBackItem{"6", "six", "six note"})
	rewritten := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", ""}, putBackItem{"6", "six", "six note"})

	h := newHarness(t)
	h.clock = 500
	h.backupForTest(start)
	if event := h.observe(start, nil, dataguard.External); event != nil {
		t.Fatalf("cold seed fired: %+v", event)
	}
	h.clock = 1000
	if event := h.observe(typed, start, dataguard.UIGestured); event != nil {
		t.Fatalf("the person's save fired: %+v", event)
	}
	h.backupForTest(typed)
	h.clock = 1000 + 2*60*60*1000
	h.backupForTest(rewritten)

	failing := keepFailsHistory{GuardHistory: h.history}
	event := h.observeWithHistory(rewritten, nil, dataguard.External, failing)
	if event == nil {
		t.Fatal("a loss whose copy could not be kept stayed silent")
	}
	if event.Reason != "" {
		t.Fatalf("reason = %q, want the judge's empty reason", event.Reason)
	}
	if len(event.Typed) != 1 || event.Typed[0].Yours != "my note" || !strings.Contains(event.Typed[0].Path, "[id:5].note") {
		t.Fatalf("client typed rows = %+v", event.Typed)
	}
	if len(h.notes) == 0 || h.notes[len(h.notes)-1].kind != "raised" {
		t.Fatalf("notifications = %+v, want a raised event", h.notes)
	}

	st := h.state()
	if st.Event == nil || st.Status != "pending" {
		t.Fatalf("state = %+v, want a pending event", st)
	}
	if st.Baseline == nil || !bytes.Equal(st.Baseline.Data, islandJSON(t, typed)) {
		t.Fatalf("baseline = %+v, want the typed island", st.Baseline)
	}
	if len(st.Typed) != 1 || st.Typed[0].Preview != "my note" {
		t.Fatalf("the raised event dropped the typing record: %+v", st.Typed)
	}
	if names, err := h.v.Pinned(h.key, h.docPath); err != nil {
		t.Fatal(err)
	} else if len(names) != 0 {
		t.Fatalf("an event was raised but versions were pinned anyway: %v", names)
	}
}

// While one event is pending, a later outside write that takes another typed value
// joins the same event, and a put-back places both.
func TestPendingEventGathersLaterTypedLosses(t *testing.T) {
	start := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", ""}, putBackItem{"6", "six", ""})
	oneNote := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", ""}, putBackItem{"6", "six", "other note"})
	typed := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", "my note"}, putBackItem{"6", "six", "other note"})
	firstLoss := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", ""}, putBackItem{"6", "six", "other note"})
	secondLoss := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", ""}, putBackItem{"6", "six", ""})

	h := newHarness(t)
	h.clock = 500
	h.backupForTest(start)
	if event := h.observe(start, nil, dataguard.External); event != nil {
		t.Fatalf("cold seed fired: %+v", event)
	}
	h.clock = 1000
	if event := h.observe(oneNote, start, dataguard.UIGestured); event != nil {
		t.Fatalf("the person's first save fired: %+v", event)
	}
	h.clock = 1100
	if event := h.observe(typed, oneNote, dataguard.UIGestured); event != nil {
		t.Fatalf("the person's second save fired: %+v", event)
	}
	h.clock = 1500
	h.backupForTest(firstLoss)
	fired := h.observe(firstLoss, nil, dataguard.External)
	if fired == nil {
		t.Fatal("the first loss did not fire")
	}
	if len(fired.Typed) != 1 || fired.Typed[0].Yours != "my note" {
		t.Fatalf("client typed rows after the first loss = %+v", fired.Typed)
	}
	h.clock = 1600
	h.backupForTest(secondLoss)
	if event := h.observe(secondLoss, nil, dataguard.External); event != nil {
		t.Fatalf("a later loss raised a second event: %+v", event)
	}

	st := h.state()
	if st.Event == nil || st.Event.ID != fired.ID {
		t.Fatalf("state = %+v, want the first event still pending", st)
	}
	yours := map[string]bool{}
	for _, row := range st.Event.Typed {
		yours[row.Preview] = true
	}
	if !yours["my note"] || !yours["other note"] || len(st.Event.Typed) != 2 {
		t.Fatalf("the pending event holds %+v, want both typed notes", st.Event.Typed)
	}

	var written []byte
	if _, err := h.guard.Resolve(h.key, h.docPath, fired.ID, "put-back", secondLoss, h.writeBack(&written)); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := islandJSON(t, written); !bytes.Equal(got, islandJSON(t, typed)) {
		t.Fatalf("island after put-back = %s, want %s", got, islandJSON(t, typed))
	}
	if st := h.state(); len(st.Typed) != 2 {
		t.Fatalf("the put-back dropped the typing record: %+v", st.Typed)
	}
}

// Dismissing accepts the page as it is, so the typing record forgets what that page
// no longer holds and a later write that changes elsewhere is quiet.
func TestDismissReconcilesTheTypingRecord(t *testing.T) {
	start := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", ""}, putBackItem{"6", "six", "six note"})
	typed := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", "my note"}, putBackItem{"6", "six", "six note"})
	rewritten := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", ""}, putBackItem{"6", "six", "six note"})
	renamed := putBackHTML("Renamed", []string{"red"}, putBackItem{"5", "five", ""}, putBackItem{"6", "six", "six note"})

	h := newHarness(t)
	h.clock = 500
	if event := h.observe(start, nil, dataguard.External); event != nil {
		t.Fatalf("cold seed fired: %+v", event)
	}
	h.clock = 1000
	if event := h.observe(typed, start, dataguard.UIGestured); event != nil {
		t.Fatalf("the person's save fired: %+v", event)
	}
	h.clock = 1500
	fired := h.observe(rewritten, nil, dataguard.External)
	if fired == nil {
		t.Fatal("the loss did not fire")
	}
	if _, err := h.guard.Resolve(h.key, h.docPath, fired.ID, "dismiss", rewritten, func([]byte) error {
		t.Fatal("dismiss wrote to the page")
		return nil
	}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if st := h.state(); len(st.Typed) != 0 {
		t.Fatalf("dismiss kept a typing record for data the page no longer holds: %+v", st.Typed)
	}

	h.clock = 2000
	if event := h.observe(renamed, nil, dataguard.External); event != nil {
		t.Fatalf("a title-only write after dismiss fired: %+v", event)
	}
	if names, err := h.v.Pinned(h.key, h.docPath); err != nil {
		t.Fatal(err)
	} else if len(names) != 0 {
		t.Fatalf("a title-only write after dismiss pinned %v", names)
	}
}

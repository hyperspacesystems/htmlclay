package dataguard_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/dataguard"
	"github.com/panphora/htmlclay/internal/server"
	"github.com/panphora/htmlclay/internal/versions"
)

const guardRulesTag = `<script type="application/json" data-rules-name="api" data-rules-version="1">{title:".title", subtitle:".subtitle", items:[".item",{name:".name",note:".note"}]}</script>`

// badRulesTag parses as a document but not as a rules tag, which is the
// fail-open case: the guard cannot read this file's data.
const badRulesTag = `<script type="application/json" data-rules-name="api" data-rules-version="2">{title:".title"}</script>`

type boardItem struct{ name, note string }

func boardHTML(title string, items ...boardItem) []byte {
	return boardWith(title, "sub", items...)
}

func boardWith(title, subtitle string, items ...boardItem) []byte {
	var b strings.Builder
	b.WriteString(`<html><body><h1 class="title">` + title + `</h1><p class="subtitle">` + subtitle + `</p><ul>`)
	for _, item := range items {
		b.WriteString(`<li class="item"><span class="name">` + item.name + `</span><span class="note">` + item.note + `</span></li>`)
	}
	b.WriteString(`</ul>` + guardRulesTag + `</body></html>`)
	return []byte(b.String())
}

var fourItems = []boardItem{
	{"alpha", "first note"},
	{"beta", "second note"},
	{"gamma", "third note"},
	{"delta", "fourth note"},
}

type notification struct {
	kind  string
	id    string
	event *dataguard.ClientEvent
}

type harness struct {
	t       *testing.T
	docPath string
	key     string
	store   *dataguard.Store
	v       *versions.Store
	history *server.GuardHistory
	guard   *dataguard.Guard
	clock   int64
	notes   []notification
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v := versions.New(filepath.Join(cfg, "versions"))
	doc := filepath.Join(dir, "board.htmlclay")
	if err := os.WriteFile(doc, []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	key := versions.Key(doc, nil)
	h := &harness{t: t, docPath: doc, key: key, store: dataguard.NewStore(v.Folder), v: v, clock: 1000}
	h.history = server.NewGuardHistory(v, key, doc)
	h.guard = dataguard.NewGuard(h.store, server.Extractor{}, func() int64 { return h.clock })
	return h
}

func (h *harness) observe(newHTML, prevHTML []byte, prov dataguard.Provenance) *dataguard.ClientEvent {
	h.t.Helper()
	event, err := h.guard.Observe(dataguard.Observation{
		Key:      h.key,
		AbsPath:  h.docPath,
		NewHTML:  newHTML,
		PrevHTML: prevHTML,
		Prov:     prov,
		History:  h.history,
		Notify: func(kind string, event *dataguard.ClientEvent, id string) {
			h.notes = append(h.notes, notification{kind: kind, id: id, event: event})
		},
	})
	if err != nil {
		h.t.Fatalf("Observe: %v", err)
	}
	return event
}

func (h *harness) state() *dataguard.State {
	h.t.Helper()
	st, err := h.store.Read(h.key, h.docPath)
	if err != nil {
		h.t.Fatalf("read state: %v", err)
	}
	return st
}

func (h *harness) recoverCopy() []byte {
	h.t.Helper()
	body, err := h.store.ReadRecover(h.key, h.docPath)
	if err != nil {
		h.t.Fatalf("read recover copy: %v", err)
	}
	return body
}

func islandOf(t *testing.T, html []byte) dataapi.Value {
	t.Helper()
	v, err := server.Extractor{}.Extract(html)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	return v
}

func islandJSON(t *testing.T, html []byte) []byte {
	t.Helper()
	b, err := dataapi.Marshal(islandOf(t, html))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestObserveWrites judges one write against the baseline. Every case starts
// cold, so the pre-write body seeds the baseline and the write is detected
// against it.
func TestObserveWrites(t *testing.T) {
	whole := boardHTML("Board", fourItems...)
	oneLeafGone := boardHTML("Board", fourItems[:3]...)
	twoItemsGone := boardHTML("Board", fourItems[:2]...)
	oneFieldEdited := boardHTML("Board renamed", fourItems...)

	cases := []struct {
		name         string
		prev, next   []byte
		prov         dataguard.Provenance
		wantFire     bool
		wantPending  bool
		wantBaseline []byte
	}{
		{
			name:         "gestured save that removes two items fires",
			prev:         whole,
			next:         twoItemsGone,
			prov:         dataguard.UIGestured,
			wantFire:     true,
			wantBaseline: islandJSON(t, whole),
		},
		{
			name:         "gestured save that edits one field advances the baseline",
			prev:         whole,
			next:         oneFieldEdited,
			prov:         dataguard.UIGestured,
			wantFire:     false,
			wantPending:  true,
			wantBaseline: islandJSON(t, oneFieldEdited),
		},
		{
			name:         "background save that removes one leaf fires",
			prev:         whole,
			next:         oneLeafGone,
			prov:         dataguard.UIBackground,
			wantFire:     true,
			wantBaseline: islandJSON(t, whole),
		},
		{
			name:         "unknown save that removes one leaf does not fire or advance",
			prev:         whole,
			next:         oneLeafGone,
			prov:         dataguard.UIUnknown,
			wantFire:     false,
			wantBaseline: islandJSON(t, whole),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			event := h.observe(c.next, c.prev, c.prov)

			if got := event != nil; got != c.wantFire {
				t.Fatalf("fired = %v, want %v (event %+v)", got, c.wantFire, event)
			}
			st := h.state()
			if st.Baseline == nil {
				t.Fatal("state has no baseline")
			}
			if !bytes.Equal(st.Baseline.Data, c.wantBaseline) {
				t.Fatalf("baseline = %s, want %s", st.Baseline.Data, c.wantBaseline)
			}
			if st.UIWorkPending != c.wantPending {
				t.Fatalf("uiWorkPending = %v, want %v", st.UIWorkPending, c.wantPending)
			}
			if st.LastSeenHash != versions.Hash(c.next) {
				t.Fatalf("lastSeenHash = %q, want hash of the new bytes", st.LastSeenHash)
			}
		})
	}
}

// A fire must keep the pre-write body itself, so Revert does not depend on a
// version the pruner may age out.
func TestObserveFiredEventKeepsRecoverCopy(t *testing.T) {
	whole := boardHTML("Board", fourItems...)
	twoItemsGone := boardHTML("Board", fourItems[:2]...)

	h := newHarness(t)
	event := h.observe(twoItemsGone, whole, dataguard.UIGestured)
	if event == nil {
		t.Fatal("no event raised")
	}
	if !event.CanRevert {
		t.Fatal("event says it cannot revert")
	}
	if got := h.recoverCopy(); !bytes.Equal(got, whole) {
		t.Fatalf("recover copy is not the pre-write body: %d bytes", len(got))
	}
	st := h.state()
	if st.Event == nil || !st.Event.HasRecoverCopy {
		t.Fatalf("stored event has no recover copy: %+v", st.Event)
	}
	if st.Status != "pending" {
		t.Fatalf("status = %q, want pending", st.Status)
	}
	if len(h.notes) != 1 || h.notes[0].kind != "raised" || h.notes[0].event == nil {
		t.Fatalf("notifications = %+v, want one raised event", h.notes)
	}
	if h.notes[0].event.ID != st.Event.ID {
		t.Fatalf("notified id %q, stored id %q", h.notes[0].event.ID, st.Event.ID)
	}
}

// The watcher backs up the incoming bytes before the guard runs, so the newest
// version IS the clobber. The recover copy must be the newest version that is
// not those bytes.
func TestObserveOutsideEditRecoversNewestDifferingVersion(t *testing.T) {
	whole := boardHTML("Board", fourItems...)
	oneLeafGone := boardHTML("Board", fourItems[:3]...)

	h := newHarness(t)
	if event := h.observe(whole, nil, dataguard.External); event != nil {
		t.Fatalf("a blind seed must not fire, got %+v", event)
	}

	for _, body := range [][]byte{whole, oneLeafGone} {
		if _, err := h.v.Backup(h.key, h.docPath, body); err != nil {
			t.Fatalf("backup: %v", err)
		}
	}

	event := h.observe(oneLeafGone, nil, dataguard.External)
	if event == nil {
		t.Fatal("outside edit that removed a leaf did not fire")
	}
	if got := h.recoverCopy(); !bytes.Equal(got, whole) {
		t.Fatalf("recover copy is not the newest differing version (%d bytes)", len(got))
	}
	if !h.state().Event.HasRecoverCopy {
		t.Fatal("stored event has no recover copy")
	}
}

// When neither the pre-write body nor a differing version exists there is
// nothing to hand back, and the event says so instead of offering a revert that
// would fail.
func TestObserveFiredEventWithoutRecoverCopy(t *testing.T) {
	whole := boardHTML("Board", fourItems...)
	oneLeafGone := boardHTML("Board", fourItems[:3]...)

	h := newHarness(t)
	h.observe(whole, nil, dataguard.External)
	event, err := h.guard.Observe(dataguard.Observation{
		Key:     h.key,
		AbsPath: h.docPath,
		NewHTML: oneLeafGone,
		Prov:    dataguard.External,
	})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if event == nil {
		t.Fatal("outside edit that removed a leaf did not fire")
	}
	if event.CanRevert {
		t.Fatal("event offers a revert with no recover copy")
	}
	st := h.state()
	if st.Event == nil || st.Event.HasRecoverCopy {
		t.Fatalf("stored event claims a recover copy: %+v", st.Event)
	}
	if body := h.recoverCopy(); body != nil {
		t.Fatalf("a recover copy was stored anyway: %d bytes", len(body))
	}
}

// An outside edit on a cold store cannot be judged: there is no baseline to
// compare it to, so it seeds one and stays quiet.
func TestObserveColdOutsideEditSeedsBlind(t *testing.T) {
	whole := boardHTML("Board", fourItems...)

	h := newHarness(t)
	if event := h.observe(whole, nil, dataguard.External); event != nil {
		t.Fatalf("cold outside edit fired: %+v", event)
	}
	st := h.state()
	if st.Event != nil {
		t.Fatalf("cold outside edit raised an event: %+v", st.Event)
	}
	if st.Baseline == nil || !bytes.Equal(st.Baseline.Data, islandJSON(t, whole)) {
		t.Fatalf("baseline not seeded from the write: %+v", st.Baseline)
	}
	if st.LastSeenHash != versions.Hash(whole) {
		t.Fatalf("lastSeenHash = %q", st.LastSeenHash)
	}
	if body := h.recoverCopy(); body != nil {
		t.Fatalf("cold outside edit stored a recover copy: %d bytes", len(body))
	}
}

// A seed that cannot be read is a fail-open path, but the write itself was seen,
// so the last-seen hash still has to advance or every catch-up pass replays it.
func TestObserveUnreadableSeedStillRecordsLastSeen(t *testing.T) {
	whole := boardHTML("Board", fourItems...)

	h := newHarness(t)
	event := h.observe(whole, []byte(`<html><body>`+badRulesTag+`</body></html>`), dataguard.UIGestured)
	if event != nil {
		t.Fatalf("unreadable seed raised an event: %+v", event)
	}
	st := h.state()
	if st.LastSeenHash != versions.Hash(whole) {
		t.Fatalf("lastSeenHash = %q, want %q", st.LastSeenHash, versions.Hash(whole))
	}
	if st.Baseline == nil || !bytes.Equal(st.Baseline.Data, islandJSON(t, whole)) {
		t.Fatalf("baseline not seeded blindly: %+v", st.Baseline)
	}
}

// The pin holds while an event is open. A write that puts every recoverable atom
// back clears it; a write that only edits one keeps the warning.
func TestObserveOpenEvent(t *testing.T) {
	whole := boardHTML("Board", fourItems...)
	twoItemsGone := boardHTML("Board", fourItems[:2]...)
	restoredWithAddition := boardHTML("Board", append(append([]boardItem{}, fourItems...), boardItem{"epsilon", "fifth note"})...)
	editedAtom := boardHTML("Board", fourItems[0], boardItem{"beta", "edited note"}, fourItems[2], fourItems[3])
	stillLosing := boardHTML("Board", boardItem{"alpha", "edited note"}, fourItems[1])

	t.Run("a later write containing all recoverable data auto-resolves", func(t *testing.T) {
		h := newHarness(t)
		fired := h.observe(twoItemsGone, whole, dataguard.UIGestured)
		if fired == nil {
			t.Fatal("no event raised")
		}
		h.clock = 2000
		if event := h.observe(restoredWithAddition, twoItemsGone, dataguard.UIGestured); event != nil {
			t.Fatalf("restore raised a new event: %+v", event)
		}
		st := h.state()
		if st.Event != nil {
			t.Fatalf("event survived the restore: %+v", st.Event)
		}
		if st.Status != "restored" {
			t.Fatalf("status = %q, want restored", st.Status)
		}
		if st.UIWorkPending {
			t.Fatal("uiWorkPending still set after a restore")
		}
		if st.Baseline == nil || !bytes.Equal(st.Baseline.Data, islandJSON(t, restoredWithAddition)) {
			t.Fatalf("baseline did not advance to the restored island: %+v", st.Baseline)
		}
		if st.LastSeenHash != versions.Hash(restoredWithAddition) {
			t.Fatal("lastSeenHash did not advance")
		}
		if len(h.notes) != 2 || h.notes[1].kind != "resolved" || h.notes[1].id != fired.ID {
			t.Fatalf("notifications = %+v, want a resolved %q", h.notes, fired.ID)
		}
	})

	t.Run("a later write that modifies a recoverable atom keeps the event", func(t *testing.T) {
		h := newHarness(t)
		fired := h.observe(twoItemsGone, whole, dataguard.UIGestured)
		if fired == nil {
			t.Fatal("no event raised")
		}
		h.clock = 2000
		if event := h.observe(editedAtom, twoItemsGone, dataguard.UIGestured); event != nil {
			t.Fatalf("kept event was re-raised: %+v", event)
		}
		st := h.state()
		if st.Event == nil || st.Event.ID != fired.ID {
			t.Fatalf("event did not survive the edit: %+v", st.Event)
		}
		if st.Event.LastWriteAt != fired.FirstDetectedAt {
			t.Fatalf("lastWriteAt = %d, want it untouched at %d", st.Event.LastWriteAt, fired.FirstDetectedAt)
		}
	})

	t.Run("a later write that still destroys updates LastWriteAt", func(t *testing.T) {
		h := newHarness(t)
		fired := h.observe(twoItemsGone, whole, dataguard.UIGestured)
		if fired == nil {
			t.Fatal("no event raised")
		}
		h.clock = 2000
		if event := h.observe(stillLosing, twoItemsGone, dataguard.UIGestured); event != nil {
			t.Fatalf("kept event was re-raised: %+v", event)
		}
		st := h.state()
		if st.Event == nil || st.Event.ID != fired.ID {
			t.Fatalf("event did not survive: %+v", st.Event)
		}
		if st.Event.LastWriteAt != 2000 {
			t.Fatalf("lastWriteAt = %d, want 2000", st.Event.LastWriteAt)
		}
	})
}

// A body the guard cannot read leaves every bookkeeping value exactly as it was
// and is not an error: the write already happened.
func TestObserveUnreadableBodyLeavesStateAlone(t *testing.T) {
	whole := boardHTML("Board", fourItems...)
	unreadable := []byte(`<html><body>` + badRulesTag + `</body></html>`)

	h := newHarness(t)
	h.observe(whole, nil, dataguard.External)
	before := h.state()

	if event := h.observe(unreadable, whole, dataguard.UIGestured); event != nil {
		t.Fatalf("unreadable body raised an event: %+v", event)
	}
	after := h.state()
	if after.LastSeenHash != before.LastSeenHash {
		t.Fatalf("lastSeenHash moved to %q", after.LastSeenHash)
	}
	if after.Baseline == nil || !bytes.Equal(after.Baseline.Data, before.Baseline.Data) {
		t.Fatalf("baseline moved: %+v", after.Baseline)
	}
	if after.UIWorkPending != before.UIWorkPending || after.Event != nil {
		t.Fatalf("state moved: %+v", after)
	}
}

// TestClientEvent pins the panel payload built from one fired write.
func TestClientEvent(t *testing.T) {
	whole := boardHTML("Board", fourItems...)
	twoItemsGone := boardHTML("", fourItems[:2]...)

	h := newHarness(t)
	h.clock = 4242
	event := h.observe(twoItemsGone, whole, dataguard.UIGestured)
	if event == nil {
		t.Fatal("no event raised")
	}
	if event.ID == "" {
		t.Fatal("event has no id")
	}
	if event.FirstDetectedAt != 4242 || event.LastWriteAt != 4242 {
		t.Fatalf("times = %d/%d, want 4242", event.FirstDetectedAt, event.LastWriteAt)
	}
	if event.Status != "pending" {
		t.Fatalf("status = %q", event.Status)
	}
	if event.LossSummary.Provenance != dataguard.UIGestured || event.LossSummary.RemovedItems != 2 {
		t.Fatalf("loss summary = %+v", event.LossSummary)
	}
	if !event.Restorable {
		t.Fatal("event is not restorable")
	}
	if !event.CanRevert {
		t.Fatal("event cannot revert")
	}
	if event.FieldCount != len(event.Preview) || event.FieldCount == 0 {
		t.Fatalf("fieldCount = %d, preview = %+v", event.FieldCount, event.Preview)
	}

	var title, items *dataguard.PreviewRow
	for i := range event.Preview {
		switch event.Preview[i].Key {
		case "title":
			title = &event.Preview[i]
		case "items":
			items = &event.Preview[i]
		}
	}
	if title == nil || title.Now != "\u2014 empty \u2014" || title.Yours != "Board" {
		t.Fatalf("title row = %+v", title)
	}
	if items == nil || items.Now != "2 items" || items.Yours != "4 items" {
		t.Fatalf("items row = %+v", items)
	}
	if event.DroppedAdditions != 0 {
		t.Fatalf("droppedAdditions = %d", event.DroppedAdditions)
	}

	st := h.state()
	if st.Event == nil || st.Event.ID != event.ID {
		t.Fatalf("stored event id does not match the payload: %+v", st.Event)
	}
	if st.Event.Status != "pending" || st.Event.LossSummary.RemovedItems != 2 {
		t.Fatalf("stored event = %+v", st.Event)
	}
}

// TestPreview pins the row text: the empty placeholder, the item count, and the
// 80-code-unit cut. The cut is measured in UTF-16 code units, which is what the
// panel's String#length counts, so a value with an emoji in it keeps its bytes
// out of the arithmetic.
func TestPreview(t *testing.T) {
	emoji := "\U0001F600" + strings.Repeat("a", 78) + "b"
	cases := []struct {
		name      string
		prevSub   string
		nextSub   string
		wantNow   string
		wantYours string
	}{
		{
			name:      "emoji is one code unit pair, not four bytes",
			prevSub:   "plain",
			nextSub:   emoji,
			wantYours: "plain",
			wantNow:   "\U0001F600" + strings.Repeat("a", 77) + "\u2026",
		},
		{
			name:      "emptied subtitle reads as empty",
			prevSub:   "some note",
			nextSub:   "",
			wantYours: "some note",
			wantNow:   "\u2014 empty \u2014",
		},
		{
			name:      "short value is not cut",
			prevSub:   "a",
			nextSub:   strings.Repeat("z", 80),
			wantYours: "a",
			wantNow:   strings.Repeat("z", 80),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prev := boardWith("Board", c.prevSub, fourItems...)
			next := boardWith("Board", c.nextSub, fourItems[:2]...)

			h := newHarness(t)
			event := h.observe(next, prev, dataguard.UIGestured)
			if event == nil {
				t.Fatal("no event raised")
			}
			var items *dataguard.PreviewRow
			for i := range event.Preview {
				if event.Preview[i].Key == "subtitle" {
					items = &event.Preview[i]
				}
			}
			if items == nil {
				t.Fatalf("no subtitle row in %+v", event.Preview)
			}
			if items.Now != c.wantNow {
				t.Fatalf("now = %q, want %q", items.Now, c.wantNow)
			}
			if items.Yours != c.wantYours {
				t.Fatalf("yours = %q, want %q", items.Yours, c.wantYours)
			}
		})
	}
}

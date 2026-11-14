package dataguard_test

import (
	"testing"

	"github.com/panphora/htmlclay/internal/dataguard"
)

// observeWithProof observes a write that says how it knew the version it edited,
// so a test can separate a page-driven write from a proven program read.
func (h *harness) observeWithProof(newHTML []byte, prov dataguard.Provenance, provenReader bool) *dataguard.ClientEvent {
	h.t.Helper()
	event, err := h.guard.Observe(dataguard.Observation{
		Key:          h.key,
		AbsPath:      h.docPath,
		NewHTML:      newHTML,
		Prov:         prov,
		IfMatched:    provenReader,
		ProvenReader: provenReader,
		History:      h.history,
		Notify: func(kind string, event *dataguard.ClientEvent, id string) {
			h.notes = append(h.notes, notification{kind: kind, id: id, event: event})
		},
	})
	if err != nil {
		h.t.Fatalf("Observe: %v", err)
	}
	return event
}

// backupForTest stores a version the way the watcher does before the guard judges
// a write: the incoming bytes are already the newest version when Observe runs.
func (h *harness) backupForTest(body []byte) {
	h.t.Helper()
	if _, err := h.v.Backup(h.key, h.docPath, body); err != nil {
		h.t.Fatalf("backup: %v", err)
	}
}

// pinnedBodies returns the bytes of every version the guard pinned.
func (h *harness) pinnedBodies() [][]byte {
	h.t.Helper()
	names, err := h.v.Pinned(h.key, h.docPath)
	if err != nil {
		h.t.Fatalf("pinned: %v", err)
	}
	var bodies [][]byte
	for _, name := range names {
		body, err := h.v.Read(h.key, h.docPath, name)
		if err != nil {
			h.t.Fatalf("read pinned version %s: %v", name, err)
		}
		bodies = append(bodies, body)
	}
	return bodies
}

func answeredTypedHTML() []byte {
	return typedQuestionsHTML(
		typedQuestion{key: "q1", answer: "one"},
		typedQuestion{key: "q2", answer: "two"},
		typedQuestion{key: "q3", answer: "three"},
		typedQuestion{key: "q4"},
	)
}

// A tool rewrote the page long after the person typed here, dropping one answer it
// did not carry over. The guard stays silent because the loss is old, but silence
// must not let pruning delete the only copy: the replaced version is pinned.
func TestSilentOutsideLossPinsTheReplacedVersion(t *testing.T) {
	unanswered := typedQuestionsHTML(
		typedQuestion{key: "q1"}, typedQuestion{key: "q2"}, typedQuestion{key: "q3"}, typedQuestion{key: "q4"},
	)
	answered := answeredTypedHTML()
	rewritten := typedQuestionsHTML(
		typedQuestion{key: "q1", answer: "one"},
		typedQuestion{key: "q2", answer: "two"},
		typedQuestion{key: "q3"},
		typedQuestion{key: "q4"},
	)

	h := newHarness(t)
	h.backupForTest(unanswered)
	if event := h.observe(unanswered, nil, dataguard.External); event != nil {
		t.Fatalf("cold seed fired: %+v", event)
	}
	if event := h.observe(answered, unanswered, dataguard.UIGestured); event != nil {
		t.Fatalf("the person's save fired: %+v", event)
	}
	h.backupForTest(answered)
	h.clock = 1000 + 2*60*60*1000
	h.backupForTest(rewritten)

	if event := h.observe(rewritten, nil, dataguard.External); event != nil {
		t.Fatalf("an old loss fired: %+v", event)
	}

	bodies := h.pinnedBodies()
	if len(bodies) == 0 {
		t.Fatal("the replaced version was not pinned")
	}
	found := false
	for _, body := range bodies {
		if string(body) == string(answered) {
			found = true
		}
	}
	if !found {
		t.Fatalf("pinned versions do not hold the replaced bytes: %q", bodies)
	}

	// A pin means the copy survives pruning: the guard kept it until the user
	// deletes it.
	h.v.PruneAll()
	if again := h.pinnedBodies(); len(again) != len(bodies) {
		t.Fatalf("pruning dropped a pinned version: %d left of %d", len(again), len(bodies))
	}
}

// An outside write that removes nothing needs no pin: there is no version to keep.
func TestOutsideWriteThatRemovesNothingPinsNothing(t *testing.T) {
	before := typedQuestionsHTML(
		typedQuestion{key: "q1", answer: "one"},
		typedQuestion{key: "q2", answer: "two"},
		typedQuestion{key: "q3"},
	)
	after := typedQuestionsHTML(
		typedQuestion{key: "q1", answer: "one"},
		typedQuestion{key: "q2", answer: "two"},
		typedQuestion{key: "q3"},
		typedQuestion{key: "q4", answer: "added by the tool"},
	)

	h := newHarness(t)
	h.backupForTest(before)
	if event := h.observe(before, nil, dataguard.External); event != nil {
		t.Fatalf("cold seed fired: %+v", event)
	}
	h.clock = 1000 + 2*60*60*1000
	h.backupForTest(after)
	if event := h.observe(after, nil, dataguard.External); event != nil {
		t.Fatalf("an addition fired: %+v", event)
	}

	if names, err := h.v.Pinned(h.key, h.docPath); err != nil {
		t.Fatal(err)
	} else if len(names) != 0 {
		t.Fatalf("an addition pinned %v", names)
	}
}

// A program that proved it read the version it wrote is making an intended change:
// the loss is the program's own, normal retention applies, and nothing is pinned.
func TestProvenReaderRemovingDataPinsNothing(t *testing.T) {
	before := answeredTypedHTML()
	after := typedQuestionsHTML(
		typedQuestion{key: "q1", answer: "one"},
		typedQuestion{key: "q2", answer: "two"},
		typedQuestion{key: "q3"},
		typedQuestion{key: "q4"},
	)

	h := newHarness(t)
	h.backupForTest(before)
	if event := h.observe(before, nil, dataguard.External); event != nil {
		t.Fatalf("cold seed fired: %+v", event)
	}
	h.backupForTest(after)
	if event := h.observeWithProof(after, dataguard.External, true); event != nil {
		t.Fatalf("a proven reader fired: %+v", event)
	}

	if names, err := h.v.Pinned(h.key, h.docPath); err != nil {
		t.Fatal(err)
	} else if len(names) != 0 {
		t.Fatalf("a proven reader's intended change pinned %v", names)
	}
}

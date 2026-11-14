package dataguard_test

import (
	"bytes"
	"path/filepath"
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

// keptCopyThenNotice drives one document to the state a user reaches after a kept
// copy and a pending notice: a silent old loss pins a version, and a later recent
// loss raises a notice on top of it. It returns the harness, the notice, the bytes
// the outside writer left on disk, and the history folder name.
func keptCopyThenNotice(t *testing.T) (*harness, *dataguard.ClientEvent, []byte, string) {
	t.Helper()
	untyped := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", ""}, putBackItem{"6", "six", "six note"})
	firstTyped := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", "my note"}, putBackItem{"6", "six", "six note"})
	secondTyped := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", "my second note"}, putBackItem{"6", "six", "six note"})

	h := newHarness(t)
	h.backupForTest(untyped)
	if event := h.observe(untyped, nil, dataguard.External); event != nil {
		t.Fatalf("cold seed fired: %+v", event)
	}
	h.clock = 1000
	h.backupForTest(firstTyped)
	if event := h.observe(firstTyped, untyped, dataguard.UIGestured); event != nil {
		t.Fatalf("the person's save fired: %+v", event)
	}

	// An old outside write takes the typed note away. The guard stays silent, so
	// the version it replaced is kept.
	h.clock = 1000 + 2*60*60*1000
	h.backupForTest(untyped)
	if event := h.observe(untyped, nil, dataguard.External); event != nil {
		t.Fatalf("an old loss fired: %+v", event)
	}
	if len(h.pinnedBodies()) == 0 {
		t.Fatal("the silent loss kept no copy")
	}

	// The person types again, and a recent outside write takes that away too.
	h.clock += 5 * 60 * 1000
	h.backupForTest(secondTyped)
	if event := h.observe(secondTyped, untyped, dataguard.UIGestured); event != nil {
		t.Fatalf("the person's second save fired: %+v", event)
	}

	h.clock += 5 * 60 * 1000
	h.backupForTest(untyped)
	fired := h.observe(untyped, nil, dataguard.External)
	if fired == nil {
		t.Fatal("a recent outside loss raised no notice")
	}

	folder, err := h.v.ExistingFolder(h.key, h.docPath)
	if err != nil {
		t.Fatalf("existing folder: %v", err)
	}
	return h, fired, untyped, filepath.Base(folder)
}

// Deleting the copies the guard kept must not disarm an open notice: answering it
// still needs the guard's own recover data and the versions still on disk, neither
// of which is a kept copy.
func TestDeletingAKeptCopyLeavesAnOpenNoticeResolvable(t *testing.T) {
	h, fired, outside, folder := keptCopyThenNotice(t)
	if !fired.CanPutBack {
		t.Fatalf("a typed note on an item that is still there offered no put-back: %+v", fired)
	}
	keptNames, err := h.v.Pinned(h.key, h.docPath)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := h.v.DeleteKeptNames(folder, keptNames); err != nil || n == 0 {
		t.Fatalf("DeleteKeptNames = %d, %v; want the kept copies gone", n, err)
	}
	if names, err := h.v.Pinned(h.key, h.docPath); err != nil {
		t.Fatal(err)
	} else if len(names) != 0 {
		t.Fatalf("pins after deleting the kept copies: %v", names)
	}

	var written []byte
	res, err := h.guard.Resolve(h.key, h.docPath, fired.ID, "put-back", outside, h.writeBack(&written))
	if err != nil {
		t.Fatalf("put-back after deleting the kept copies: %v", err)
	}
	if res.Status != "put-back" {
		t.Fatalf("status = %q, want put-back", res.Status)
	}
	want := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", "my second note"}, putBackItem{"6", "six", "six note"})
	if got := islandJSON(t, written); !bytes.Equal(got, islandJSON(t, want)) {
		t.Fatalf("island after put-back = %s, want %s", got, islandJSON(t, want))
	}
}

// Undo the change still has its recover copy after the kept copies are deleted.
func TestDeletingAKeptCopyLeavesRevertAvailable(t *testing.T) {
	h, fired, outside, folder := keptCopyThenNotice(t)
	if !fired.Restorable {
		t.Fatalf("a page with rows to fill offered no restore: %+v", fired)
	}
	keptNames, err := h.v.Pinned(h.key, h.docPath)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := h.v.DeleteKeptNames(folder, keptNames); err != nil || n == 0 {
		t.Fatalf("DeleteKeptNames = %d, %v; want the kept copies gone", n, err)
	}

	var written []byte
	res, err := h.guard.Resolve(h.key, h.docPath, fired.ID, "revert", outside, h.writeBack(&written))
	if err != nil {
		t.Fatalf("revert after deleting the kept copies: %v", err)
	}
	if res.Status != "reverted" {
		t.Fatalf("status = %q, want reverted", res.Status)
	}
	want := h.recoverCopy()
	if !bytes.Equal(islandJSON(t, written), islandJSON(t, want)) {
		t.Fatalf("island after revert = %s, want the recover copy %s", islandJSON(t, written), islandJSON(t, want))
	}
}

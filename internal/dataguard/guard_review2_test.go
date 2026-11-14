package dataguard

import "testing"

// H1: in a list without ids, an item added by a gesture and then removed by an
// outside write is a loss even when another item's boolean is carried over.
func TestJudgeOutsideIdlessAddedItemRemoved(t *testing.T) {
	before := provenReaderIsland(t, `{"items":[{"name":"alpha","done":false},{"name":"beta","done":false}]}`)
	added := provenReaderIsland(t, `{"items":[{"name":"alpha","done":false},{"name":"beta","done":false},{"name":"gamma","done":false}]}`)
	typed := UpdateTyped(before, added, nil, 1_000)
	if len(typed) != 2 {
		t.Fatalf("typed = %+v, want the added item's name and done", typed)
	}
	toggled := provenReaderIsland(t, `{"items":[{"name":"alpha","done":true},{"name":"beta","done":false},{"name":"gamma","done":false}]}`)
	typed = UpdateTyped(added, toggled, typed, 2_000)
	outside := provenReaderIsland(t, `{"items":[{"name":"alpha","done":true},{"name":"beta","done":false}]}`)
	verdict := JudgeOutside(toggled, outside, ClassifyDestruction(toggled, outside), typed, 62_000)
	if !verdict.Fire || verdict.Reason != "recent" {
		t.Fatalf("verdict = %+v, want a recent loss", verdict)
	}
	found := false
	for _, e := range verdict.Gone {
		if e.Path == "items[*].name" && e.Preview == "gamma" {
			found = true
		}
	}
	if !found {
		t.Fatalf("gone = %+v, want the removed item's name", verdict.Gone)
	}
}

// H2: a tool that writes the old text back over a value the person just changed
// is losing their work, while an intended new value is not.
func TestJudgeOutsidePuttingTheOldValueBack(t *testing.T) {
	before := provenReaderIsland(t, `{"profile":{"name":"Bob"}}`)
	typedDoc := provenReaderIsland(t, `{"profile":{"name":"Alice"}}`)
	typed := UpdateTyped(before, typedDoc, nil, 1_000)
	if len(typed) != 1 || typed[0].Prev != stableDigest("Bob") {
		t.Fatalf("typed = %+v, want Alice with Bob recorded as prev", typed)
	}
	reverted := provenReaderIsland(t, `{"profile":{"name":"Bob"}}`)
	verdict := JudgeOutside(typedDoc, reverted, ClassifyDestruction(typedDoc, reverted), typed, 61_000)
	if !verdict.Fire || verdict.Reason != "recent" {
		t.Fatalf("reverted verdict = %+v, want a recent loss", verdict)
	}
	changed := provenReaderIsland(t, `{"profile":{"name":"Carol"}}`)
	if verdict := JudgeOutside(typedDoc, changed, ClassifyDestruction(typedDoc, changed), typed, 61_000); verdict.Fire {
		t.Fatalf("an intended edit fired: %+v", verdict)
	}
}

// M2: an outside write that only adds ids changes no value, so it cannot lose a
// typed one, however old the typing is.
func TestJudgeOutsideAddingIdsIsSilent(t *testing.T) {
	before := provenReaderIsland(t, `{"items":[{"note":""},{"note":""},{"note":""}]}`)
	typedDoc := provenReaderIsland(t, `{"items":[{"note":"one"},{"note":"two"},{"note":"three"}]}`)
	typed := UpdateTyped(before, typedDoc, nil, 1_000)
	if len(typed) != 3 {
		t.Fatalf("typed = %+v, want three notes", typed)
	}
	identified := provenReaderIsland(t, `{"items":[{"id":"1","note":"one"},{"id":"2","note":"two"},{"id":"3","note":"three"}]}`)
	D := ClassifyDestruction(typedDoc, identified)
	verdict := JudgeOutside(typedDoc, identified, D, typed, 1_000+3*24*60*60*1000)
	if verdict.Fire {
		t.Fatalf("adding ids fired: %+v (D = %+v)", verdict, *D)
	}
}

// L8: an unreadable write on a page that never held data has nothing to lose.
func TestJudgeOutsideUnreadableNeedsData(t *testing.T) {
	base := provenReaderIsland(t, `{"title":"","note":""}`)
	if verdict := JudgeOutside(base, nil, &Destruction{}, nil, 1_000); verdict.Fire {
		t.Fatalf("a page with no data fired: %+v", verdict)
	}
	if verdict := JudgeOutside(provenReaderIsland(t, `{"title":"kept"}`), nil, &Destruction{}, nil, 1_000); !verdict.Fire || verdict.Reason != "unreadable" {
		t.Fatalf("a page with data did not report unreadable: %+v", verdict)
	}
}

// L3: only a strong tag naming exactly one version is an exact version.
func TestNamesExactVersionRejectsWeakAndMultiple(t *testing.T) {
	for _, header := range []string{`W/"abc"`, `"a", "b"`, `*`, ``} {
		if NamesExactVersion(header) {
			t.Fatalf("header %q counted as exact", header)
		}
	}
	if !NamesExactVersion(`"abc"`) {
		t.Fatal(`a strong single tag did not count as exact`)
	}
}

// R5: an answer re-keyed by an outside write that only added ids is still there,
// so an old answer set plus one unanswered question is not a loss, even though the
// blank item makes the write count as destructive.
func TestJudgeOutsideAnswersKeptWhenIdsAdded(t *testing.T) {
	before := provenReaderIsland(t, `{"questions":[{"answer":""},{"answer":""},{"answer":""},{"answer":""}]}`)
	answered := provenReaderIsland(t, `{"questions":[{"answer":"a1"},{"answer":"a2"},{"answer":"a3"},{"answer":""}]}`)
	typed := UpdateTyped(before, answered, nil, 1_000)
	if len(typed) != 3 {
		t.Fatalf("typed = %+v, want the three answers", typed)
	}
	identified := provenReaderIsland(t, `{"questions":[{"id":"1","answer":"a1"},{"id":"2","answer":"a2"},{"id":"3","answer":"a3"},{"id":"4","answer":""}]}`)
	D := ClassifyDestruction(answered, identified)
	if D == nil || (!D.AnyDestruction && D.ModifiedAtoms == 0) {
		t.Fatalf("D = %+v, want the write to look destructive so the answers are judged", D)
	}
	verdict := JudgeOutside(answered, identified, D, typed, 1_000+3*24*60*60*1000)
	if verdict.Fire {
		t.Fatalf("adding ids fired: %+v (D = %+v)", verdict, *D)
	}
	kept := DropGone(typed, identified)
	if len(kept) != 3 {
		t.Fatalf("DropGone kept %+v, want the three answers", kept)
	}
	for _, e := range kept {
		if e.Preview != "a1" && e.Preview != "a2" && e.Preview != "a3" {
			t.Fatalf("DropGone kept %+v, want only the answers", kept)
		}
	}
}

// R5: a boolean re-keyed alongside its item gets no second chance, because a
// shared boolean is not evidence the person's own value survived.
func TestTypedPresentNoSecondChanceForBooleans(t *testing.T) {
	typed := []TypedValue{{Path: "items[*].done", Digest: stableDigest(true), Preview: "true", At: 1_000}}
	identified := provenReaderIsland(t, `{"items":[{"id":"1","done":true}]}`)
	if present := typedPresent(typed, identified); present[0] {
		t.Fatalf("a re-keyed boolean counted as present: %+v", present)
	}
}

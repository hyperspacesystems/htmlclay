package dataguard_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dataguard"
)

const putBackRulesTag = `<script type="application/json" data-rules-name="api" data-rules-version="1">{title:".title", tags:".tag[]", items:[".item",{id:".id",title:".title",note:".note"}]}</script>`

type putBackItem struct{ id, title, note string }

func putBackHTML(title string, tags []string, items ...putBackItem) []byte {
	var b strings.Builder
	b.WriteString(`<html><body><h1 class="title">` + title + `</h1>`)
	for _, tag := range tags {
		b.WriteString(`<span class="tag">` + tag + `</span>`)
	}
	b.WriteString(`<ul>`)
	for _, item := range items {
		b.WriteString(`<li class="item"><span class="id">` + item.id + `</span><span class="title">` + item.title + `</span>`)
		b.WriteString(`<span class="note">` + item.note + `</span></li>`)
	}
	b.WriteString(`</ul>` + putBackRulesTag + `</body></html>`)
	return []byte(b.String())
}

// typedNoteThenOutside is the shared setup: a person types a note on the item id
// 5, and an outside write takes it away while changing the page elsewhere.
func typedNoteThenOutside(t *testing.T, rewritten []byte) (*harness, *dataguard.ClientEvent, []byte) {
	t.Helper()
	start := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", ""}, putBackItem{"6", "six", "six note"})
	typed := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", "my note"}, putBackItem{"6", "six", "six note"})

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
		t.Fatal("an outside write that removed the typed note did not fire")
	}
	if fired.Reason != "recent" {
		t.Fatalf("reason = %q, want recent", fired.Reason)
	}
	return h, fired, typed
}

// Put it back writes only the typed values back, so the writer's other changes
// stay exactly as it left them.
func TestPutBackKeepsTheWritersOtherChanges(t *testing.T) {
	rewritten := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", ""}, putBackItem{"6", "SIX", "six note"})
	h, fired, _ := typedNoteThenOutside(t, rewritten)
	if !fired.CanPutBack {
		t.Fatalf("a typed note on an item that is still there offered no put-back: %+v", fired)
	}
	if len(fired.Typed) != 1 || fired.Typed[0].Yours != "my note" {
		t.Fatalf("client typed rows = %+v", fired.Typed)
	}

	var written []byte
	res, err := h.guard.Resolve(h.key, h.docPath, fired.ID, "put-back", rewritten, h.writeBack(&written))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Status != "put-back" || res.Choice != "put-back" || res.ID != fired.ID {
		t.Fatalf("result = %+v", res)
	}
	want := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", "my note"}, putBackItem{"6", "SIX", "six note"})
	if got := islandJSON(t, written); !bytes.Equal(got, islandJSON(t, want)) {
		t.Fatalf("island after put-back = %s, want %s", got, islandJSON(t, want))
	}

	st := h.state()
	if st.Event != nil || st.Status != "put-back" {
		t.Fatalf("state = %+v, want a put-back event", st)
	}
	if st.Baseline == nil || !bytes.Equal(st.Baseline.Data, islandJSON(t, want)) {
		t.Fatalf("baseline = %+v, want the put-back island", st.Baseline)
	}
	if len(st.Typed) != 1 || st.Typed[0].Preview != "my note" {
		t.Fatalf("the put-back dropped the typing record: %+v", st.Typed)
	}
}

// A typed entry on a scalar list is appended back where the writer left the rest
// of the list.
func TestPutBackAppendsToAScalarList(t *testing.T) {
	start := putBackHTML("Board", []string{"red", "blue"})
	tagged := putBackHTML("Board", []string{"red", "blue", "green"})
	rewritten := putBackHTML("Board", []string{"red", "blue"})

	h := newHarness(t)
	h.clock = 500
	if event := h.observe(start, nil, dataguard.External); event != nil {
		t.Fatalf("cold seed fired: %+v", event)
	}
	h.clock = 1000
	if event := h.observe(tagged, start, dataguard.UIGestured); event != nil {
		t.Fatalf("the person's save fired: %+v", event)
	}
	h.clock = 1500
	fired := h.observe(rewritten, nil, dataguard.External)
	if fired == nil {
		t.Fatal("an outside write that removed the typed tag did not fire")
	}
	if !fired.CanPutBack {
		t.Fatalf("a typed tag on a list that is still there offered no put-back: %+v", fired)
	}

	var written []byte
	res, err := h.guard.Resolve(h.key, h.docPath, fired.ID, "put-back", rewritten, h.writeBack(&written))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Status != "put-back" {
		t.Fatalf("status = %q, want put-back", res.Status)
	}
	if got, want := islandJSON(t, written), islandJSON(t, tagged); !bytes.Equal(got, want) {
		t.Fatalf("island after put-back = %s, want %s", got, want)
	}
}

// A typed value whose whole item is gone cannot be put back, so the card offers
// nothing and the choice answers the refusal restore answers.
func TestPutBackRefusesWhenTheItemIsGone(t *testing.T) {
	rewritten := putBackHTML("Board", []string{"red"}, putBackItem{"6", "SIX", "six note"})
	h, fired, _ := typedNoteThenOutside(t, rewritten)
	if fired.CanPutBack {
		t.Fatalf("a typed note on a deleted item offered a put-back: %+v", fired)
	}

	res, err := h.guard.Resolve(h.key, h.docPath, fired.ID, "put-back", rewritten, func([]byte) error {
		t.Fatal("a put-back that cannot place a value wrote to the page")
		return nil
	})
	if !errors.Is(err, dataguard.ErrNotRestorable) {
		t.Fatalf("Resolve = %+v, %v; want ErrNotRestorable", res, err)
	}
	st := h.state()
	if st.Event == nil || st.Event.ID != fired.ID || st.Status != "pending" {
		t.Fatalf("the refused put-back changed the event: %+v", st)
	}
}

// Undo the change still writes the whole old data back, the writer's own edits
// included, which is the difference put-back makes.
func TestRestoreStillUndoesTheWritersChanges(t *testing.T) {
	rewritten := putBackHTML("Board", []string{"red"}, putBackItem{"5", "five", ""}, putBackItem{"6", "SIX", "six note"})
	h, fired, typed := typedNoteThenOutside(t, rewritten)
	if !fired.Restorable {
		t.Fatalf("a page with rows to fill offered no restore: %+v", fired)
	}

	var written []byte
	res, err := h.guard.Resolve(h.key, h.docPath, fired.ID, "restore", rewritten, h.writeBack(&written))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Status != "restored" {
		t.Fatalf("status = %q, want restored", res.Status)
	}
	if got, want := islandJSON(t, written), islandJSON(t, typed); !bytes.Equal(got, want) {
		t.Fatalf("island after restore = %s, want %s", got, want)
	}
}

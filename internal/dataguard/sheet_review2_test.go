package dataguard_test

import (
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dataguard"
)

const reviewSheetSource = `<!doctype html><html><head><script type="application/json" data-rules-name="api" data-rules-version="2">{"sheets":"[items=sheets]@codec(sheet)","note":"p@textContent"}</script></head><body><div items="sheets"><section item data-id="s1"><h2 bind="name">Budget</h2><table grid><colgroup><col data-id="a" data-pos="G"><col data-id="b" data-pos="V"><col data-id="c" data-pos="k"></colgroup><tbody><tr data-id="r1" data-pos="V"><td>one</td><td>two</td><td>three</td></tr></tbody></table></section></div><p>Saved note</p></body></html>`

func reviewSheetHTML(cell string) []byte {
	return []byte(strings.Replace(reviewSheetSource, ">one<", ">"+cell+"<", 1))
}

// A sheet keeps the old rule for outside writes and records no typing at all, so
// Put it back is never offered for a sheet cell.
func TestSheetOutsideWriteUsesShouldFireAndRecordsNoTyping(t *testing.T) {
	start := reviewSheetHTML("one")
	edited := reviewSheetHTML("mine")
	cleared := reviewSheetHTML("")

	h := newHarness(t)
	h.clock = 500
	h.backupForTest(start)
	if event := h.observe(start, nil, dataguard.External); event != nil {
		t.Fatalf("cold seed fired: %+v", event)
	}
	h.clock = 1000
	if event := h.observe(edited, start, dataguard.UIGestured); event != nil {
		t.Fatalf("the person's sheet edit fired: %+v", event)
	}
	if st := h.state(); len(st.Typed) != 0 {
		t.Fatalf("a sheet recorded typed values: %+v", st.Typed)
	}

	base := islandOf(t, edited)
	inc := islandOf(t, cleared)
	want := dataguard.ShouldFire(dataguard.External, true, base, inc, dataguard.ClassifyDestruction(base, inc), false)

	h.clock = 2000
	h.backupForTest(cleared)
	event := h.observe(cleared, nil, dataguard.External)
	if (event != nil) != want {
		t.Fatalf("event = %+v, want ShouldFire to decide %v", event, want)
	}
	if st := h.state(); len(st.Typed) != 0 {
		t.Fatalf("a sheet outside write recorded typed values: %+v", st.Typed)
	}
}

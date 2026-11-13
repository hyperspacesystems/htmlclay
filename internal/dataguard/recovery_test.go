package dataguard_test

import (
	"bytes"
	"html"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/dataguard"
)

func TestLegacyRecoveryComparison(t *testing.T) {
	article := `<article data-blockeditor="" data-rev="saved"><p>Recovered paragraph</p></article>`
	tag := `<script type="application/json" merge="blockeditor" clay="live">{"v":1,"y":{"seed":"saved"}}</script>`
	doc := func(body, rule string) []byte {
		return []byte(`<html><head><script data-rules-name="api" data-rules-version="1">` + rule + `</script></head><body>` + body + `</body></html>`)
	}
	for _, operation := range []string{"save", "open"} {
		for _, scenario := range []string{"recovered", "marked", "example", "partial"} {
			t.Run(operation+"/"+scenario, func(t *testing.T) {
				h := newHarness(t)
				data := dataapi.NewObject()
				data.Set("content", tag+article)
				current := doc(article, `{"content":"body@innerHTML"}`)
				if scenario == "example" {
					data = dataapi.NewObject()
					data.Set("note", tag+article)
					current = doc(`<pre>`+html.EscapeString(article)+`</pre>`, `{"note":"pre@textContent"}`)
				}
				if scenario == "partial" {
					current = bytes.Replace(current, []byte("Recovered paragraph"), []byte("Still missing"), 1)
				}
				recoverable := dataapi.NewObject()
				recoverable.Set("api", data)
				if scenario == "marked" {
					recoverable.Set(dataguard.FormKey, "stored")
				}
				pinned, err := dataapi.Marshal(recoverable)
				if err != nil {
					t.Fatal(err)
				}
				st := &dataguard.State{Version: 1, Baseline: &dataguard.Baseline{Data: pinned}, Event: &dataguard.Event{ID: "old-event", RecoverableData: pinned, Status: "pending"}, Status: "pending"}
				if err := h.store.Write(h.key, h.docPath, st); err != nil {
					t.Fatal(err)
				}
				pinned = append([]byte(nil), h.state().Event.RecoverableData...)
				if operation == "save" {
					h.observe(current, doc("clobbered", `{"content":"body@innerHTML"}`), dataguard.UIBackground)
				} else {
					event, err := h.guard.CurrentEvent(h.key, h.docPath, func() ([]byte, error) { return current, nil })
					if err != nil || (event != nil) != (scenario != "recovered") {
						t.Fatalf("CurrentEvent = %+v, %v", event, err)
					}
				}
				after := h.state()
				if (after.Event != nil) != (scenario != "recovered") {
					t.Fatalf("event = %+v", after.Event)
				}
				if after.Event != nil && !bytes.Equal(after.Event.RecoverableData, pinned) {
					t.Fatal("pinned recovery changed")
				}
				if after.Event == nil && (after.Status != "restored" || strings.Contains(string(after.Baseline.Data), "merge=")) {
					t.Fatalf("restored state = %+v", after)
				}
			})
		}
	}
}

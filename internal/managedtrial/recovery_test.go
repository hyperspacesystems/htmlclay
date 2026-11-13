//go:build managedtrial

package managedtrial

import (
	"html"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/dataguard"
)

func TestManagedLegacyRecoveryComparison(t *testing.T) {
	article := `<article data-blockeditor="" data-rev="saved"><p>Recovered paragraph</p></article>`
	tag := `<script type="application/json" merge="blockeditor" clay="live">{"v":1,"y":{"seed":"saved"}}</script>`
	doc := func(body, rule string) string {
		return `<html><head><script data-rules-name="api" data-rules-version="1">` + rule + `</script></head><body>` + body + `</body></html>`
	}
	for _, operation := range []string{"save", "open"} {
		for _, scenario := range []string{"recovered", "marked", "example", "partial"} {
			t.Run(operation+"/"+scenario, func(t *testing.T) {
				root := t.TempDir()
				data := dataapi.NewObject()
				data.Set("content", tag+article)
				current := doc(article, `{"content":"body@innerHTML"}`)
				if scenario == "example" {
					data = dataapi.NewObject()
					data.Set("note", tag+article)
					current = doc(`<pre>`+html.EscapeString(article)+`</pre>`, `{"note":"pre@textContent"}`)
				}
				if scenario == "partial" {
					current = strings.Replace(current, "Recovered paragraph", "Still missing", 1)
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
				state := emptyGuard()
				state.Baseline = newGuardBaseline(recoverable, 0)
				state.Event = &guardRecordEvent{ID: "old-event", RecoverableData: recoverable, Status: "pending"}
				state.Status = "pending"
				writeGuard(root, "doc.html", state)
				if operation == "save" {
					previous := doc("clobbered", `{"content":"body@innerHTML"}`)
					runDataLossGuard(runGuardArgs{BaseDir: root, Name: "doc.html", NewHTML: current, PrevContent: &previous, Prov: dataguard.UIBackground})
				} else if event := getGuardEvent(root, "doc.html", &current, nil); (event != nil) != (scenario != "recovered") {
					t.Fatalf("getGuardEvent = %+v", event)
				}
				after, err := readGuard(root, "doc.html")
				if err != nil || after == nil {
					t.Fatalf("readGuard = %+v, %v", after, err)
				}
				if (after.Event != nil) != (scenario != "recovered") {
					t.Fatalf("event = %+v", after.Event)
				}
				if after.Event != nil {
					got, err := dataapi.Marshal(after.Event.RecoverableData)
					if err != nil || string(got) != string(pinned) {
						t.Fatal("pinned recovery changed")
					}
				} else if after.Status != "restored" || !dataguard.IslandsEqual(after.Baseline.Data, mustExtractRecovery(t, current)) {
					t.Fatalf("restored state = %+v", after)
				}
			})
		}
	}
}

func mustExtractRecovery(t *testing.T, html string) dataapi.Value {
	t.Helper()
	island, err := extractIsland(html)
	if err != nil {
		t.Fatal(err)
	}
	return island
}

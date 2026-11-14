package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/dataguard"
	"github.com/panphora/htmlclay/internal/specwire"
)

const sheetGuardSource = `<!doctype html><html><head><script type="application/json" data-rules-name="api" data-rules-version="2">{"sheets":"[items=sheets]@codec(sheet)","note":"p@textContent"}</script></head><body><div items="sheets"><section item data-id="s1"><h2 bind="name">Budget</h2><table grid><colgroup><col data-id="a" data-pos="G"><col data-id="b" data-pos="V"><col data-id="c" data-pos="k"></colgroup><tbody><tr data-id="r1" data-pos="V"><td>one</td><td>two</td><td>three</td></tr></tbody></table></section></div><p>Saved note</p></body></html>`

func TestSheetGuardAPI(t *testing.T) {
	for _, c := range []struct {
		name, value, header string
		fire                bool
	}{
		{"matched overwrite", "updated", "exact", false},
		{"unmatched overwrite", "updated", "", true},
		{"wildcard overwrite", "updated", "*", true},
		{"matched clear by a program", "", "exact", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv, _, _ := setupHandlerTest(t)
			file, path := registerFixture(t, srv, "sheet.htmlclay", sheetGuardSource)
			if w := serve(t, srv, "/fixtures/sheet.htmlclay", nil); w.Code != 200 {
				t.Fatalf("serve = %d", w.Code)
			}
			key := guardKey(t, file)
			original := []byte(sheetGuardSource)
			before := []byte(strings.Replace(sheetGuardSource, ">one<", ">two<", 1))
			if _, err := srv.guard.Observe(dataguard.Observation{Key: key, AbsPath: path, NewHTML: original, PrevHTML: before, Prov: dataguard.UIGestured}); err != nil {
				t.Fatal(err)
			}
			state, err := srv.guardStore.Read(key, path)
			if err != nil {
				t.Fatal(err)
			}
			if !state.UIWorkPending {
				t.Fatal("human work was not seeded")
			}
			header := c.header
			if header == "exact" {
				header = specwire.Etag(original)
			}
			headers := map[string]string{}
			if header != "" {
				headers["If-Match"] = header
			}
			value, _ := json.Marshal(c.value)
			body := fmt.Sprintf(`{"patch":[{"range":"B1","values":[[%s]]}]}`, value)
			w := postDataWrite(t, srv, "/_/api/fixtures/sheet.htmlclay", body, headers)
			if w.Code != 200 {
				t.Fatalf("patch = %d: %s", w.Code, w.Body.String())
			}
			srv.guardQueue.Settle()
			state, err = srv.guardStore.Read(key, path)
			if err != nil {
				t.Fatal(err)
			}
			if state.UIWorkPending {
				t.Fatal("API observation did not finish")
			}
			if (state.Event != nil) != c.fire {
				t.Fatalf("event = %+v, want fire %v", state.Event, c.fire)
			}
			if !strings.Contains(string(state.Baseline.Data), `"@form":"stored"`) {
				t.Fatal("baseline has no stored marker")
			}
			if c.fire {
				island, err := Extractor{}.Extract(original)
				if err != nil {
					t.Fatal(err)
				}
				if (Extractor{}).CanRestore([]byte(readDoc(t, path)), island) {
					t.Fatal("digest offered Restore")
				}
				if !state.Event.HasRecoverCopy {
					t.Fatal("whole-file Revert unavailable")
				}
			}
		})
	}
}

// A page is never a proven reader, even when ClayJS sends the exact version it read
// along with the file's Save-Token: clearing a cell still raises an event.
func TestSheetGuardPageMatchedClearStillFires(t *testing.T) {
	srv, _, _ := setupHandlerTest(t)
	file, path := registerFixture(t, srv, "sheet.htmlclay", sheetGuardSource)
	if w := serve(t, srv, "/fixtures/sheet.htmlclay", nil); w.Code != 200 {
		t.Fatalf("serve = %d", w.Code)
	}
	key := guardKey(t, file)
	original := []byte(sheetGuardSource)
	if _, err := srv.guard.Observe(dataguard.Observation{Key: key, AbsPath: path, NewHTML: original, PrevHTML: original, Prov: dataguard.UIGestured}); err != nil {
		t.Fatal(err)
	}
	state, err := srv.guardStore.Read(key, path)
	if err != nil {
		t.Fatal(err)
	}
	if !state.UIWorkPending {
		t.Fatal("human work was not seeded")
	}
	value, _ := json.Marshal("")
	body := fmt.Sprintf(`{"patch":[{"range":"B1","values":[[%s]]}]}`, value)
	headers := map[string]string{
		"Sec-Fetch-Site":  "same-origin",
		"Origin":          fmt.Sprintf("http://127.0.0.1:%d", srv.port),
		helperTokenHeader: file.Token,
		"If-Match":        specwire.Etag(original),
	}
	w := postDataWrite(t, srv, "/_/api/fixtures/sheet.htmlclay", body, headers)
	if w.Code != 200 {
		t.Fatalf("patch = %d: %s", w.Code, w.Body.String())
	}
	srv.guardQueue.Settle()
	state, err = srv.guardStore.Read(key, path)
	if err != nil {
		t.Fatal(err)
	}
	if state.Event == nil {
		t.Fatal("a page's matched clear raised no event")
	}
}

func TestSheetGuardUnreadableCodecAndTag(t *testing.T) {
	for _, c := range []struct {
		name, after  string
		provenReader bool
	}{
		{"codec", strings.Replace(sheetGuardSource, "@codec(sheet)", "@codec(unknown)", 1), false},
		{"tag", strings.Replace(sheetGuardSource, `data-rules-version="2"`, `data-rules-version="99"`, 1), false},
		{"tag by a proven reader", strings.Replace(sheetGuardSource, `data-rules-version="2"`, `data-rules-version="99"`, 1), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv, _, _ := setupHandlerTest(t)
			file, path := registerFixture(t, srv, "sheet.htmlclay", sheetGuardSource)
			if w := serve(t, srv, "/fixtures/sheet.htmlclay", nil); w.Code != 200 {
				t.Fatalf("serve = %d", w.Code)
			}
			event, err := srv.guard.Observe(dataguard.Observation{Key: guardKey(t, file), AbsPath: path, NewHTML: []byte(c.after), PrevHTML: []byte(sheetGuardSource), Prov: dataguard.External, IfMatched: true, ProvenReader: c.provenReader})
			if err != nil {
				t.Fatal(err)
			}
			if event == nil || event.Restorable || !event.CanRevert {
				t.Fatalf("event = %+v", event)
			}
			encoded, _ := json.Marshal(event.Preview)
			if strings.Contains(string(encoded), "@form") || strings.Contains(string(encoded), "@codecs") {
				t.Fatalf("metadata in preview: %s", encoded)
			}
			if c.name == "codec" {
				island, err := Extractor{}.Extract([]byte(c.after))
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := dataapi.Marshal(island)
				if !strings.Contains(string(raw), `"note":"Saved note"`) || !strings.Contains(string(raw), `"sheets":"unknown"`) {
					t.Fatalf("lost readable data or declaration: %s", raw)
				}
			}
		})
	}
}

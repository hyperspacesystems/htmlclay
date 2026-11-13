//go:build managedtrial

package managedtrial

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/dataguard"
	"github.com/panphora/htmlclay/internal/specwire"
	"github.com/panphora/htmlclay/internal/testutil"
)

func TestManagedGuardIfMatch(t *testing.T) {
	original := `<html><head><script data-rules-name="api" data-rules-version="1">{"note":"h1"}</script></head><body><h1>Saved</h1></body></html>`
	for _, c := range []struct {
		name, value, header string
		fire                bool
	}{
		{"matched overwrite", "Updated", "exact", false},
		{"unmatched overwrite", "Updated", "", true},
		{"wildcard overwrite", "Updated", "*", true},
		{"matched clear", "", "exact", true},
		{"unverified header", "Updated", "unverified", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "doc.html")
			if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			hooks := newGuardHooks(nil)
			host := newCommitHost(root, "root-a", newConsentRegistry(root, nil), hooks, nil)
			srv := &rootServer{baseDir: root, commit: host}
			runDataLossGuard(runGuardArgs{BaseDir: root, Name: "doc.html", NewHTML: original, PrevContent: &original, Prov: dataguard.UIGestured})
			state, err := readGuard(root, "doc.html")
			if err != nil || !state.UIWorkPending {
				t.Fatalf("seed = %+v, %v", state, err)
			}
			req := httptest.NewRequest("POST", "/_/api/doc.html", nil)
			header := c.header
			if header == "exact" || header == "unverified" {
				header = specwire.Etag([]byte(original))
			}
			if header != "" {
				req.Header.Set("If-Match", header)
			}
			data := dataapi.NewObject()
			data.Set("note", c.value)
			if c.header == "unverified" {
				result, err := host.commit(&commitRequest{Kind: "data-api", Name: "doc.html", Text: true, Bytes: []byte(strings.Replace(original, "Saved", "Updated", 1)), DataLossPrev: &original, Headers: map[string]string{"if-match": header}})
				if err != nil || result.Outcome != "committed" {
					t.Fatalf("commit = %+v, %v", result, err)
				}
			} else {
				response := httptest.NewRecorder()
				if err := srv.applySiteData(response, req, "doc.html", path, data, dataQuery{}); err != nil {
					t.Fatal(err)
				}
				if response.Code != 200 {
					t.Fatalf("write = %d: %s", response.Code, response.Body.String())
				}
			}
			state, err = readGuard(root, "doc.html")
			if err != nil {
				t.Fatal(err)
			}
			if state.UIWorkPending || (state.Event != nil) != c.fire {
				t.Fatalf("guard = %+v, want fire %v", state, c.fire)
			}
		})
	}
}

func TestManagedSheetExtraction(t *testing.T) {
	var vectors []struct{ Name, HTML string }
	testutil.LoadVectors(t, "data-api.json", &vectors)
	source := ""
	for _, v := range vectors {
		if v.Name == "sheet-read-two-tabs" {
			source = v.HTML
		}
	}
	if source == "" {
		t.Fatal("missing shared sheet fixture")
	}
	island, err := extractIsland(source)
	if err != nil {
		t.Fatal(err)
	}
	if !dataguard.HasCodecs(island) {
		t.Fatal("sheet extracted as ordinary data")
	}
	if _, ok := restoreDataOntoHTML(source, island); ok {
		t.Fatal("digest offered Restore")
	}
	api, err := apiExtractViaTag([]byte(source))
	if err != nil || api == nil {
		t.Fatalf("API read = %v, %v", api, err)
	}
	sidecar, err := extractAPIData(source)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := dataapi.Marshal(api)
	if err != nil {
		t.Fatal(err)
	}
	if string(sidecar) != string(expected) {
		t.Fatal("sidecar and API read differ")
	}
}

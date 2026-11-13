//go:build managedtrial

package managedtrial

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Local answers both data API routes through resolveWriteTarget
// (hyperclay-local src/main/server.js:240-245), whose name check refuses the
// reserved backups folder sites-versions before any path is resolved. A backup
// copy under sites-versions/ is therefore never readable or writable through
// /_/api, and the refusal is Local's 400 {"error":"Invalid file path"}.
func TestAPIRefusesReservedSitesVersions(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the handler reads that checkout's bundled runtime release")
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	page := func(heading string) []byte {
		return []byte(`<!doctype html><html><head><title>Notes</title>` +
			`<script type="application/json" data-rules-name="api" data-rules-version="1">{"title":"h1"}</script>` +
			`</head><body><h1>` + heading + `</h1></body></html>`)
	}
	writeCommitFile(t, root, "notes.html", page("current"))
	backupRel := filepath.Join("sites-versions", "notes", "2026-01-01-00-00-00-000.html")
	writeCommitFile(t, root, backupRel, page("backup"))

	handler, err := newRootHandler(rootSpec{id: "root-reserved", path: root}, config{localSource: localSource})
	if err != nil {
		t.Fatal(err)
	}
	address, port := serveTestServer(t, handler)
	origin := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Control: the same route answers an ordinary document, so a refusal below
	// is the reserved-folder check and not a route that never answers.
	status, _, answer := serveSend(t, address, http.MethodGet, "/_/api/notes.html", nil, nil, port)
	if status != http.StatusOK {
		t.Fatalf("GET /_/api/notes.html = %d, want 200 (%s)", status, answer)
	}

	before := serveFileList(t, root)
	want := map[string]any{"error": "Invalid file path"}
	backupURL := "/_/api/sites-versions/notes/2026-01-01-00-00-00-000.html"

	status, _, answer = serveSend(t, address, http.MethodGet, backupURL, nil, nil, port)
	var got map[string]any
	if err := json.Unmarshal(answer, &got); err != nil {
		t.Fatalf("GET %s answered non-JSON %d: %s", backupURL, status, answer)
	}
	if status != http.StatusBadRequest || !reflect.DeepEqual(got, want) {
		t.Errorf("GET %s = %d %s, want 400 %v", backupURL, status, answer, want)
	}

	headers := map[string]string{"Content-Type": "application/json", "Origin": origin}
	status, _, answer = serveSend(t, address, http.MethodPost, backupURL, headers, []byte(`{"title":"changed"}`), port)
	got = nil
	if err := json.Unmarshal(answer, &got); err != nil {
		t.Fatalf("POST %s answered non-JSON %d: %s", backupURL, status, answer)
	}
	if status != http.StatusBadRequest || !reflect.DeepEqual(got, want) {
		t.Errorf("POST %s = %d %s, want 400 %v", backupURL, status, answer, want)
	}

	if after := serveFileList(t, root); !reflect.DeepEqual(after, before) {
		t.Errorf("a refused data API request changed the tree:\n got %v\nwant %v", after, before)
	}
}

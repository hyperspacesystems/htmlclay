package served

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/served/assets"
)

type unsavedReportCall struct {
	path    string
	tab     string
	seq     int64
	unsaved bool
}

func TestServedUnsavedReport(t *testing.T) {
	var calls []unsavedReportCall
	host := NewHost(Options{
		Assets:          assets.ProductFS(),
		CompileTailwind: func(string) (string, error) { return "", nil },
		OnUnsavedReport: func(absPath, tab string, seq int64, unsaved, frozen bool) {
			calls = append(calls, unsavedReportCall{path: absPath, tab: tab, seq: seq, unsaved: unsaved})
		},
	})
	t.Cleanup(host.Close)

	dir := raceTempDir(t)
	handler, err := host.AddRoot(RootSpec{ID: guardV2RootID, Path: dir, PublicHost: guardV2Host})
	if err != nil {
		t.Fatalf("AddRoot: %v", err)
	}
	root, ok := handler.(*rootServer)
	if !ok {
		t.Fatalf("AddRoot returned %T, want *rootServer", handler)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	stand := &guardV2Stand{host: host, server: server, root: root, dir: dir}
	docPath := filepath.Join(dir, guardV2Name)
	if err := os.WriteFile(docPath, []byte(guardV2Doc("T", "note")), 0o644); err != nil {
		t.Fatal(err)
	}

	meta := stand.send(t, true, http.MethodGet, "/_/meta", "", "", nil)
	if meta.status != http.StatusOK {
		t.Fatalf("GET /_/meta = %s, want 200", meta)
	}
	var extensions []string
	for _, raw := range meta.body["extensions"].([]any) {
		extensions = append(extensions, raw.(string))
	}
	if !slices.IsSorted(extensions) || !slices.Contains(extensions, "unsaved-report-v1") {
		t.Fatalf("extensions = %v, want sorted and containing unsaved-report-v1", extensions)
	}

	body := `{"tab":"t1","seq":1,"unsaved":true}`
	answer := stand.send(t, true, http.MethodPost, "/_/unsaved", "application/json", body, nil)
	if answer.status != http.StatusNoContent {
		t.Fatalf("same-origin POST /_/unsaved = %s, want 204", answer)
	}
	if len(calls) != 1 || calls[0] != (unsavedReportCall{path: docPath, tab: "t1", seq: 1, unsaved: true}) {
		t.Fatalf("recorder got %+v, want %s t1 1 true", calls, docPath)
	}

	calls = nil
	answer = stand.send(t, false, http.MethodPost, "/_/unsaved", "application/json", body, map[string]string{
		"Sec-Fetch-Site": "cross-site",
		"Origin":         "http://evil.example",
		"Document-URL":   "http://" + guardV2Host + "/" + guardV2Name,
	})
	if answer.status != http.StatusForbidden {
		t.Fatalf("cross-origin POST /_/unsaved = %s, want 403", answer)
	}
	if len(calls) != 0 {
		t.Fatalf("a cross-origin report reached the recorder: %+v", calls)
	}

	answer = stand.send(t, true, http.MethodPost, "/_/unsaved", "application/json", body, map[string]string{
		"Document-URL": "http://" + guardV2Host + "/missing.html",
	})
	if answer.status != http.StatusNotFound {
		t.Fatalf("POST /_/unsaved naming a missing file = %s, want 404", answer)
	}
	if len(calls) != 0 {
		t.Fatalf("a report for a missing file reached the recorder: %+v", calls)
	}
	// The tab of a document deleted or renamed while it was open says "saved" as it
	// closes, and that is taken, or Quit would ask about it until the tab expired.
	answer = stand.send(t, true, http.MethodPost, "/_/unsaved", "application/json", `{"tab":"t1","seq":2,"unsaved":false}`, map[string]string{
		"Document-URL": "http://" + guardV2Host + "/missing.html",
	})
	if answer.status != http.StatusNoContent {
		t.Fatalf("POST /_/unsaved saying a missing file is saved = %s, want 204", answer)
	}
	if want := (unsavedReportCall{path: filepath.Join(dir, "missing.html"), tab: "t1", seq: 2}); len(calls) != 1 || calls[0] != want {
		t.Fatalf("recorder got %+v, want %+v", calls, want)
	}

	localHost := NewHost(Options{
		Assets:          assets.ProductFS(),
		CompileTailwind: func(string) (string, error) { return "", nil },
	})
	t.Cleanup(localHost.Close)
	localDir := raceTempDir(t)
	localHandler, err := localHost.AddRoot(RootSpec{ID: "local-root", Path: localDir})
	if err != nil {
		t.Fatalf("AddRoot: %v", err)
	}
	localServer := httptest.NewServer(localHandler)
	t.Cleanup(localServer.Close)

	localStatus, _, localMeta := forwardSend(t, localServer, http.MethodGet, "/_/meta", nil, nil)
	if localStatus != http.StatusOK {
		t.Fatalf("local GET /_/meta = %d %s, want 200", localStatus, localMeta)
	}
	var localExtensions []string
	if err := json.Unmarshal(localMeta, &struct {
		Extensions *[]string `json:"extensions"`
	}{Extensions: &localExtensions}); err != nil {
		t.Fatalf("local meta is not JSON: %v (%s)", err, localMeta)
	}
	for _, name := range localExtensions {
		if name == "unsaved-report-v1" {
			t.Fatalf("a root with no public host listed unsaved-report-v1: %v", localExtensions)
		}
	}
	localStatus, _, localAnswer := forwardSend(t, localServer, http.MethodPost, "/_/unsaved", strings.NewReader(body), nil)
	if localStatus != http.StatusNotFound {
		t.Fatalf("a root with no public host answered POST /_/unsaved %d %s, want 404", localStatus, localAnswer)
	}
}

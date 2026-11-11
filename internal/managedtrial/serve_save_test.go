//go:build managedtrial

package managedtrial

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestSaveGolden pins POST /_/save and the answers it gives to Local's own:
// every status, header and body of the recorded sequence, and the tree the
// writes leave behind.
//
// The one file this host does not write is the data guard's record under
// `.hyperclay/guard/`, which G7 owns; Local writes one per document, so those
// are left out of the comparison.
func TestSaveGolden(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the golden answers were recorded from a real Hyperclay Local checkout, and the handler reads that checkout's bundled runtime release")
	}
	runGoldenCasesWith(t, filepath.Join("testdata", "save-golden"), localSource, goldenOptions{
		skipFile: func(path string) bool {
			return path == ".hyperclay/guard" || strings.HasPrefix(path, ".hyperclay/guard/")
		},
		// AD1's extension list is the same difference TestMetaGolden records.
		normalize: func(answer *serveAnswer) {
			if answer.Name == "meta-after-save" {
				normalizeMetaAnswer(answer)
			}
		},
	})
}

// The page fixtures are the commit golden's, so the sidecar and the inline tag
// are the same bytes Local's own scenarios use.
const (
	saveGoldenYJSTag  = `<script type="application/json" merge="blockeditor" clay="live">{"v":1,"g":0,"y":{"seed":"AAACQQMEAQAIQgALJwAoAQUAJwAoAIUkHGF0dHJzdGl0bGVsZXZlbHRmbG93YXR0cnNwMXRFAQEEBQIBBwEAAAEBAQACQQACQQEBBwB9AXcHaGVhZGluZ3YBAWJ3BXRpdGxldwlwYXJhZ3JhcGh2AQFidwJwMQA="}}</script>`
	saveGoldenPageRev = "093360993130d818"
)

// saveGoldenArticle is the page every AD2 fixture is built from: a blockeditor-tagged
// article whose revision is the one the save carries.
func saveGoldenArticle(rev string) string {
	return `<article class="blockeditor" clay="freeze" data-blockeditor="1" data-rev="` + rev + `"><h1 data-id="title">Title</h1>
<p data-id="p1">Body</p></article>`
}

func saveGoldenPage(rev string) string {
	return strings.Join([]string{
		"<!doctype html>",
		`<html lang="en">`,
		"<head>",
		`<meta charset="utf-8">`,
		"<title>Doc</title>",
		"</head>",
		"<body>",
		saveGoldenArticle(rev),
		"</body>",
		"</html>",
		"",
	}, "\n")
}

// TestSaveAD2StaleLineage is AD2 at the HTTP level: a save made against an older
// page lineage, which Local rebases, is refused here with a 409 and nothing
// written. It is not part of the golden comparison, because Local answers 200.
func TestSaveAD2StaleLineage(t *testing.T) {
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the handler reads that checkout's bundled runtime release")
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stale := saveGoldenPage("bbbbbbbbbbbbbbbb")
	writeCommitFile(t, root, "stale.html", []byte(stale))
	writeCommitFile(t, root, filepath.Join(".hyperclay", "blockeditor-state", "stale.html.json"),
		[]byte(fmt.Sprintf(`{"v":1,"rev":"bbbbbbbbbbbbbbbb","tag":%s}`, mustJSON(saveGoldenYJSTag))))
	writeCommitFile(t, root, filepath.Join(".hyperclay", "versions", "stale", "2026-09-01-00-00-00-000+0000.html"),
		[]byte(saveGoldenPage("aaaaaaaaaaaaaaaa")))

	before := serveFileList(t, root)

	handler, err := newRootHandler(rootSpec{id: "root-ad2", path: root}, config{localSource: localSource})
	if err != nil {
		t.Fatal(err)
	}
	address, port := serveTestServer(t, handler)

	headers := map[string]string{
		"Document-URL": fmt.Sprintf("http://127.0.0.1:%d/stale.html", port),
		"Content-Type": "text/html",
	}
	body := []byte(saveGoldenPage("aaaaaaaaaaaaaaaa"))
	status, gotHeaders, answer := serveSend(t, address, http.MethodPost, "/_/save", headers, body, port)

	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", status, answer)
	}
	var decoded struct {
		Msg     string `json:"msg"`
		MsgType string `json:"msgType"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(answer, &decoded); err != nil {
		t.Fatalf("answer is not JSON: %v (%s)", err, answer)
	}
	want := struct {
		Msg     string `json:"msg"`
		MsgType string `json:"msgType"`
		Code    string `json:"code"`
	}{
		Msg:     "This page has newer edits on this computer. Reload it, then save again.",
		MsgType: "error",
		Code:    "stale-blockeditor-lineage",
	}
	if decoded != want {
		t.Errorf("answer = %+v, want %+v", decoded, want)
	}
	if got := gotHeaders["content-type"]; got != jsonContentType {
		t.Errorf("content-type = %q, want %q", got, jsonContentType)
	}

	after := serveFileList(t, root)
	if !reflect.DeepEqual(after, before) {
		t.Errorf("a refused save changed the tree:\n got %v\nwant %v", after, before)
	}
	if data, err := os.ReadFile(filepath.Join(root, "stale.html")); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(data, []byte(stale)) {
		t.Errorf("a refused save rewrote stale.html")
	}
}

// serveTestServer runs one handler on a loopback listener and answers its
// address and port.
func serveTestServer(t *testing.T, handler http.Handler) (string, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: readHeaderLimit}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Close() })
	port := listener.Addr().(*net.TCPAddr).Port
	return fmt.Sprintf("127.0.0.1:%d", port), port
}

//go:build managedtrial

package managedtrial

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// consultSavePage is the document every consulted save writes.
const consultSavePage = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title>Consulted</title>
  </head>
  <body>
    <h1>Saved</h1>
  </body>
</html>
`

// consultSaveAPIPage is the page a consulted data API write edits.
const consultSaveAPIPage = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title>Index</title>
    <script type="application/json" data-rules-name="api" data-rules-version="1">{"title":"h1"}</script>
  </head>
  <body>
    <h1>Index</h1>
  </body>
</html>
`

// consultSavePeer starts the fake service and answers its address.
func consultSavePeer(t *testing.T, script *consultScript) servicePeer {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(script.handle))
	t.Cleanup(server.Close)
	return servicePeer{URL: server.URL, Secret: strings.Repeat("ab", 32), Epoch: "aaaaaaaaaaaaaaaa"}
}

// consultSaveServer serves one root with a service attached, the way the binary
// builds them.
func consultSaveServer(t *testing.T, peer servicePeer) (*httptest.Server, *rootServer, string) {
	t.Helper()
	localSource := os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE")
	if localSource == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is unset: the root server reads that checkout's bundled runtime release")
	}
	dir := consultTestRoot(t)
	events := newEventRing()
	link := newServiceLink(events, peer)
	handler, err := newRootHandler(rootSpec{id: "root-consult", path: dir}, config{localSource: localSource, events: events, service: link})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server, handler, dir
}

// consultShortAborts keeps a settle-failed abort retry from outliving a test.
func consultShortAborts(t *testing.T) {
	t.Helper()
	previous := abortRetryInterval
	abortRetryInterval = 10 * time.Millisecond
	t.Cleanup(func() { abortRetryInterval = previous })
}

// consultSavePost sends one POST /_/save of a complete document.
func consultSavePost(t *testing.T, server *httptest.Server, name, body string, headers map[string]string) (int, http.Header, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/_/save", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "text/html")
	request.Header.Set("Document-URL", server.URL+"/"+name)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, response.Header, answer
}

// consultWaitFor waits for one call the host makes off its own goroutine.
func consultWaitFor(t *testing.T, script *consultScript, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if script.count(path) > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s never arrived", path)
}

// consultTestDisk is one document's bytes on disk, or nothing when it is absent.
func consultTestDisk(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// TestConsultedSaveProceeds is the whole consulted save: admit before the first
// backup, the service's own logical and file on disk, and settle at Local's
// confirmation point.
func TestConsultedSaveProceeds(t *testing.T) {
	consultShortAborts(t)
	admitted := strings.Replace(consultSavePage, "  </body>", "    <!-- admitted -->\n  </body>", 1)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/admit":
			return map[string]any{
				"decision":    "proceed",
				"logical":     map[string]any{"encoding": "utf8", "data": admitted},
				"file":        map[string]any{"encoding": "utf8", "data": admitted},
				"reservation": "r1",
			}
		case "/v1/settle":
			return map[string]any{"ok": true}
		}
		return nil
	})
	server, _, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))

	status, _, answer := consultSavePost(t, server, "page.html", consultSavePage, nil)
	if status != http.StatusOK {
		t.Fatalf("save answered %d: %s", status, answer)
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != admitted {
		t.Fatalf("page.html = %q, want the answered file", onDisk)
	}

	admit := script.only(t, "/v1/admit").Body
	for key, want := range map[string]any{"lane": "save", "origin": "browser"} {
		if admit[key] != want {
			t.Fatalf("admit %s = %v, want %v", key, admit[key], want)
		}
	}
	if incoming := consultTestData(t, admit["incoming"], "incoming"); incoming != consultSavePage {
		t.Fatalf("admit incoming = %q, want the posted bytes", incoming)
	}
	headers := consultTestObject(t, admit["headers"], "headers")
	if len(headers) != len(admitHeaders) {
		t.Fatalf("admit headers = %v, want all %d lane headers", headers, len(admitHeaders))
	}

	settle := script.only(t, "/v1/settle").Body
	if settle["outcome"] != "committed" {
		t.Fatalf("settle outcome = %v, want committed", settle["outcome"])
	}
	if settle["reservation"] != "r1" {
		t.Fatalf("settle reservation = %v, want r1", settle["reservation"])
	}
	if logical := consultTestData(t, settle["logical"], "logical"); logical != admitted {
		t.Fatalf("settle logical = %q, want the answered logical", logical)
	}
	if settle["sender"] != "server-save" {
		t.Fatalf("settle sender = %v, want server-save", settle["sender"])
	}
	identity := consultTestObject(t, settle["identity"], "identity")
	if ino, _ := identity["ino"].(string); ino == "" {
		t.Fatalf("settle identity.ino = %v, want the written file's inode", identity["ino"])
	}

	if script.count("/v1/abort") != 0 {
		t.Fatalf("abort was called %d times, want none", script.count("/v1/abort"))
	}
	paths := []string{}
	for _, exchange := range script.exchanges {
		paths = append(paths, exchange.Path)
	}
	if len(paths) != 2 || paths[0] != "/v1/admit" || paths[1] != "/v1/settle" {
		t.Fatalf("the calls were %v, want admit then settle", paths)
	}
}

// TestConsultedSaveRefused answers a client with the refusal exactly as the
// service decided it, and leaves the document alone.
func TestConsultedSaveRefused(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/admit" {
			return map[string]any{
				"decision": "refuse",
				"status":   409,
				"code":     "peers-editing",
				"body":     map[string]any{"msg": "x", "code": "peers-editing"},
				"headers":  map[string]any{"X-Test": "1"},
			}
		}
		return nil
	})
	server, _, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))

	status, headers, answer := consultSavePost(t, server, "page.html", consultSavePage, nil)
	if status != http.StatusConflict {
		t.Fatalf("save answered %d: %s", status, answer)
	}
	if got := headers.Get("X-Test"); got != "1" {
		t.Fatalf("X-Test = %q, want 1", got)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(answer, &decoded); err != nil {
		t.Fatalf("answer %s is not JSON: %v", answer, err)
	}
	if decoded["msg"] != "x" || decoded["code"] != "peers-editing" || len(decoded) != 2 {
		t.Fatalf("answer = %s, want the service's own body", answer)
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != consultSavePage {
		t.Fatalf("page.html = %q, want the bytes unchanged", onDisk)
	}
	if entries, err := os.ReadDir(filepath.Join(dir, versionsDirName, "page")); err == nil && len(entries) != 0 {
		t.Fatalf("a refused save left %d versions", len(entries))
	}

	consultWaitFor(t, script, "/v1/abort")
	abort := script.only(t, "/v1/abort").Body
	if abort["reason"] != "refused" {
		t.Fatalf("abort reason = %v, want refused", abort["reason"])
	}
	if abort["consultId"] != consultTestID(t, script.only(t, "/v1/admit").Body["consultId"]) {
		t.Fatalf("abort consultId = %v, want the admit's", abort["consultId"])
	}
}

// TestConsultedSaveServiceDown refuses a write it cannot admit, writing nothing.
func TestConsultedSaveServiceDown(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closed.Close()
	server, _, dir := consultSaveServer(t, servicePeer{
		URL: closed.URL, Secret: strings.Repeat("ab", 32), Epoch: "aaaaaaaaaaaaaaaa",
	})
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))

	status, headers, answer := consultSavePost(t, server, "page.html", consultSavePage, nil)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("save answered %d: %s", status, answer)
	}
	if got := headers.Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
	if string(answer) != `{"reason":"service-unavailable","retryable":true}` {
		t.Fatalf("answer = %s, want a retryable service-unavailable", answer)
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != consultSavePage {
		t.Fatalf("page.html = %q, want the bytes unchanged", onDisk)
	}
	if entries, err := os.ReadDir(filepath.Join(dir, versionsDirName, "page")); err == nil && len(entries) != 0 {
		t.Fatalf("a refused save left %d versions", len(entries))
	}
}

// TestConsultedSaveSettleFails keeps the bytes, bars the document until the
// service reconciles it, and lifts the bar when it does.
func TestConsultedSaveSettleFails(t *testing.T) {
	consultShortAborts(t)
	admitted := strings.Replace(consultSavePage, "  </body>", "    <!-- admitted -->\n  </body>", 1)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/admit":
			return map[string]any{
				"decision": "proceed",
				"logical":  map[string]any{"encoding": "utf8", "data": admitted},
				"file":     map[string]any{"encoding": "utf8", "data": admitted},
			}
		case "/v1/settle":
			return map[string]any{"ok": false, "error": "x"}
		}
		return nil
	})
	server, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))

	status, _, answer := consultSavePost(t, server, "page.html", consultSavePage, nil)
	if status != http.StatusOK {
		t.Fatalf("save answered %d: %s", status, answer)
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != admitted {
		t.Fatalf("page.html = %q, want the write committed", onDisk)
	}

	status, _, answer = consultSavePost(t, server, "page.html", consultSavePage, nil)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("the next save answered %d: %s", status, answer)
	}
	if string(answer) != `{"reason":"unconfirmed","retryable":true}` {
		t.Fatalf("answer = %s, want a retryable unconfirmed", answer)
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != admitted {
		t.Fatalf("page.html = %q, want the barred save to write nothing", onDisk)
	}

	handler.commit.service.unconfirmed.lift(upathJoin(dir, "page.html"))
	status, _, answer = consultSavePost(t, server, "page.html", consultSavePage, nil)
	if status != http.StatusOK {
		t.Fatalf("the save after reconciling answered %d: %s", status, answer)
	}
}

// TestConsultedSaveArtifactsBeforeWrite writes an admit batch before the first
// backup, while the document still holds its old bytes.
func TestConsultedSaveArtifactsBeforeWrite(t *testing.T) {
	consultShortAborts(t)
	var documentPath string
	sawOldBytes := false
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/admit":
			return map[string]any{
				"pending": true,
				"artifacts": []any{
					map[string]any{
						"artifactId": "aaaaaaaaaaaaaaaa",
						"type":       "safety-version",
						"logical":    map[string]any{"encoding": "utf8", "data": "<p>safe</p>"},
					},
				},
			}
		case "/v1/artifacts-written":
			onDisk, err := os.ReadFile(documentPath)
			sawOldBytes = err == nil && string(onDisk) == consultSavePage
			return map[string]any{
				"decision": "proceed",
				"logical":  map[string]any{"encoding": "utf8", "data": consultSavePage},
				"file":     map[string]any{"encoding": "utf8", "data": consultSavePage},
			}
		}
		return nil
	})
	server, _, dir := consultSaveServer(t, consultSavePeer(t, script))
	documentPath = filepath.Join(dir, "page.html")
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))

	status, _, answer := consultSavePost(t, server, "page.html", consultSavePage, nil)
	if status != http.StatusOK {
		t.Fatalf("save answered %d: %s", status, answer)
	}
	if !sawOldBytes {
		t.Fatal("the artifact batch was written after the document had already changed")
	}
	versions := consultTestFiles(t, filepath.Join(dir, versionsDirName, "page"))
	found := false
	for _, path := range versions {
		if consultTestDisk(t, path) == "<p>safe</p>" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no version of %v holds the admitted artifact", versions)
	}
}

// TestControlCommitCarriesRefusal reports a decided refusal through the control
// API, keyed by the opId the service sent.
func TestControlCommitCarriesRefusal(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/admit" {
			return map[string]any{
				"decision": "refuse",
				"status":   409,
				"code":     "peers-editing",
				"body":     map[string]any{"msg": "x", "code": "peers-editing"},
				"headers":  map[string]any{"X-Test": "1"},
			}
		}
		return nil
	})
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	control := controlTestServer(t, handler.control)

	status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"root":     "root-consult",
		"name":     "page.html",
		"kind":     "browser-save",
		"opHandle": "cccccccccccccccc",
		"bytes":    map[string]any{"encoding": "utf8", "data": consultSavePage},
	})
	if status != http.StatusOK {
		t.Fatalf("commit answered %d: %v", status, answer)
	}
	if answer["outcome"] != "refused" || answer["reason"] != "peers-editing" {
		t.Fatalf("answer = %v, want a peers-editing refusal", answer)
	}
	refusal := consultTestObject(t, answer["refusal"], "refusal")
	if refusal["status"] != float64(409) || refusal["code"] != "peers-editing" {
		t.Fatalf("refusal = %v, want the service's status and code", refusal)
	}
	body := consultTestObject(t, refusal["body"], "refusal.body")
	if body["msg"] != "x" || body["code"] != "peers-editing" {
		t.Fatalf("refusal.body = %v, want the service's own body", refusal["body"])
	}
	headers := consultTestObject(t, refusal["headers"], "refusal.headers")
	if headers["X-Test"] != "1" {
		t.Fatalf("refusal.headers = %v, want X-Test", headers)
	}
	if admit := script.only(t, "/v1/admit").Body; admit["opHandle"] != "cccccccccccccccc" {
		t.Fatalf("admit opHandle = %v, want cccccccccccccccc", admit["opHandle"])
	}
}

// TestConsultedDataAPIRefusal answers a data API write with the service's
// refusal and leaves the document alone.
func TestConsultedDataAPIRefusal(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/admit" {
			return map[string]any{
				"decision": "refuse",
				"status":   409,
				"code":     "peers-editing",
				"body":     map[string]any{"msg": "x", "code": "peers-editing"},
				"headers":  map[string]any{"X-Test": "1"},
			}
		}
		return nil
	})
	server, _, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "index.html", []byte(consultSaveAPIPage))

	request, err := http.NewRequest(http.MethodPost, server.URL+"/_/api/index.html", bytes.NewReader([]byte(`{"title":"New"}`)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("data API write answered %d: %s", response.StatusCode, answer)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(answer, &decoded); err != nil {
		t.Fatalf("answer %s is not JSON: %v", answer, err)
	}
	if decoded["msg"] != "x" || decoded["code"] != "peers-editing" {
		t.Fatalf("answer = %s, want the service's own body", answer)
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "index.html")); onDisk != consultSaveAPIPage {
		t.Fatalf("index.html = %q, want the bytes unchanged", onDisk)
	}
}

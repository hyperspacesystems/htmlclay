//go:build managedtrial

package managedtrial

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/specwire"
)

// consultExchange is one request the fake service recorded: its path and its
// decoded JSON body.
type consultExchange struct {
	Path string
	Body map[string]any
}

// consultScript is the fake JS service one test talks to: it records every
// request and answers from the test's function, which sees the call's path, its
// body and how many calls that path has had.
type consultScript struct {
	mu        sync.Mutex
	calls     map[string]int
	exchanges []consultExchange
	answer    func(path string, body map[string]any, call int) any
}

func newConsultScript(answer func(path string, body map[string]any, call int) any) *consultScript {
	return &consultScript{calls: make(map[string]int), answer: answer}
}

func (s *consultScript) handle(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	s.mu.Lock()
	s.calls[r.URL.Path]++
	call := s.calls[r.URL.Path]
	s.exchanges = append(s.exchanges, consultExchange{Path: r.URL.Path, Body: body})
	answer := s.answer(r.URL.Path, body, call)
	s.mu.Unlock()
	if answer == nil {
		answer = map[string]any{"ok": true}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(answer)
}

func (s *consultScript) count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[path]
}

func (s *consultScript) find(path string) []consultExchange {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := []consultExchange{}
	for _, exchange := range s.exchanges {
		if exchange.Path == path {
			found = append(found, exchange)
		}
	}
	return found
}

func (s *consultScript) only(t *testing.T, path string) consultExchange {
	t.Helper()
	found := s.find(path)
	if len(found) != 1 {
		t.Fatalf("%s was called %d times, want once", path, len(found))
	}
	return found[0]
}

// consultTestLink attaches a link to the fake service.
func consultTestLink(t *testing.T, script *consultScript) *serviceLink {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(script.handle))
	t.Cleanup(server.Close)
	return newServiceLink(newEventRing(), servicePeer{
		URL:    server.URL,
		Secret: strings.Repeat("ab", 32),
		Epoch:  "aaaaaaaaaaaaaaaa",
	})
}

// consultTestRoot is one served root's real folder.
func consultTestRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func consultWriteFile(t *testing.T, root, rel string, content []byte) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(target), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func consultTestID(t *testing.T, value any) string {
	t.Helper()
	text, ok := value.(string)
	if !ok || len(text) != 32 || strings.Trim(text, "0123456789abcdef") != "" {
		t.Fatalf("consultId = %v, want 32 lowercase hex", value)
	}
	return text
}

func consultTestObject(t *testing.T, value any, name string) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %v, want an object", name, value)
	}
	return object
}

func consultTestData(t *testing.T, value any, name string) string {
	t.Helper()
	object := consultTestObject(t, value, name)
	data, _ := object["data"].(string)
	if object["encoding"] != "utf8" {
		t.Fatalf("%s = %v, want utf8 bytes", name, value)
	}
	return data
}

func consultTestReceipts(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["receipts"].([]any)
	if !ok {
		t.Fatalf("artifacts-written body carries no receipts: %v", body)
	}
	receipts := make([]map[string]any, 0, len(raw))
	for _, value := range raw {
		receipts = append(receipts, consultTestObject(t, value, "receipt"))
	}
	return receipts
}

// consultTestVersionFile is the one version file in a directory, by content.
func consultTestFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	files := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	return files
}

// TestConsultAdmitProceed is one admit: the snapshot the service reads instead
// of the folder, the five lane headers, and the proceed answer it accepts.
func TestConsultAdmitProceed(t *testing.T) {
	root := consultTestRoot(t)
	content := []byte("<p>x</p>")
	filePath := filepath.Join(root, "a.html")
	consultWriteFile(t, root, "a.html", content)

	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path != "/v1/admit" {
			return map[string]any{"ok": true}
		}
		return map[string]any{
			"decision":    "proceed",
			"logical":     map[string]any{"encoding": "utf8", "data": "L"},
			"file":        map[string]any{"encoding": "utf8", "data": "F"},
			"reservation": "r1",
			"artifacts":   []any{},
		}
	})
	link := consultTestLink(t, script)
	link.setRoot("root-0", root)
	c := link.begin("1111111111111111", nil, "root-0", root, "a.html", filePath, "save", "browser")

	answer, err := c.admit(admitArgs{Incoming: content, Headers: map[string]string{"if-match": `"e"`}})
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if answer.Decision != "proceed" {
		t.Fatalf("admit decision = %q, want proceed", answer.Decision)
	}
	if c.Reservation == nil || *c.Reservation != "r1" {
		t.Fatalf("reservation = %v, want r1", c.Reservation)
	}

	body := script.only(t, "/v1/admit").Body
	consultTestID(t, body["consultId"])
	if body["opId"] != "1111111111111111" {
		t.Fatalf("opId = %v", body["opId"])
	}
	for key, want := range map[string]string{
		"root":          "root-0",
		"name":          "a.html",
		"canonicalPath": filePath,
		"lane":          "save",
		"origin":        "browser",
	} {
		if body[key] != want {
			t.Fatalf("%s = %v, want %s", key, body[key], want)
		}
	}
	current := consultTestObject(t, body["current"], "current")
	if current["exists"] != true {
		t.Fatalf("current.exists = %v, want true", current["exists"])
	}
	if current["checksum"] != specwire.Etag(content) {
		t.Fatalf("current.checksum = %v, want %s", current["checksum"], specwire.Etag(content))
	}
	if incoming := consultTestData(t, body["incoming"], "incoming"); incoming != string(content) {
		t.Fatalf("incoming.data = %q, want %q", incoming, content)
	}
	if versions, ok := body["versions"].([]any); !ok || len(versions) != 0 {
		t.Fatalf("versions = %v, want []", body["versions"])
	}
	headers := consultTestObject(t, body["headers"], "headers")
	if len(headers) != len(admitHeaders) {
		t.Fatalf("headers = %v, want all %d lane headers", headers, len(admitHeaders))
	}
	for _, name := range admitHeaders {
		if _, ok := headers[name]; !ok {
			t.Fatalf("headers = %v, want %s present", headers, name)
		}
	}
	if headers["if-match"] != `"e"` {
		t.Fatalf("headers.if-match = %v, want %q", headers["if-match"], `"e"`)
	}
	for _, name := range []string{"page-conversion", "page-write", "page-format", "persistent"} {
		if headers[name] != nil {
			t.Fatalf("headers.%s = %v, want null", name, headers[name])
		}
	}
}

// TestConsultAdmitRejectsBadAnswers is the contract over the three answers a
// service may not give.
func TestConsultAdmitRejectsBadAnswers(t *testing.T) {
	cases := []struct {
		name   string
		answer map[string]any
	}{
		{"proceed without file", map[string]any{
			"decision": "proceed",
			"logical":  map[string]any{"encoding": "utf8", "data": "L"},
		}},
		{"refuse with 200", map[string]any{"decision": "refuse", "status": 200, "code": "nope"}},
		{"unknown decision", map[string]any{"decision": "maybe"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := consultTestRoot(t)
			consultWriteFile(t, root, "a.html", []byte("<p>x</p>"))
			answer := testCase.answer
			script := newConsultScript(func(path string, body map[string]any, call int) any {
				if path == "/v1/admit" {
					return answer
				}
				return nil
			})
			link := consultTestLink(t, script)
			link.setRoot("root-0", root)
			c := link.begin("2222222222222222", nil, "root-0", root, "a.html", filepath.Join(root, "a.html"), "save", "browser")
			if _, err := c.admit(admitArgs{Incoming: []byte("<p>x</p>")}); err == nil {
				t.Fatalf("admit accepted %v", answer)
			}
		})
	}
}

// TestConsultArtifactBatches is the artifact loop: a pending answer's batch is
// written in order, receipts carry what landed, and the post's answer is the
// next one.
func TestConsultArtifactBatches(t *testing.T) {
	rootA := consultTestRoot(t)
	rootB := consultTestRoot(t)
	consultWriteFile(t, rootA, "a.html", []byte("<p>a</p>"))

	script := newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/admit":
			return map[string]any{
				"pending": true,
				"artifacts": []any{
					map[string]any{"artifactId": "aaaaaaaaaaaaaaaa", "type": "safety-version", "logical": map[string]any{"encoding": "utf8", "data": "S"}},
					map[string]any{"artifactId": "bbbbbbbbbbbbbbbb", "type": "recovered-copy", "root": "root-1", "name": "sub/b.html", "logical": map[string]any{"encoding": "utf8", "data": "R"}},
					map[string]any{"artifactId": "cccccccccccccccc", "type": "recovered-copy", "root": "root-9", "name": "x.html", "logical": map[string]any{"encoding": "utf8", "data": "X"}},
					map[string]any{"artifactId": "nothex", "type": "safety-version", "logical": map[string]any{"encoding": "utf8", "data": "Z"}},
				},
			}
		case "/v1/artifacts-written":
			return map[string]any{
				"decision": "proceed",
				"logical":  map[string]any{"encoding": "utf8", "data": "L"},
				"file":     map[string]any{"encoding": "utf8", "data": "F"},
			}
		}
		return nil
	})
	link := consultTestLink(t, script)
	link.setRoot("root-0", rootA)
	link.setRoot("root-1", rootB)
	c := link.begin("3333333333333333", nil, "root-0", rootA, "a.html", filepath.Join(rootA, "a.html"), "save", "browser")

	if _, err := c.admit(admitArgs{Incoming: []byte("<p>a</p>")}); err != nil {
		t.Fatalf("admit: %v", err)
	}

	versions := consultTestFiles(t, filepath.Join(rootA, ".hyperclay", "versions", "a"))
	if len(versions) != 1 {
		t.Fatalf("safety versions = %v, want one", versions)
	}
	if data, err := os.ReadFile(versions[0]); err != nil || string(data) != "S" {
		t.Fatalf("safety version = %q (%v), want S", data, err)
	}
	recovered := consultTestFiles(t, filepath.Join(rootB, ".hyperclay", "recovered", "sub", "b"))
	if len(recovered) != 1 {
		t.Fatalf("recovered copies = %v, want one", recovered)
	}
	if data, err := os.ReadFile(recovered[0]); err != nil || string(data) != "R" {
		t.Fatalf("recovered copy = %q (%v), want R", data, err)
	}

	body := script.only(t, "/v1/artifacts-written").Body
	if body["consultId"] != c.ID {
		t.Fatalf("artifacts-written consultId = %v, want %s", body["consultId"], c.ID)
	}
	receipts := consultTestReceipts(t, body)
	if len(receipts) != 4 {
		t.Fatalf("receipts = %v, want four", receipts)
	}
	wantIDs := []string{"aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb", "cccccccccccccccc", "nothex"}
	for index, want := range wantIDs {
		if receipts[index]["artifactId"] != want {
			t.Fatalf("receipt %d artifactId = %v, want %s", index, receipts[index]["artifactId"], want)
		}
	}
	for index, pair := range []struct {
		file    string
		content string
	}{
		{versions[0], "S"},
		{recovered[0], "R"},
	} {
		receipt := receipts[index]
		if receipt["version"] != filepath.Base(pair.file) {
			t.Fatalf("receipt %d version = %v, want %s", index, receipt["version"], filepath.Base(pair.file))
		}
		sum := sha256.Sum256([]byte(pair.content))
		if receipt["sha256"] != hex.EncodeToString(sum[:]) {
			t.Fatalf("receipt %d sha256 = %v, want %s", index, receipt["sha256"], hex.EncodeToString(sum[:]))
		}
		if receipt["error"] != nil {
			t.Fatalf("receipt %d error = %v, want null", index, receipt["error"])
		}
	}
	if receipts[2]["error"] != "unknown root" {
		t.Fatalf("receipt 2 error = %v, want unknown root", receipts[2]["error"])
	}
	if receipts[3]["error"] != "invalid artifactId" {
		t.Fatalf("receipt 3 error = %v, want invalid artifactId", receipts[3]["error"])
	}
}

// TestConsultRunawayBatches bounds a service that keeps answering pending.
func TestConsultRunawayBatches(t *testing.T) {
	root := consultTestRoot(t)
	consultWriteFile(t, root, "a.html", []byte("<p>a</p>"))
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/admit" || path == "/v1/artifacts-written" {
			return map[string]any{"pending": true, "artifacts": []any{}}
		}
		return nil
	})
	link := consultTestLink(t, script)
	link.setRoot("root-0", root)
	c := link.begin("4444444444444444", nil, "root-0", root, "a.html", filepath.Join(root, "a.html"), "save", "browser")

	if _, err := c.admit(admitArgs{Incoming: []byte("<p>a</p>")}); err == nil {
		t.Fatal("a service that never stops answering pending was accepted")
	}
	if calls := script.count("/v1/artifacts-written"); calls != maxPendingBatches {
		t.Fatalf("artifacts-written called %d times, want %d", calls, maxPendingBatches)
	}
}

// TestConsultSettle is the confirmation point: identity sent with a landed
// write, and a failed settle that bars the document and retries its abort.
func TestConsultSettle(t *testing.T) {
	t.Run("committed", func(t *testing.T) {
		root := consultTestRoot(t)
		consultWriteFile(t, root, "a.html", []byte("<p>a</p>"))
		script := newConsultScript(func(path string, body map[string]any, call int) any {
			if path == "/v1/settle" {
				return map[string]any{"ok": true}
			}
			return nil
		})
		link := consultTestLink(t, script)
		link.setRoot("root-0", root)
		filePath := filepath.Join(root, "a.html")
		c := link.begin("5555555555555555", nil, "root-0", root, "a.html", filePath, "save", "browser")

		if err := c.settle(settleArgs{Outcome: "committed", Sender: "server-save"}); err != nil {
			t.Fatalf("settle: %v", err)
		}
		body := script.only(t, "/v1/settle").Body
		identity := consultTestObject(t, body["identity"], "identity")
		ino, _ := identity["ino"].(string)
		if ino == "" {
			t.Fatalf("identity.ino = %v, want the file's inode", identity["ino"])
		}
		if link.barred(filePath) {
			t.Fatal("a confirmed write left the document barred")
		}
	})

	t.Run("settle-failed", func(t *testing.T) {
		previous := abortRetryInterval
		abortRetryInterval = 10 * time.Millisecond
		t.Cleanup(func() { abortRetryInterval = previous })

		root := consultTestRoot(t)
		consultWriteFile(t, root, "b.html", []byte("<p>b</p>"))
		script := newConsultScript(func(path string, body map[string]any, call int) any {
			if path == "/v1/settle" {
				return map[string]any{"ok": false, "error": "x"}
			}
			return nil
		})
		link := consultTestLink(t, script)
		link.setRoot("root-0", root)
		filePath := filepath.Join(root, "b.html")
		c := link.begin("6666666666666666", nil, "root-0", root, "b.html", filePath, "save", "browser")

		if err := c.settle(settleArgs{Outcome: "committed", Sender: "server-save"}); err == nil {
			t.Fatal("a failed settle was accepted")
		}
		if !link.barred(filePath) {
			t.Fatal("a failed settle after the bytes landed left the document unbarred")
		}
		deadline := time.Now().Add(time.Second)
		for {
			found := script.find("/v1/abort")
			if len(found) > 0 {
				body := found[0].Body
				if body["reason"] != "settle-failed" || body["consultId"] != c.ID {
					t.Fatalf("abort body = %v, want settle-failed for %s", body, c.ID)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("no abort reached the service within a second")
			}
			time.Sleep(5 * time.Millisecond)
		}
	})

	t.Run("failed", func(t *testing.T) {
		previous := abortRetryInterval
		abortRetryInterval = 10 * time.Millisecond
		t.Cleanup(func() { abortRetryInterval = previous })

		root := consultTestRoot(t)
		consultWriteFile(t, root, "c.html", []byte("<p>c</p>"))
		script := newConsultScript(func(path string, body map[string]any, call int) any {
			if path == "/v1/settle" {
				return map[string]any{"ok": false, "error": "x"}
			}
			return nil
		})
		link := consultTestLink(t, script)
		link.setRoot("root-0", root)
		filePath := filepath.Join(root, "c.html")
		c := link.begin("7777777777777777", nil, "root-0", root, "c.html", filePath, "save", "browser")

		if err := c.settle(settleArgs{Outcome: "failed", Sender: "server-save"}); err == nil {
			t.Fatal("a failed settle was accepted")
		}
		if link.barred(filePath) {
			t.Fatal("a write that never landed barred the document")
		}
		deadline := time.Now().Add(time.Second)
		for {
			found := script.find("/v1/abort")
			if len(found) > 0 {
				body := found[0].Body
				if body["reason"] != "failed" || body["consultId"] != c.ID {
					t.Fatalf("abort body = %v, want failed for %s", body, c.ID)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("no abort reached the service within a second")
			}
			time.Sleep(5 * time.Millisecond)
		}
	})
}

// TestRebaseVersions is the stale-save history the service rebases onto: only
// the version files at the incoming page's rev, and only when the incoming page
// is the case Local's rebase handles.
func TestRebaseVersions(t *testing.T) {
	root := consultTestRoot(t)
	stored := saveGoldenPage("bbbbbbbbbbbbbbbb")
	version := saveGoldenPage("aaaaaaaaaaaaaaaa")
	consultWriteFile(t, root, "stale.html", []byte(stored))
	consultWriteFile(t, root, filepath.Join(".hyperclay", "page-state", "stale.html.json"),
		[]byte(`{"v":1,"rev":"bbbbbbbbbbbbbbbb","tag":`+mustJSON(saveGoldenYJSTag)+`}`))
	consultWriteFile(t, root, filepath.Join(".hyperclay", "versions", "stale", "2026-09-01-00-00-00-000+0000.html"), []byte(version))

	found := rebaseVersions(root, "stale.html", version)
	if len(found) != 1 {
		t.Fatalf("rebaseVersions at the version's rev = %v, want that one version", found)
	}
	if found[0][0].Value != "2026-09-01-00-00-00-000+0000.html" {
		t.Fatalf("version name = %v", found[0][0].Value)
	}
	payload, ok := found[0][1].Value.(orderedObject)
	if !ok || len(payload) != 2 || payload[0].Value != "utf8" || payload[1].Value != version {
		t.Fatalf("version bytes = %v, want %q as utf8", found[0][1].Value, version)
	}

	if found := rebaseVersions(root, "stale.html", stored); len(found) != 0 {
		t.Fatalf("rebaseVersions at the stored rev = %v, want none", found)
	}

	withState := strings.Replace(version, "<article", saveGoldenYJSTag+"\n<article", 1)
	if _, state := splitPage(withState); state == nil {
		t.Fatalf("the fixture carries no page state: %q", withState)
	}
	if found := rebaseVersions(root, "stale.html", withState); len(found) != 0 {
		t.Fatalf("rebaseVersions for a page carrying state = %v, want none", found)
	}

	bare := consultTestRoot(t)
	consultWriteFile(t, bare, "stale.html", []byte(stored))
	if found := rebaseVersions(bare, "stale.html", version); len(found) != 0 {
		t.Fatalf("rebaseVersions with no stored record = %v, want none", found)
	}
}

// TestPublishRecoveredCopy is Local's recovered-copy writer: one version-styled
// file under the document's recovered folder, and no path out of it.
func TestPublishRecoveredCopy(t *testing.T) {
	root := consultTestRoot(t)
	published, err := publishRecoveredCopy(root, "sub/x.html", []byte("<p>r</p>"))
	if err != nil {
		t.Fatalf("publishRecoveredCopy: %v", err)
	}
	if !strings.HasSuffix(published.Filename, ".html") || !IsVersionName(published.Filename) {
		t.Fatalf("published name = %q, want a version name", published.Filename)
	}
	want := upathJoin(root, ".hyperclay", "recovered", "sub", "x", published.Filename)
	if published.Full != want {
		t.Fatalf("published.Full = %q, want %q", published.Full, want)
	}
	if data, err := os.ReadFile(want); err != nil || string(data) != "<p>r</p>" {
		t.Fatalf("recovered copy = %q (%v)", data, err)
	}

	for _, rel := range []string{"../x.html", ".hidden/x.html", "x.txt", `a\b.html`} {
		if _, err := publishRecoveredCopy(root, rel, []byte("<p>r</p>")); err == nil {
			t.Fatalf("publishRecoveredCopy accepted %q", rel)
		}
	}

	outside := consultTestRoot(t)
	symlinked := consultTestRoot(t)
	if err := os.MkdirAll(filepath.Join(symlinked, ".hyperclay"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(symlinked, ".hyperclay", "recovered")); err != nil {
		t.Fatal(err)
	}
	if _, err := publishRecoveredCopy(symlinked, "x.html", []byte("<p>r</p>")); err == nil {
		t.Fatal("publishRecoveredCopy wrote through a symlinked recovered folder")
	}
}

// TestControlReadSidecarIsStoredRecord is the read's sidecar as Local's
// page-state readers use it: the whole stored record, its lineage included.
func TestControlReadSidecarIsStoredRecord(t *testing.T) {
	root := controlTestRoot(t, "root-0")
	server := controlTestServer(t, root)

	rev := "093360993130d818"
	previousRev := "1111111111111111"
	consultWriteFile(t, root.baseDir, "page.html", []byte(saveGoldenPage(rev)))
	consultWriteFile(t, root.baseDir, filepath.Join(".hyperclay", "page-state", "page.html.json"),
		[]byte(`{"v":1,"rev":"`+rev+`","tag":`+mustJSON(saveGoldenYJSTag)+`,"generation":"`+strings.Repeat("a", 32)+`","prev":{"rev":"`+previousRev+`","tag":`+mustJSON(saveGoldenYJSTag)+`,"generation":"`+strings.Repeat("b", 32)+`"}}`))

	status, answer := controlJSON(t, server, controlSecret, http.MethodGet, "/v1/read?root=root-0&name=page.html", nil)
	if status != http.StatusOK {
		t.Fatalf("read page.html = %d %v, want 200", status, answer)
	}
	sidecar := consultTestObject(t, answer["sidecar"], "sidecar")
	if sidecar["v"] != float64(1) || sidecar["rev"] != rev || sidecar["tag"] != saveGoldenYJSTag {
		t.Fatalf("sidecar = %v, want the stored record", sidecar)
	}
	if sidecar["generation"] != strings.Repeat("a", 32) {
		t.Fatalf("sidecar.generation = %v, want the stored generation", sidecar["generation"])
	}
	prev := consultTestObject(t, sidecar["prev"], "sidecar.prev")
	if prev["rev"] != previousRev || prev["generation"] != strings.Repeat("b", 32) {
		t.Fatalf("sidecar.prev = %v, want the stored previous record", prev)
	}
}

//go:build managedtrial

package managedtrial

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// errKilled is what a test's killSelf panics with, so a write that reaches a
// kill point unwinds to the test instead of ending the test binary.
var errKilled = errors.New("killed")

// replaceKillSelf makes every fault's kill point panic, and restores the real
// one with the fault list when the test ends.
func replaceKillSelf(t *testing.T) {
	t.Helper()
	original := killSelf
	killSelf = func() { panic(errKilled) }
	t.Cleanup(func() {
		killSelf = original
		clearFaults()
	})
}

// faultFileContent is one file's bytes, read after a faulted write.
func faultFileContent(t *testing.T, filePath string) string {
	t.Helper()
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read %s: %v", filePath, err)
	}
	return string(content)
}

// expectKill runs a write that must reach a kill point: the sentinel panic is
// the only way it may end.
func expectKill(t *testing.T, write func() error) {
	t.Helper()
	killed := false
	func() {
		defer func() {
			recovered := recover()
			if recovered == errKilled {
				killed = true
				return
			}
			if recovered != nil {
				panic(recovered)
			}
		}()
		if err := write(); err != nil {
			t.Errorf("write returned %v before the kill", err)
		}
	}()
	if !killed {
		t.Fatal("write finished without being killed")
	}
}

// TestFaultDurableFail is P9's durable case: an injected directory-sync failure
// after the rename reports the write indeterminate, the bytes landed, the fault
// is spent, and the next durable write succeeds.
func TestFaultDurableFail(t *testing.T) {
	replaceKillSelf(t)
	dir := t.TempDir()
	docPath := filepath.Join(dir, "doc.html")

	armFault(faultDurableFail, "doc.html", 1)

	if err := atomicWriteFile(docPath, []byte("loose"), nil, false); err != nil {
		t.Fatalf("non-durable write: err = %v, want nil", err)
	}
	if content := faultFileContent(t, docPath); content != "loose" {
		t.Fatalf("non-durable write: content = %q, want %q", content, "loose")
	}

	err := atomicWriteFile(docPath, []byte("kept"), nil, true)
	var afterRename *AfterRenameError
	if !errors.As(err, &afterRename) {
		t.Fatalf("durable write under the fault: err = %v, want an *AfterRenameError", err)
	}
	if content := faultFileContent(t, docPath); content != "kept" {
		t.Fatalf("durable write under the fault: content = %q, want %q", content, "kept")
	}

	if err := atomicWriteFile(docPath, []byte("later"), nil, true); err != nil {
		t.Fatalf("durable write after the fault: err = %v, want nil", err)
	}
	if content := faultFileContent(t, docPath); content != "later" {
		t.Fatalf("durable write after the fault: content = %q, want %q", content, "later")
	}
}

// TestFaultKillBeforeRename is P12's first half: a kill just before the rename
// leaves the old file whole.
func TestFaultKillBeforeRename(t *testing.T) {
	replaceKillSelf(t)
	dir := t.TempDir()
	docPath := filepath.Join(dir, "doc.html")

	if err := atomicWriteFile(docPath, []byte("old"), nil, true); err != nil {
		t.Fatalf("first write: err = %v, want nil", err)
	}
	armFault(faultKillBeforeRename, "doc.html", 1)

	expectKill(t, func() error { return atomicWriteFile(docPath, []byte("new"), nil, true) })

	if content := faultFileContent(t, docPath); content != "old" {
		t.Fatalf("kill before the rename: content = %q, want the old %q", content, "old")
	}
}

// TestFaultKillAfterRename is P12's second half: a kill just after the rename
// leaves the new file whole.
func TestFaultKillAfterRename(t *testing.T) {
	replaceKillSelf(t)
	dir := t.TempDir()
	docPath := filepath.Join(dir, "doc.html")

	if err := atomicWriteFile(docPath, []byte("old"), nil, true); err != nil {
		t.Fatalf("first write: err = %v, want nil", err)
	}
	armFault(faultKillAfterRename, "doc.html", 1)

	expectKill(t, func() error { return atomicWriteFile(docPath, []byte("new"), nil, true) })

	if content := faultFileContent(t, docPath); content != "new" {
		t.Fatalf("kill after the rename: content = %q, want the new %q", content, "new")
	}
}

// TestFaultMatchesLogicalPath proves a fault names a document, not every path
// that merely ends the same way: the sidecar and a longer name are untouched,
// and a nested name fires for its own file.
func TestFaultMatchesLogicalPath(t *testing.T) {
	clearFaults()
	t.Cleanup(clearFaults)
	dir := t.TempDir()

	armFault(faultKillBeforeRename, "doc.html", 1)
	if takeFault(faultKillBeforeRename, filepath.Join(dir, ".hyperclay", "page-state", "doc.html.json")) {
		t.Error("a fault for doc.html fired for its page-state sidecar")
	}
	if takeFault(faultKillBeforeRename, filepath.Join(dir, "xdoc.html")) {
		t.Error("a fault for doc.html fired for xdoc.html")
	}
	if !takeFault(faultKillBeforeRename, filepath.Join(dir, "doc.html")) {
		t.Error("a fault for doc.html did not fire for the document")
	}

	armFault(faultKillBeforeRename, "sub/doc.html", 1)
	if !takeFault(faultKillBeforeRename, filepath.Join(dir, "sub", "doc.html")) {
		t.Error("a fault for sub/doc.html did not fire for the nested document")
	}
}

// TestFaultDropReply is P12's lost answer: a commit whose reply never arrives
// wrote its bytes, and /v1/op reconciles it before any retry.
func TestFaultDropReply(t *testing.T) {
	clearFaults()
	t.Cleanup(clearFaults)
	root := controlTestRoot(t, "root-0")
	server := controlTestServer(t, root)

	doc := "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<title>Notes</title>\n</head>\n<body>\n<h1>Notes</h1>\n</body>\n</html>\n"
	status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"opId":  "aaaaaaaaaaaaaaaa",
		"root":  "root-0",
		"name":  "notes.html",
		"kind":  "create",
		"bytes": map[string]any{"encoding": "utf8", "data": doc},
	})
	if status != http.StatusOK || answer["outcome"] != "committed" {
		t.Fatalf("create notes.html: status = %d, answer = %v", status, answer)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/fault", map[string]any{
		"point": "drop-reply",
		"rel":   "notes.html",
	})
	if status != http.StatusOK || answer["armed"] != float64(1) {
		t.Fatalf("arm drop-reply: status = %d, answer = %v, want armed 1", status, answer)
	}

	opID := "bbbbbbbbbbbbbbbb"
	saved := doc + "<!-- saved -->\n"
	payload, err := json.Marshal(map[string]any{
		"opId":  opID,
		"root":  "root-0",
		"name":  "notes.html",
		"kind":  "browser-save",
		"bytes": map[string]any{"encoding": "utf8", "data": saved},
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/commit", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+controlSecret)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err == nil {
		response.Body.Close()
		t.Fatalf("commit under drop-reply answered %d, want a transport error", response.StatusCode)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodGet, "/v1/op/"+opID, nil)
	if status != http.StatusOK || answer["state"] != "committed" {
		t.Fatalf("op %s: status = %d, answer = %v, want a committed op", opID, status, answer)
	}
	if content := faultFileContent(t, filepath.Join(root.baseDir, "notes.html")); content != saved {
		t.Fatalf("drop-reply: content = %q, want the saved bytes", content)
	}
}

// TestFaultRoute is the route's own contract: an unknown point, an empty rel
// and an out-of-range count are refused, clear disarms, and the control
// listener's bearer check covers it.
func TestFaultRoute(t *testing.T) {
	clearFaults()
	t.Cleanup(clearFaults)
	root := controlTestRoot(t, "root-0")
	server := controlTestServer(t, root)

	if status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/fault", map[string]any{
		"point": "no-such-point", "rel": "a.html",
	}); status != http.StatusBadRequest {
		t.Fatalf("unknown point: status = %d, answer = %v, want 400", status, answer)
	}
	if status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/fault", map[string]any{
		"point": "durable-fail", "rel": "a.html", "count": 0,
	}); status != http.StatusBadRequest {
		t.Fatalf("count 0: status = %d, answer = %v, want 400", status, answer)
	}
	if status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/fault", map[string]any{
		"point": "clear",
	}); status != http.StatusOK || answer["armed"] != float64(0) {
		t.Fatalf("clear: status = %d, answer = %v, want armed 0", status, answer)
	}
	if status, answer := controlJSON(t, server, "", http.MethodPost, "/v1/fault", map[string]any{
		"point": "durable-fail", "rel": "a.html",
	}); status != http.StatusUnauthorized {
		t.Fatalf("no bearer: status = %d, answer = %v, want 401", status, answer)
	}
}

// tailwindCompileRecorder records the documents the pinned compiler was asked
// about, so a test can say whether and for which bytes it ran.
type tailwindCompileRecorder struct {
	mu       sync.Mutex
	compiled []string
}

func recordTailwindCompiles(t *testing.T) *tailwindCompileRecorder {
	t.Helper()
	recorder := &tailwindCompileRecorder{}
	previous := tailwindCompiler
	tailwindCompiler = func(localSource, html string) (string, error) {
		recorder.mu.Lock()
		recorder.compiled = append(recorder.compiled, html)
		recorder.mu.Unlock()
		return tailwindCSSFor(html), nil
	}
	t.Cleanup(func() { tailwindCompiler = previous })
	return recorder
}

func (r *tailwindCompileRecorder) saw(html string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, compiled := range r.compiled {
		if compiled == html {
			return true
		}
	}
	return false
}

func (r *tailwindCompileRecorder) waitFor(t *testing.T, html string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !r.saw(html) {
		if time.Now().After(deadline) {
			t.Fatalf("the compiler was not called for the expected document within 2s")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// tailwindHoldFixture serves one root with a document linking
// /tailwindcss/site.css, and a control server to arm and release the hold.
func tailwindHoldFixture(t *testing.T) (root, address string, port int, control *httptest.Server) {
	t.Helper()
	clearFaults()
	t.Cleanup(clearFaults)
	localSource := requireTailwindLocalSource(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, root, "site.html", []byte(tailwindSitePage("base")))
	handler, err := newRootHandler(rootSpec{id: "root-tailwind-hold", path: root}, config{localSource: localSource})
	if err != nil {
		t.Fatal(err)
	}
	address, port = serveTestServer(t, handler)
	control = controlTestServer(t, controlTestRoot(t, "root-tailwind-hold-control"))
	return root, address, port, control
}

func waitForHeldTailwind(t *testing.T, control *httptest.Server, name string, want float64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/fault", map[string]any{
			"point": "held", "rel": name,
		})
		if status != http.StatusOK {
			t.Fatalf("held %s: status = %d, answer = %v", name, status, answer)
		}
		if answer["held"] == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("held %s = %v, want %v within 2s", name, answer["held"], want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func tailwindSitePage(classes string) string {
	return strings.Replace(tailwindPage(classes), "/tailwindcss/tw.css", "/tailwindcss/site.css", 1)
}

// TestFaultTailwindHold is P11 in the real binary: a held compile lets the save
// answer with the old stylesheet, and releasing the hold lets it install.
func TestFaultTailwindHold(t *testing.T) {
	root, address, port, control := tailwindHoldFixture(t)
	recorder := recordTailwindCompiles(t)
	first := tailwindSitePage("one")

	if status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/fault", map[string]any{
		"point": "tailwind-hold", "rel": "site",
	}); status != http.StatusOK || answer["armed"] != float64(1) {
		t.Fatalf("arm tailwind-hold: status = %d, answer = %v, want armed 1", status, answer)
	}
	tailwindSave(t, address, port, "site.html", first)
	waitForHeldTailwind(t, control, "site", 1)
	if recorder.saw(first) {
		t.Fatal("the held compile called the compiler before it was released")
	}

	if status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/fault", map[string]any{
		"point": "release", "rel": "site",
	}); status != http.StatusOK || answer["released"] != float64(1) {
		t.Fatalf("release site: status = %d, answer = %v, want released 1", status, answer)
	}
	recorder.waitFor(t, first)
	waitForTailwindCSS(t, tailwindCSSPath(root, "site"), tailwindCSSFor(first))
}

// TestFaultTailwindHoldOlderCompileLoses is P11's ordering rule in the real
// binary: an older compile released after a newer one installed writes nothing.
func TestFaultTailwindHoldOlderCompileLoses(t *testing.T) {
	root, address, port, control := tailwindHoldFixture(t)
	recorder := recordTailwindCompiles(t)
	first, second := tailwindSitePage("one"), tailwindSitePage("two")

	if status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/fault", map[string]any{
		"point": "tailwind-hold", "rel": "site", "count": 1,
	}); status != http.StatusOK || answer["armed"] != float64(1) {
		t.Fatalf("arm tailwind-hold: status = %d, answer = %v, want armed 1", status, answer)
	}
	tailwindSave(t, address, port, "site.html", first)
	waitForHeldTailwind(t, control, "site", 1)

	tailwindSave(t, address, port, "site.html", second)
	waitForTailwindCSS(t, tailwindCSSPath(root, "site"), tailwindCSSFor(second))

	if status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/fault", map[string]any{
		"point": "release", "rel": "site",
	}); status != http.StatusOK || answer["released"] != float64(1) {
		t.Fatalf("release site: status = %d, answer = %v, want released 1", status, answer)
	}
	recorder.waitFor(t, first)
	time.Sleep(200 * time.Millisecond)

	if got := readTailwindCSS(t, root, "site"); got != tailwindCSSFor(second) {
		t.Errorf("stylesheet = %q, want the newer save's %q", got, tailwindCSSFor(second))
	}
}

// TestFaultTailwindHoldClear is clear's contract over a held compile: clearing
// every fault releases it.
func TestFaultTailwindHoldClear(t *testing.T) {
	_, address, port, control := tailwindHoldFixture(t)
	recorder := recordTailwindCompiles(t)
	first := tailwindSitePage("one")

	if status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/fault", map[string]any{
		"point": "tailwind-hold", "rel": "site",
	}); status != http.StatusOK || answer["armed"] != float64(1) {
		t.Fatalf("arm tailwind-hold: status = %d, answer = %v, want armed 1", status, answer)
	}
	tailwindSave(t, address, port, "site.html", first)
	waitForHeldTailwind(t, control, "site", 1)

	if status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/fault", map[string]any{
		"point": "clear",
	}); status != http.StatusOK || answer["armed"] != float64(0) {
		t.Fatalf("clear: status = %d, answer = %v, want armed 0", status, answer)
	}
	recorder.waitFor(t, first)
	waitForHeldTailwind(t, control, "site", 0)
}

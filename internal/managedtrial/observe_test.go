//go:build managedtrial

package managedtrial

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/specwire"
)

// observeHost is one served root with a service attached, and its commit host.
func observeHost(t *testing.T, script *consultScript) (*commitHost, string) {
	t.Helper()
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	return handler.commit, dir
}

// observePageDocument is one page document's logical form and the file it
// splits into, from the recorded ClayJS golden: the logical carries the page
// tag and the file does not, so the two differ.
func observePageDocument(t *testing.T) (string, string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "pagesplit-golden", "golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden pageSplitGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if len(golden.Cases) == 0 {
		t.Fatal("golden.json holds no cases")
	}
	return pageGoldenInput(t, golden.Cases[0].Input), pageGoldenText(t, golden.Cases[0].Split.File)
}

// TestObserveOutsideNone is an outside edit the service judges its own or
// already handled: the snapshot the watcher sends, and nothing written.
func TestObserveOutsideNone(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/observe" {
			return map[string]any{"action": "none"}
		}
		return nil
	})
	host, dir := observeHost(t, script)
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	outside := consultSavePage[:len(consultSavePage)-len("</html>\n")] + "  <p>outside</p>\n</html>\n"
	consultWriteFile(t, dir, "page.html", []byte(outside))

	host.observeOutside("change", "page.html")

	observe := script.only(t, "/v1/observe").Body
	for key, want := range map[string]any{
		"event": "change", "name": "page.html", "root": "root-consult",
		"lane": "observe", "origin": "external",
		"canonicalPath": upathJoin(dir, "page.html"),
	} {
		if observe[key] != want {
			t.Fatalf("observe %s = %v, want %v", key, observe[key], want)
		}
	}
	current := consultTestObject(t, observe["current"], "current")
	if current["exists"] != true {
		t.Fatalf("current exists = %v, want true", current["exists"])
	}
	if want := specwire.Etag([]byte(outside)); current["checksum"] != want {
		t.Fatalf("current checksum = %v, want %s", current["checksum"], want)
	}
	if count := script.count("/v1/admit"); count != 0 {
		t.Fatalf("admit was called %d times, want none", count)
	}
	if disk := consultTestDisk(t, filepath.Join(dir, "page.html")); disk != outside {
		t.Fatalf("the document on disk is %q, want the outside bytes", disk)
	}
}

// TestObserveOutsideRebase is a rebase answer: the external-rebase lane runs
// against the judged checksum, writes the answered file and settles as the
// file watcher's.
func TestObserveOutsideRebase(t *testing.T) {
	consultShortAborts(t)
	logical, file := observePageDocument(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/observe":
			return map[string]any{
				"action":  "rebase",
				"logical": map[string]any{"encoding": "utf8", "data": logical},
			}
		case "/v1/admit":
			return map[string]any{
				"decision": "proceed",
				"logical":  map[string]any{"encoding": "utf8", "data": logical},
				"file":     map[string]any{"encoding": "utf8", "data": file},
			}
		case "/v1/settle":
			return map[string]any{"ok": true}
		}
		return nil
	})
	host, dir := observeHost(t, script)
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))

	host.observeOutside("change", "page.html")

	admit := script.only(t, "/v1/admit").Body
	for key, want := range map[string]any{"lane": "rebase", "origin": "external"} {
		if admit[key] != want {
			t.Fatalf("admit %s = %v, want %v", key, admit[key], want)
		}
	}
	if disk := consultTestDisk(t, filepath.Join(dir, "page.html")); disk != file {
		t.Fatalf("the document on disk is %q, want the answered file", disk)
	}
	settle := script.only(t, "/v1/settle").Body
	if settle["sender"] != "file-watcher" {
		t.Fatalf("settle sender = %v, want file-watcher", settle["sender"])
	}
	if settle["outcome"] != "committed" {
		t.Fatalf("settle outcome = %v, want committed", settle["outcome"])
	}
}

// TestObserveOutsideChangedMeanwhile is the edit that lands between the
// snapshot and the rebase: the lane refuses `changed` and nothing is admitted.
func TestObserveOutsideChangedMeanwhile(t *testing.T) {
	consultShortAborts(t)
	logical, _ := observePageDocument(t)
	other := consultSavePage[:len(consultSavePage)-len("</html>\n")] + "  <p>later</p>\n</html>\n"
	var dir string
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path != "/v1/observe" {
			return nil
		}
		if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(other), 0o644); err != nil {
			t.Errorf("rewriting the document: %v", err)
		}
		return map[string]any{
			"action":  "rebase",
			"logical": map[string]any{"encoding": "utf8", "data": logical},
		}
	})
	host, root := observeHost(t, script)
	dir = root
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))

	host.observeOutside("change", "page.html")

	if count := script.count("/v1/admit"); count != 0 {
		t.Fatalf("admit was called %d times, want none", count)
	}
	if disk := consultTestDisk(t, filepath.Join(dir, "page.html")); disk != other {
		t.Fatalf("the document on disk is %q, want the bytes that landed meanwhile", disk)
	}
}

// TestObserveOutsideUnlink is a removal: the snapshot says the document is not
// there.
func TestObserveOutsideUnlink(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/observe" {
			return map[string]any{"action": "none"}
		}
		return nil
	})
	host, dir := observeHost(t, script)
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	if err := os.Remove(filepath.Join(dir, "page.html")); err != nil {
		t.Fatal(err)
	}

	host.observeOutside("unlink", "page.html")

	observe := script.only(t, "/v1/observe").Body
	if observe["event"] != "unlink" {
		t.Fatalf("observe event = %v, want unlink", observe["event"])
	}
	current := consultTestObject(t, observe["current"], "current")
	if current["exists"] != false {
		t.Fatalf("current exists = %v, want false", current["exists"])
	}
}

// TestObserveOutsideNotDocument is any other file: nothing is sent.
func TestObserveOutsideNotDocument(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		return map[string]any{"action": "none"}
	})
	host, dir := observeHost(t, script)
	consultWriteFile(t, dir, "notes.txt", []byte("notes"))

	host.observeOutside("change", "notes.txt")

	if found := script.find("/v1/observe"); len(found) != 0 {
		t.Fatalf("observe was called with %v, want nothing", found)
	}
}

// TestObserveOutsideRecoveredCopy is the pending batch before the answer: the
// room's unsaved edits land as a recovered copy under the root's folder, and
// the receipt names what was written.
func TestObserveOutsideRecoveredCopy(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/observe":
			return map[string]any{
				"pending": true,
				"artifacts": []any{
					map[string]any{
						"artifactId": "aaaaaaaaaaaaaaaa", "type": "recovered-copy",
						"root": "root-consult", "name": "page.html",
						"logical": map[string]any{"encoding": "utf8", "data": "<p>unsaved</p>"},
					},
				},
			}
		case "/v1/artifacts-written":
			return map[string]any{"action": "none"}
		}
		return nil
	})
	host, dir := observeHost(t, script)
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))

	host.observeOutside("change", "page.html")

	copies := consultTestFiles(t, filepath.Join(dir, ".hyperclay", "recovered", "page"))
	if len(copies) != 1 {
		t.Fatalf("recovered copies = %v, want one", copies)
	}
	if data := consultTestDisk(t, copies[0]); data != "<p>unsaved</p>" {
		t.Fatalf("recovered copy = %q, want the unsaved edits", data)
	}
	receipts := consultTestReceipts(t, script.only(t, "/v1/artifacts-written").Body)
	if len(receipts) != 1 {
		t.Fatalf("receipts = %v, want one", receipts)
	}
	if receipts[0]["version"] != filepath.Base(copies[0]) {
		t.Fatalf("receipt version = %v, want %s", receipts[0]["version"], filepath.Base(copies[0]))
	}
	sum := sha256.Sum256([]byte("<p>unsaved</p>"))
	if receipts[0]["sha256"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("receipt sha256 = %v, want %s", receipts[0]["sha256"], hex.EncodeToString(sum[:]))
	}
}

// TestWatcherPublishesToObserve is the wiring: a reported document reaches the
// service, and anything else does not.
func TestWatcherPublishesToObserve(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/observe" {
			return map[string]any{"action": "none"}
		}
		return nil
	})
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	consultWriteFile(t, dir, "sub/kept.txt", []byte("kept"))
	watcher := handler.control.watcher
	known := watcher.snapshot()

	watcher.publish("change", "page.html", known["page.html"])
	watcher.publish("addDir", "sub", known["sub"])

	if count := script.count("/v1/observe"); count != 1 {
		t.Fatalf("observe was called %d times, want once", count)
	}
	observe := script.only(t, "/v1/observe").Body
	if observe["event"] != "change" || observe["name"] != "page.html" {
		t.Fatalf("observe = %v, want the page document's change", observe)
	}
}

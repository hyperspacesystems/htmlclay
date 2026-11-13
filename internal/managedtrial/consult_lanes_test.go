//go:build managedtrial

package managedtrial

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// consultLaneFile is the file part the replace and rebase lanes store, and the
// document on disk once the split page state is removed.
const consultLaneFile = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Lane</title>
</head>
<body>
<article class="blockeditor" clay="freeze" data-blockeditor="1" data-rev="093360993130d818"><h1 data-id="title"></h1></article>
</body>
</html>
`

// consultLaneTag is the page state of consultLaneLogical.
const consultLaneTag = `<script type="application/json" merge="blockeditor" clay="live">{"v":1,"g":0,"y":{"seed":"AAACQQMEAQAIQgALJwAoAQUAJwAoAIUkHGF0dHJzdGl0bGVsZXZlbHRmbG93YXR0cnNwMXRFAQEEBQIBBwEAAAEBAQACQQACQQEBBwB9AXcHaGVhZGluZ3YBAWJ3BXRpdGxldwlwYXJhZ3JhcGh2AQFidwJwMQA="}}</script>`

// consultLaneLogical is the document the service answers a replace with: the
// file part plus the page state the split takes off.
var consultLaneLogical = strings.Replace(consultLaneFile, "<article", consultLaneTag+"\n<article", 1)

// consultLaneCommit posts one control API commit.
func consultLaneCommit(t *testing.T, control *httptest.Server, body map[string]any) (int, map[string]any) {
	t.Helper()
	return controlJSON(t, control, controlSecret, http.MethodPost, "/v1/commit", body)
}

// consultLaneUTF8 is one utf8 payload.
func consultLaneUTF8(data string) map[string]any {
	return map[string]any{"encoding": "utf8", "data": data}
}

// consultLaneVersionFiles is one document's version files, oldest first.
func consultLaneVersionFiles(t *testing.T, dir string) []string {
	t.Helper()
	dir = filepath.Join(dir, versionsDirName, "page")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	names := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	files := make([]string, 0, len(names))
	for _, name := range names {
		files = append(files, filepath.Join(dir, name))
	}
	return files
}

// consultLaneRefusal answers a control commit with the refusal it carries.
func consultLaneRefusal(t *testing.T, answer map[string]any) map[string]any {
	t.Helper()
	if answer["outcome"] != "refused" {
		t.Fatalf("answer = %v, want a refusal", answer)
	}
	return consultTestObject(t, answer["refusal"], "refusal")
}

// TestConsultedReplaceWithSafety is the consulted replace: admit with edit and
// safety writes the service's own safety version, the lane stores the answered
// logical and file and skips its own normalization and safety backup, and
// settles with the reservation and the written file's identity.
func TestConsultedReplaceWithSafety(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/admit":
			return map[string]any{
				"pending": true,
				"artifacts": []any{map[string]any{
					"artifactId": "aaaaaaaaaaaaaaaa",
					"type":       "safety-version",
					"logical":    consultLaneUTF8("<p>safety</p>"),
				}},
			}
		case "/v1/artifacts-written":
			return map[string]any{
				"decision":    "proceed",
				"logical":     consultLaneUTF8(consultLaneLogical),
				"file":        consultLaneUTF8(consultLaneFile),
				"reservation": "r1",
				"artifacts":   []any{},
			}
		case "/v1/settle":
			return map[string]any{"ok": true}
		}
		return nil
	})
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	control := controlTestServer(t, handler.control)

	status, answer := consultLaneCommit(t, control, map[string]any{
		"root":   "root-consult",
		"name":   "page.html",
		"kind":   "restore",
		"edit":   true,
		"safety": true,
		"bytes":  consultLaneUTF8(consultSavePage),
	})
	if status != http.StatusOK || answer["outcome"] != "committed" {
		t.Fatalf("commit answered %d: %v, want committed", status, answer)
	}

	admit := script.only(t, "/v1/admit").Body
	for key, want := range map[string]any{"lane": "replace", "origin": "restore", "safety": true, "edit": true} {
		if admit[key] != want {
			t.Fatalf("admit %s = %v, want %v", key, admit[key], want)
		}
	}

	versions := consultLaneVersionFiles(t, dir)
	if len(versions) != 2 {
		t.Fatalf("version files = %v, want exactly two", versions)
	}
	if content := consultTestDisk(t, versions[0]); content != "<p>safety</p>" {
		t.Fatalf("the older version holds %q, want the safety artifact", content)
	}
	if content := consultTestDisk(t, versions[1]); content != consultLaneFile {
		t.Fatalf("the newer version holds %q, want the answered file", content)
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != consultLaneFile {
		t.Fatalf("page.html = %q, want the answered file", onDisk)
	}

	settle := script.only(t, "/v1/settle").Body
	for key, want := range map[string]any{
		"outcome": "committed", "reservation": "r1", "sender": "server-save",
	} {
		if settle[key] != want {
			t.Fatalf("settle %s = %v, want %v", key, settle[key], want)
		}
	}
	if settle["identity"] == nil {
		t.Fatalf("settle identity = nil, want the written file's identity")
	}
}

// TestConsultedReplaceRefused answers the service's refusal and leaves the
// document, its versions and the service's reservation alone.
func TestConsultedReplaceRefused(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/admit" {
			return map[string]any{
				"decision": "refuse",
				"status":   409,
				"code":     "peers-editing",
				"body":     map[string]any{"msg": "x", "code": "peers-editing"},
			}
		}
		return nil
	})
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	control := controlTestServer(t, handler.control)

	status, answer := consultLaneCommit(t, control, map[string]any{
		"root":   "root-consult",
		"name":   "page.html",
		"kind":   "restore",
		"safety": true,
		"bytes":  consultLaneUTF8("<p>restore</p>"),
	})
	if status != http.StatusOK {
		t.Fatalf("commit answered %d: %v", status, answer)
	}
	refusal := consultLaneRefusal(t, answer)
	if refusal["status"] != float64(409) || refusal["code"] != "peers-editing" {
		t.Fatalf("refusal = %v, want the service's status and code", refusal)
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != consultSavePage {
		t.Fatalf("page.html = %q, want the bytes unchanged", onDisk)
	}
	if versions := consultLaneVersionFiles(t, dir); len(versions) != 0 {
		t.Fatalf("version files = %v, want none", versions)
	}
	consultWaitFor(t, script, "/v1/abort")
	if abort := script.only(t, "/v1/abort").Body; abort["reason"] != "refused" {
		t.Fatalf("abort reason = %v, want refused", abort["reason"])
	}
	if count := script.count("/v1/settle"); count != 0 {
		t.Fatalf("settle was called %d times, want none", count)
	}
}

// TestConsultedRebase is the consulted external rebase: admit recovers the
// room's edits first, the answered bytes are written, and the write settles as
// the file watcher's.
func TestConsultedRebase(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/admit":
			return map[string]any{
				"decision": "proceed",
				"logical":  consultLaneUTF8(consultSavePage),
				"file":     consultLaneUTF8(consultSavePage),
			}
		case "/v1/settle":
			return map[string]any{"ok": true}
		}
		return nil
	})
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	control := controlTestServer(t, handler.control)

	status, answer := consultLaneCommit(t, control, map[string]any{
		"root":  "root-consult",
		"name":  "page.html",
		"kind":  "external-rebase",
		"bytes": consultLaneUTF8(consultSavePage),
	})
	if status != http.StatusOK || answer["outcome"] != "committed" {
		t.Fatalf("commit answered %d: %v, want committed", status, answer)
	}
	admit := script.only(t, "/v1/admit").Body
	for key, want := range map[string]any{"lane": "rebase", "origin": "external"} {
		if admit[key] != want {
			t.Fatalf("admit %s = %v, want %v", key, admit[key], want)
		}
	}
	if settle := script.only(t, "/v1/settle").Body; settle["sender"] != "file-watcher" {
		t.Fatalf("settle sender = %v, want file-watcher", settle["sender"])
	}
}

// TestConsultedExact is the consulted sync download: no admit, one settle as
// the sync engine's, and a failed settle leaves the document unconfirmed.
func TestConsultedExact(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/settle" {
			return map[string]any{"ok": false, "error": "x"}
		}
		return nil
	})
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	control := controlTestServer(t, handler.control)

	status, answer := consultLaneCommit(t, control, map[string]any{
		"root":  "root-consult",
		"name":  "page.html",
		"kind":  "sync-download",
		"bytes": consultLaneUTF8(consultSavePage),
	})
	if status != http.StatusOK || answer["outcome"] != "indeterminate" {
		t.Fatalf("commit answered %d: %v, want indeterminate", status, answer)
	}
	if count := script.count("/v1/admit"); count != 0 {
		t.Fatalf("admit was called %d times, want none", count)
	}
	settle := script.only(t, "/v1/settle").Body
	for key, want := range map[string]any{
		"lane": "exact", "origin": "sync-download", "sender": "sync-engine",
	} {
		if settle[key] != want {
			t.Fatalf("settle %s = %v, want %v", key, settle[key], want)
		}
	}
	if settle["etag"] != nil {
		t.Fatalf("settle etag = %v, want null", settle["etag"])
	}
	if settle["identity"] == nil {
		t.Fatalf("settle identity = nil, want the written file's identity")
	}

	status, answer = consultLaneCommit(t, control, map[string]any{
		"root":  "root-consult",
		"name":  "page.html",
		"kind":  "browser-save",
		"bytes": consultLaneUTF8(consultSavePage),
	})
	if status != http.StatusOK {
		t.Fatalf("second commit answered %d: %v", status, answer)
	}
	refusal := consultLaneRefusal(t, answer)
	if refusal["status"] != float64(http.StatusServiceUnavailable) || refusal["code"] != "unconfirmed" {
		t.Fatalf("refusal = %v, want 503 unconfirmed", refusal)
	}
}

// TestConsultedCreate is the consulted create: the page state and bytes land,
// then one settle as the save lane's; there is no admit.
func TestConsultedCreate(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/settle" {
			return map[string]any{"ok": true}
		}
		return nil
	})
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	control := controlTestServer(t, handler.control)

	status, answer := consultLaneCommit(t, control, map[string]any{
		"root":  "root-consult",
		"name":  "page.html",
		"kind":  "create",
		"bytes": consultLaneUTF8(consultSavePage),
	})
	if status != http.StatusOK || answer["outcome"] != "committed" {
		t.Fatalf("commit answered %d: %v, want committed", status, answer)
	}
	if count := script.count("/v1/admit"); count != 0 {
		t.Fatalf("admit was called %d times, want none", count)
	}
	settle := script.only(t, "/v1/settle").Body
	for key, want := range map[string]any{"lane": "create", "sender": "server-save"} {
		if settle[key] != want {
			t.Fatalf("settle %s = %v, want %v", key, settle[key], want)
		}
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != consultSavePage {
		t.Fatalf("page.html = %q, want the created document", onDisk)
	}
}

// TestConsultedExactBinarySkipsService writes a binary download no page
// document names, and reaches no consultation at all.
func TestConsultedExactBinarySkipsService(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		return nil
	})
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	control := controlTestServer(t, handler.control)

	content := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	status, answer := consultLaneCommit(t, control, map[string]any{
		"root":  "root-consult",
		"name":  "a.png",
		"kind":  "sync-download",
		"bytes": map[string]any{"encoding": "base64", "data": base64.StdEncoding.EncodeToString(content)},
	})
	if status != http.StatusOK || answer["outcome"] != "committed" {
		t.Fatalf("commit answered %d: %v, want committed", status, answer)
	}
	for _, path := range []string{"/v1/admit", "/v1/artifacts-written", "/v1/settle", "/v1/abort"} {
		if count := script.count(path); count != 0 {
			t.Fatalf("%s was called %d times, want none", path, count)
		}
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "a.png")); onDisk != string(content) {
		t.Fatalf("a.png = %q, want the downloaded bytes", onDisk)
	}
}

// TestConsultedUnconfirmedBarsReplace is the write after a failed exact settle:
// the document is unconfirmed, so a replace is refused before any admit.
func TestConsultedUnconfirmedBarsReplace(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/settle" {
			return map[string]any{"ok": false, "error": "x"}
		}
		return nil
	})
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	control := controlTestServer(t, handler.control)

	status, answer := consultLaneCommit(t, control, map[string]any{
		"root":  "root-consult",
		"name":  "page.html",
		"kind":  "sync-download",
		"bytes": consultLaneUTF8(consultSavePage),
	})
	if status != http.StatusOK || answer["outcome"] != "indeterminate" {
		t.Fatalf("exact commit answered %d: %v, want indeterminate", status, answer)
	}
	admitBefore := script.count("/v1/admit")

	status, answer = consultLaneCommit(t, control, map[string]any{
		"root":   "root-consult",
		"name":   "page.html",
		"kind":   "restore",
		"safety": true,
		"bytes":  consultLaneUTF8("<p>restore</p>"),
	})
	if status != http.StatusOK {
		t.Fatalf("restore answered %d: %v", status, answer)
	}
	refusal := consultLaneRefusal(t, answer)
	if refusal["status"] != float64(http.StatusServiceUnavailable) || refusal["code"] != "unconfirmed" {
		t.Fatalf("refusal = %v, want 503 unconfirmed", refusal)
	}
	if after := script.count("/v1/admit"); after != admitBefore {
		t.Fatalf("admit was called %d times for the barred replace, want none", after-admitBefore)
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != consultSavePage {
		t.Fatalf("page.html = %q, want the exact write's bytes", onDisk)
	}
}

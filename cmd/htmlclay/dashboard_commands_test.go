package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
)

// guardService runs the service with Local's write guard on the served folder
// and answers where its report lands once the service exits.
func guardService(t *testing.T, dir string) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(t.TempDir(), "guard.json")
	guard := filepath.Join(root, "service", "local", "trial", "fs-guard.cjs")
	t.Setenv("HTMLCLAY_SERVICE_CMD", "node -r "+guard+" "+filepath.Join(root, "service", "main.cjs"))
	roots, _ := json.Marshal([]string{dir})
	t.Setenv("FS_GUARD_ROOTS", string(roots))
	t.Setenv("FS_GUARD_OUT", report)
	return report
}

// guardWrites stops the service and answers every write the guard saw it make
// in the served folder, allowed or not.
func guardWrites(t *testing.T, a *app, report string) []string {
	t.Helper()
	a.service.close()
	var got struct {
		Forbidden []struct{ Op, Path string } `json:"forbidden"`
		Allowed   []struct{ Op, Path string } `json:"allowed"`
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		b, err := os.ReadFile(report)
		if err == nil && json.Unmarshal(b, &got) == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the guard wrote no report: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	var writes []string
	for _, w := range append(got.Forbidden, got.Allowed...) {
		writes = append(writes, w.Op+" "+w.Path)
	}
	return writes
}

// TestDashboardCommandsCreateFolder makes a document from a template and a
// folder through the dashboard: Go writes both, the service writes nothing in
// the folder, and a name already taken is refused with nothing changed.
func TestDashboardCommandsCreateFolder(t *testing.T) {
	serviceRepoTarget(t)
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	report := guardService(t, dir)

	a := newServiceTestApp(t, home, t.TempDir())
	serveThroughDo(t, a, "serve-1", dir, "notes")
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
	c := openDashboard(t, a)
	id := c.rootIDOf()

	if status, body := c.command(id, map[string]any{"command": "folder", "parent": nil, "name": "Made"}); status != http.StatusOK || body != `{"path":"Made"}` {
		t.Fatalf("folder = %d %s", status, body)
	}
	if info, err := os.Stat(filepath.Join(dir, "Made")); err != nil || !info.IsDir() {
		t.Fatalf("the folder is not on disk: %v", err)
	}
	if status, body := c.command(id, map[string]any{"command": "create", "template": "blank", "parent": "Made", "name": "page.html"}); status != http.StatusOK || body != `{"path":"Made/page.html"}` {
		t.Fatalf("create = %d %s", status, body)
	}
	made, err := os.ReadFile(filepath.Join(dir, "Made", "page.html"))
	if err != nil || !strings.Contains(strings.ToLower(string(made)), "<html") {
		t.Fatalf("the created document = %q, %v", made, err)
	}
	status, body := c.api(http.MethodGet, "/api/roots/"+url.PathEscape(id)+"/tree", nil)
	if status != http.StatusOK || !strings.Contains(body, `"path":"Made/page.html"`) {
		t.Fatalf("tree after the commands = %d %s", status, body)
	}

	before, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if status, body := c.command(id, map[string]any{"command": "create", "template": "blank", "parent": nil, "name": "index.html"}); status != http.StatusConflict {
		t.Fatalf("create over an existing document = %d %s", status, body)
	}
	if status, body := c.command(id, map[string]any{"command": "folder", "parent": nil, "name": "Made"}); status != http.StatusConflict {
		t.Fatalf("a folder that exists = %d %s", status, body)
	}
	if after, _ := os.ReadFile(filepath.Join(dir, "index.html")); !bytes.Equal(before, after) {
		t.Fatal("a refused create changed the document")
	}

	if writes := guardWrites(t, a, report); len(writes) != 0 {
		t.Fatalf("the service wrote in the served folder: %v", writes)
	}
}

// pageStateFile is where Go keeps a document's page state in a served folder.
func pageStateFile(dir, name string) string {
	return filepath.Join(dir, ".hyperclay", "blockeditor-state", filepath.FromSlash(name)+".json")
}

// TestDashboardCommandsMove renames, moves and duplicates through the dashboard:
// Go moves each file and carries its page state in the same op, the service
// writes nothing in the folder, and a name already taken is refused.
func TestDashboardCommandsMove(t *testing.T) {
	serviceRepoTarget(t)
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	writeTestFile(t, filepath.Join(dir, "page.html"), servedTestDocument)
	writeTestFile(t, filepath.Join(dir, "shelf", "kept.html"), servedTestDocument)
	writeTestFile(t, pageStateFile(dir, "page.html"), `{"v":1,"rev":"abc","tag":"tag"}`)
	writeTestFile(t, pageStateFile(dir, "shelf/kept.html"), `{"v":1,"rev":"def","tag":"tag"}`)
	report := guardService(t, dir)

	a := newServiceTestApp(t, home, t.TempDir())
	serveThroughDo(t, a, "serve-1", dir, "notes")
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
	c := openDashboard(t, a)
	id := c.rootIDOf()
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		return err == nil
	}
	stateAt := func(name string) bool {
		_, err := os.Stat(pageStateFile(dir, name))
		return err == nil
	}

	if status, body := c.command(id, map[string]any{"command": "rename", "path": "page.html", "name": "renamed.html"}); status != http.StatusOK || body != `{"path":"renamed.html"}` {
		t.Fatalf("rename = %d %s", status, body)
	}
	if exists("page.html") || !exists("renamed.html") || stateAt("page.html") || !stateAt("renamed.html") {
		t.Fatalf("after rename: page.html %v, renamed.html %v, state at old %v, at new %v",
			exists("page.html"), exists("renamed.html"), stateAt("page.html"), stateAt("renamed.html"))
	}

	if status, body := c.command(id, map[string]any{"command": "move", "path": "renamed.html", "parent": "shelf"}); status != http.StatusOK || body != `{"path":"shelf/renamed.html"}` {
		t.Fatalf("move = %d %s", status, body)
	}
	if exists("renamed.html") || !exists("shelf/renamed.html") || !stateAt("shelf/renamed.html") {
		t.Fatal("the move did not carry the file and its page state into shelf/")
	}

	if status, body := c.command(id, map[string]any{"command": "rename", "path": "shelf", "name": "cupboard"}); status != http.StatusOK || body != `{"path":"cupboard"}` {
		t.Fatalf("folder rename = %d %s", status, body)
	}
	if !exists("cupboard/kept.html") || !stateAt("cupboard/kept.html") || stateAt("shelf/kept.html") {
		t.Fatal("the folder rename did not carry its documents' page state")
	}

	if status, body := c.command(id, map[string]any{"command": "duplicate", "path": "index.html", "name": "copy.html"}); status != http.StatusOK || body != `{"path":"copy.html"}` {
		t.Fatalf("duplicate = %d %s", status, body)
	}
	original, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if copied, err := os.ReadFile(filepath.Join(dir, "copy.html")); err != nil || !bytes.Equal(copied, original) {
		t.Fatalf("the duplicate = %q, %v", copied, err)
	}

	if status, body := c.command(id, map[string]any{"command": "rename", "path": "index.html", "name": "copy.html"}); status != http.StatusConflict {
		t.Fatalf("a rename onto a taken name = %d %s", status, body)
	}
	if after, _ := os.ReadFile(filepath.Join(dir, "copy.html")); !bytes.Equal(after, original) || !exists("index.html") {
		t.Fatal("a refused rename changed a file")
	}
	status, body := c.api(http.MethodGet, "/api/roots/"+url.PathEscape(id)+"/tree", nil)
	for _, want := range []string{`"path":"cupboard/renamed.html"`, `"path":"copy.html"`} {
		if status != http.StatusOK || !strings.Contains(body, want) {
			t.Fatalf("tree lacks %s: %d %s", want, status, body)
		}
	}
	if strings.Contains(body, `"path":"shelf`) {
		t.Fatalf("tree still lists shelf/: %s", body)
	}

	if writes := guardWrites(t, a, report); len(writes) != 0 {
		t.Fatalf("the service wrote in the served folder: %v", writes)
	}
}

// TestDashboardCommandsTrash trashes, restores and empties the Trash through
// the dashboard: Go moves each file and drops its page state, a restore onto a
// taken name gets a new name, empty-trash keeps the Trash folder, a .trash that
// links elsewhere is refused with that folder untouched, and the service writes
// nothing in the folder.
func TestDashboardCommandsTrash(t *testing.T) {
	serviceRepoTarget(t)
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	writeTestFile(t, filepath.Join(dir, "page.html"), servedTestDocument)
	writeTestFile(t, pageStateFile(dir, "page.html"), `{"v":1,"rev":"abc","tag":"tag"}`)
	report := guardService(t, dir)

	a := newServiceTestApp(t, home, t.TempDir())
	serveThroughDo(t, a, "serve-1", dir, "notes")
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
	c := openDashboard(t, a)
	id := c.rootIDOf()
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		return err == nil
	}

	if status, body := c.command(id, map[string]any{"command": "trash", "path": "page.html"}); status != http.StatusOK || body != `{"path":".trash/page.html"}` {
		t.Fatalf("trash = %d %s", status, body)
	}
	if exists("page.html") || !exists(".trash/page.html") {
		t.Fatal("the trash did not move the file into .trash/")
	}
	if _, err := os.Stat(pageStateFile(dir, "page.html")); err == nil {
		t.Fatal("trashing kept the page state")
	}

	writeTestFile(t, filepath.Join(dir, "page.html"), servedTestDocument)
	if status, body := c.command(id, map[string]any{"command": "trash", "path": "page.html"}); status != http.StatusOK || body != `{"path":".trash/page-1.html"}` {
		t.Fatalf("a second trash of the same name = %d %s", status, body)
	}
	if status, body := c.command(id, map[string]any{"command": "restore-trash", "trashPath": ".trash/page.html"}); status != http.StatusOK || body != `{"path":"page.html"}` {
		t.Fatalf("restore = %d %s", status, body)
	}
	writeTestFile(t, filepath.Join(dir, ".trash", "index.html"), servedTestDocument)
	if status, body := c.command(id, map[string]any{"command": "restore-trash", "trashPath": ".trash/index.html"}); status != http.StatusOK || body != `{"path":"index (restored).html"}` {
		t.Fatalf("a restore onto a taken name = %d %s", status, body)
	}

	if status, body := c.command(id, map[string]any{"command": "empty-trash"}); status != http.StatusOK {
		t.Fatalf("empty-trash = %d %s", status, body)
	}
	if names, err := os.ReadDir(filepath.Join(dir, ".trash")); err != nil || len(names) != 0 {
		t.Fatalf("after empty-trash the Trash holds %d entries (%v)", len(names), err)
	}
	if !exists("page.html") || !exists("index.html") || !exists("index (restored).html") {
		t.Fatal("empty-trash touched a file outside the Trash")
	}

	if writes := guardWrites(t, a, report); len(writes) != 0 {
		t.Fatalf("the service wrote in the served folder: %v", writes)
	}
}

// TestDashboardEmptyTrashLinkedElsewhere is the one command that deletes, aimed
// at a .trash that links to another folder: it is refused and that folder keeps
// everything.
func TestDashboardEmptyTrashLinkedElsewhere(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("making symlinks on Windows needs Developer Mode")
	}
	serviceRepoTarget(t)
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	outside := filepath.Join(home, "elsewhere")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	writeTestFile(t, filepath.Join(outside, "keep.html"), servedTestDocument)
	if err := os.Symlink(outside, filepath.Join(dir, ".trash")); err != nil {
		t.Fatal(err)
	}

	a := newServiceTestApp(t, home, t.TempDir())
	serveThroughDo(t, a, "serve-1", dir, "notes")
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
	c := openDashboard(t, a)

	if status, body := c.command(c.rootIDOf(), map[string]any{"command": "empty-trash"}); status != http.StatusForbidden {
		t.Fatalf("empty-trash through a link = %d %s", status, body)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep.html")); err != nil {
		t.Fatalf("empty-trash reached through the link: %v", err)
	}
}

// listenOnServedPort serves the app on the loopback port its folders' addresses
// name, as main does, so the service can reach Go's own routes.
func listenOnServedPort(t *testing.T, a *app) {
	t.Helper()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", servedTestPort))
	if err != nil {
		t.Fatalf("listen on the served port: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(a.serveHost), ReadHeaderTimeout: 10 * time.Second}
	go server.Serve(ln)
	t.Cleanup(func() { server.Close() })
}

// TestDashboardSourceSave edits a document's source, lists and opens its
// versions and restores one through the dashboard: each write is Go's own save
// or restore route, a stale editor is refused with the file unchanged, and the
// service writes nothing in the folder.
func TestDashboardSourceSave(t *testing.T) {
	serviceRepoTarget(t)
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	report := guardService(t, dir)

	a := newServiceTestApp(t, home, t.TempDir())
	listenOnServedPort(t, a)
	serveThroughDo(t, a, "serve-1", dir, "notes")
	waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
	c := openDashboard(t, a)
	root := "/api/roots/" + url.PathEscape(c.rootIDOf())
	type source struct {
		Kind string `json:"kind"`
		HTML string `json:"html"`
		Etag string `json:"etag"`
	}
	read := func() source {
		t.Helper()
		status, body := c.api(http.MethodGet, root+"/source?path=index.html", nil)
		var got source
		if status != http.StatusOK || json.Unmarshal([]byte(body), &got) != nil || got.Etag == "" {
			t.Fatalf("source = %d %s", status, body)
		}
		return got
	}
	onDisk := func() string {
		b, err := os.ReadFile(filepath.Join(dir, "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	first := read()
	edited := strings.Replace(first.HTML, "</body>", "<p>edited in the code editor</p></body>", 1)
	status, body := c.api(http.MethodPut, root+"/source", map[string]any{"path": "index.html", "html": edited, "etag": first.Etag, "operationId": "op-1"})
	var saved source
	if status != http.StatusOK || json.Unmarshal([]byte(body), &saved) != nil || saved.Kind != "saved" || saved.Etag == first.Etag {
		t.Fatalf("save = %d %s", status, body)
	}
	if !strings.Contains(onDisk(), "edited in the code editor") {
		t.Fatalf("the save is not on disk: %s", onDisk())
	}

	before := onDisk()
	status, body = c.api(http.MethodPut, root+"/source", map[string]any{"path": "index.html", "html": first.HTML, "etag": first.Etag, "operationId": "op-2"})
	if status != http.StatusConflict && status != http.StatusPreconditionFailed {
		t.Fatalf("a save from a stale editor = %d %s", status, body)
	}
	if onDisk() != before {
		t.Fatal("a refused save changed the file")
	}

	status, body = c.api(http.MethodGet, root+"/versions?path=index.html", nil)
	var versions struct {
		Versions []struct{ ID string } `json:"versions"`
	}
	if status != http.StatusOK || json.Unmarshal([]byte(body), &versions) != nil || len(versions.Versions) == 0 {
		t.Fatalf("versions = %d %s", status, body)
	}
	var restoreID string
	for _, v := range versions.Versions {
		status, body := c.api(http.MethodGet, root+"/version?path=index.html&id="+url.QueryEscape(v.ID), nil)
		if status != http.StatusOK {
			t.Fatalf("version %s = %d %s", v.ID, status, body)
		}
		if !strings.Contains(body, "edited in the code editor") {
			restoreID = v.ID
		}
	}
	if restoreID == "" {
		t.Fatalf("no version holds the document before the edit: %v", versions.Versions)
	}
	current := read()
	status, body = c.api(http.MethodPost, root+"/restore", map[string]any{"path": "index.html", "versionId": restoreID, "etag": current.Etag, "operationId": "op-3"})
	if status != http.StatusOK || !strings.Contains(body, `"kind":"saved"`) {
		t.Fatalf("restore = %d %s", status, body)
	}
	if strings.Contains(onDisk(), "edited in the code editor") {
		t.Fatal("the restore left the edit on disk")
	}

	if writes := guardWrites(t, a, report); len(writes) != 0 {
		t.Fatalf("the service wrote in the served folder: %v", writes)
	}
}

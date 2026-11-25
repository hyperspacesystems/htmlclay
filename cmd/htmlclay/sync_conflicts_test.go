package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/served"
)

// conflictsFixture has the shape of conflicts.json as Local's
// tests/unit/reconcile-conflicts.test.js and reconcile/conflicts.js write it.
const conflictsFixture = `{
  "901": {
    "kind": "both-edited",
    "path": "board.html",
    "localChecksum": "aa11aa11aa11aa11",
    "remoteEtag": "bb22bb22bb22bb22",
    "remoteCopy": ".hyperclay/conflicts/board.remote-bb22bb22.html",
    "detectedAt": 1790000000000
  },
  "notes/new.html": {
    "kind": "unbound",
    "path": "notes/new.html",
    "detectedAt": 1790000000500
  }
}`

func conflictItems(snapshot appcore.Snapshot) []appcore.WaitingItem {
	var items []appcore.WaitingItem
	for _, item := range snapshot.Waiting {
		if item.Kind == appcore.WaitingConflict {
			items = append(items, item)
		}
	}
	return items
}

func TestM4ConflictsRead(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("HTMLCLAY_STUB_BIND_LOG", filepath.Join(scratch, "bind"))
	a, setDiscovery, _ := connectTestApp(t, true)
	setDiscovery(`{"ok":true,"actor":{"id":12,"username":"maya","fullName":"Maya Lin"},"accounts":[` +
		`{"id":12,"kind":"personal","username":"maya","displayName":"maya","role":"owner","sync":{"enabled":true,"reason":null}}]}`)
	if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
		t.Fatalf("connect refused: %+v", refusal)
	}
	row := savedSync(t, a).Accounts[0]
	if refusal := setUpThroughDo(t, a, "s1", row.FolderID, filepath.Join(a.rt.home, "maya")); refusal != nil {
		t.Fatalf("set up refused: %+v", refusal)
	}
	waitForBinds(t, a)
	session := savedSync(t, a).SyncSessions[0]
	file := filepath.Join(a.conflictsMetaDir(session), "conflicts.json")
	write := func(body string) {
		t.Helper()
		writeTestFile(t, file, body)
	}
	report := func(seq int64, conflicts int, events ...served.SyncEvent) {
		a.takeSyncReport(served.SyncReport{Seq: seq, Events: events, Sessions: []served.SyncSessionReport{{
			SessionID: session.ID, RootID: session.RootID, Status: "idle", Running: true, Conflicts: conflicts, Paused: json.RawMessage("null"),
		}}})
	}

	t.Run("held conflicts show with no service", func(t *testing.T) {
		write(conflictsFixture)
		a.rt.syncMu.Lock()
		a.rt.syncConflicts = syncConflicts{}
		a.rt.syncLast = nil
		a.rt.syncMu.Unlock()
		a.loadSyncAtStart()
		items := conflictItems(a.snapshot())
		if len(items) != 2 {
			t.Fatalf("conflict items = %+v", items)
		}
		if items[0].ID != "w-"+session.ID+":901:1790000000000" || !strings.Contains(items[0].Title, "board.html") || items[0].Detail != "both-edited" {
			t.Fatalf("first item = %+v", items[0])
		}
		if items[0].SubjectID == nil || *items[0].SubjectID != session.RootID {
			t.Fatalf("subject = %v, want the folder %s", items[0].SubjectID, session.RootID)
		}
	})

	t.Run("a changed count reads again, an unchanged one does not", func(t *testing.T) {
		report(1, 2)
		write(`{"901": {"kind": "both-edited", "path": "board.html", "detectedAt": 1790000000000}}`)
		report(2, 2)
		if got := len(conflictItems(a.snapshot())); got != 2 {
			t.Fatalf("an unchanged count read again: %d items", got)
		}
		report(3, 1)
		if got := len(conflictItems(a.snapshot())); got != 1 {
			t.Fatalf("after the count changed: %d items", got)
		}
	})

	t.Run("a resolved event reads again", func(t *testing.T) {
		write(`{}`)
		report(4, 1, served.SyncEvent{ID: "e1", SessionID: session.ID, RootID: session.RootID, Action: "resolved", File: "board.html"})
		if got := len(conflictItems(a.snapshot())); got != 0 {
			t.Fatalf("after resolved: %d items", got)
		}
	})

	t.Run("a file that does not parse keeps the last list", func(t *testing.T) {
		write(`{"901": {"kind": "both-edited", "path": "board.html", "detectedAt": 1790000000000}}`)
		report(5, 1, served.SyncEvent{ID: "e2", SessionID: session.ID, RootID: session.RootID, Action: "conflict", File: "board.html"})
		write(`{"901": `)
		report(6, 2)
		if got := len(conflictItems(a.snapshot())); got != 1 {
			t.Fatalf("a broken file changed the list: %d items", got)
		}
	})

	t.Run("a missing file is no conflicts", func(t *testing.T) {
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
		report(7, 0)
		if got := len(conflictItems(a.snapshot())); got != 0 {
			t.Fatalf("a missing file: %d items", got)
		}
	})

	t.Run("a report whose count disagrees with the cached list reads again", func(t *testing.T) {
		write(`{"901": {"kind": "both-edited", "path": "board.html", "detectedAt": 1790000000000}}`)
		report(8, 1)
		// Simulate resolveConflict clearing conflicts.json and re-reading before a 0-count report arrives.
		write(`{}`)
		a.readHeldConflicts()
		if got := len(conflictItems(a.snapshot())); got != 0 {
			t.Fatalf("after local clear: %d items", got)
		}
		// A second conflict lands while syncLast still holds Conflicts=1.
		write(`{"902": {"kind": "both-edited", "path": "second.html", "detectedAt": 1790000000200}}`)
		report(9, 1)
		items := conflictItems(a.snapshot())
		if len(items) != 1 || !strings.Contains(items[0].Title, "second.html") {
			t.Fatalf("second conflict after stale syncLast = %+v, want second.html", items)
		}
	})
}

func TestM4ConflictRoutes(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("HTMLCLAY_STUB_BIND_LOG", filepath.Join(scratch, "bind"))
	a, setDiscovery, _ := connectTestApp(t, true)
	setDiscovery(`{"ok":true,"actor":{"id":12,"username":"maya","fullName":"Maya Lin"},"accounts":[` +
		`{"id":12,"kind":"personal","username":"maya","displayName":"maya","role":"owner","sync":{"enabled":true,"reason":null}}]}`)
	if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
		t.Fatalf("connect refused: %+v", refusal)
	}
	row := savedSync(t, a).Accounts[0]
	dir := filepath.Join(a.rt.home, "maya")
	if refusal := setUpThroughDo(t, a, "s1", row.FolderID, dir); refusal != nil {
		t.Fatalf("set up refused: %+v", refusal)
	}
	waitForBinds(t, a)
	session := savedSync(t, a).SyncSessions[0]
	writeTestFile(t, filepath.Join(a.conflictsMetaDir(session), "conflicts.json"), conflictsFixture)
	writeTestFile(t, filepath.Join(dir, "board.html"), "<p>mine</p>")
	writeTestFile(t, filepath.Join(dir, ".hyperclay", "conflicts", "board.remote-bb22bb22.html"), "<p>theirs</p>")
	folder := folderInSnapshot(t, a, row.FolderID)
	if folder.URL == "" {
		t.Fatalf("the synced folder is not served: %+v", folder)
	}
	base := strings.TrimSuffix(folder.URL, "/") + "/"

	get := func(method string) (int, map[string][]dashboardConflict) {
		t.Helper()
		rec := httptest.NewRecorder()
		(&dashboardApp{a: a}).ServeHTTP(rec, httptest.NewRequest(method, "/api/app/conflicts", nil))
		var body map[string][]dashboardConflict
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("GET /api/app/conflicts did not parse: %v: %s", err, rec.Body.String())
			}
		}
		return rec.Code, body
	}

	t.Run("each held conflict, with both versions' addresses and its choices", func(t *testing.T) {
		status, body := get(http.MethodGet)
		if status != http.StatusOK || len(body["conflicts"]) != 2 {
			t.Fatalf("GET = %d %+v", status, body)
		}
		board, unbound := body["conflicts"][0], body["conflicts"][1]
		if board.ID != session.ID+":901:1790000000000" || board.FolderID != row.FolderID || board.FolderName != folder.Name || board.Path != "board.html" || board.Kind != "both-edited" {
			t.Fatalf("board = %+v", board)
		}
		if board.DetectedAt != "2026-09-21T14:13:20.000Z" {
			t.Fatalf("detectedAt = %q", board.DetectedAt)
		}
		if strings.Join(board.Keep, ",") != "mine,theirs,both" {
			t.Fatalf("keep = %v", board.Keep)
		}
		if board.MineURL == nil || *board.MineURL != base+"board.html" || board.TheirsURL == nil || *board.TheirsURL != base+"_/sync/theirs/board.html" {
			t.Fatalf("urls = %v %v", board.MineURL, board.TheirsURL)
		}
		if unbound.Path != "notes/new.html" || strings.Join(unbound.Keep, ",") != "mine,theirs,both" || unbound.MineURL != nil || unbound.TheirsURL != nil {
			t.Fatalf("a conflict with no file here and no kept copy = %+v", unbound)
		}
	})

	t.Run("the served host finds the kept copy, and only for a held conflict that has one", func(t *testing.T) {
		if copy, ok := a.heldCopyFor(row.FolderID, "board.html"); !ok || copy != ".hyperclay/conflicts/board.remote-bb22bb22.html" {
			t.Fatalf("heldCopyFor board = %q %v", copy, ok)
		}
		if _, ok := a.heldCopyFor(row.FolderID, "notes/new.html"); ok {
			t.Fatal("a conflict with no kept copy answered one")
		}
		if _, ok := a.heldCopyFor("another-folder", "board.html"); ok {
			t.Fatal("another folder's conflict answered")
		}
	})

	t.Run("only GET", func(t *testing.T) {
		if status, _ := get(http.MethodPost); status != http.StatusMethodNotAllowed {
			t.Fatalf("POST = %d", status)
		}
	})
}

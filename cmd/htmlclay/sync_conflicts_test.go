package main

import (
	"encoding/json"
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
}

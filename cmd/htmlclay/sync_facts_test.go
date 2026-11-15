package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/served"
)

func waitingOfKind(snapshot appcore.Snapshot, kind string) int {
	n := 0
	for _, item := range snapshot.Waiting {
		if item.Kind == kind {
			n++
		}
	}
	return n
}

func TestM4SyncFacts(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("HTMLCLAY_STUB_CONFIGURE_LOG", filepath.Join(scratch, "configure"))
	t.Setenv("HTMLCLAY_STUB_BIND_LOG", filepath.Join(scratch, "bind"))
	a, setDiscovery, _ := connectTestApp(t, true)
	setDiscovery(`{"ok":true,"actor":{"id":12,"username":"maya","fullName":"Maya Lin"},"accounts":[` +
		`{"id":12,"kind":"personal","username":"maya","displayName":"maya","role":"owner","sync":{"enabled":true,"reason":null}},` +
		`{"id":40,"kind":"team","username":"studio","displayName":"Studio","role":"editor","sync":{"enabled":true,"reason":null}}]}`)
	if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
		t.Fatalf("connect refused: %+v", refusal)
	}
	rows := savedSync(t, a).Accounts
	personal, team := rows[0], rows[1]
	for i, row := range rows {
		dir := filepath.Join(a.rt.home, row.Username)
		if refusal := setUpThroughDo(t, a, "s"+string(rune('1'+i)), row.FolderID, dir); refusal != nil {
			t.Fatalf("set up %s refused: %+v", row.Username, refusal)
		}
		waitForBinds(t, a)
	}
	sessionOf := map[string]string{}
	for _, session := range savedSync(t, a).SyncSessions {
		sessionOf[session.RootID] = session.ID
	}
	report := func(seq int64, keyRefused bool, rows ...served.SyncSessionReport) {
		a.takeSyncReport(served.SyncReport{Seq: seq, Sessions: rows, KeyRefused: keyRefused})
	}
	at := "2026-11-15T07:00:00Z"
	row := func(folder string, edit func(*served.SyncSessionReport)) served.SyncSessionReport {
		r := served.SyncSessionReport{SessionID: sessionOf[folder], RootID: folder, Status: "idle", Running: true, LastSyncAt: &at, Paused: json.RawMessage("null")}
		if edit != nil {
			edit(&r)
		}
		return r
	}
	stateOf := func(folder string) string {
		return folderInSnapshot(t, a, folder).Sync.State
	}

	t.Run("each folder's state follows the report", func(t *testing.T) {
		report(1, false, row(personal.FolderID, nil), row(team.FolderID, nil))
		if got := stateOf(personal.FolderID); got != appcore.FolderSyncUpToDate {
			t.Fatalf("idle = %s", got)
		}
		report(2, false, row(personal.FolderID, func(r *served.SyncSessionReport) { r.Status = "syncing"; r.PendingCount = 3 }), row(team.FolderID, func(r *served.SyncSessionReport) { r.PendingCount = 2 }))
		snapshot := a.snapshot()
		if snapshot.Sync.SyncingFiles != 5 || snapshot.Sync.LastFinishedAt == nil {
			t.Fatalf("sync = %+v", snapshot.Sync)
		}
		if got := stateOf(personal.FolderID); got != appcore.FolderSyncSyncing {
			t.Fatalf("syncing = %s", got)
		}
		report(3, false, row(personal.FolderID, func(r *served.SyncSessionReport) { r.Conflicts = 2 }), row(team.FolderID, nil))
		if got := stateOf(personal.FolderID); got != appcore.FolderSyncConflicts {
			t.Fatalf("conflicts = %s", got)
		}
		report(4, false, row(personal.FolderID, func(r *served.SyncSessionReport) {
			r.Paused = json.RawMessage(`{"reason":"folder-missing","since":"2026-11-15T07:00:00Z"}`)
		}), row(team.FolderID, func(r *served.SyncSessionReport) { r.Blocked = 1 }))
		snapshot = a.snapshot()
		if got := stateOf(personal.FolderID); got != appcore.FolderSyncPaused {
			t.Fatalf("a missing folder = %s", got)
		}
		// The folder is still on disk, so the engine's pause adds no second
		// folderMissing item to the one the disk check would make.
		if waitingOfKind(snapshot, appcore.WaitingFolderMissing) != 0 || waitingOfKind(snapshot, appcore.WaitingFileTooBig) != 1 {
			t.Fatalf("waiting = %+v", snapshot.Waiting)
		}
		report(5, false, row(personal.FolderID, nil), row(team.FolderID, nil))
		if refusal := a.setSyncPaused(true); refusal != nil {
			t.Fatal(refusal)
		}
		if got := stateOf(team.FolderID); got != appcore.FolderSyncPaused {
			t.Fatalf("the switch = %s", got)
		}
		if refusal := a.setSyncPaused(false); refusal != nil {
			t.Fatal(refusal)
		}
		if refusal := a.setFolderSynced(team.FolderID, false); refusal != nil {
			t.Fatal(refusal)
		}
		if got := stateOf(team.FolderID); got != appcore.FolderSyncOff {
			t.Fatalf("stopped = %s", got)
		}
		if refusal := a.setFolderSynced(team.FolderID, true); refusal != nil {
			t.Fatal(refusal)
		}
	})

	t.Run("a refused key counts once", func(t *testing.T) {
		revoked := func(r *served.SyncSessionReport) {
			r.Paused = json.RawMessage(`{"reason":"key-revoked","since":"2026-11-15T07:00:00Z"}`)
		}
		report(6, true, row(personal.FolderID, revoked), row(team.FolderID, revoked))
		_, _, events, _ := a.syncFacts()
		refusedEvents := 0
		for _, e := range events {
			if e.Kind == appcore.WaitingSyncKeyRefused {
				refusedEvents++
			}
		}
		if refusedEvents != 1 {
			t.Fatalf("syncKeyRefused events = %d, want 1: %+v", refusedEvents, events)
		}
		snapshot := a.snapshot()
		if got := waitingOfKind(snapshot, appcore.WaitingSyncKeyRefused); got != 1 {
			t.Fatalf("syncKeyRefused items = %d, want 1: %+v", got, snapshot.Waiting)
		}
		if got := waitingOfKind(snapshot, appcore.WaitingFolderMissing); got != 0 {
			t.Fatalf("a revoked key made folderMissing items: %+v", snapshot.Waiting)
		}
	})
}

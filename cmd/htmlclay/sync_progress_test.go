package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/served"
)

// TestM4FirstDownloadProgress: a first download's progress reaches the folder
// in the snapshot while it runs, and only then (08-sync.md 8.42, decision 20).
func TestM4FirstDownloadProgress(t *testing.T) {
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

	seq := int64(0)
	download := func(progress *served.SyncProgress) *appcore.SyncDownload {
		seq++
		a.takeSyncReport(served.SyncReport{Seq: seq, Sessions: []served.SyncSessionReport{{
			SessionID: session.ID, RootID: row.FolderID, Status: "syncing", Running: true,
			Paused: json.RawMessage("null"), Progress: progress,
		}}})
		for _, f := range a.snapshot().Folders {
			if f.ID == row.FolderID {
				return f.Sync.Download
			}
		}
		t.Fatalf("folder %s has no sync in the snapshot", row.FolderID)
		return nil
	}

	got := download(&served.SyncProgress{Phase: "download", Done: 3, Total: 31, BytesDone: 1, BytesTotal: 2})
	if got == nil || *got != (appcore.SyncDownload{Done: 3, Total: 31, BytesDone: 1, BytesTotal: 2}) {
		t.Fatalf("download = %+v, want 3 of 31", got)
	}
	if got := download(&served.SyncProgress{Phase: "download", Done: 31, Total: 31}); got != nil {
		t.Fatalf("a finished download still shows %+v", got)
	}
	if got := download(&served.SyncProgress{Phase: "upload", Done: 1, Total: 4}); got != nil {
		t.Fatalf("an upload shows a download bar %+v", got)
	}
	if got := download(nil); got != nil {
		t.Fatalf("no progress shows %+v", got)
	}
}

// TestM4FirstDownloadReadsSyncing: a folder reads syncing from its set up until
// its first download answers, before the service reports it and after the last
// file is in (08-sync.md F-8.55h).
func TestM4FirstDownloadReadsSyncing(t *testing.T) {
	scratch := t.TempDir()
	hold := filepath.Join(scratch, "hold")
	t.Setenv("HTMLCLAY_STUB_BIND_LOG", filepath.Join(scratch, "bind"))
	t.Setenv("HTMLCLAY_STUB_BIND_HOLD", hold)
	a, setDiscovery, _ := connectTestApp(t, true)
	setDiscovery(`{"ok":true,"actor":{"id":12,"username":"maya","fullName":"Maya Lin"},"accounts":[` +
		`{"id":12,"kind":"personal","username":"maya","displayName":"maya","role":"owner","sync":{"enabled":true,"reason":null}},` +
		`{"id":40,"kind":"team","username":"studio","displayName":"Studio","role":"editor","sync":{"enabled":true,"reason":null}}]}`)
	if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
		t.Fatalf("connect refused: %+v", refusal)
	}
	team := savedSync(t, a).Accounts[1]
	writeTestFile(t, hold, "")
	t.Cleanup(func() { os.Remove(hold) })
	if refusal := setUpThroughDo(t, a, "s1", team.FolderID, filepath.Join(a.rt.home, "Studio")); refusal != nil {
		t.Fatalf("set up refused: %+v", refusal)
	}
	if got := folderInSnapshot(t, a, team.FolderID).Sync.State; got != appcore.FolderSyncSyncing {
		t.Fatalf("before the service reports the folder: %s, want %s", got, appcore.FolderSyncSyncing)
	}
	session := savedSync(t, a).SyncSessions[0]
	a.takeSyncReport(served.SyncReport{Seq: 1, Sessions: []served.SyncSessionReport{{
		SessionID: session.ID, RootID: team.FolderID, Status: "idle", Running: true, Paused: json.RawMessage("null"),
	}}})
	if got := folderInSnapshot(t, a, team.FolderID).Sync.State; got != appcore.FolderSyncSyncing {
		t.Fatalf("after the last file, before the download answers: %s, want %s", got, appcore.FolderSyncSyncing)
	}
	if err := os.Remove(hold); err != nil {
		t.Fatal(err)
	}
	waitForBinds(t, a)
	if got := folderInSnapshot(t, a, team.FolderID).Sync.State; got != appcore.FolderSyncUpToDate {
		t.Fatalf("after the download answered: %s, want %s", got, appcore.FolderSyncUpToDate)
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/served"
	"github.com/panphora/htmlclay/internal/shellapi"
	"github.com/panphora/htmlclay/internal/synchistory"
)

// TestM4HistoryRoutes is 08-sync.md step 8.39: a report's events become the
// folder's history, which the dashboard and the Mac app read a page at a time,
// and today's uploads and downloads reach the folder's sync facts.
func TestM4HistoryRoutes(t *testing.T) {
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

	now := time.Now().UTC()
	yesterday := now.Add(-48 * time.Hour)
	actions := []string{"upload", "download", "upload", "relocate", "trash", "conflict", "resolved", "create"}
	var events []served.SyncEvent
	for i := 0; i < 60; i++ {
		at := now
		if i < 10 {
			at = yesterday
		}
		events = append(events, served.SyncEvent{
			ID: fmt.Sprintf("%s:run-%d", session.ID, i), SessionID: session.ID, RootID: row.FolderID,
			At: at.Format(time.RFC3339), Action: actions[i%len(actions)], File: fmt.Sprintf("f%d.html", i),
		})
	}
	paused := json.RawMessage("null")
	report := served.SyncReport{Seq: 1, Sessions: []served.SyncSessionReport{{SessionID: session.ID, RootID: row.FolderID, Status: "idle", Running: true, Paused: paused}}, Events: events}
	a.takeSyncReport(report)
	report.Seq = 2
	a.takeSyncReport(report)

	dashboard := func(path string) (int, synchistory.Page) {
		t.Helper()
		rec := httptest.NewRecorder()
		(&dashboardApp{a: a}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		var page synchistory.Page
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
				t.Fatalf("%s did not parse: %v", path, err)
			}
		}
		return rec.Code, page
	}
	const secret = "history-test-secret"
	shell := shellapi.New(shellBackend{a}, secret, 4321, make(chan struct{}))
	viaShell := func(path string) (int, synchistory.Page) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = "127.0.0.1:4321"
		req.Header.Set("Authorization", "Bearer "+secret)
		rec := httptest.NewRecorder()
		shell.ServeHTTP(rec, req)
		var page synchistory.Page
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
				t.Fatalf("%s did not parse: %v", path, err)
			}
		}
		return rec.Code, page
	}

	// create is not history: 60 events less the 7 creates (i % 8 == 7).
	const kept = 60 - 7

	t.Run("a report sent twice is one history, read newest first a page at a time", func(t *testing.T) {
		status, first := dashboard("/api/app/folders/" + row.FolderID + "/history")
		if status != http.StatusOK || len(first.Entries) != syncHistoryPageSize || first.Next == nil {
			t.Fatalf("first page: %d, %d entries, next %v", status, len(first.Entries), first.Next)
		}
		if first.Entries[0].ID != session.ID+":run-59" || first.Entries[0].Kind != synchistory.Renamed || first.Entries[0].Path != "f59.html" {
			t.Fatalf("newest = %+v", first.Entries[0])
		}
		status, rest := dashboard("/api/app/folders/" + row.FolderID + "/history?before=" + *first.Next)
		if status != http.StatusOK || len(first.Entries)+len(rest.Entries) != kept || rest.Next != nil {
			t.Fatalf("second page: %d, %d entries, next %v", status, len(rest.Entries), rest.Next)
		}
	})

	t.Run("the Mac app reads the same page", func(t *testing.T) {
		_, fromDashboard := dashboard("/api/app/folders/" + row.FolderID + "/history")
		status, fromShell := viaShell("/shell/v1/folders/" + row.FolderID + "/history")
		if status != http.StatusOK || len(fromShell.Entries) != len(fromDashboard.Entries) || fromShell.Entries[0] != fromDashboard.Entries[0] || *fromShell.Next != *fromDashboard.Next {
			t.Fatalf("shell %d %+v", status, fromShell.Next)
		}
	})

	t.Run("a folder sync does not keep is not there", func(t *testing.T) {
		for _, id := range []string{"0000000000000000", "../../etc"} {
			if status, _ := dashboard("/api/app/folders/" + id + "/history"); status != http.StatusNotFound {
				t.Errorf("dashboard %s = %d", id, status)
			}
			if status, _ := viaShell("/shell/v1/folders/" + id + "/history"); status != http.StatusNotFound {
				t.Errorf("shell %s = %d", id, status)
			}
		}
	})

	t.Run("today's uploads and downloads reach the folder's sync facts", func(t *testing.T) {
		wantUp, wantDown := 0, 0
		for i := 10; i < 60; i++ {
			switch actions[i%len(actions)] {
			case "upload":
				wantUp++
			case "download":
				wantDown++
			}
		}
		for _, f := range a.snapshot().Folders {
			if f.ID == row.FolderID {
				if f.Sync.UploadedToday != wantUp || f.Sync.DownloadedToday != wantDown {
					t.Fatalf("today = %d up, %d down; want %d, %d", f.Sync.UploadedToday, f.Sync.DownloadedToday, wantUp, wantDown)
				}
				return
			}
		}
		t.Fatal("no folder row")
	})
}

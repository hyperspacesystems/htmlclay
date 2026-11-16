package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
)

func setUpThroughDo(t *testing.T, a *app, id, folderID, path string) *appcore.Refusal {
	t.Helper()
	_, refusal := a.do(context.Background(), coreAction(t, id, "folder.setUpTeam", map[string]any{"id": folderID, "path": path}), ChannelTray)
	return refusal
}

func bindsSent(t *testing.T, log string) []syncBindRequest {
	t.Helper()
	data, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var sent []syncBindRequest
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var body syncBindRequest
		if err := json.Unmarshal([]byte(line), &body); err != nil {
			t.Fatalf("bind line %q: %v", line, err)
		}
		sent = append(sent, body)
	}
	return sent
}

// waitForBinds waits for every first download to end, failing rather than hanging.
func waitForBinds(t *testing.T, a *app) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		a.rt.syncBinds.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("a first download did not end")
	}
}

func TestM4SetUpFolder(t *testing.T) {
	scratch := t.TempDir()
	configureLog := filepath.Join(scratch, "configure")
	bindLog := filepath.Join(scratch, "bind")
	hold := filepath.Join(scratch, "hold")
	answer := filepath.Join(scratch, "answer")
	t.Setenv("HTMLCLAY_STUB_CONFIGURE_LOG", configureLog)
	t.Setenv("HTMLCLAY_STUB_BIND_LOG", bindLog)
	t.Setenv("HTMLCLAY_STUB_BIND_HOLD", hold)
	t.Setenv("HTMLCLAY_STUB_BIND", answer)
	preview := filepath.Join(scratch, "preview")
	t.Setenv("HTMLCLAY_STUB_PREVIEW", preview)
	a, setDiscovery, _ := connectTestApp(t, true)
	setDiscovery(`{"ok":true,"actor":{"id":12,"username":"maya","fullName":"Maya Lin"},"accounts":[` +
		`{"id":12,"kind":"personal","username":"maya","displayName":"maya","role":"owner","sync":{"enabled":true,"reason":null}},` +
		`{"id":40,"kind":"team","username":"studio","displayName":"Studio","role":"editor","sync":{"enabled":true,"reason":null}},` +
		`{"id":41,"kind":"team","username":"lab","displayName":"Lab","role":"editor","sync":{"enabled":true,"reason":null}},` +
		`{"id":42,"kind":"team","username":"press","displayName":"Press","role":"viewer","sync":{"enabled":false,"reason":"viewer"}}]}`)
	if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
		t.Fatalf("connect refused: %+v", refusal)
	}
	rows := savedSync(t, a).Accounts
	personal, team, lab, press := rows[0], rows[1], rows[2], rows[3]
	if personal.Kind != "personal" || team.Kind != "team" || team.AccountID != 40 || lab.AccountID != 41 {
		t.Fatalf("fixture rows = %+v", rows)
	}

	offered := 0
	for _, e := range a.facts().Events {
		if e.Kind == appcore.WaitingTeamToSetUp && e.SubjectID == team.FolderID {
			offered++
		}
	}
	if offered != 1 {
		t.Fatalf("set-up items for a team that syncs = %d, want 1", offered)
	}

	t.Run("a team folder with files is refused and unchanged", func(t *testing.T) {
		dir := filepath.Join(a.rt.home, "Studio with files")
		writeTestFile(t, filepath.Join(dir, "mine.html"), "<p>mine</p>")
		refusal := setUpThroughDo(t, a, "s1", team.FolderID, dir)
		if refusal == nil || refusal.Code != appcore.RefuseSyncFolderNotEmpty {
			t.Fatalf("refusal = %+v, want %s", refusal, appcore.RefuseSyncFolderNotEmpty)
		}
		if data, err := os.ReadFile(filepath.Join(dir, "mine.html")); err != nil || string(data) != "<p>mine</p>" {
			t.Fatalf("the file changed: %q %v", data, err)
		}
		if _, ok := a.rt.cfg.ServedFolderByID(team.FolderID); ok {
			t.Fatal("the folder was served")
		}
		for _, entry := range a.rt.cfg.TeamFolderList() {
			if entry.FolderID == team.FolderID {
				t.Fatalf("the folder was marked: %+v", entry)
			}
		}
		if got := savedSync(t, a).SyncSessions; len(got) != 0 {
			t.Fatalf("session rows = %+v", got)
		}
		if got := bindsSent(t, bindLog); len(got) != 0 {
			t.Fatalf("binds = %+v", got)
		}
	})

	t.Run("a team document is never broadly granted", func(t *testing.T) {
		writeTestFile(t, hold, "")
		t.Cleanup(func() { os.Remove(hold) })
		dir := filepath.Join(a.rt.home, "Studio")
		if refusal := setUpThroughDo(t, a, "s2", team.FolderID, dir); refusal != nil {
			t.Fatalf("set up refused: %+v", refusal)
		}
		deadline := time.Now().Add(10 * time.Second)
		for len(bindsSent(t, bindLog)) == 0 && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		binds := bindsSent(t, bindLog)
		if len(binds) != 1 {
			t.Fatalf("binds = %+v", binds)
		}
		bind := binds[0]
		if bind.Mode != "empty" || bind.Root.ID != team.FolderID || bind.Session.RootID != team.FolderID || bind.Session.AccountID != 40 || !bind.Session.Enabled {
			t.Fatalf("bind = %+v", bind)
		}

		// While the first download runs, a document it puts in the folder is a team
		// document already, and configure does not start the session on its own.
		document := filepath.Join(bind.Root.Path, "index.html")
		writeTestFile(t, document, "<p>from the team</p>")
		if scope := a.scopeOf(document); scope != (config.Scope{Kind: config.ScopeTeam, Key: "team:40"}) {
			t.Fatalf("scope during the first download = %+v, want team:40", scope)
		}
		before := len(configuresSent(t, configureLog))
		a.sendSyncWanted()
		during := waitForConfigure(t, configureLog, before, func(syncWanted) bool { return true })
		for _, session := range during.Sessions {
			if session.ID == bind.Session.ID {
				t.Fatal("configure named the session while its bind ran")
			}
		}

		if err := os.Remove(hold); err != nil {
			t.Fatal(err)
		}
		waitForBinds(t, a)
		after := waitForConfigure(t, configureLog, before+1, func(w syncWanted) bool {
			for _, session := range w.Sessions {
				if session.ID == bind.Session.ID {
					return true
				}
			}
			return false
		})
		if len(after.Roots) != 2 {
			t.Fatalf("roots after the bind = %+v", after.Roots)
		}
		if served, ok := a.rt.cfg.ServedFolderByID(team.FolderID); !ok || !served.Serving {
			t.Fatalf("served = %+v %v", served, ok)
		}
	})

	t.Run("a folder already in HTML Clay is refused before the counts", func(t *testing.T) {
		dir := filepath.Join(a.rt.home, "Old")
		writeTestFile(t, filepath.Join(dir, "notes.html"), "<p>old</p>")
		if err := a.rt.cfg.TrustFolderWithID(dir, "", "fedcba9876543210"); err != nil {
			t.Fatal(err)
		}
		_, refusal := a.do(context.Background(), coreAction(t, "s-old", "folder.setUpTeam", map[string]any{"id": personal.FolderID, "path": dir, "existing": true}), ChannelTray)
		if refusal == nil || refusal.Code != appcore.RefuseSyncFolderInUse {
			t.Fatalf("refusal = %+v, want %s", refusal, appcore.RefuseSyncFolderInUse)
		}
	})

	t.Run("no upload before the person confirms", func(t *testing.T) {
		writeTestFile(t, preview, `{"ok":true,"files":3,"bytes":30,"toUpload":2,"toDownload":1,"differ":1}`)
		dir := filepath.Join(a.rt.home, "Maya")
		writeTestFile(t, filepath.Join(dir, "notes.html"), "<p>mine</p>")
		bindsBefore := len(bindsSent(t, bindLog))
		_, refusal := a.do(context.Background(), coreAction(t, "s3", "folder.setUpTeam", map[string]any{"id": personal.FolderID, "path": dir, "existing": true}), ChannelTray)
		if refusal == nil || refusal.Code != appcore.RefuseConfirmNeeded || refusal.Counts == nil ||
			*refusal.Counts != (appcore.SyncCounts{ToUpload: 2, ToDownload: 1, Differ: 1}) {
			t.Fatalf("refusal = %+v", refusal)
		}
		if got := len(bindsSent(t, bindLog)); got != bindsBefore {
			t.Fatalf("a bind was sent before the person confirmed")
		}
		if _, ok := a.rt.cfg.ServedFolderByID(personal.FolderID); ok {
			t.Fatal("the folder was served before the person confirmed")
		}
		for _, row := range savedSync(t, a).SyncSessions {
			if row.RootID == personal.FolderID {
				t.Fatalf("a row was added before the person confirmed: %+v", row)
			}
		}

		_, refusal = a.do(context.Background(), coreAction(t, "s4", "folder.setUpTeam", map[string]any{"id": personal.FolderID, "path": dir, "existing": true, "confirmed": true}), ChannelTray)
		if refusal != nil {
			t.Fatalf("confirmed set up refused: %+v", refusal)
		}
		waitForBinds(t, a)
		binds := bindsSent(t, bindLog)
		if len(binds) != bindsBefore+1 || binds[len(binds)-1].Mode != "existing" || binds[len(binds)-1].Root.ID != personal.FolderID {
			t.Fatalf("binds = %+v", binds[bindsBefore:])
		}
		if data, err := os.ReadFile(filepath.Join(dir, "notes.html")); err != nil || string(data) != "<p>mine</p>" {
			t.Fatalf("the file changed: %q %v", data, err)
		}
	})

	t.Run("a bind that cannot resume drops its row and stops serving, keeping the folder", func(t *testing.T) {
		writeTestFile(t, answer, `{"ok":false,"error":"inventory-incomplete","resumable":false}`)
		dir := filepath.Join(a.rt.home, "Lab")
		if refusal := setUpThroughDo(t, a, "s5", lab.FolderID, dir); refusal != nil {
			t.Fatalf("set up refused: %+v", refusal)
		}
		waitForBinds(t, a)
		for _, row := range savedSync(t, a).SyncSessions {
			if row.RootID == lab.FolderID {
				t.Fatalf("the row stayed: %+v", row)
			}
		}
		if served, ok := a.rt.cfg.ServedFolderByID(lab.FolderID); ok && served.Serving {
			t.Fatalf("the folder is still served: %+v", served)
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("the folder is gone: %v", err)
		}
		if _, ok := a.rt.cfg.ServedFolderByID(lab.FolderID); ok {
			t.Fatal("the served entry stayed, so the folder reads as on this computer")
		}
		if a.trustedAt(dir) {
			t.Fatal("the folder stayed trusted after its set-up failed")
		}
		seen := 0
		for _, f := range a.snapshot().Folders {
			if f.ID == lab.FolderID {
				seen++
				if f.Sync.SetUpFailed != "inventory-incomplete" {
					t.Fatalf("the snapshot says the set-up failed for %q, want the engine's reason", f.Sync.SetUpFailed)
				}
			}
		}
		if seen != 1 {
			t.Fatalf("the snapshot lists folder %s %d times, want once", lab.FolderID, seen)
		}

		writeTestFile(t, answer, `{"ok":true,"files":0}`)
		if refusal := setUpThroughDo(t, a, "s6", lab.FolderID, dir); refusal != nil {
			t.Fatalf("a second set up after the failure was refused: %+v", refusal)
		}
		waitForBinds(t, a)
		if served, ok := a.rt.cfg.ServedFolderByID(lab.FolderID); !ok || !served.Serving {
			t.Fatalf("the second set up did not serve the folder: %+v %v", served, ok)
		}
		if a.rt.syncBindFailed[lab.FolderID] != "" {
			t.Fatal("a new set-up kept the last failure's reason")
		}
	})

	t.Run("a team the server does not sync is not offered and is refused", func(t *testing.T) {
		for _, e := range a.facts().Events {
			if e.SubjectID == press.FolderID {
				t.Fatalf("a view-only team was offered: %+v", e)
			}
		}
		refusal := setUpThroughDo(t, a, "s7", press.FolderID, filepath.Join(a.rt.home, "Press"))
		if refusal == nil || refusal.Code != appcore.RefuseNotAllowedNow {
			t.Fatalf("refusal = %+v, want %s", refusal, appcore.RefuseNotAllowedNow)
		}
		if _, ok := a.rt.cfg.ServedFolderByID(press.FolderID); ok {
			t.Fatal("a view-only team folder was served")
		}
	})
}

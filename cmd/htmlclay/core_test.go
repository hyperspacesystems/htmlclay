package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
)

// coreAction builds an action the way a door sends one.
func coreAction(t *testing.T, id, typ string, args any) appcore.Action {
	t.Helper()
	var raw json.RawMessage
	if args != nil {
		data, err := json.Marshal(args)
		if err != nil {
			t.Fatalf("marshal %s args: %v", typ, err)
		}
		raw = data
	}
	return appcore.Action{ID: id, Type: typ, Args: raw}
}

// snapshotFolder answers the snapshot's folder with id.
func snapshotFolder(t *testing.T, snapshot appcore.Snapshot, id string) appcore.Folder {
	t.Helper()
	for _, folder := range snapshot.Folders {
		if folder.ID == id {
			return folder
		}
	}
	t.Fatalf("no folder %q in %+v", id, snapshot.Folders)
	return appcore.Folder{}
}

// snapshotFolderAt answers the snapshot's folder whose path is path.
func snapshotFolderAt(t *testing.T, snapshot appcore.Snapshot, path string) appcore.Folder {
	t.Helper()
	for _, folder := range snapshot.Folders {
		if folder.Path == path {
			return folder
		}
	}
	t.Fatalf("no folder at %q in %+v", path, snapshot.Folders)
	return appcore.Folder{}
}

func evalTempHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return home
}

func TestCoreFactsListServedAndTrusted(t *testing.T) {
	home := evalTempHome(t)
	servedDir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(servedDir, "index.html"), servedTestDocument)
	trustedDir := filepath.Join(home, "proj")
	writeTestFile(t, filepath.Join(trustedDir, "page.htmlclay"), servedTestDocument)
	innerDir := filepath.Join(trustedDir, "inner")
	if err := os.MkdirAll(innerDir, 0755); err != nil {
		t.Fatal(err)
	}

	a := newServedTestApp(t, home, t.TempDir())
	if err := a.trustFolder(trustedDir); err != nil {
		t.Fatalf("trustFolder(%s): %v", trustedDir, err)
	}
	if err := a.trustFolder(innerDir); err != nil {
		t.Fatalf("trustFolder(%s): %v", innerDir, err)
	}
	entry, refusal, err := a.serveFolder(servedDir, "notes")
	if err != nil || refusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
	}

	snapshot := a.snapshot()
	served := snapshotFolder(t, snapshot, entry.ID)
	if !served.Served || !served.Trusted {
		t.Errorf("served folder = served %v trusted %v, want both", served.Served, served.Trusted)
	}
	if want := servedHostName("notes", servedTestPort); served.Address != want {
		t.Errorf("served folder address = %q, want %q", served.Address, want)
	}
	if !served.Can.StopServing {
		t.Error("served folder cannot stop serving")
	}

	trusted := snapshotFolderAt(t, snapshot, trustedDir)
	if want := "t" + shortHash(trustedDir); trusted.ID != want {
		t.Errorf("trusted folder id = %q, want %q", trusted.ID, want)
	}
	if trusted.Served {
		t.Error("trusted folder is served")
	}
	if !trusted.Can.Serve {
		t.Errorf("trusted folder cannot serve: %+v %+v", trusted.Can, trusted.Why)
	}
	if trusted.ParentID != nil {
		t.Errorf("outer trusted folder parent = %q, want none", *trusted.ParentID)
	}

	inner := snapshotFolderAt(t, snapshot, innerDir)
	if inner.ParentID == nil || *inner.ParentID != trusted.ID {
		t.Errorf("inner trusted folder parent = %v, want %q", inner.ParentID, trusted.ID)
	}
}

func TestCoreServeThroughDo(t *testing.T) {
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	snapshot, refusal := a.do(context.Background(), coreAction(t, "serve-1", "folder.serve", map[string]string{
		"path": dir,
		"name": "notes",
	}), ChannelTray)
	if refusal != nil {
		t.Fatalf("folder.serve refused: %+v", refusal)
	}
	served := snapshotFolderAt(t, snapshot, dir)
	if !served.Served || served.Address != servedHostName("notes", servedTestPort) {
		t.Fatalf("returned snapshot folder = %+v, want it served at notes", served)
	}

	rec := servedRequest(t, a, "GET", servedHostName("notes", servedTestPort), "/index.html", "", nil)
	if rec.Code != 200 {
		t.Fatalf("GET index.html = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}

func TestCoreRefusals(t *testing.T) {
	t.Run("stop trusting a served folder", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
		a := newServedTestApp(t, home, t.TempDir())
		entry, refusal, err := a.serveFolder(dir, "notes")
		if err != nil || refusal != nil {
			t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
		}
		_, refused := a.do(context.Background(), coreAction(t, "stop-trust", "folder.stopTrusting", map[string]string{"id": entry.ID}), ChannelTray)
		if refused == nil || refused.Code != "stop-serving-first" {
			t.Fatalf("folder.stopTrusting = %+v, want stop-serving-first", refused)
		}
	})

	t.Run("serve home", func(t *testing.T) {
		home := evalTempHome(t)
		a := buildTestApp(t, home, t.TempDir())
		_, refusal := a.do(context.Background(), coreAction(t, "serve-home", "folder.serve", map[string]string{"path": home, "name": "home"}), ChannelTray)
		if refusal == nil || refusal.Code != appcore.RefuseIsHome {
			t.Fatalf("folder.serve of home = %+v, want %q", refusal, appcore.RefuseIsHome)
		}
	})

	t.Run("unknown folder", func(t *testing.T) {
		home := evalTempHome(t)
		a := buildTestApp(t, home, t.TempDir())
		_, refusal := a.do(context.Background(), coreAction(t, "stop-unknown", "folder.stopServing", map[string]string{"id": "t000000000000000"}), ChannelTray)
		if refusal == nil || refusal.Code != appcore.RefuseNoSuchFolder {
			t.Fatalf("folder.stopServing of an unknown id = %+v, want %q", refusal, appcore.RefuseNoSuchFolder)
		}
	})

	t.Run("nothing runs sync yet", func(t *testing.T) {
		home := evalTempHome(t)
		a := buildTestApp(t, home, t.TempDir())
		_, refusal := a.do(context.Background(), coreAction(t, "connect", "sync.connect", map[string]string{"username": "sam", "key": "hcsk_x"}), ChannelTray)
		if refusal == nil || refusal.Code != appcore.RefuseNotYet {
			t.Fatalf("sync.connect = %+v, want %q", refusal, appcore.RefuseNotYet)
		}
	})
}

func TestCoreRepeatedIDAnswersOnce(t *testing.T) {
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	action := coreAction(t, "same-id", "folder.serve", map[string]string{"path": dir, "name": "notes"})
	first, firstRefusal := a.do(context.Background(), action, ChannelTray)
	if firstRefusal != nil {
		t.Fatalf("first folder.serve refused: %+v", firstRefusal)
	}
	if len(a.rt.cfg.ServedFolderList()) != 1 {
		t.Fatalf("%d served folders after one serve, want 1", len(a.rt.cfg.ServedFolderList()))
	}

	second, secondRefusal := a.do(context.Background(), action, ChannelTray)
	if secondRefusal != nil {
		t.Fatalf("repeated folder.serve refused: %+v", secondRefusal)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("a repeated id answered differently:\n first = %+v\nsecond = %+v", first, second)
	}
	if len(a.rt.cfg.ServedFolderList()) != 1 {
		t.Fatalf("%d served folders after a repeated serve, want 1", len(a.rt.cfg.ServedFolderList()))
	}
}

func TestCoreDashboardCannotGrant(t *testing.T) {
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	_, refusal := a.do(context.Background(), coreAction(t, "trust-1", "folder.trust", map[string]string{"path": dir}), ChannelDashboard)
	if refusal == nil || refusal.Code != appcore.RefuseNeedsConfirmation {
		t.Fatalf("folder.trust from the dashboard = %+v, want %q", refusal, appcore.RefuseNeedsConfirmation)
	}
	if trusted := a.rt.cfg.TrustedFolderList(); len(trusted) != 0 {
		t.Fatalf("the dashboard trusted %+v, want nothing trusted", trusted)
	}

	entry, servedRefusal, err := a.serveFolder(dir, "notes")
	if err != nil || servedRefusal != nil {
		t.Fatalf("serveFolder: refusal=%+v err=%v", servedRefusal, err)
	}
	snapshot, refusal := a.do(context.Background(), coreAction(t, "stop-1", "folder.stopServing", map[string]string{"id": entry.ID}), ChannelDashboard)
	if refusal != nil {
		t.Fatalf("folder.stopServing from the dashboard refused: %+v", refusal)
	}
	if served := snapshotFolder(t, snapshot, entry.ID); served.Served {
		t.Error("the folder is still served after the dashboard stopped it")
	}
}

func TestCoreSubscribeSeesChange(t *testing.T) {
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	before := a.snapshot().Rev
	updates, unsubscribe := a.subscribe()
	defer unsubscribe()

	if _, refusal := a.do(context.Background(), coreAction(t, "serve-2", "folder.serve", map[string]string{"path": dir, "name": "notes"}), ChannelTray); refusal != nil {
		t.Fatalf("folder.serve refused: %+v", refusal)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case snapshot := <-updates:
			if snapshot.Rev <= before {
				continue
			}
			for _, folder := range snapshot.Folders {
				if folder.Path == dir && folder.Served {
					return
				}
			}
		case <-deadline:
			t.Fatal("no snapshot showing the served folder arrived within 2s")
		}
	}
}

func TestCoreMarkSeenClearsDot(t *testing.T) {
	home := evalTempHome(t)
	gone := filepath.Join(home, "gone")
	if err := os.MkdirAll(gone, 0755); err != nil {
		t.Fatal(err)
	}

	a := buildTestApp(t, home, t.TempDir())
	if err := a.trustFolder(gone); err != nil {
		t.Fatalf("trustFolder: %v", err)
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	snapshot := a.snapshot()
	var item *appcore.WaitingItem
	for i := range snapshot.Waiting {
		if snapshot.Waiting[i].Kind == appcore.WaitingFolderMissing {
			item = &snapshot.Waiting[i]
		}
	}
	if item == nil {
		t.Fatalf("no missing-folder waiting item in %+v", snapshot.Waiting)
	}
	if !item.EarnsDot || item.Seen {
		t.Fatalf("missing folder item = %+v, want it unseen and earning the dot", item)
	}
	if !snapshot.Counts.Dot || snapshot.Counts.Total != 1 {
		t.Fatalf("counts = %+v, want the dot on and one item", snapshot.Counts)
	}
	itemID := item.ID

	seen, refusal := a.do(context.Background(), coreAction(t, "seen-1", "waiting.markSeen", map[string][]string{"ids": {itemID}}), ChannelTray)
	if refusal != nil {
		t.Fatalf("waiting.markSeen refused: %+v", refusal)
	}
	if seen.Counts.Dot {
		t.Errorf("the dot is still on after marking the item seen: %+v", seen.Waiting)
	}
	if seen.Counts.Total != 1 {
		t.Errorf("total = %d after marking seen, want 1", seen.Counts.Total)
	}
	for _, waiting := range seen.Waiting {
		if waiting.ID == itemID && !waiting.Seen {
			t.Errorf("item %s is still unseen", itemID)
		}
	}
}

func TestCorePreferences(t *testing.T) {
	home := evalTempHome(t)
	a := buildTestApp(t, home, t.TempDir())

	snapshot, refusal := a.do(context.Background(), coreAction(t, "pref-1", "prefs.set", map[string]any{"key": "aiEditing", "value": false}), ChannelTray)
	if refusal != nil {
		t.Fatalf("prefs.set aiEditing refused: %+v", refusal)
	}
	if a.rt.cfg.AIEditEnabled() {
		t.Error("AI editing is still on in config")
	}
	if snapshot.Preferences.AIEditing {
		t.Error("AI editing is still on in the snapshot")
	}

	_, refusal = a.do(context.Background(), coreAction(t, "pref-2", "prefs.set", map[string]any{"key": "appearance", "value": "dark"}), ChannelTray)
	if refusal == nil || refusal.Code != appcore.RefuseNotYet {
		t.Fatalf("prefs.set appearance = %+v, want %q", refusal, appcore.RefuseNotYet)
	}
}

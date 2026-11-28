package main

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/mirror"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/shellapi"
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
	// The id is the one the folder keeps for life, minted when it was trusted, not
	// one derived from its path.
	wantID := ""
	for _, tf := range a.rt.cfg.TrustedFolderList() {
		if tf.Path == trustedDir {
			wantID = tf.ID
		}
	}
	if wantID == "" || trusted.ID != wantID {
		t.Errorf("trusted folder id = %q, want the folder's own id %q", trusted.ID, wantID)
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

	// A folder trusted inside a trusted folder has no address of its own (7.33),
	// so it is part of its parent and has no parent to merge into.
	inner := snapshotFolderAt(t, snapshot, innerDir)
	if inner.ParentID != nil {
		t.Errorf("inner trusted folder without its own address has parent %q, want none", *inner.ParentID)
	}
	if inner.Can.Merge {
		t.Error("inner trusted folder without its own address can merge")
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
	t.Run("stop trusting a served folder cascades and stays untrusted after reload", func(t *testing.T) {
		home := evalTempHome(t)
		cfgBase := t.TempDir()
		dir := filepath.Join(home, "notes")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
		a := newServedTestApp(t, home, cfgBase)
		entry, refusal, err := a.serveFolder(dir, "notes")
		if err != nil || refusal != nil {
			t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
		}
		snapshot, refused := a.do(context.Background(), coreAction(t, "stop-trust", "folder.stopTrusting", map[string]any{"id": entry.ID}), ChannelTray)
		if refused != nil {
			t.Fatalf("folder.stopTrusting refused: %+v", refused)
		}
		if len(snapshot.Folders) != 0 {
			t.Fatalf("folders after stopTrusting = %+v, want empty", snapshot.Folders)
		}
		// Guard Spike 3 trap: reload config from disk and confirm sanitizeServedFolders does NOT resurrect it.
		reloaded, _, err := config.LoadFrom(cfgBase, platform.DirIdentity)
		if err != nil {
			t.Fatalf("config.LoadFrom: %v", err)
		}
		if len(reloaded.TrustedFolderList()) != 0 || len(reloaded.ServedFolderList()) != 0 {
			t.Fatalf("after reload: trusted=%+v served=%+v, want both empty", reloaded.TrustedFolderList(), reloaded.ServedFolderList())
		}
	})

	t.Run("stop trusting a served folder with unsaved edits refuses until forced", func(t *testing.T) {
		home := evalTempHome(t)
		cfgBase := t.TempDir()
		dir := filepath.Join(home, "notes")
		docPath := filepath.Join(dir, "index.html")
		writeTestFile(t, docPath, servedTestDocument)
		a := newServedTestApp(t, home, cfgBase)
		entry, refusal, err := a.serveFolder(dir, "notes")
		if err != nil || refusal != nil {
			t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
		}
		a.noteUnsavedReport(docPath, "tab-1", 1, true, false)

		_, refused := a.do(context.Background(), coreAction(t, "stop-trust-dirty", "folder.stopTrusting", map[string]any{"id": entry.ID}), ChannelTray)
		if refused == nil || refused.Code != appcore.RefuseUnsavedEdits {
			t.Fatalf("stopTrusting dirty = %+v, want %q", refused, appcore.RefuseUnsavedEdits)
		}
		if len(refused.Documents) == 0 {
			t.Fatalf("stopTrusting dirty refusal missing Documents: %+v", refused)
		}
		if sf, ok := a.rt.cfg.ServedFolderByID(entry.ID); !ok || !sf.Serving {
			t.Fatalf("refused stopTrusting mutated served state: ok=%v sf=%+v", ok, sf)
		}
		if len(a.rt.cfg.TrustedFolderList()) != 1 {
			t.Fatalf("refused stopTrusting mutated trusted state: %+v", a.rt.cfg.TrustedFolderList())
		}

		// Retry with force: true succeeds cleanly and tears down mount + served entry + trust.
		_, refused = a.do(context.Background(), coreAction(t, "stop-trust-force", "folder.stopTrusting", map[string]any{"id": entry.ID, "force": true}), ChannelTray)
		if refused != nil {
			t.Fatalf("forced stopTrusting refused: %+v", refused)
		}
		if len(a.rt.cfg.TrustedFolderList()) != 0 || len(a.rt.cfg.ServedFolderList()) != 0 {
			t.Fatalf("after forced stopTrusting: trusted=%+v served=%+v, want both empty", a.rt.cfg.TrustedFolderList(), a.rt.cfg.ServedFolderList())
		}
	})

	t.Run("stop trusting a synced folder requires confirmation", func(t *testing.T) {
		home := evalTempHome(t)
		cfgBase := t.TempDir()
		dir := filepath.Join(home, "synced")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
		a := newServedTestApp(t, home, cfgBase)
		entry, refusal, err := a.serveFolder(dir, "synced")
		if err != nil || refusal != nil {
			t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
		}
		syncCfg, err := config.LoadSync(a.rt.configDir)
		if err != nil {
			t.Fatal(err)
		}
		syncCfg.Account = &config.SyncAccount{ActorID: 1, Username: "sam"}
		syncCfg.Accounts = []config.SyncAccountRow{{AccountID: 1, Kind: "personal", Username: "sam", FolderID: entry.ID}}
		syncCfg.SyncSessions = []config.SyncSession{{ID: "sess-1", RootID: entry.ID, AccountID: 1, Enabled: true}}
		if err := syncCfg.Save(); err != nil {
			t.Fatal(err)
		}
		a.rt.syncMu.Lock()
		a.rt.syncRecord = syncCfg
		a.rt.syncMu.Unlock()

		_, refused := a.do(context.Background(), coreAction(t, "stop-trust-sync-unconfirmed", "folder.stopTrusting", map[string]any{"id": entry.ID}), ChannelTray)
		if refused == nil || refused.Code != appcore.RefuseConfirmSyncStop {
			t.Fatalf("unconfirmed stopTrusting on synced folder = %+v, want %q", refused, appcore.RefuseConfirmSyncStop)
		}
		reloadedSync, err := config.LoadSync(a.rt.configDir)
		if err != nil || len(reloadedSync.SyncSessions) != 1 || !reloadedSync.SyncSessions[0].Enabled {
			t.Fatalf("unconfirmed stopTrusting disabled sync: %+v %v", reloadedSync, err)
		}

		_, refused = a.do(context.Background(), coreAction(t, "stop-trust-sync-confirmed", "folder.stopTrusting", map[string]any{"id": entry.ID, "confirmed": true}), ChannelTray)
		if refused != nil {
			t.Fatalf("confirmed stopTrusting on synced folder refused: %+v", refused)
		}
		reloadedSync, err = config.LoadSync(a.rt.configDir)
		if err != nil || len(reloadedSync.SyncSessions) != 1 || reloadedSync.SyncSessions[0].Enabled {
			t.Fatalf("confirmed stopTrusting did not stop sync: %+v %v", reloadedSync, err)
		}
		if len(a.rt.cfg.TrustedFolderList()) != 0 || len(a.rt.cfg.ServedFolderList()) != 0 {
			t.Fatalf("after confirmed stopTrusting: trusted=%+v served=%+v, want both empty", a.rt.cfg.TrustedFolderList(), a.rt.cfg.ServedFolderList())
		}
	})

	t.Run("app.openSettings is offered only on macOS shell and delivers FrameOpenSettings", func(t *testing.T) {
		own := newTestAppAs(t, t.TempDir(), t.TempDir(), false)
		if own.snapshot().Host.Can["app.openSettings"] {
			t.Fatal("non-shell host must not advertise app.openSettings")
		}
		shell := newTestAppAs(t, t.TempDir(), t.TempDir(), true)
		if !shell.snapshot().Host.Can["app.openSettings"] {
			t.Fatal("shell host must advertise app.openSettings")
		}
		frames, stop := shell.rt.notices.subscribe()
		defer stop()
		if _, refusal := shell.do(context.Background(), coreAction(t, "open-settings", "app.openSettings", map[string]string{"pane": "ai"}), ChannelDashboard); refusal != nil {
			t.Fatalf("app.openSettings refused: %+v", refusal)
		}
		select {
		case frame := <-frames:
			if frame.Type != shellapi.FrameOpenSettings || frame.Detail != "ai" {
				t.Fatalf("frame = %+v, want type=%q detail=ai", frame, shellapi.FrameOpenSettings)
			}
		default:
			t.Fatal("app.openSettings did not deliver a notice frame")
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

	t.Run("a malformed sync key is refused before anything runs", func(t *testing.T) {
		home := evalTempHome(t)
		a := buildTestApp(t, home, t.TempDir())
		_, refusal := a.do(context.Background(), coreAction(t, "connect", "sync.connect", map[string]string{"username": "sam", "key": "hcsk_x"}), ChannelTray)
		if refusal == nil || refusal.Code != appcore.RefuseSyncKeyShape {
			t.Fatalf("sync.connect = %+v, want %q", refusal, appcore.RefuseSyncKeyShape)
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
	cfgBase := t.TempDir()
	a := buildTestApp(t, home, cfgBase)

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

	snapshot, refusal = a.do(context.Background(), coreAction(t, "pref-2", "prefs.set", map[string]any{"key": "appearance", "value": "dark"}), ChannelTray)
	if refusal != nil {
		t.Fatalf("prefs.set appearance refused: %+v", refusal)
	}
	if snapshot.Preferences.Appearance != appcore.AppearanceDark {
		t.Errorf("appearance = %q in the snapshot, want %q", snapshot.Preferences.Appearance, appcore.AppearanceDark)
	}
	reloaded, _, err := config.LoadFrom(cfgBase, platform.DirIdentity)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if got := reloaded.Appearance(); got != appcore.AppearanceDark {
		t.Errorf("appearance = %q after a reload, want %q", got, appcore.AppearanceDark)
	}

	_, refusal = a.do(context.Background(), coreAction(t, "pref-3", "prefs.set", map[string]any{"key": "appearance", "value": "purple"}), ChannelTray)
	if refusal == nil || refusal.Code != appcore.RefuseBadArguments {
		t.Fatalf("prefs.set appearance purple = %+v, want %q", refusal, appcore.RefuseBadArguments)
	}
}

func TestCoreOfflineCopiesSettings(t *testing.T) {
	home := evalTempHome(t)
	cfgBase := t.TempDir()
	a := buildTestApp(t, home, cfgBase)
	store, err := mirror.Open(t.TempDir(), mirror.DefaultLimits)
	if err != nil {
		t.Fatalf("open the mirror: %v", err)
	}
	a.rt.mirror = &mirror.Handler{Store: store, Off: func() bool { return !a.rt.cfg.OfflineCopies() }}
	if err := store.Put("a.localhost", "https://cdn.example/a.js", "text/javascript", []byte("x=1"), "page"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	snapshot := a.snapshot()
	if !snapshot.Offline.Available {
		t.Error("offline copies are unavailable with a mirror open")
	}
	if snapshot.Offline.Files != 1 {
		t.Errorf("offline files = %d, want 1", snapshot.Offline.Files)
	}
	if snapshot.Offline.Bytes <= 0 {
		t.Errorf("offline bytes = %d, want more than 0", snapshot.Offline.Bytes)
	}
	if !snapshot.Preferences.OfflineCopies {
		t.Error("offline copies read as off in the snapshot, want on")
	}
	if snapshot.Preferences.OfflineSpace != "500mb" {
		t.Errorf("offline space = %q in the snapshot, want 500mb", snapshot.Preferences.OfflineSpace)
	}

	snapshot, refusal := a.do(context.Background(), coreAction(t, "off-1", "prefs.set", map[string]any{"key": "offlineSpace", "value": "unlimited"}), ChannelTray)
	if refusal != nil {
		t.Fatalf("prefs.set offlineSpace unlimited refused: %+v", refusal)
	}
	if got := a.rt.cfg.OfflineSpace(); got != "unlimited" {
		t.Errorf("OfflineSpace() = %q, want unlimited", got)
	}
	if got := store.Limits(); got != (mirror.Limits{}) {
		t.Errorf("store limits = %+v, want no limits", got)
	}
	if snapshot.Preferences.OfflineSpace != "unlimited" {
		t.Errorf("offline space = %q in the snapshot, want unlimited", snapshot.Preferences.OfflineSpace)
	}

	_, refusal = a.do(context.Background(), coreAction(t, "off-2", "prefs.set", map[string]any{"key": "offlineSpace", "value": "5gb"}), ChannelTray)
	if refusal == nil || refusal.Code != appcore.RefuseBadArguments {
		t.Fatalf("prefs.set offlineSpace 5gb = %+v, want %q", refusal, appcore.RefuseBadArguments)
	}
	if got := a.rt.cfg.OfflineSpace(); got != "unlimited" {
		t.Errorf("OfflineSpace() = %q after a bad value, want unlimited", got)
	}

	if _, refusal = a.do(context.Background(), coreAction(t, "off-3", "offline.clear", nil), ChannelTray); refusal != nil {
		t.Fatalf("offline.clear refused: %+v", refusal)
	}
	if bytes, files := store.Usage(); bytes != 0 || files != 0 {
		t.Errorf("usage after offline.clear = %d bytes, %d files, want 0 and 0", bytes, files)
	}
	if !a.rt.cfg.OfflineCopies() {
		t.Error("offline.clear switched offline copies off, want them still on")
	}

	if err := store.Put("a.localhost", "https://cdn.example/a.js", "text/javascript", []byte("x=1"), "page"); err != nil {
		t.Fatalf("Put again: %v", err)
	}
	if _, refusal = a.do(context.Background(), coreAction(t, "off-4", "prefs.set", map[string]any{"key": "offlineCopies", "value": false}), ChannelTray); refusal != nil {
		t.Fatalf("prefs.set offlineCopies false refused: %+v", refusal)
	}
	if a.rt.cfg.OfflineCopies() {
		t.Error("offline copies are still on in config")
	}
	if bytes, files := store.Usage(); bytes != 0 || files != 0 {
		t.Errorf("usage after switching off = %d bytes, %d files, want 0 and 0", bytes, files)
	}
	if got := string(a.offlineBoot([]byte("<html><head>"))); strings.Contains(got, "/_/offline/boot.js") {
		t.Errorf("a document still loads the boot script with offline copies off: %q", got)
	}
	rec := httptest.NewRecorder()
	a.rt.mirror.Serve(rec, httptest.NewRequest("GET", "http://a.localhost/_/offline/sw.js", nil), "a.localhost")
	if body := rec.Body.String(); !strings.Contains(body, "unregister()") {
		t.Errorf("the worker served with offline copies off does not unregister: %q", body)
	}
}

func TestCoreFileOpen(t *testing.T) {
	t.Run("opens a file inside home", func(t *testing.T) {
		home := evalTempHome(t)
		file := filepath.Join(home, "notes", "page.htmlclay")
		writeTestFile(t, file, servedTestDocument)

		a := newServedTestApp(t, home, t.TempDir())
		var launched []string
		a.rt.launchBrowser = func(target string) error {
			launched = append(launched, target)
			return nil
		}
		snapshot, refusal := a.do(context.Background(), coreAction(t, "open-1", "file.open", map[string]string{"path": file}), ChannelTray)
		if refusal != nil {
			t.Fatalf("file.open refused: %+v", refusal)
		}
		if len(launched) != 1 || !strings.HasPrefix(launched[0], "http://") {
			t.Fatalf("file.open launched %q, want exactly one http:// URL", launched)
		}
		name := filepath.Base(file)
		for _, doc := range snapshot.Documents {
			if doc.Name == name {
				return
			}
		}
		t.Fatalf("file.open did not list %q in %+v", name, snapshot.Documents)
	})

	t.Run("a missing file", func(t *testing.T) {
		home := evalTempHome(t)
		file := filepath.Join(home, "notes", "gone.htmlclay")

		a := newServedTestApp(t, home, t.TempDir())
		var launched []string
		a.rt.launchBrowser = func(target string) error {
			launched = append(launched, target)
			return nil
		}
		_, refusal := a.do(context.Background(), coreAction(t, "open-missing", "file.open", map[string]string{"path": file}), ChannelTray)
		if refusal == nil || refusal.Code != appcore.RefuseFileMissing {
			t.Fatalf("file.open of a missing file = %+v, want %q", refusal, appcore.RefuseFileMissing)
		}
		if len(launched) != 0 {
			t.Fatalf("file.open of a missing file launched %q, want nothing", launched)
		}
	})

	t.Run("a file outside home", func(t *testing.T) {
		home := evalTempHome(t)
		outside, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if session.EqualOrUnder(home, outside) {
			t.Skipf("%s is inside the test home %s", outside, home)
		}
		file := filepath.Join(outside, "page.htmlclay")
		writeTestFile(t, file, servedTestDocument)

		a := newServedTestApp(t, home, t.TempDir())
		var launched []string
		a.rt.launchBrowser = func(target string) error {
			launched = append(launched, target)
			return nil
		}
		_, refusal := a.do(context.Background(), coreAction(t, "open-outside", "file.open", map[string]string{"path": file}), ChannelTray)
		if refusal == nil || refusal.Code != appcore.RefuseFileOutsideHome {
			t.Fatalf("file.open of a file outside home = %+v, want %q", refusal, appcore.RefuseFileOutsideHome)
		}
		if len(launched) != 0 {
			t.Fatalf("file.open of a file outside home launched %q, want nothing", launched)
		}
	})

	t.Run("an empty path", func(t *testing.T) {
		home := evalTempHome(t)

		a := newServedTestApp(t, home, t.TempDir())
		var launched []string
		a.rt.launchBrowser = func(target string) error {
			launched = append(launched, target)
			return nil
		}
		_, refusal := a.do(context.Background(), coreAction(t, "open-empty", "file.open", map[string]string{"path": ""}), ChannelTray)
		if refusal == nil || refusal.Code != appcore.RefuseBadArguments {
			t.Fatalf("file.open with no path = %+v, want %q", refusal, appcore.RefuseBadArguments)
		}
		if len(launched) != 0 {
			t.Fatalf("file.open with no path launched %q, want nothing", launched)
		}
	})

	t.Run("a file inside a served folder", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "notes")
		file := filepath.Join(dir, "page.htmlclay")
		writeTestFile(t, file, servedTestDocument)

		a := newServedTestApp(t, home, t.TempDir())
		if _, refusal, err := a.serveFolder(dir, "notes"); err != nil || refusal != nil {
			t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
		}
		var launched []string
		a.rt.launchBrowser = func(target string) error {
			launched = append(launched, target)
			return nil
		}
		_, refusal := a.do(context.Background(), coreAction(t, "open-served", "file.open", map[string]string{"path": file}), ChannelTray)
		if refusal != nil {
			t.Fatalf("file.open of a served file refused: %+v", refusal)
		}
		want := servedFileURL("notes", "page.htmlclay")
		if len(launched) != 1 || launched[0] != want {
			t.Fatalf("file.open of a served file launched %q, want exactly %q", launched, want)
		}
	})
}

func TestCoreDashboardCannotOpenFile(t *testing.T) {
	home := evalTempHome(t)
	file := filepath.Join(home, "notes", "page.htmlclay")
	writeTestFile(t, file, servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	var launched []string
	a.rt.launchBrowser = func(target string) error {
		launched = append(launched, target)
		return nil
	}
	_, refusal := a.do(context.Background(), coreAction(t, "dashboard-open", "file.open", map[string]string{"path": file}), ChannelDashboard)
	if refusal == nil || refusal.Code != appcore.RefuseNeedsConfirmation {
		t.Fatalf("file.open from the dashboard = %+v, want %q", refusal, appcore.RefuseNeedsConfirmation)
	}
	if len(launched) != 0 {
		t.Fatalf("file.open from the dashboard launched %q, want nothing", launched)
	}
}

func TestOpenFileStillNotifies(t *testing.T) {
	home := evalTempHome(t)
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if session.EqualOrUnder(home, outside) {
		t.Skipf("%s is inside the test home %s", outside, home)
	}
	file := filepath.Join(outside, "page.htmlclay")
	writeTestFile(t, file, servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	titles := make(chan string, 4)
	a.rt.notify = func(title, message string) error {
		titles <- title
		return nil
	}
	a.rt.launchBrowser = func(string) error {
		t.Error("opening a file outside home launched a browser")
		return nil
	}
	a.openFile(file)

	select {
	case title := <-titles:
		if title != "HTML Clay can't open this file" {
			t.Fatalf("notification title = %q, want %q", title, "HTML Clay can't open this file")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("opening a file outside home did not notify")
	}
}

func TestCoreEachAnswerHasItsOwnRev(t *testing.T) {
	home := evalTempHome(t)
	a := newServedTestApp(t, home, t.TempDir())

	before := a.snapshot().Rev

	first, refusal := a.do(context.Background(), coreAction(t, "rev-on", "prefs.set", map[string]any{"key": "aiEditing", "value": true}), ChannelTray)
	if refusal != nil {
		t.Fatalf("prefs.set aiEditing = true refused: %+v", refusal)
	}
	second, refusal := a.do(context.Background(), coreAction(t, "rev-off", "prefs.set", map[string]any{"key": "aiEditing", "value": false}), ChannelTray)
	if refusal != nil {
		t.Fatalf("prefs.set aiEditing = false refused: %+v", refusal)
	}

	if !first.Preferences.AIEditing || second.Preferences.AIEditing {
		t.Fatalf("aiEditing = %v then %v, want true then false", first.Preferences.AIEditing, second.Preferences.AIEditing)
	}
	if first.Rev <= before {
		t.Errorf("first answer rev = %d, want above the starting %d", first.Rev, before)
	}
	if second.Rev <= before {
		t.Errorf("second answer rev = %d, want above the starting %d", second.Rev, before)
	}
	if second.Rev <= first.Rev {
		t.Errorf("second answer rev = %d, want above the first answer's %d", second.Rev, first.Rev)
	}
}

func TestCoreRunningActionIsReserved(t *testing.T) {
	home := evalTempHome(t)
	a := newServedTestApp(t, home, t.TempDir())
	backend := shellBackend{a}

	entered := make(chan struct{})
	release := make(chan struct{})
	var calls int32
	a.core.beforeEffect = func(appcore.Effect) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(entered)
			<-release
		}
	}

	action := coreAction(t, "slow-1", "prefs.set", map[string]any{"key": "aiEditing", "value": true})
	type outcome struct {
		snapshot appcore.Snapshot
		refusal  *appcore.Refusal
	}
	done := make(chan outcome, 1)
	go func() {
		snapshot, refusal := a.do(context.Background(), action, ChannelTray)
		done <- outcome{snapshot: snapshot, refusal: refusal}
	}()

	<-entered

	if _, refusal, ok := backend.Answer("slow-1"); !ok || refusal == nil || refusal.Code != appcore.RefuseStillRunning {
		t.Fatalf("Answer while running = %+v ok=%v, want %q", refusal, ok, appcore.RefuseStillRunning)
	}
	if _, refusal := a.do(context.Background(), action, ChannelTray); refusal == nil || refusal.Code != appcore.RefuseStillRunning {
		t.Fatalf("repeated action while running = %+v, want %q", refusal, appcore.RefuseStillRunning)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("effects ran %d times while the first was running, want 1", got)
	}

	close(release)
	result := <-done
	if result.refusal != nil {
		t.Fatalf("the running action refused: %+v", result.refusal)
	}
	answer, refusal, ok := backend.Answer("slow-1")
	if !ok || refusal != nil {
		t.Fatalf("Answer after the action = %+v ok=%v, want the answer", refusal, ok)
	}
	if answer.Rev != result.snapshot.Rev {
		t.Errorf("stored answer rev = %d, want the action's own %d", answer.Rev, result.snapshot.Rev)
	}
}

func TestCoreDocumentCount(t *testing.T) {
	home := evalTempHome(t)
	dir := filepath.Join(home, "proj")
	writeTestFile(t, filepath.Join(dir, "a.html"), servedTestDocument)
	writeTestFile(t, filepath.Join(dir, "b.htmlclay"), servedTestDocument)
	writeTestFile(t, filepath.Join(dir, "sub", "c.htm"), servedTestDocument)
	writeTestFile(t, filepath.Join(dir, "notes.txt"), "notes")
	writeTestFile(t, filepath.Join(dir, ".hyperclay", "versions", "a", "1.html"), servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	if err := a.trustFolder(dir); err != nil {
		t.Fatalf("trustFolder: %v", err)
	}
	folder := snapshotFolderAt(t, a.snapshot(), dir)
	if folder.DocumentCount != 3 {
		t.Errorf("folder documentCount = %d, want 3", folder.DocumentCount)
	}
}

func TestCoreWatcherPublishesUnannouncedChange(t *testing.T) {
	home := evalTempHome(t)
	dir := filepath.Join(home, "proj")
	writeTestFile(t, filepath.Join(dir, "a.html"), servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	if err := a.trustFolder(dir); err != nil {
		t.Fatalf("trustFolder: %v", err)
	}
	a.core.watchEvery = 50 * time.Millisecond

	updates, unsubscribe := a.subscribe()
	select {
	case <-updates:
	case <-time.After(500 * time.Millisecond):
	}

	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(3 * time.Second)
	found := false
	for !found {
		select {
		case snapshot := <-updates:
			for _, folder := range snapshot.Folders {
				if folder.Path == dir && folder.State == appcore.FolderMissing {
					found = true
				}
			}
		case <-deadline:
			t.Fatal("no snapshot showing the folder missing arrived within 3s")
		}
	}

	unsubscribe()
	gone := time.After(1 * time.Second)
	for {
		a.core.mu.Lock()
		watching := a.core.watching
		a.core.mu.Unlock()
		if !watching {
			return
		}
		select {
		case <-gone:
			t.Fatal("the facts watcher is still running a second after the last unsubscribe")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestCoreNoWatcherWithoutSubscribers(t *testing.T) {
	a := newServedTestApp(t, evalTempHome(t), t.TempDir())
	a.core.watchEvery = 50 * time.Millisecond
	time.Sleep(200 * time.Millisecond)

	a.core.mu.Lock()
	watching := a.core.watching
	counts := len(a.core.countCache)
	a.core.mu.Unlock()
	if watching {
		t.Error("the facts watcher runs with no subscriber")
	}
	if counts != 0 {
		t.Errorf("countCache holds %d folders with no subscriber, want none", counts)
	}
}

func TestCoreDocumentOpenedAt(t *testing.T) {
	home := evalTempHome(t)
	file := filepath.Join(home, "notes", "page.htmlclay")
	writeTestFile(t, file, servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	a.rt.launchBrowser = func(string) error { return nil }
	if _, refusal := a.do(context.Background(), coreAction(t, "open-opened-at", "file.open", map[string]string{"path": file}), ChannelTray); refusal != nil {
		t.Fatalf("file.open refused: %+v", refusal)
	}

	id := "d" + shortHash(file)
	openedAt := func(snapshot appcore.Snapshot) *time.Time {
		t.Helper()
		for _, doc := range snapshot.Documents {
			if doc.ID == id {
				return doc.OpenedAt
			}
		}
		t.Fatalf("no document %q in %+v", id, snapshot.Documents)
		return nil
	}

	first := openedAt(a.snapshot())
	if first == nil {
		t.Fatal("the open document has no openedAt")
	}
	if since := time.Since(*first); since > time.Minute || since < -time.Minute {
		t.Errorf("openedAt = %v, want within the last minute", first)
	}
	second := openedAt(a.snapshot())
	if second == nil || !second.Equal(*first) {
		t.Errorf("second snapshot openedAt = %v, want the first %v", second, first)
	}
}

// runEffectCases reads core_effects.go and answers every string literal in a case
// of runEffect's switch on e.Type.
func runEffectCases(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "core_effects.go", nil, 0)
	if err != nil {
		t.Fatalf("parse core_effects.go: %v", err)
	}
	var cases []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "runEffect" || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			sel, ok := sw.Tag.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Type" {
				return true
			}
			for _, stmt := range sw.Body.List {
				clause, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, expr := range clause.List {
					lit, ok := expr.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					typ, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatalf("unquote %s: %v", lit.Value, err)
					}
					cases = append(cases, typ)
				}
			}
			return true
		})
	}
	if len(cases) == 0 {
		t.Fatal("found no case in runEffect's switch on e.Type")
	}
	sort.Strings(cases)
	return cases
}

// TestImplementedActionsMatchRunEffect keeps implementedActions and the cases of
// runEffect equal: a case with no entry would say not-yet after the click, and an
// entry with no case would promise what the host cannot do.
func TestImplementedActionsMatchRunEffect(t *testing.T) {
	cases := runEffectCases(t)
	listed := append([]string(nil), implementedActions...)
	sort.Strings(listed)

	inCases := map[string]bool{}
	for _, typ := range cases {
		inCases[typ] = true
	}
	inList := map[string]bool{}
	for _, typ := range listed {
		inList[typ] = true
	}
	var missingFromList, missingFromSwitch []string
	for _, typ := range cases {
		if !inList[typ] {
			missingFromList = append(missingFromList, typ)
		}
	}
	for _, typ := range listed {
		if !inCases[typ] {
			missingFromSwitch = append(missingFromSwitch, typ)
		}
	}
	if len(missingFromList) > 0 || len(missingFromSwitch) > 0 {
		t.Fatalf("implementedActions and runEffect differ: runEffect cases missing from implementedActions: %v; implementedActions entries with no runEffect case: %v", missingFromList, missingFromSwitch)
	}
	for _, typ := range listed {
		if _, ok := appcore.ActionClasses[typ]; !ok {
			t.Fatalf("implementedActions lists %s, which is no action type", typ)
		}
	}
}

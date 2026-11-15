package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/secret"
)

const (
	connectKeyOne = "hcsk_1111111111111111111111111111111111111111111111111111111111111111"
	connectKeyTwo = "hcsk_2222222222222222222222222222222222222222222222222222222222222222"
	discoverMaya  = `{"ok":true,"actor":{"id":12,"username":"maya","fullName":"Maya Lin"},"accounts":[` +
		`{"id":12,"kind":"personal","username":"maya","displayName":"maya","role":"owner","sync":{"enabled":true,"reason":null}},` +
		`{"id":40,"kind":"team","username":"studio","displayName":"Studio","role":"viewer","webUrl":"https://hyperclay.com/_/team/studio/dashboard","sync":{"enabled":false,"reason":"plan-lapsed"}}]}`
	discoverRefused = `{"ok":false,"code":"key-refused","status":401}`
)

// connectTestApp is an app serving one folder through the stub service, with a
// memory store and the discovery answer in a file the test rewrites.
func connectTestApp(t *testing.T, serve bool) (a *app, setDiscovery func(string), record string) {
	t.Helper()
	record = stubServiceCommand(t)
	answer := filepath.Join(t.TempDir(), "discover.json")
	setDiscovery = func(body string) {
		if err := os.WriteFile(answer, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setDiscovery(discoverMaya)
	t.Setenv("HTMLCLAY_STUB_DISCOVER", answer)
	setSyncServer(t, "http://127.0.0.1:9")
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	a = newServiceTestApp(t, home, t.TempDir())
	a.rt.syncStore = secret.NewMemory()
	if serve {
		serveThroughDo(t, a, "serve-1", dir, "notes")
		waitForServiceState(t, a, appcore.ServiceRunning, 20*time.Second)
	}
	return a, setDiscovery, record
}

func connectThroughDo(t *testing.T, a *app, id, username, key string) *appcore.Refusal {
	t.Helper()
	args := map[string]string{"key": key}
	if username != "" {
		args["username"] = username
	}
	_, refusal := a.do(context.Background(), coreAction(t, id, "sync.connect", args), ChannelTray)
	return refusal
}

func storedKey(t *testing.T, a *app) string {
	t.Helper()
	key, _, err := a.rt.syncStore.Get(syncKeyName)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func savedSync(t *testing.T, a *app) *config.Sync {
	t.Helper()
	record, err := config.LoadSync(a.rt.configDir)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func lastSyncLine(t *testing.T, record string) string {
	t.Helper()
	lines := syncLinesSent(t, record)
	if len(lines) == 0 {
		t.Fatal("no sync line was sent")
	}
	return lines[len(lines)-1]
}

func TestM4Connect(t *testing.T) {
	t.Run("a good key is stored with its account and rows", func(t *testing.T) {
		a, _, record := connectTestApp(t, true)
		if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
			t.Fatalf("connect refused: %+v", refusal)
		}
		if got := storedKey(t, a); got != connectKeyOne {
			t.Fatalf("stored %q", got)
		}
		saved := savedSync(t, a)
		if saved.Account == nil || saved.Account.Username != "maya" || saved.Account.ActorID != 12 || saved.Account.FullName != "Maya Lin" ||
			saved.Account.KeySuffix != "1111" || saved.Account.KeyStore != secret.KindMemory {
			t.Fatalf("account = %+v", saved.Account)
		}
		if len(saved.Accounts) != 2 || saved.Accounts[1].Username != "studio" || saved.Accounts[1].Sync.Enabled || *saved.Accounts[1].Sync.Reason != "plan-lapsed" {
			t.Fatalf("rows = %+v", saved.Accounts)
		}
		id := regexp.MustCompile(`^[0-9a-f]{16}$`)
		if !id.MatchString(saved.Accounts[0].FolderID) || saved.Accounts[0].FolderID == saved.Accounts[1].FolderID {
			t.Fatalf("folder ids %q %q", saved.Accounts[0].FolderID, saved.Accounts[1].FolderID)
		}
		if !strings.Contains(lastSyncLine(t, record), connectKeyOne) {
			t.Fatal("the service was not handed the key")
		}

		snapshot := a.snapshot()
		if !snapshot.Sync.Connected || snapshot.Sync.Username != "maya" || snapshot.Sync.KeySuffix != "1111" {
			t.Fatalf("snapshot sync = %+v", snapshot.Sync)
		}
		var teamFolder bool
		for _, folder := range snapshot.Folders {
			if folder.ID == saved.Accounts[1].FolderID && folder.Origin.Kind == appcore.OriginTeam && folder.Origin.Team == "studio" {
				teamFolder = true
			}
		}
		if !teamFolder {
			t.Fatalf("the team is not a folder in the snapshot: %+v", snapshot.Folders)
		}
		var toSetUp int
		for _, item := range snapshot.Waiting {
			if item.Kind == appcore.WaitingTeamToSetUp && item.SubjectID != nil && *item.SubjectID == saved.Accounts[1].FolderID {
				toSetUp++
			}
		}
		if toSetUp != 1 {
			t.Fatalf("waiting items for the team to set up = %d, want 1: %+v", toSetUp, snapshot.Waiting)
		}

		before := saved.Accounts[1].FolderID
		if refusal := connectThroughDo(t, a, "c2", "@MAYA", connectKeyTwo); refusal != nil {
			t.Fatalf("a typed username in another case refused: %+v", refusal)
		}
		if after := savedSync(t, a).Accounts[1].FolderID; after != before {
			t.Fatalf("a row's folder id changed from %s to %s", before, after)
		}
	})

	t.Run("a refused key is not stored", func(t *testing.T) {
		a, setDiscovery, record := connectTestApp(t, true)
		setDiscovery(discoverRefused)
		refusal := connectThroughDo(t, a, "c1", "maya", connectKeyOne)
		if refusal == nil || refusal.Code != appcore.RefuseSyncKeyRefused {
			t.Fatalf("refusal = %+v, want %s", refusal, appcore.RefuseSyncKeyRefused)
		}
		if got := storedKey(t, a); got != "" {
			t.Fatal("a refused key was stored")
		}
		if savedSync(t, a).Account != nil {
			t.Fatal("a refused key left an account")
		}
		if got := lastSyncLine(t, record); got != `{"sync":{"v":1,"key":null}}` {
			t.Fatalf("the service still holds the refused key: last line %q", got)
		}
	})

	t.Run("a key for another account is refused", func(t *testing.T) {
		a, _, _ := connectTestApp(t, true)
		refusal := connectThroughDo(t, a, "c1", "sam", connectKeyOne)
		if refusal == nil || refusal.Code != appcore.RefuseSyncWrongAccount {
			t.Fatalf("refusal = %+v, want %s", refusal, appcore.RefuseSyncWrongAccount)
		}
		if storedKey(t, a) != "" {
			t.Fatal("a key for another account was stored")
		}
	})

	t.Run("a failed reconnect keeps the working key", func(t *testing.T) {
		a, setDiscovery, record := connectTestApp(t, true)
		if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
			t.Fatalf("connect refused: %+v", refusal)
		}
		setDiscovery(discoverRefused)
		if refusal := connectThroughDo(t, a, "c2", "", connectKeyTwo); refusal == nil {
			t.Fatal("a refused key connected")
		}
		if storedKey(t, a) != connectKeyOne {
			t.Fatal("the working key was replaced")
		}
		if !strings.Contains(lastSyncLine(t, record), connectKeyOne) {
			t.Fatal("the service was not given the working key back")
		}
	})

	t.Run("a malformed key never reaches the service", func(t *testing.T) {
		a, _, record := connectTestApp(t, true)
		refusal := connectThroughDo(t, a, "c1", "", "hcsk_short")
		if refusal == nil || refusal.Code != appcore.RefuseSyncKeyShape {
			t.Fatalf("refusal = %+v", refusal)
		}
		if lines := syncLinesSent(t, record); len(lines) != 0 {
			t.Fatalf("sent %d lines", len(lines))
		}
	})

	t.Run("with no folder served connect starts the service, and disconnect lets it stop", func(t *testing.T) {
		a, _, _ := connectTestApp(t, false)
		if state := a.service.state(); state == appcore.ServiceRunning {
			t.Fatalf("the service runs with nothing served: %s", state)
		}
		if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
			t.Fatalf("connect refused: %+v", refusal)
		}
		if storedKey(t, a) != connectKeyOne {
			t.Fatal("the key was not stored")
		}
		waitForServiceState(t, a, appcore.ServiceRunning, 5*time.Second)
		if refusal := a.disconnectSync(); refusal != nil {
			t.Fatalf("disconnect refused: %+v", refusal)
		}
		waitForServiceState(t, a, appcore.ServiceOff, 20*time.Second)
	})

	t.Run("with no service program connect needs one", func(t *testing.T) {
		a, _, _ := connectTestApp(t, false)
		a.service.command = nil
		refusal := connectThroughDo(t, a, "c1", "", connectKeyOne)
		if refusal == nil || refusal.Code != appcore.RefuseServiceNeeded {
			t.Fatalf("refusal = %+v, want %s", refusal, appcore.RefuseServiceNeeded)
		}
	})

	t.Run("disconnect forgets the key and keeps the session rows switched off", func(t *testing.T) {
		a, _, record := connectTestApp(t, true)
		if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
			t.Fatalf("connect refused: %+v", refusal)
		}
		a.rt.syncMu.Lock()
		a.rt.syncRecord.SyncSessions = []config.SyncSession{{ID: "s1", RootID: "0123456789abcdef", AccountID: 12, Kind: "personal", Enabled: true}}
		a.rt.syncMu.Unlock()
		if _, refusal := a.do(context.Background(), coreAction(t, "d1", "sync.disconnect", nil), ChannelTray); refusal != nil {
			t.Fatalf("disconnect refused: %+v", refusal)
		}
		if storedKey(t, a) != "" {
			t.Fatal("the key is still stored")
		}
		saved := savedSync(t, a)
		if saved.Account != nil || len(saved.SyncSessions) != 1 || saved.SyncSessions[0].Enabled {
			t.Fatalf("after disconnect: account %+v sessions %+v", saved.Account, saved.SyncSessions)
		}
		if got := lastSyncLine(t, record); got != `{"sync":{"v":1,"key":null}}` {
			t.Fatalf("last line %q, want the clear", got)
		}
	})
}

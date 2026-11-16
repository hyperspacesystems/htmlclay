package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/synchistory"
)

// TestM4ConflictResolve is 08-sync.md step 8.32: conflict.resolve checks the
// choice against the held conflict's kind and sends it to the service, and the
// service's answers become the person's refusals.
func TestM4ConflictResolve(t *testing.T) {
	scratch := t.TempDir()
	resolveLog := filepath.Join(scratch, "resolve")
	answer := filepath.Join(scratch, "answer")
	t.Setenv("HTMLCLAY_STUB_BIND_LOG", filepath.Join(scratch, "bind"))
	t.Setenv("HTMLCLAY_STUB_RESOLVE_LOG", resolveLog)
	t.Setenv("HTMLCLAY_STUB_RESOLVE", answer)
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
	writeTestFile(t, filepath.Join(a.conflictsMetaDir(session), "conflicts.json"), `{
  "901": {"kind": "both-edited", "path": "board.html", "remoteCopy": ".hyperclay/conflicts/board.remote-bb22bb22.html", "detectedAt": 1790000000000},
  "902": {"kind": "remote-deleted", "path": "gone.html", "detectedAt": 1790000000100}
}`)
	a.readHeldConflicts()
	both := session.ID + ":901:1790000000000"
	deleted := session.ID + ":902:1790000000100"

	answerWith := func(body string) {
		t.Helper()
		if err := os.WriteFile(answer, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sent := func() []map[string]string {
		t.Helper()
		data, err := os.ReadFile(resolveLog)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		var lines []map[string]string
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			var body map[string]string
			if err := json.Unmarshal([]byte(line), &body); err != nil {
				t.Fatalf("resolve line %q: %v", line, err)
			}
			lines = append(lines, body)
		}
		return lines
	}
	resolve := func(id, conflict, path, keep string) *appcore.Refusal {
		t.Helper()
		_, refusal := a.do(context.Background(), coreAction(t, id, "conflict.resolve", map[string]string{"id": conflict, "path": path, "keep": keep}), ChannelTray)
		return refusal
	}
	code := func(r *appcore.Refusal) string {
		if r == nil {
			return "ok"
		}
		return r.Code
	}

	t.Run("a choice goes to the service for that session and file", func(t *testing.T) {
		answerWith(`{"ok":true}`)
		if r := resolve("r1", both, "board.html", "theirs"); r != nil {
			t.Fatalf("refused: %+v", r)
		}
		got := sent()
		if len(got) != 1 || got[0]["sessionId"] != session.ID || got[0]["path"] != "board.html" || got[0]["keep"] != "theirs" {
			t.Fatalf("sent %+v", got)
		}
		page, ok := a.folderHistory(row.FolderID, "")
		if !ok || len(page.Entries) == 0 || page.Entries[0].Kind != synchistory.Settled || page.Entries[0].Path != "board.html" || page.Entries[0].Detail != "theirs" {
			t.Fatalf("history after a settled conflict = %+v, want a settled entry for board.html kept theirs", page.Entries)
		}
	})

	t.Run("a deleted-there conflict offers two choices", func(t *testing.T) {
		before := len(sent())
		if r := resolve("r2", deleted, "gone.html", "both"); code(r) != appcore.RefuseBadArguments || r.Field != "keep" {
			t.Fatalf("keep both = %+v, want bad-arguments on keep", r)
		}
		if len(sent()) != before {
			t.Fatal("a refused choice reached the service")
		}
		if r := resolve("r3", deleted, "gone.html", "mine"); r != nil {
			t.Fatalf("keep mine refused: %+v", r)
		}
	})

	t.Run("a conflict no longer held, or another file, changes nothing", func(t *testing.T) {
		before := len(sent())
		if r := resolve("r4", session.ID+":999:1", "board.html", "mine"); code(r) != appcore.RefuseConflictGone {
			t.Fatalf("unknown conflict = %+v", r)
		}
		if r := resolve("r5", both, "other.html", "mine"); code(r) != appcore.RefuseConflictChanged {
			t.Fatalf("other path = %+v", r)
		}
		if len(sent()) != before {
			t.Fatal("a refused choice reached the service")
		}
	})

	t.Run("the service's answers become refusals", func(t *testing.T) {
		for i, c := range []struct{ answer, want string }{
			{`{"ok":false,"error":"no-conflict"}`, appcore.RefuseConflictGone},
			{`{"ok":false,"error":"changed"}`, appcore.RefuseConflictChanged},
			{`{"ok":false,"error":"stale-etag"}`, appcore.RefuseConflictChanged},
			{`{"ok":false,"error":"copy-taken"}`, appcore.RefuseFailed},
		} {
			answerWith(c.answer)
			if r := resolve(fmt.Sprintf("r6-%d", i), both, "board.html", "mine"); code(r) != c.want {
				t.Errorf("%s = %+v, want %s", c.answer, r, c.want)
			}
		}
	})

	t.Run("paused sends nothing", func(t *testing.T) {
		answerWith(`{"ok":true}`)
		if _, r := a.do(context.Background(), coreAction(t, "p1", "sync.setPaused", map[string]bool{"paused": true}), ChannelTray); r != nil {
			t.Fatalf("pause refused: %+v", r)
		}
		before := len(sent())
		if r := resolve("r7", both, "board.html", "mine"); code(r) != appcore.RefuseSyncPaused {
			t.Fatalf("paused = %+v", r)
		}
		if len(sent()) != before {
			t.Fatal("a paused choice reached the service")
		}
	})

	t.Run("an unreadable key keeps the conflict, says why, and reads paused", func(t *testing.T) {
		answerWith(`{"ok":true}`)
		if _, r := a.do(context.Background(), coreAction(t, "p3", "sync.setPaused", map[string]bool{"paused": false}), ChannelTray); r != nil {
			t.Fatalf("resume refused: %+v", r)
		}
		a.rt.syncKeyUnread.Store(true)
		defer a.rt.syncKeyUnread.Store(false)
		before := len(sent())
		if r := resolve("r9", both, "board.html", "mine"); code(r) != appcore.RefuseSyncKeyStopped {
			t.Fatalf("key unreadable = %+v, want %s", r, appcore.RefuseSyncKeyStopped)
		}
		if len(sent()) != before {
			t.Fatal("a choice with an unreadable key reached the service")
		}
		for _, f := range a.snapshot().Folders {
			if f.ID == row.FolderID && f.Sync.State != appcore.FolderSyncPaused {
				t.Fatalf("folder reads %q with an unreadable key, want paused", f.Sync.State)
			}
		}
	})

	t.Run("a folder switched off, or a disconnect, keeps the conflict and sends nothing", func(t *testing.T) {
		answerWith(`{"ok":true}`)
		if _, r := a.do(context.Background(), coreAction(t, "p2", "sync.setPaused", map[string]bool{"paused": false}), ChannelTray); r != nil {
			t.Fatalf("resume refused: %+v", r)
		}
		if r := setSyncedThroughDo(t, a, "off1", row.FolderID, false); r != nil {
			t.Fatalf("switch off refused: %+v", r)
		}
		before := len(sent())
		if r := resolve("r8", both, "board.html", "mine"); code(r) != appcore.RefuseSyncPaused {
			t.Fatalf("folder off = %+v, want %s", r, appcore.RefuseSyncPaused)
		}
		if len(sent()) != before {
			t.Fatal("a choice for a folder switched off reached the service")
		}
		if len(a.dashboardConflicts()) == 0 {
			t.Fatal("a folder switched off dropped its held conflicts from the dashboard")
		}
		if _, r := a.do(context.Background(), coreAction(t, "d1", "sync.disconnect", map[string]any{}), ChannelTray); r != nil {
			t.Fatalf("disconnect refused: %+v", r)
		}
		if got := a.dashboardConflicts(); len(got) != 0 {
			t.Fatalf("dashboard conflicts while disconnected = %+v, want none", got)
		}
	})
}

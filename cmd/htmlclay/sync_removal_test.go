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
	"github.com/panphora/htmlclay/internal/served"
)

// TestM4RemovalHeld is 08-sync.md step 8.34: a removal of many attachments the
// engine holds is a Needs you item for its folder, and the person's choice,
// restore or delete, goes to the service as chosen.
func TestM4RemovalHeld(t *testing.T) {
	scratch := t.TempDir()
	removalLog := filepath.Join(scratch, "removal")
	answer := filepath.Join(scratch, "answer")
	t.Setenv("HTMLCLAY_STUB_BIND_LOG", filepath.Join(scratch, "bind"))
	t.Setenv("HTMLCLAY_STUB_REMOVAL_LOG", removalLog)
	t.Setenv("HTMLCLAY_STUB_REMOVAL", answer)
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
	report := func(held *served.SyncRemovalHeld) {
		seq++
		a.takeSyncReport(served.SyncReport{Seq: seq, Sessions: []served.SyncSessionReport{{
			SessionID: session.ID, RootID: row.FolderID, Status: "idle", Running: true,
			Paused: json.RawMessage("null"), RemovalHeld: held,
		}}})
	}
	items := func() []appcore.WaitingItem {
		var held []appcore.WaitingItem
		for _, item := range a.snapshot().Waiting {
			if item.Kind == appcore.WaitingRemovalHeld {
				held = append(held, item)
			}
		}
		return held
	}
	answerWith := func(body string) {
		t.Helper()
		if err := os.WriteFile(answer, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sent := func() []map[string]string {
		t.Helper()
		data, err := os.ReadFile(removalLog)
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
				t.Fatalf("removal line %q: %v", line, err)
			}
			lines = append(lines, body)
		}
		return lines
	}
	n := 0
	choose := func(folder, choice string) *appcore.Refusal {
		t.Helper()
		n++
		_, refusal := a.do(context.Background(), coreAction(t, fmt.Sprintf("rm%d", n), "folder.resolveRemoval", map[string]string{"id": folder, "choice": choice}), ChannelTray)
		return refusal
	}
	code := func(r *appcore.Refusal) string {
		if r == nil {
			return "ok"
		}
		return r.Code
	}
	held := &served.SyncRemovalHeld{Files: 24, Paths: []string{"uploads/a.png", "uploads/b.png"}}

	t.Run("a held removal is one counted item in the folder's pane", func(t *testing.T) {
		if len(items()) != 0 {
			t.Fatalf("items before any hold: %+v", items())
		}
		report(held)
		got := items()
		if len(got) != 1 || got[0].SubjectID == nil || *got[0].SubjectID != row.FolderID || !strings.Contains(got[0].Title, "24") || got[0].Pane != appcore.PaneFolders {
			t.Fatalf("items = %+v", got)
		}
		if a.snapshot().Counts.ByPane[appcore.PaneFolders] < 1 {
			t.Fatalf("counts = %+v", a.snapshot().Counts)
		}
	})

	t.Run("restore goes to the service as restore, for that session", func(t *testing.T) {
		answerWith(`{"ok":true}`)
		if r := choose(row.FolderID, "restore"); r != nil {
			t.Fatalf("restore refused: %+v", r)
		}
		got := sent()
		if len(got) != 1 || got[0]["sessionId"] != session.ID || got[0]["choice"] != "restore" {
			t.Fatalf("sent %+v", got)
		}
		if r := choose(row.FolderID, "delete"); r != nil {
			t.Fatalf("delete refused: %+v", r)
		}
		if got := sent(); len(got) != 2 || got[1]["choice"] != "delete" {
			t.Fatalf("sent %+v", got)
		}
	})

	t.Run("an unknown choice or another folder sends nothing", func(t *testing.T) {
		before := len(sent())
		if r := choose(row.FolderID, "keep"); code(r) != appcore.RefuseBadArguments || r.Field != "choice" {
			t.Fatalf("keep = %+v", r)
		}
		if r := choose("0000000000000000", "restore"); code(r) != appcore.RefuseRemovalGone {
			t.Fatalf("another folder = %+v", r)
		}
		if len(sent()) != before {
			t.Fatal("a refused choice reached the service")
		}
	})

	t.Run("the service's answers become refusals", func(t *testing.T) {
		answerWith(`{"ok":false,"error":"no-removal"}`)
		if r := choose(row.FolderID, "restore"); code(r) != appcore.RefuseRemovalGone {
			t.Fatalf("no-removal = %+v", r)
		}
		answerWith(`{"ok":false,"error":"not-connected"}`)
		if r := choose(row.FolderID, "restore"); code(r) != appcore.RefuseFailed {
			t.Fatalf("not-connected = %+v", r)
		}
	})

	t.Run("a report with no hold clears the item, and a choice then changes nothing", func(t *testing.T) {
		report(nil)
		if len(items()) != 0 {
			t.Fatalf("items after the hold ended: %+v", items())
		}
		before := len(sent())
		if r := choose(row.FolderID, "restore"); code(r) != appcore.RefuseRemovalGone {
			t.Fatalf("after the hold = %+v", r)
		}
		if len(sent()) != before {
			t.Fatal("a choice with no hold reached the service")
		}
	})
}

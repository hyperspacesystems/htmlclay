package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
)

func setSyncedThroughDo(t *testing.T, a *app, id, folderID string, on bool) *appcore.Refusal {
	t.Helper()
	_, refusal := a.do(context.Background(), coreAction(t, id, "folder.setSynced", map[string]any{"id": folderID, "on": on}), ChannelTray)
	return refusal
}

func folderInSnapshot(t *testing.T, a *app, id string) appcore.Folder {
	t.Helper()
	for _, folder := range a.snapshot().Folders {
		if folder.ID == id {
			return folder
		}
	}
	t.Fatalf("folder %s is not in the snapshot", id)
	return appcore.Folder{}
}

func TestM4SetSynced(t *testing.T) {
	scratch := t.TempDir()
	configureLog := filepath.Join(scratch, "configure")
	bindLog := filepath.Join(scratch, "bind")
	t.Setenv("HTMLCLAY_STUB_CONFIGURE_LOG", configureLog)
	t.Setenv("HTMLCLAY_STUB_BIND_LOG", bindLog)
	a, _, _ := connectTestApp(t, true)
	if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
		t.Fatalf("connect refused: %+v", refusal)
	}
	personal := savedSync(t, a).Accounts[0]
	if refusal := setUpThroughDo(t, a, "s1", personal.FolderID, filepath.Join(a.rt.home, "Maya")); refusal != nil {
		t.Fatalf("set up refused: %+v", refusal)
	}
	waitForBinds(t, a)
	rows := savedSync(t, a).SyncSessions
	if len(rows) != 1 || !rows[0].Enabled {
		t.Fatalf("rows after set up = %+v", rows)
	}
	session := rows[0]
	if folder := folderInSnapshot(t, a, personal.FolderID); !folder.Can.StopSyncing || folder.Origin.Kind != appcore.OriginAccount {
		t.Fatalf("a synced folder = %+v", folder)
	}

	t.Run("a held conflict survives stop and start", func(t *testing.T) {
		held := filepath.Join(a.rt.configDir, "service", "sync-meta", "v2", session.ID, "conflicts.json")
		writeTestFile(t, held, `[{"rel":"index.html"}]`)

		before := len(configuresSent(t, configureLog))
		if refusal := setSyncedThroughDo(t, a, "off", personal.FolderID, false); refusal != nil {
			t.Fatalf("off refused: %+v", refusal)
		}
		rows := savedSync(t, a).SyncSessions
		if len(rows) != 1 || rows[0].ID != session.ID || rows[0].Enabled {
			t.Fatalf("rows after off = %+v", rows)
		}
		off := waitForConfigure(t, configureLog, before, func(w syncWanted) bool {
			return len(w.Sessions) == 1 && !w.Sessions[0].Enabled
		})
		if off.Sessions[0].ID != session.ID {
			t.Fatalf("configure after off = %+v", off)
		}
		if folder := folderInSnapshot(t, a, personal.FolderID); !folder.Can.Sync || folder.Can.StopSyncing {
			t.Fatalf("a stopped folder = %+v", folder)
		}

		bindsBefore := len(bindsSent(t, bindLog))
		before = len(configuresSent(t, configureLog))
		if refusal := setSyncedThroughDo(t, a, "on", personal.FolderID, true); refusal != nil {
			t.Fatalf("on refused: %+v", refusal)
		}
		rows = savedSync(t, a).SyncSessions
		if len(rows) != 1 || rows[0].ID != session.ID || !rows[0].Enabled {
			t.Fatalf("rows after on = %+v", rows)
		}
		waitForConfigure(t, configureLog, before, func(w syncWanted) bool {
			return len(w.Sessions) == 1 && w.Sessions[0].ID == session.ID && w.Sessions[0].Enabled
		})
		if got := len(bindsSent(t, bindLog)); got != bindsBefore {
			t.Fatal("starting again sent a bind")
		}
		if data, err := os.ReadFile(held); err != nil || string(data) != `[{"rel":"index.html"}]` {
			t.Fatalf("the held conflict is gone: %q %v", data, err)
		}
	})
}

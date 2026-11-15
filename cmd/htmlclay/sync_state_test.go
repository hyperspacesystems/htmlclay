package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/served"
)

// configuresSent reads the stub service's configure log, one body per line.
func configuresSent(t *testing.T, log string) []syncWanted {
	t.Helper()
	data, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var sent []syncWanted
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var body syncWanted
		if err := json.Unmarshal([]byte(line), &body); err != nil {
			t.Fatalf("configure line %q: %v", line, err)
		}
		sent = append(sent, body)
	}
	return sent
}

func waitForConfigure(t *testing.T, log string, after int, ok func(syncWanted) bool) syncWanted {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		sent := configuresSent(t, log)
		for i := after; i < len(sent); i++ {
			if ok(sent[i]) {
				return sent[i]
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no configure after %d matched: %+v", after, configuresSent(t, log))
	return syncWanted{}
}

func editSyncRecord(t *testing.T, a *app, edit func(*config.Sync)) {
	t.Helper()
	a.rt.syncMu.Lock()
	defer a.rt.syncMu.Unlock()
	record, err := a.syncRecordLocked()
	if err != nil {
		t.Fatal(err)
	}
	edit(record)
	if err := record.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestM4SyncState(t *testing.T) {
	log := filepath.Join(t.TempDir(), "configure")
	t.Setenv("HTMLCLAY_STUB_CONFIGURE_LOG", log)
	a, _, _ := connectTestApp(t, true)

	if sent := configuresSent(t, log); len(sent) != 0 {
		t.Fatalf("configure was sent with no account: %+v", sent)
	}
	if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
		t.Fatalf("connect refused: %+v", refusal)
	}
	first := waitForConfigure(t, log, 0, func(syncWanted) bool { return true })
	if len(first.Roots) != 1 || first.DeviceID == "" || first.Paused {
		t.Fatalf("the first configure = %+v", first)
	}
	root := first.Roots[0]

	t.Run("each change sends the whole wanted state", func(t *testing.T) {
		editSyncRecord(t, a, func(record *config.Sync) {
			record.SyncSessions = []config.SyncSession{{ID: "s1", RootID: root.ID, AccountID: 12, Kind: "personal", Enabled: true}}
		})
		before := len(configuresSent(t, log))
		if refusal := a.setSyncPaused(true); refusal != nil {
			t.Fatal(refusal)
		}
		paused := waitForConfigure(t, log, before, func(w syncWanted) bool { return w.Paused })
		if len(paused.Sessions) != 1 || paused.Sessions[0].ID != "s1" || paused.Roots[0] != root {
			t.Fatalf("paused configure = %+v", paused)
		}
		before = len(configuresSent(t, log))
		if refusal := a.setSyncPaused(false); refusal != nil {
			t.Fatal(refusal)
		}
		waitForConfigure(t, log, before, func(w syncWanted) bool { return !w.Paused && len(w.Sessions) == 1 })
	})

	t.Run("a report keeps the engine's fields of a row", func(t *testing.T) {
		a.takeSyncReport(served.SyncReport{Seq: 1, Sessions: []served.SyncSessionReport{{
			SessionID: "s1", RootID: root.ID,
			Record: json.RawMessage(`{"id":"s1","rootId":"` + root.ID + `","accountId":12,"kind":"personal","cached":{"username":"maya","displayName":"Maya L","role":"owner"},"paused":{"reason":"removed","since":"2026-11-15T07:00:00Z"},"legacyMetaDir":"/old/meta","enabled":true}`),
		}}})
		row := savedSync(t, a).SyncSessions[0]
		if row.Paused == nil || row.Paused.Reason != "removed" || row.Cached.DisplayName != "Maya L" || row.LegacyMetaDir == nil || *row.LegacyMetaDir != "/old/meta" {
			t.Fatalf("row = %+v", row)
		}
	})

	t.Run("the service cannot turn a folder's sync on", func(t *testing.T) {
		editSyncRecord(t, a, func(record *config.Sync) { record.SyncSessions[0].Enabled = false })
		a.takeSyncReport(served.SyncReport{Seq: 2, Sessions: []served.SyncSessionReport{{
			SessionID: "s1", RootID: root.ID,
			Record: json.RawMessage(`{"id":"s1","rootId":"ffffffffffffffff","accountId":12,"kind":"team","cached":{"username":"maya","displayName":"Maya L","role":"owner"},"paused":null,"legacyMetaDir":null,"enabled":true}`),
		}}})
		row := savedSync(t, a).SyncSessions[0]
		if row.Enabled || row.RootID != root.ID || row.Kind != "personal" {
			t.Fatalf("the report changed Go's own fields: %+v", row)
		}
		if row.Paused != nil {
			t.Fatalf("the report's pause was not kept: %+v", row.Paused)
		}
		editSyncRecord(t, a, func(record *config.Sync) { record.SyncSessions[0].Enabled = true })
	})

	t.Run("a refused key stops every session and stays stored", func(t *testing.T) {
		before := len(configuresSent(t, log))
		a.takeSyncReport(served.SyncReport{Seq: 3, KeyRefused: true})
		saved := savedSync(t, a)
		if saved.KeyRefused == nil {
			t.Fatal("keyRefused was not set")
		}
		if got := storedKey(t, a); got != connectKeyOne {
			t.Fatalf("the key was not kept: %q", got)
		}
		waitForConfigure(t, log, before, func(w syncWanted) bool { return len(w.Sessions) == 0 && len(w.Roots) == 1 })
		if refusal := connectThroughDo(t, a, "c2", "", connectKeyTwo); refusal != nil {
			t.Fatalf("reconnect refused: %+v", refusal)
		}
		if savedSync(t, a).KeyRefused != nil {
			t.Fatal("a new key did not clear keyRefused")
		}
		after := len(configuresSent(t, log))
		a.sendSyncWanted()
		waitForConfigure(t, log, after, func(w syncWanted) bool { return len(w.Sessions) == 1 })
	})
}

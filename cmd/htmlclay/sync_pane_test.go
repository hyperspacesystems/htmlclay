package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/secret"
)

// TestM4SyncPaneFacts covers what 08-sync.md 8.43's Sync pane reads from the
// host: where the key is kept, and the sync log folder Finder shows.
func TestM4SyncPaneFacts(t *testing.T) {
	a, _, _ := connectTestApp(t, false)
	if got := a.snapshot().Sync.KeyStore; got != "" {
		t.Fatalf("not connected: keyStore = %q, want empty", got)
	}
	if refusal := connectThroughDo(t, a, "c1", "", connectKeyOne); refusal != nil {
		t.Fatalf("connect refused: %+v", refusal)
	}
	if got := a.snapshot().Sync.KeyStore; got != secret.KindMemory {
		t.Fatalf("keyStore = %q, want %q (the store the key went to)", got, secret.KindMemory)
	}

	var shown string
	a.rt.reveal = func(path string) error {
		shown = path
		return nil
	}
	if refusal := m3ProgramDo(t, a, ChannelShell, "sync.revealLog", map[string]any{}); refusal != nil {
		t.Fatalf("sync.revealLog = %+v", refusal)
	}
	want := filepath.Join(a.rt.configDir, "service", "logs", "sync")
	if shown != want {
		t.Fatalf("revealed %q, want %q", shown, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("the sync log folder was not made: %v", err)
	}
}

// The Mac app shows the sync log row only when the host says it can.
func TestM4SyncRevealLogIsOffered(t *testing.T) {
	a, _, _ := connectTestApp(t, false)
	if !a.snapshot().Host.Can["sync.revealLog"] {
		t.Fatal("the snapshot does not offer sync.revealLog")
	}
}

// The Troubleshooting row shows htmlclay.log itself.
func TestRevealLogShowsTheAppLog(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	writeTestFile(t, a.appLogPath(), "a log line\n")
	var shown string
	a.rt.reveal = func(path string) error {
		shown = path
		return nil
	}
	if refusal := a.runEffect(context.Background(), appcore.Effect{Type: "app.revealLog"}); refusal != nil {
		t.Fatalf("app.revealLog = %+v", refusal)
	}
	if shown != a.appLogPath() {
		t.Fatalf("revealed %q, want %q", shown, a.appLogPath())
	}
}

// "Sync has been on" is read from sync.json: a session row counts, an empty
// record does not.
func TestSyncEverOn(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	writeTestFile(t, filepath.Join(a.rt.configDir, "sync.json"), `{"v":1,"deviceId":"d","syncSessions":[{"id":"s-1","rootId":"r-1","accountId":40,"kind":"team"}]}`)
	if facts, _, _, _ := a.syncFacts(); !facts.EverOn {
		t.Fatal("a sync.json with one session row did not read as sync having been on")
	}
	b := newTestApp(t, t.TempDir())
	writeTestFile(t, filepath.Join(b.rt.configDir, "sync.json"), `{"v":1,"deviceId":"d"}`)
	if facts, _, _, _ := b.syncFacts(); facts.EverOn {
		t.Fatal("an empty sync.json read as sync having been on")
	}
}

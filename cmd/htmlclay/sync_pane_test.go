package main

import (
	"os"
	"path/filepath"
	"testing"

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

package appcore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestM4OwnMarker(t *testing.T) {
	writeMarker := func(t *testing.T, dir, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, ".hyperclay"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".hyperclay", "sync-root.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	check := func(t *testing.T, home, folder string, own []string) *Refusal {
		t.Helper()
		return CheckServe(GatherServeFacts(ServeInputs{
			Path: folder, Name: "notes", Home: home, OwnSyncRoots: own,
			NameAvailable: func(string) error { return nil },
		}))
	}
	newHome := func(t *testing.T) (string, string) {
		t.Helper()
		home, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		folder := filepath.Join(home, "notes")
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		return home, folder
	}
	const ours = "0123456789abcdef"

	t.Run("our synced folder still serves", func(t *testing.T) {
		home, folder := newHome(t)
		writeMarker(t, folder, `{"rootId":"`+ours+`","createdAt":"2026-11-15T00:00:00Z"}`)
		if refusal := check(t, home, folder, []string{ours}); refusal != nil {
			t.Fatalf("refused: %+v", refusal)
		}
	})

	t.Run("Local's folder is still refused", func(t *testing.T) {
		home, folder := newHome(t)
		writeMarker(t, folder, `{"rootId":"fedcba9876543210"}`)
		if refusal := check(t, home, folder, []string{ours}); refusal == nil || refusal.Code != RefuseBoundToLocalSync {
			t.Fatalf("refusal = %+v, want %s", refusal, RefuseBoundToLocalSync)
		}
		if refusal := check(t, home, folder, nil); refusal == nil || refusal.Code != RefuseBoundToLocalSync {
			t.Fatalf("with no synced folders: refusal = %+v", refusal)
		}
	})

	t.Run("a Local folder three levels inside our own is refused", func(t *testing.T) {
		home, folder := newHome(t)
		writeMarker(t, folder, `{"rootId":"`+ours+`"}`)
		writeMarker(t, filepath.Join(folder, "a", "b", "c"), `{"rootId":"fedcba9876543210"}`)
		if refusal := check(t, home, folder, []string{ours}); refusal == nil || refusal.Code != RefuseBoundToLocalSync {
			t.Fatalf("refusal = %+v", refusal)
		}
	})

	t.Run("a marker whose rootId cannot be read is Local's", func(t *testing.T) {
		home, folder := newHome(t)
		writeMarker(t, folder, `{}`)
		if refusal := check(t, home, folder, []string{ours, ""}); refusal == nil || refusal.Code != RefuseBoundToLocalSync {
			t.Fatalf("refusal = %+v", refusal)
		}
	})
}

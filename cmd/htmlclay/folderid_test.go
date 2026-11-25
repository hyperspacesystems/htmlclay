package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/testutil"
)

// trustedFolderIDFor answers the id the config keeps for the trusted folder at path.
func trustedFolderIDFor(t *testing.T, a *app, path string) string {
	t.Helper()
	for _, tf := range a.rt.cfg.TrustedFolderList() {
		if tf.Path == path {
			return tf.ID
		}
	}
	t.Fatalf("no trusted folder at %q in %+v", path, a.rt.cfg.TrustedFolderList())
	return ""
}

func TestM3FolderIDStaysWhenServed(t *testing.T) {
	t.Run("trust, read the id from the snapshot, serve, same id", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "proj")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

		a := newServedTestApp(t, home, t.TempDir())
		if err := a.trustFolder(dir); err != nil {
			t.Fatalf("trustFolder(%s): %v", dir, err)
		}
		trusted := trustedFolderIDFor(t, a, dir)
		if len(trusted) != 16 {
			t.Fatalf("trusted folder id = %q, want 16 hex characters", trusted)
		}
		folder := snapshotFolderAt(t, a.snapshot(), dir)
		if folder.ID != trusted {
			t.Fatalf("snapshot folder id = %q, want the id the config keeps %q", folder.ID, trusted)
		}
		if !folder.Trusted {
			t.Errorf("snapshot folder %s is not trusted", dir)
		}

		entry, refusal, err := a.serveFolder(dir, "proj")
		if err != nil || refusal != nil {
			t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
		}
		if entry.ID != trusted {
			t.Fatalf("served folder id = %q, want the folder's id %q from before it was served", entry.ID, trusted)
		}
		folder = snapshotFolderAt(t, a.snapshot(), dir)
		if folder.ID != trusted {
			t.Fatalf("snapshot folder id = %q after serving, want %q", folder.ID, trusted)
		}
		if !folder.Served {
			t.Error("the folder is not served in the snapshot")
		}
		if got := trustedFolderIDFor(t, a, dir); got != trusted {
			t.Fatalf("trusted folder id = %q after serving, want %q", got, trusted)
		}
	})

	t.Run("stop serving keeps the id", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "proj")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

		a := newServedTestApp(t, home, t.TempDir())
		if err := a.trustFolder(dir); err != nil {
			t.Fatalf("trustFolder(%s): %v", dir, err)
		}
		entry, refusal, err := a.serveFolder(dir, "proj")
		if err != nil || refusal != nil {
			t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
		}
		if err := a.stopServing(entry.ID); err != nil {
			t.Fatalf("stopServing(%q): %v", entry.ID, err)
		}
		folder := snapshotFolderAt(t, a.snapshot(), dir)
		if folder.ID != entry.ID {
			t.Fatalf("snapshot folder id = %q after stopping, want %q", folder.ID, entry.ID)
		}
		if folder.Served {
			t.Error("the folder is still served in the snapshot")
		}
		if got := trustedFolderIDFor(t, a, dir); got != entry.ID {
			t.Fatalf("trusted folder id = %q after stopping, want %q", got, entry.ID)
		}
	})

	t.Run("a config from the current build with a served folder keeps that folder's id after load", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "proj")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

		cfgBase := t.TempDir()
		seeded := fmt.Sprintf(`{"workspaceFolders":[{"path":%q,"identity":%q}],"servedFolders":[{"id":"fedcba9876543210","name":"proj","path":%q,"identity":%q,"serving":true}]}`, dir, platform.DirIdentity(dir), dir, platform.DirIdentity(dir))
		if err := os.MkdirAll(config.DirFrom(cfgBase), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(config.DirFrom(cfgBase), "config.json"), []byte(seeded), 0600); err != nil {
			t.Fatal(err)
		}

		a := newServedTestApp(t, home, cfgBase)
		folder := snapshotFolderAt(t, a.snapshot(), dir)
		if folder.ID != "fedcba9876543210" {
			t.Fatalf("snapshot folder id = %q, want the served folder's own id fedcba9876543210", folder.ID)
		}
		if !folder.Served {
			t.Error("the served folder is not served in the snapshot")
		}
	})

	t.Run("a restart changes no id", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "proj")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

		cfgBase := t.TempDir()
		first := newServedTestApp(t, home, cfgBase)
		if err := first.trustFolder(dir); err != nil {
			t.Fatalf("trustFolder(%s): %v", dir, err)
		}
		entry, refusal, err := first.serveFolder(dir, "proj")
		if err != nil || refusal != nil {
			t.Fatalf("serveFolder: refusal=%+v err=%v", refusal, err)
		}
		before := snapshotFolderAt(t, first.snapshot(), dir).ID
		if before != entry.ID {
			t.Fatalf("snapshot folder id = %q, want the served folder's id %q", before, entry.ID)
		}

		second := newServedTestApp(t, home, cfgBase)
		if got := snapshotFolderAt(t, second.snapshot(), dir).ID; got != before {
			t.Fatalf("after the restart the folder has id %q, want %q", got, before)
		}
	})

	t.Run("a failed save at load leaves the old file as it was and the app still starts", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "proj")
		writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

		cfgBase := t.TempDir()
		seeded := fmt.Sprintf(`{"workspaceFolders":[{"path":%q,"identity":%q}],"servedFolders":[{"id":"0011223344556677","name":"proj","path":%q,"identity":%q,"serving":true}]}`, dir, platform.DirIdentity(dir), dir, platform.DirIdentity(dir))
		cfgDir := config.DirFrom(cfgBase)
		if err := os.MkdirAll(cfgDir, 0755); err != nil {
			t.Fatal(err)
		}
		configPath := filepath.Join(cfgDir, "config.json")
		if err := os.WriteFile(configPath, []byte(seeded), 0600); err != nil {
			t.Fatal(err)
		}

		a := newServedTestApp(t, home, cfgBase)
		cfg, res, err := config.LoadFrom(cfgBase, platform.DirIdentity)
		if err != nil {
			t.Fatalf("LoadFrom: %v", err)
		}
		a.rt.cfg = cfg
		a.loaded = res
		if !res.AssignedFolderIDs {
			t.Error("Load did not report copying the served folder's id onto its trusted entry")
		}

		testutil.RefuseReplace(t, configPath)

		a.finishUpgrade()

		onDisk, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("reading the config back: %v", err)
		}
		if string(onDisk) != seeded {
			t.Fatalf("a failed save rewrote the config: %s", onDisk)
		}
		folder := snapshotFolderAt(t, a.snapshot(), dir)
		if folder.ID != "0011223344556677" {
			t.Fatalf("snapshot folder id = %q after the failed save, want 0011223344556677", folder.ID)
		}
	})
}

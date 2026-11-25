package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// trustedFolderID answers the id the trusted entry at path holds.
func trustedFolderID(t *testing.T, cfg *Config, path string) string {
	t.Helper()
	for _, w := range cfg.TrustedFolderList() {
		if w.Path == path {
			return w.ID
		}
	}
	t.Fatalf("no trusted folder at %q in %+v", path, cfg.TrustedFolderList())
	return ""
}

func TestM3FolderID(t *testing.T) {
	t.Run("trust gives the folder one id", func(t *testing.T) {
		cfg, _, err := LoadFrom(t.TempDir(), noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.AddTrustedFolder("/folders/one", "id:one") {
			t.Fatal("the first trust of a folder added nothing")
		}
		id := trustedFolderID(t, cfg, "/folders/one")
		if !servedIDPattern.MatchString(id) {
			t.Fatalf("trusted folder id = %q, want 16 lowercase hex", id)
		}
		if cfg.AddTrustedFolder("/folders/one", "id:one") {
			t.Fatal("trusting a folder twice added a second entry")
		}
		if got := trustedFolderID(t, cfg, "/folders/one"); got != id {
			t.Fatalf("trusting the same folder again changed its id from %q to %q", id, got)
		}
	})

	t.Run("serving keeps the trusted id", func(t *testing.T) {
		cfg, _, err := LoadFrom(t.TempDir(), noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.AddTrustedFolder("/folders/notes", "id:notes") {
			t.Fatal("trust added nothing")
		}
		trusted := trustedFolderID(t, cfg, "/folders/notes")
		entry, err := cfg.AddServedFolder("/folders/notes", "id:notes", "notes")
		if err != nil {
			t.Fatalf("AddServedFolder: %v", err)
		}
		if entry.ID != trusted {
			t.Fatalf("served folder id = %q, want the trusted folder's id %q", entry.ID, trusted)
		}
		if got := trustedFolderID(t, cfg, "/folders/notes"); got != trusted {
			t.Fatalf("serving changed the trusted folder's id from %q to %q", trusted, got)
		}
	})

	t.Run("serving an untrusted folder gives both the same id", func(t *testing.T) {
		cfg, _, err := LoadFrom(t.TempDir(), noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		entry, err := cfg.AddServedFolder("/folders/fresh", "id:fresh", "fresh")
		if err != nil {
			t.Fatalf("AddServedFolder: %v", err)
		}
		if !servedIDPattern.MatchString(entry.ID) {
			t.Fatalf("served folder id = %q, want 16 lowercase hex", entry.ID)
		}
		if got := trustedFolderID(t, cfg, "/folders/fresh"); got != entry.ID {
			t.Fatalf("trusted folder id = %q, want the served folder's id %q", got, entry.ID)
		}
	})

	t.Run("a config from before ids gains them on load", func(t *testing.T) {
		baseDir := t.TempDir()
		writeConfigJSON(t, baseDir, `{"workspaceFolders":[{"path":"/folders/old","identity":"id:old"}]}`)
		cfg, res, err := LoadFrom(baseDir, noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		if !res.AssignedFolderIDs {
			t.Error("Load did not report giving the folder an id")
		}
		id := trustedFolderID(t, cfg, "/folders/old")
		if !servedIDPattern.MatchString(id) {
			t.Fatalf("trusted folder id = %q, want 16 lowercase hex", id)
		}
	})

	t.Run("a config with a served folder keeps that folder's id after load", func(t *testing.T) {
		baseDir := t.TempDir()
		writeConfigJSON(t, baseDir, `{"workspaceFolders":[{"path":"/folders/notes","identity":"id:notes"}],"servedFolders":[{"id":"0123456789abcdef","name":"notes","path":"/folders/notes","identity":"id:notes","serving":true}]}`)
		cfg, res, err := LoadFrom(baseDir, noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		if got := trustedFolderID(t, cfg, "/folders/notes"); got != "0123456789abcdef" {
			t.Fatalf("trusted folder id = %q, want the served folder's own id 0123456789abcdef", got)
		}
		if !res.AssignedFolderIDs {
			t.Error("Load did not report copying the served folder's id onto its trusted entry")
		}
	})

	t.Run("a restart changes no id", func(t *testing.T) {
		baseDir := t.TempDir()
		cfg, _, err := LoadFrom(baseDir, noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.AddTrustedFolder("/folders/a", "id:a") {
			t.Fatal("trust added nothing")
		}
		entry, err := cfg.AddServedFolder("/folders/b", "id:b", "b")
		if err != nil {
			t.Fatalf("AddServedFolder: %v", err)
		}
		before := map[string]string{
			"/folders/a": trustedFolderID(t, cfg, "/folders/a"),
			"/folders/b": trustedFolderID(t, cfg, "/folders/b"),
		}
		if before["/folders/b"] != entry.ID {
			t.Fatalf("served folder id = %q, trusted id = %q, want them equal", entry.ID, before["/folders/b"])
		}
		if err := cfg.Save(); err != nil {
			t.Fatalf("Save: %v", err)
		}

		reloaded, res, err := LoadFrom(baseDir, noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		if res.AssignedFolderIDs {
			t.Error("a settled config was given new ids on load")
		}
		for path, want := range before {
			if got := trustedFolderID(t, reloaded, path); got != want {
				t.Errorf("after the restart %s has id %q, want %q", path, got, want)
			}
		}
	})

	t.Run("a replaced folder gets a new id and none of the old grants", func(t *testing.T) {
		const path = "/folders/replaced"
		cfg, _, err := LoadFrom(t.TempDir(), noIdentity)
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.AddTrustedFolder(path, "id:old") {
			t.Fatal("trust added nothing")
		}
		oldID := trustedFolderID(t, cfg, path)
		removed, ok := cfg.RemoveTrustedFolder(path)
		if !ok {
			t.Fatal("the trusted folder was not there to remove")
		}
		if removed.ID != oldID || removed.Identity != "id:old" {
			t.Fatalf("removed entry = %+v, want id %q and identity id:old", removed, oldID)
		}
		if !cfg.AddTrustedFolder(path, "id:new") {
			t.Fatal("trusting the folder now at that path added nothing")
		}
		newID := trustedFolderID(t, cfg, path)
		if newID == oldID {
			t.Fatalf("the folder trusted at the same path kept the old folder's id %q", oldID)
		}
		if !servedIDPattern.MatchString(newID) {
			t.Fatalf("replaced folder id = %q, want 16 lowercase hex", newID)
		}
	})
}

// TestM3FolderIDsAreSavedOnce pins that Load itself writes nothing: the ids it
// hands out only reach disk when the caller saves the reported change.
func TestM3FolderIDsAreSavedOnce(t *testing.T) {
	baseDir := t.TempDir()
	body := `{"workspaceFolders":[{"path":"/folders/old","identity":"id:old"}]}`
	writeConfigJSON(t, baseDir, body)
	if _, _, err := LoadFrom(baseDir, noIdentity); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(DirFrom(baseDir), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Fatalf("Load rewrote the config: %s", data)
	}

	cfg, _, err := LoadFrom(baseDir, noIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	var onDisk struct {
		TrustedFolders []TrustedFolder `json:"workspaceFolders"`
	}
	data, err = os.ReadFile(filepath.Join(DirFrom(baseDir), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatal(err)
	}
	if len(onDisk.TrustedFolders) != 1 || onDisk.TrustedFolders[0].ID != trustedFolderID(t, cfg, "/folders/old") {
		t.Fatalf("the saved config = %s, want the id Load handed out", data)
	}
}

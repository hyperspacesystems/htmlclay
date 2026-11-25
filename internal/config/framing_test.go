package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestM3ServedAllowFraming is step 7.14's config half: the choice is kept per
// folder, it survives a save and a load, and it is left out of the file while it
// is off.
func TestM3ServedAllowFraming(t *testing.T) {
	baseDir := t.TempDir()
	cfg, _, err := LoadFrom(baseDir, noIdentity)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := cfg.AddServedFolder("/served/framed", "id:framed", "framed")
	if err != nil {
		t.Fatal(err)
	}
	if entry.AllowFraming {
		t.Fatalf("a new served folder allows frames: %+v", entry)
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(DirFrom(baseDir), "config.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(raw), "allowFraming") {
		t.Fatalf("the saved config holds allowFraming while it is off: %s", raw)
	}

	if !cfg.SetServedAllowFraming(entry.ID, true) {
		t.Fatal("SetServedAllowFraming reported no such id")
	}
	if cfg.SetServedAllowFraming("0123456789abcdef", true) {
		t.Error("SetServedAllowFraming on an unknown id reported success")
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	raw, err = os.ReadFile(filepath.Join(DirFrom(baseDir), "config.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var stored struct {
		ServedFolders []struct {
			ID           string `json:"id"`
			AllowFraming bool   `json:"allowFraming"`
		} `json:"servedFolders"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("the saved config: %v", err)
	}
	if len(stored.ServedFolders) != 1 || !stored.ServedFolders[0].AllowFraming {
		t.Fatalf("the saved config holds %+v, want the folder allowing frames", stored.ServedFolders)
	}

	loaded, _, err := LoadFrom(baseDir, noIdentity)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got, ok := loaded.ServedFolderByID(entry.ID)
	if !ok || !got.AllowFraming {
		t.Fatalf("the served folder after a load = %+v (found %v), want it allowing frames", got, ok)
	}
	if !loaded.SetServedAllowFraming(entry.ID, false) {
		t.Fatal("SetServedAllowFraming(false) reported no such id")
	}
	if got, _ := loaded.ServedFolderByID(entry.ID); got.AllowFraming {
		t.Fatalf("the served folder after refusing frames = %+v", got)
	}
}

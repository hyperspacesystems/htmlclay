package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestServedNameShape(t *testing.T) {
	long := strings.Repeat("a", 40)
	accepted := []string{"a", "notes", "my-notes", "a1", long}
	for _, name := range accepted {
		if err := ValidServedName(name); err != nil {
			t.Errorf("ValidServedName(%q) = %v, want nil", name, err)
		}
	}
	refused := []string{"", strings.Repeat("a", 41), "-a", "a-", "A", "my_notes", "my.notes", "é"}
	for _, name := range refused {
		if err := ValidServedName(name); !errors.Is(err, ErrServedNameInvalid) {
			t.Errorf("ValidServedName(%q) = %v, want ErrServedNameInvalid", name, err)
		}
	}
	for name := range reservedServedNames {
		if err := ValidServedName(name); !errors.Is(err, ErrServedNameInvalid) {
			t.Errorf("ValidServedName(%q) = %v, want ErrServedNameInvalid", name, err)
		}
	}
}

func TestServedAddAndLookup(t *testing.T) {
	baseDir := t.TempDir()
	cfg, _, err := LoadFrom(baseDir, noIdentity)
	if err != nil {
		t.Fatal(err)
	}

	first, err := cfg.AddServedFolder("/served/first", "id:first", "first")
	if err != nil {
		t.Fatalf("add first: %v", err)
	}
	second, err := cfg.AddServedFolder("/served/second", "id:second", "second")
	if err != nil {
		t.Fatalf("add second: %v", err)
	}

	for _, f := range []ServedFolder{first, second} {
		if !servedIDPattern.MatchString(f.ID) {
			t.Errorf("id %q is not 16 lowercase hex", f.ID)
		}
		if !f.Serving {
			t.Errorf("%s is not serving right after it was added", f.Name)
		}
	}
	if first.ID == second.ID {
		t.Errorf("both folders got the id %q", first.ID)
	}

	for _, want := range []ServedFolder{first, second} {
		if got, ok := cfg.ServedFolderByID(want.ID); !ok || got != want {
			t.Errorf("ServedFolderByID(%q) = %+v, %v, want %+v", want.ID, got, ok, want)
		}
		if got, ok := cfg.ServedFolderByName(want.Name); !ok || got != want {
			t.Errorf("ServedFolderByName(%q) = %+v, %v, want %+v", want.Name, got, ok, want)
		}
		if got, ok := cfg.ServedFolderForPath(want.Path); !ok || got != want {
			t.Errorf("ServedFolderForPath(%q) = %+v, %v, want %+v", want.Path, got, ok, want)
		}
	}

	for _, want := range []ServedFolder{first, second} {
		found := false
		for _, w := range cfg.TrustedFolderList() {
			if w.Path == want.Path {
				found = true
				if w.Identity != want.Identity {
					t.Errorf("%s trusted identity = %q, want %q", want.Path, w.Identity, want.Identity)
				}
			}
		}
		if !found {
			t.Errorf("%s is served but not trusted", want.Path)
		}
	}

	if _, err := cfg.AddServedFolder("/served/third", "", "first"); !errors.Is(err, ErrServedNameTaken) {
		t.Errorf("taken name error = %v, want ErrServedNameTaken", err)
	}
	if _, err := cfg.AddServedFolder("/served/second", "", "third"); !errors.Is(err, ErrAlreadyServed) {
		t.Errorf("served path error = %v, want ErrAlreadyServed", err)
	}

	if !cfg.AddTrustedFolder("/served/already", "id:already") {
		t.Fatal("the trusted folder was already present")
	}
	if _, err := cfg.AddServedFolder("/served/already", "id:already", "already"); err != nil {
		t.Fatalf("add over a trusted folder: %v", err)
	}
	n := 0
	for _, w := range cfg.TrustedFolderList() {
		if w.Path == "/served/already" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d trusted entries for a path that was trusted before it was served, want 1", n)
	}
}

func TestServedRemoveRetiresTheName(t *testing.T) {
	baseDir := t.TempDir()
	cfg, _, err := LoadFrom(baseDir, noIdentity)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := cfg.AddServedFolder("/served/gone", "id:gone", "gone")
	if err != nil {
		t.Fatal(err)
	}

	removed, ok := cfg.RemoveServedFolder(entry.ID)
	if !ok || removed != entry {
		t.Fatalf("RemoveServedFolder = %+v, %v, want %+v, true", removed, ok, entry)
	}
	if _, ok := cfg.ServedFolderByID(entry.ID); ok {
		t.Error("the removed folder is still found by id")
	}
	retired := false
	for _, name := range cfg.RetiredServedNames {
		if name == entry.Name {
			retired = true
		}
	}
	if !retired {
		t.Errorf("retired names = %v, want %q among them", cfg.RetiredServedNames, entry.Name)
	}
	if _, err := cfg.AddServedFolder("/served/again", "", entry.Name); !errors.Is(err, ErrServedNameRetired) {
		t.Errorf("reuse of a retired name = %v, want ErrServedNameRetired", err)
	}
	stillTrusted := false
	for _, w := range cfg.TrustedFolderList() {
		if w.Path == entry.Path {
			stillTrusted = true
		}
	}
	if !stillTrusted {
		t.Error("removing a served folder dropped its trusted folder")
	}
	if _, ok := cfg.RemoveServedFolder(entry.ID); ok {
		t.Error("removing the same folder twice reported a removal")
	}
}

func TestServedSetServing(t *testing.T) {
	baseDir := t.TempDir()
	cfg, _, err := LoadFrom(baseDir, noIdentity)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := cfg.AddServedFolder("/served/toggle", "id:toggle", "toggle")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SetServing(entry.ID, false) {
		t.Fatal("SetServing(false) reported no such id")
	}
	if got, _ := cfg.ServedFolderByID(entry.ID); got.Serving {
		t.Error("the folder is still serving after SetServing(false)")
	}
	if !cfg.SetServing(entry.ID, true) {
		t.Fatal("SetServing(true) reported no such id")
	}
	if got, _ := cfg.ServedFolderByID(entry.ID); !got.Serving {
		t.Error("the folder is not serving after SetServing(true)")
	}
	if cfg.SetServing("0123456789abcdef", false) {
		t.Error("SetServing on an unknown id reported success")
	}
}

func TestServedRoundTrip(t *testing.T) {
	baseDir := t.TempDir()
	cfg, _, err := LoadFrom(baseDir, noIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.AddServedFolder("/served/kept", "id:kept", "kept"); err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := cfg.AddServedFolder("/served/stopped", "id:stopped", "stopped")
	if err != nil {
		t.Fatal(err)
	}
	gone, err := cfg.AddServedFolder("/served/gone", "id:gone", "gone")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SetServing(stopped.ID, false) {
		t.Fatal("SetServing reported no such id")
	}
	if _, ok := cfg.RemoveServedFolder(gone.ID); !ok {
		t.Fatal("RemoveServedFolder reported no such id")
	}
	if kept, ok := cfg.ServedFolderByName("kept"); !ok || !kept.Serving {
		t.Fatalf("the kept folder before save = %+v, %v, want it serving", kept, ok)
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, _, err := LoadFrom(baseDir, noIdentity)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got, want := loaded.ServedFolderList(), cfg.ServedFolderList(); !reflect.DeepEqual(got, want) {
		t.Errorf("served folders after reload = %+v, want %+v", got, want)
	}
	if got, want := loaded.RetiredServedNames, cfg.RetiredServedNames; !reflect.DeepEqual(got, want) {
		t.Errorf("retired served names after reload = %v, want %v", got, want)
	}
	if got := loaded.ServedFolderList(); len(got) != 2 {
		t.Errorf("%d served folders after reload, want 2", len(got))
	}
}

func TestServedLoadSanitizes(t *testing.T) {
	baseDir := t.TempDir()
	writeConfigJSON(t, baseDir, `{
  "servedFolders": [
    {"id": "0123456789abcdef", "name": "notes", "path": "/served/a", "serving": true},
    {"id": "XYZ", "name": "badid", "path": "/served/badid", "serving": true},
    {"id": "1111111111111111", "name": "Bad_Name", "path": "/served/badname", "serving": true},
    {"id": "0123456789abcdef", "name": "other", "path": "/served/other", "serving": true},
    {"id": "2222222222222222", "name": "notes", "path": "/served/dupname", "serving": true},
    {"id": "3333333333333333", "name": "duppath", "path": "/served/a", "serving": true},
    {"id": "fedcba9876543210", "name": "second", "path": "/served/b", "serving": false}
  ],
  "retiredServedNames": ["old", "old", "Bad"]
}`)

	cfg, _, err := LoadFrom(baseDir, noIdentity)
	if err != nil {
		t.Fatal(err)
	}
	want := []ServedFolder{
		{ID: "0123456789abcdef", Name: "notes", Path: "/served/a", Serving: true},
		{ID: "fedcba9876543210", Name: "second", Path: "/served/b", Serving: false},
	}
	if got := cfg.ServedFolderList(); !reflect.DeepEqual(got, want) {
		t.Fatalf("served folders after load = %+v, want %+v", got, want)
	}
	trusted := make(map[string]bool)
	for _, w := range cfg.TrustedFolderList() {
		trusted[w.Path] = true
	}
	for _, f := range want {
		if !trusted[f.Path] {
			t.Errorf("%s is served but not trusted after load (trusted: %v)", f.Path, trusted)
		}
	}
	if len(trusted) != len(want) {
		t.Errorf("trusted folders after load = %v, want only the served paths", trusted)
	}
	if got, want := cfg.RetiredServedNames, []string{"old"}; !reflect.DeepEqual(got, want) {
		t.Errorf("retired served names after load = %v, want %v", got, want)
	}
}

func TestServedOlderConfigLoads(t *testing.T) {
	baseDir := t.TempDir()
	writeConfigJSON(t, baseDir, `{"startOnLogin": true}`)

	cfg, _, err := LoadFrom(baseDir, noIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServedFolders != nil {
		t.Errorf("ServedFolders = %+v, want nil", cfg.ServedFolders)
	}
	if cfg.RetiredServedNames != nil {
		t.Errorf("RetiredServedNames = %v, want nil", cfg.RetiredServedNames)
	}
	if !cfg.StartOnLogin {
		t.Error("an unrelated setting did not load")
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(DirFrom(baseDir), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "servedFolders") {
		t.Errorf("save wrote servedFolders: %s", raw)
	}
	if strings.Contains(string(raw), "retiredServedNames") {
		t.Errorf("save wrote retiredServedNames: %s", raw)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("the saved config does not parse: %v", err)
	}
}

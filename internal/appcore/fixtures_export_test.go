package appcore

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestShellFixturesMatch keeps the Swift tests' copy of the snapshots equal to the
// goldens: after `go test -update`, run scripts/export-shell-fixtures.sh.
func TestShellFixturesMatch(t *testing.T) {
	src := filepath.Join("testdata", "v1")
	dst := filepath.Join("..", "..", "macos", "Tests", "PreviewCoreTests", "Fixtures", "v1")
	list := func(dir string) []string {
		names, err := filepath.Glob(filepath.Join(dir, "*.snapshot.json"))
		if err != nil {
			t.Fatal(err)
		}
		for i, n := range names {
			names[i] = filepath.Base(n)
		}
		sort.Strings(names)
		return names
	}
	want, got := list(src), list(dst)
	if len(want) == 0 {
		t.Fatal("no golden snapshots found")
	}
	if len(want) != len(got) {
		t.Fatalf("Swift fixtures %v, goldens %v; run scripts/export-shell-fixtures.sh", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("Swift fixtures %v, goldens %v; run scripts/export-shell-fixtures.sh", got, want)
		}
		a, errA := os.ReadFile(filepath.Join(src, name))
		b, errB := os.ReadFile(filepath.Join(dst, name))
		if errA != nil || errB != nil {
			t.Fatal(errA, errB)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s differs from the Swift copy; run scripts/export-shell-fixtures.sh", name)
		}
	}
	actions, err := os.ReadFile(filepath.Join(src, "actions.json"))
	if err != nil {
		t.Fatal(err)
	}
	swiftActions, err := os.ReadFile(filepath.Join(dst, "actions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actions, swiftActions) {
		t.Errorf("actions.json differs from the Swift copy; run scripts/export-shell-fixtures.sh")
	}
	menu, err := os.ReadFile(filepath.Join(src, "menu.json"))
	if err != nil {
		t.Fatal(err)
	}
	swiftMenu, err := os.ReadFile(filepath.Join(dst, "menu.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(menu, swiftMenu) {
		t.Errorf("menu.json differs from the Swift copy; run scripts/export-shell-fixtures.sh")
	}
}

// actionClass is one action type and the class it carries, as the Mac app reads
// them: the app sends an action of a class its window may send.
type actionClass struct {
	Type  string `json:"type"`
	Class string `json:"class"`
}

// TestActionListFixture writes the action types and their classes for the Swift
// tests, so the app's list cannot drift from Go's.
func TestActionListFixture(t *testing.T) {
	list := make([]actionClass, 0, len(ActionClasses))
	for name, class := range ActionClasses {
		list = append(list, actionClass{Type: name, Class: class.String()})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Type < list[j].Type })
	got, err := json.MarshalIndent(struct {
		V       int           `json:"v"`
		Actions []actionClass `json:"actions"`
	}{V: 1, Actions: list}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "v1", "actions.json")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("testdata/v1/actions.json is stale: go test ./internal/appcore -run TestActionListFixture -update, then scripts/export-shell-fixtures.sh")
	}
}

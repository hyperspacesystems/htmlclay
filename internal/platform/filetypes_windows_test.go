package platform

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func setChoice(t *testing.T, path, id string) {
	t.Helper()
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer k.Close()
	if err := k.SetStringValue("ProgId", id); err != nil {
		t.Fatalf("set ProgId: %v", err)
	}
}

// It writes the current user's file associations, so it runs only on CI's
// throwaway Windows runner.
func TestStopOpeningClearsOurChoiceAndKeepsAnotherAppsOne(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("changes this user's .htmlclay association; runs on CI only")
	}
	t.Cleanup(func() {
		for _, path := range htmlclayChoiceKeys {
			_ = deleteTree(registry.CURRENT_USER, path)
		}
		_ = unregisterFileTypes()
	})
	if err := registerFileTypes(filepath.Join(t.TempDir(), "htmlclay.exe"), FileTypeOptions{HTMLClay: true}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if got := readFileTypes().Default; got != "htmlclay" {
		t.Fatalf("after registering: default = %q, want htmlclay", got)
	}
	if !keyExists(capabilitiesPath + `\FileAssociations`) {
		t.Fatal("registering did not list HTML Clay for Settings, Default apps")
	}
	setChoice(t, htmlclayChoiceKeys[1], progID)
	setChoice(t, htmlclayChoiceKeys[0], "Another.App")

	if err := stopOpening(); err != nil {
		t.Fatalf("stopOpening: %v", err)
	}

	if keyExists(htmlclayChoiceKeys[1]) {
		t.Fatal("Stop opening left the choice that names HTML Clay")
	}
	if !keyExists(htmlclayChoiceKeys[0]) {
		t.Fatal("Stop opening deleted another app's choice; only Make default may")
	}
	if classDefaultIs(`.htmlclay`, progID) {
		t.Fatal("Stop opening left HTML Clay as the class default")
	}
}

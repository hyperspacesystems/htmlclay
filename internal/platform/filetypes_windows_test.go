package platform

import (
	"os"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// It writes the current user's file associations, so it runs only on CI's
// throwaway Windows runner.
func TestStopOpeningClearsTheChooserChoice(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("changes this user's .htmlclay association; runs on CI only")
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, userChoicePath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create UserChoice: %v", err)
	}
	if err := k.SetStringValue("ProgId", progID); err != nil {
		k.Close()
		t.Fatalf("set ProgId: %v", err)
	}
	k.Close()
	t.Cleanup(func() { _ = registry.DeleteKey(registry.CURRENT_USER, userChoicePath) })
	if got := readFileTypes().Default; got != "htmlclay" {
		t.Fatalf("before: default = %q, want htmlclay", got)
	}

	if err := stopOpening(); err != nil {
		t.Fatalf("stopOpening: %v", err)
	}

	if got := readFileTypes().Default; got == "htmlclay" {
		t.Fatal("after Stop opening, Windows still lists HTML Clay as the opener of .htmlclay files")
	}
}

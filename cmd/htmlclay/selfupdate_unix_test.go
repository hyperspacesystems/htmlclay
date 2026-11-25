//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/update"
)

// The successful path up to the exit: the handoff is written for the new
// version, the swap happens, the state names the handoff for the supervisor,
// and the app quits. The install is a shell script so the supervisor Apply
// starts from the previous binary exits at once.
func TestApplyUpdateHandsOffSwapsAndQuits(t *testing.T) {
	a, install := updateTestApp(t)
	os.WriteFile(install, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	staged := filepath.Join(filepath.Dir(install), ".htmlclay.new")
	os.WriteFile(staged, []byte("#!/bin/sh\n# new\nexit 0\n"), 0o755)
	quit := false
	a.rt.quitForUpdate = func() { quit = true }

	if err := a.applyUpdate(update.Ready{Version: "9.9.9", StagedPath: staged}, install); err != nil {
		t.Fatalf("applyUpdate: %v", err)
	}
	if !quit {
		t.Fatal("the app did not quit after applying the update")
	}
	handoff := filepath.Join(a.rt.configDir, updateHandoffFile)
	if _, err := os.Stat(handoff); err != nil {
		t.Fatalf("no handoff for the new version: %v", err)
	}
	st, err := update.ReadState(update.UpdatesDir(a.rt.configDir))
	if err != nil || st == nil {
		t.Fatalf("state %+v, %v", st, err)
	}
	if st.To != "9.9.9" || st.From != version || st.HandoffPath != handoff {
		t.Fatalf("state %+v", st)
	}
	data, _ := os.ReadFile(install)
	if string(data) != "#!/bin/sh\n# new\nexit 0\n" {
		t.Fatal("the staged binary is not at the install path")
	}
	if !a.updating.Load() {
		t.Fatal("an applied update must keep a second click from starting another")
	}
}

// An update downloaded before a quit installs at the next start, and the file
// that start was asked to open reaches the new version once.
func TestAWaitingUpdateInstallsAtStartAndKeepsTheFile(t *testing.T) {
	a, install := updateTestApp(t)
	os.WriteFile(install, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	staged := filepath.Join(filepath.Dir(install), ".htmlclay.new")
	os.WriteFile(staged, []byte("#!/bin/sh\n# new\nexit 0\n"), 0o755)
	if err := update.RecordWaiting(update.UpdatesDir(a.rt.configDir), "9.9.9", staged); err != nil {
		t.Fatal(err)
	}

	if !a.installWaiting(install, []string{"/home/sam/notes.htmlclay"}) {
		t.Fatal("the waiting update was not installed at start")
	}
	if data, _ := os.ReadFile(install); string(data) != "#!/bin/sh\n# new\nexit 0\n" {
		t.Fatal("the staged binary is not at the install path")
	}
	if got := a.takeOpenAfterUpdate(); len(got) != 1 || got[0] != "/home/sam/notes.htmlclay" {
		t.Fatalf("the new version opens %q, want the file the start was asked to open", got)
	}
	if got := a.takeOpenAfterUpdate(); got != nil {
		t.Fatalf("the file opened again on a later start: %q", got)
	}
}

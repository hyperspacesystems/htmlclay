package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/platform"
)

// The Mac app owns file types, so the host it started refuses the actions and
// the snapshot says it does not manage them.
func TestFileTypesAreTheMacAppsJobOnTheMac(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the Mac app only runs on macOS")
	}
	a := newShellTestApp(t, t.TempDir())
	refusal := a.runEffect(context.Background(), appcore.Effect{Type: "fileTypes.makeDefault"})
	if refusal == nil || refusal.Code != appcore.RefuseNotOnThisSystem {
		t.Fatalf("fileTypes.makeDefault = %+v, want %s", refusal, appcore.RefuseNotOnThisSystem)
	}
	if a.fileTypeFacts().Managed {
		t.Fatal("the Mac host says it manages file types")
	}
}

// Make default on Windows that had to open Settings answers finish-in-settings,
// so Settings, Files says to finish there rather than "try again".
func TestMakeDefaultThatOpenedSettingsSaysFinishThere(t *testing.T) {
	if got := fileTypeRefusal(fmt.Errorf("make default: %w", platform.ErrFinishInSettings)); got.Code != appcore.RefuseFinishInSettings {
		t.Fatalf("Settings opened: refusal %q, want %q", got.Code, appcore.RefuseFinishInSettings)
	}
	if got := fileTypeRefusal(errors.New("access denied")); got.Code != appcore.RefuseFailed {
		t.Fatalf("a registry failure: refusal %q, want %q", got.Code, appcore.RefuseFailed)
	}
}

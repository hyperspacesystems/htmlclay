package main

import (
	"context"
	"runtime"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
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

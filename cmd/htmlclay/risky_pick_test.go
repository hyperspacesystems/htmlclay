package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/testutil"
)

// A file picked from the not-executable question gets the same risky-file check
// as a file picked from the first question.
func TestM3NotExecutablePickIsChecked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no executable bit: a program there runs by its extension, so this question is never asked")
	}
	settledSetup(t)
	home := t.TempDir()
	a := newShellTestApp(t, home)
	noDialog(t, a)
	closePendingRequests(t, a)
	document := writeHelperPage(t, home, "search")
	program := writeTestProgram(t, t.TempDir(), "search", 0o644)
	if _, err := a.rt.cfg.AddHelperProgram("search", program); err != nil {
		t.Fatal(err)
	}
	downloads := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatal(err)
	}
	picked := writeTestProgram(t, downloads, "picked", 0o755)

	plan := startSetup(t, a, document, "search")
	found := waitForSetupRequest(t, a, appcore.RequestFound)
	if err := answerSetup(t, a, "r1", found, helperAnswerAllowDocument, ""); err != nil {
		t.Fatal(err)
	}
	notExec := waitForSetupRequest(t, a, appcore.RequestRiskyFile)
	if notExec.Detail != helperDetailNotExecutable {
		t.Fatalf("first risky detail = %q", notExec.Detail)
	}
	if err := answerSetup(t, a, "r2", notExec, helperAnswerChooseFile, picked); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case got := <-plan:
			allowed, ok := got.allowed["search"]
			t.Fatalf("the flow finished without asking about the picked file; allowed=%+v ok=%v (picked %s)", allowed, ok, picked)
		default:
		}
		for _, r := range a.snapshot().Requests {
			if r.Kind == appcore.RequestRiskyFile && r.Detail != helperDetailNotExecutable {
				if err := answerSetup(t, a, "r3", r, helperAnswerDeny, ""); err != nil {
					t.Fatal(err)
				}
				got := testutil.Receive(t, 5*time.Second, "the setup flow", plan)
				if _, ok := got.allowed["search"]; ok {
					t.Fatalf("a denied pick was allowed: %+v", got.allowed)
				}
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no question about the picked file")
}

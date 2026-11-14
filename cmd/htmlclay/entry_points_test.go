package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/testutil"
)

// noDialog makes any call that would open a platform dialog fail this test: it
// stands in for showing the window, so a flow that asks outside the request
// window fails here instead of on the machine.
func noDialog(t *testing.T, a *app) {
	t.Helper()
	a.rt.dialogAttempt = func(name string) {
		t.Errorf("a %s dialog was opened; every question must go through the request window", name)
	}
}

// closePendingRequests leaves no question open when the subtest ends.
func closePendingRequests(t *testing.T, a *app) {
	t.Helper()
	t.Cleanup(func() {
		for _, p := range a.registry().Pending() {
			a.registry().Close(p.ID)
		}
	})
}

// TestM3ConfirmationEntryPoints proves every entry point a page, a person or the
// Mac app can reach while the Mac app runs this host answers through the request
// window or is refused: no Go code opens a platform dialog, because nobody would
// see it.
func TestM3ConfirmationEntryPoints(t *testing.T) {
	t.Run("a read question from a page", func(t *testing.T) {
		home := evalTempHome(t)
		page := filepath.Join(home, "work", "review", "fable", "index.html")
		asset := filepath.Join(home, "work", "review", "shared", "redpen.js")
		writeTestFile(t, page, "<html><body>fable</body></html>")
		writeTestFile(t, asset, "console.log('redpen')")

		a := newTestApp(t, home)
		a.shell = true
		noDialog(t, a)
		s, _ := a.openForTest(t, page)

		done := make(chan int, 1)
		go func() {
			code, _, _ := fetchWithContext(context.Background(), s.url(filepath.Join("work", "review", "shared", "redpen.js")))
			done <- code
		}()
		request := waitForReadRequest(t, a, appcore.RequestFolderRead)
		if !slices.Contains(request.Answers, readChoiceTrustFolder) {
			t.Fatalf("the read question offered %v, want %s among them", request.Answers, readChoiceTrustFolder)
		}
		if _, refusal := a.do(context.Background(), testAnswer(t, "e1", request, "deny"), ChannelShell); refusal != nil {
			t.Fatalf("the shell's answer was refused: %+v", refusal)
		}
		if code := testutil.Receive(t, 10*time.Second, "the denied read", done); code != 403 {
			t.Errorf("a denied read = %d, want 403", code)
		}
	})

	t.Run("a page's own-folder trust question", func(t *testing.T) {
		home := evalTempHome(t)
		proj := filepath.Join(home, "proj")
		file := filepath.Join(proj, "index.htmlclay")
		writeTestFile(t, file, "<html><body>doc</body></html>")

		a := newTestApp(t, home)
		a.shell = true
		noDialog(t, a)

		type answer struct {
			url string
			ok  bool
		}
		done := make(chan answer, 1)
		go func() {
			url, ok := a.trustFromPage(file, true)
			done <- answer{url, ok}
		}()
		request := waitForReadRequest(t, a, appcore.RequestFolderReadAgain)
		if want := []string{"deny", readChoiceTrustFolder}; !slices.Equal(request.Answers, want) {
			t.Fatalf("the own-folder question offered %v, want %v", request.Answers, want)
		}
		if _, refusal := a.do(context.Background(), testAnswer(t, "e2", request, "deny"), ChannelShell); refusal != nil {
			t.Fatalf("the shell's answer was refused: %+v", refusal)
		}
		got := testutil.Receive(t, 10*time.Second, "the trust question to settle", done)
		if got.ok {
			t.Error("a denied own-folder question trusted the folder")
		}
		if trusted := a.rt.cfg.TrustedFolderList(); len(trusted) != 0 {
			t.Errorf("a denied own-folder question trusted %v", trusted)
		}
	})

	t.Run("a program question for a document", func(t *testing.T) {
		a, document, program := newSetupConfirmationApp(t)
		noDialog(t, a)
		plan := startSetup(t, a, document, "search")
		request := waitForSetupRequest(t, a, appcore.RequestFound)
		if request.Path != program {
			t.Fatalf("the question names %q, want %q", request.Path, program)
		}
		if !slices.Contains(request.Answers, helperAnswerAllowDocument) {
			t.Fatalf("the program question offered %v, want %s among them", request.Answers, helperAnswerAllowDocument)
		}
		if err := answerSetup(t, a, "e3", request, helperAnswerAllowDocument, ""); err != nil {
			t.Fatal(err)
		}
		got := testutil.Receive(t, 5*time.Second, "the setup flow", plan)
		if allowed, ok := got.allowed["search"]; !ok || allowed.Path != program || allowed.AnyDocument {
			t.Fatalf("allowed = %+v, %v, want one document at %s", allowed, ok, program)
		}
	})

	t.Run("the wider program grant is an answer, not a dialog", func(t *testing.T) {
		a, document, _ := newSetupConfirmationApp(t)
		noDialog(t, a)
		plan := startSetup(t, a, document, "search")
		request := waitForSetupRequest(t, a, appcore.RequestFound)
		if !slices.Contains(request.Answers, helperAnswerAllowAllTrusted) {
			t.Fatalf("the program question offered %v, want %s among them", request.Answers, helperAnswerAllowAllTrusted)
		}
		if err := answerSetup(t, a, "e4", request, helperAnswerAllowAllTrusted, ""); err != nil {
			t.Fatal(err)
		}
		got := testutil.Receive(t, 5*time.Second, "the setup flow", plan)
		if allowed, ok := got.allowed["search"]; !ok || !allowed.AnyDocument {
			t.Fatalf("allowed = %+v, %v, want any-document", allowed, ok)
		}
	})

	t.Run("a risky program pick warns in the window", func(t *testing.T) {
		settledSetup(t)
		a, document, _ := newSetupConfirmationApp(t)
		noDialog(t, a)
		downloads := filepath.Join(a.rt.home, "Downloads")
		if err := os.MkdirAll(downloads, 0o755); err != nil {
			t.Fatal(err)
		}
		picked := writeTestProgram(t, downloads, "picked", 0o755)

		plan := startSetup(t, a, document, "search")
		found := waitForSetupRequest(t, a, appcore.RequestFound)
		if err := answerSetup(t, a, "e5", found, helperAnswerChooseFile, picked); err != nil {
			t.Fatal(err)
		}
		risky := waitForSetupRequest(t, a, appcore.RequestRiskyFile)
		if risky.Detail != helperDetailInDownloads {
			t.Fatalf("the risky question says %q, want %s", risky.Detail, helperDetailInDownloads)
		}
		if err := answerSetup(t, a, "e6", risky, helperAnswerAllowAnyway, ""); err != nil {
			t.Fatal(err)
		}
		got := testutil.Receive(t, 5*time.Second, "the setup flow", plan)
		if allowed, ok := got.allowed["search"]; !ok || allowed.Path != picked || allowed.AnyDocument {
			t.Fatalf("allowed = %+v, %v, want one document at %s", allowed, ok, picked)
		}
	})

	t.Run("the program file comes from the window, not a picker", func(t *testing.T) {
		settledSetup(t)
		home := t.TempDir()
		a := newTestApp(t, home)
		a.shell = true
		noDialog(t, a)
		closePendingRequests(t, a)
		document := writeHelperPage(t, home, "search")
		picked := writeTestProgram(t, t.TempDir(), "search", 0o755)

		plan := startSetup(t, a, document, "search")
		request := waitForSetupRequest(t, a, appcore.RequestNoFile)
		if !slices.Contains(request.Answers, helperAnswerChooseFile) {
			t.Fatalf("the no-file question offered %v, want %s among them", request.Answers, helperAnswerChooseFile)
		}
		if err := answerSetup(t, a, "e7", request, helperAnswerChooseFile, picked); err != nil {
			t.Fatal(err)
		}
		got := testutil.Receive(t, 5*time.Second, "the setup flow", plan)
		if allowed, ok := got.allowed["search"]; !ok || allowed.Path != picked || allowed.AnyDocument {
			t.Fatalf("allowed = %+v, %v, want one document at %s", allowed, ok, picked)
		}
	})

	t.Run("making a program executable waits for an answer", func(t *testing.T) {
		home := t.TempDir()
		a := newTestApp(t, home)
		a.shell = true
		noDialog(t, a)
		closePendingRequests(t, a)
		document := writeHelperPage(t, home, "search")
		program := writeTestProgram(t, t.TempDir(), "search", 0o644)
		if _, err := a.rt.cfg.AddHelperProgram("search", program); err != nil {
			t.Fatal(err)
		}

		plan := startSetup(t, a, document, "search")
		found := waitForSetupRequest(t, a, appcore.RequestFound)
		if err := answerSetup(t, a, "e8", found, helperAnswerAllowDocument, ""); err != nil {
			t.Fatal(err)
		}
		risky := waitForSetupRequest(t, a, appcore.RequestRiskyFile)
		if risky.Detail != helperDetailNotExecutable || risky.Path != program {
			t.Fatalf("the not-executable question = %q at %q, want %s at %s", risky.Detail, risky.Path, helperDetailNotExecutable, program)
		}
		if info, err := os.Stat(program); err != nil || info.Mode()&0o111 != 0 {
			t.Fatalf("the file became executable before the person answered: %v, %v", info, err)
		}
		if err := answerSetup(t, a, "e9", risky, helperAnswerAllowAnyway, ""); err != nil {
			t.Fatal(err)
		}
		got := testutil.Receive(t, 5*time.Second, "the setup flow", plan)
		if _, ok := got.allowed["search"]; !ok {
			t.Fatalf("allowed = %+v, want search allowed after allow-anyway", got.allowed)
		}
		if info, err := os.Stat(program); err != nil || info.Mode()&0o111 == 0 {
			t.Fatalf("an answered allow-anyway left the file not executable: %v, %v", info, err)
		}
	})

	t.Run("Quit keeps its one refusal", func(t *testing.T) {
		a := newServedTestApp(t, evalTempHome(t), t.TempDir())
		a.shell = true
		noDialog(t, a)
		a.service = unsavedService(t, a)
		_, refusal := a.do(context.Background(), coreAction(t, "e10", "host.quit", map[string]any{}), ChannelShell)
		if refusal == nil || refusal.Code != appcore.RefuseQuitUnsaved || len(refusal.Documents) != 1 || refusal.Documents[0] != "index.html" {
			t.Fatalf("host.quit = %+v, want %s naming index.html", refusal, appcore.RefuseQuitUnsaved)
		}
	})

	t.Run("a direct trust choice", func(t *testing.T) {
		home := evalTempHome(t)
		a := newTestApp(t, home)
		a.shell = true
		noDialog(t, a)
		a.pickAndTrustFolder()
		if trusted := a.rt.cfg.TrustedFolderList(); len(trusted) != 0 {
			t.Errorf("a refused folder choice trusted %v", trusted)
		}
	})

	t.Run("a direct untrust choice", func(t *testing.T) {
		home := evalTempHome(t)
		dir := filepath.Join(home, "proj")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		a := newTestApp(t, home)
		a.shell = true
		if err := a.trustFolder(dir); err != nil {
			t.Fatal(err)
		}
		noDialog(t, a)
		a.removeTrustedFolder(dir)
		if !hasFolder(a.rt.cfg.TrustedFolderList(), dir) {
			t.Errorf("a refused removal dropped %s from the trusted list", dir)
		}
	})

	t.Run("relink on the programs page", func(t *testing.T) {
		settledSetup(t)
		a := newProgramsApp(t)
		a.shell = true
		noDialog(t, a)
		path := writeTestProgram(t, t.TempDir(), "tweet-score", 0o755)
		program, err := a.rt.cfg.AddHelperProgram("tweet-score", path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		writeTestProgram(t, t.TempDir(), "tweet-score", 0o755)

		if outcome := a.programsAction("relink", program.ID, "", ""); outcome != "unchanged" {
			t.Fatalf("relink = %q, want unchanged", outcome)
		}
		if p, _ := a.rt.cfg.LookupHelperProgram(program.ID); p.Path != path {
			t.Errorf("a refused relink pointed the program at %q, want %q", p.Path, path)
		}
	})

	t.Run("widen on the programs page", func(t *testing.T) {
		a := newProgramsApp(t)
		a.shell = true
		noDialog(t, a)
		program, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0o755))
		if err != nil {
			t.Fatal(err)
		}

		if outcome := a.programsAction("widen", program.ID, "", ""); outcome != "unchanged" {
			t.Fatalf("widen = %q, want unchanged", outcome)
		}
		if p, _ := a.rt.cfg.LookupHelperProgram(program.ID); p.AnyDocument {
			t.Error("a refused widen allowed the program for every trusted document")
		}
	})
}

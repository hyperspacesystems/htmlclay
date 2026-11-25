package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
)

// grantDashboard is a dashboard on a host that confirms grants with its own
// dialogs, as on Windows and Linux, with one folder to work with.
func grantDashboard(t *testing.T) (*dashboardNoService, string) {
	t.Helper()
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)
	// Sync is not offered on Windows yet. These tests run the grant logic as
	// Linux on every system, and the one test about Windows says so itself.
	previous := serveOS
	serveOS = "linux"
	t.Cleanup(func() { serveOS = previous })
	c := openDashboardWithNoService(t, home)
	c.app.startServed()
	t.Cleanup(func() { c.app.served.Close() })
	c.app.nativeGrants = true
	// No test may put a real dialog on the screen: a dialog with no seam set is
	// refused and fails the test.
	c.app.rt.dialogAttempt = func(name string) { t.Errorf("a real %s dialog was about to open", name) }
	// Nor a real notification.
	c.app.rt.notify = func(title, message string) error {
		t.Logf("notice: %s: %s", title, message)
		return nil
	}
	return c, dir
}

func grantWait(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDashboardGrantIsAskedInASystemDialog(t *testing.T) {
	c, dir := grantDashboard(t)
	prompts := make(chan string, 4)
	release := make(chan struct{})
	c.app.rt.selectFolder = func(prompt string) (string, bool, error) {
		prompts <- prompt
		<-release
		return dir, true, nil
	}

	// A page that names the folder itself is refused: the dialog chooses it.
	status, body := c.act("g0", "folder.trust", map[string]any{"path": dir})
	if code := refusalOf(t, status, body); code != appcore.RefuseBadArguments {
		t.Fatalf("folder.trust with a path from the page = %s", code)
	}
	status, body = c.act("k0", "sync.connect", map[string]any{"key": "hcsk_" + strings.Repeat("5e", 32)})
	if code := refusalOf(t, status, body); code != appcore.RefuseBadArguments {
		t.Fatalf("sync.connect with a key from the page = %s", code)
	}

	// So is a page that says the person has already seen a warning.
	for i, args := range []map[string]any{
		{"acknowledged": []string{"inside-trusted"}},
		{"confirmed": true},
		{"destination": dir},
	} {
		status, body = c.act(fmt.Sprintf("a%d", i), "folder.trust", args)
		if code := refusalOf(t, status, body); code != appcore.RefuseBadArguments {
			t.Fatalf("folder.trust with %v from the page = %s", args, code)
		}
	}

	status, body = c.act("g1", "folder.trust", nil)
	if status != http.StatusAccepted || !strings.Contains(body, `"pending":true`) {
		t.Fatalf("folder.trust from the dashboard = %d %s, want 202 pending", status, body)
	}
	select {
	case <-prompts:
	case <-time.After(5 * time.Second):
		t.Fatal("no folder picker opened")
	}
	if trusted := c.app.rt.cfg.TrustedFolderList(); len(trusted) != 0 {
		t.Fatalf("the folder was trusted before the person answered: %v", trusted)
	}

	// One dialog at a time.
	status, body = c.act("g2", "folder.serve", nil)
	if code := refusalOf(t, status, body); code != appcore.RefuseDialogOpen {
		t.Fatalf("a second grant while a dialog is open = %s", code)
	}

	close(release)
	grantWait(t, "the folder to be trusted", func() bool { return len(c.app.rt.cfg.TrustedFolderList()) == 1 })
	grantWait(t, "the dialog to be done", func() bool { return !c.app.grantOpen.Load() })
	if !c.snapshot().Host.Can["folder.trust"] {
		t.Fatal("the dashboard's snapshot does not offer folder.trust on a host with its own dialogs")
	}
	if !c.snapshot().Host.Can["program.add"] {
		t.Fatal("the dashboard's snapshot does not offer program.add on a host with its own dialogs")
	}
}

func TestDashboardGrantDoesNothingWhenThePersonSaysNo(t *testing.T) {
	c, dir := grantDashboard(t)
	c.app.rt.selectFolder = func(string) (string, bool, error) { return "", false, nil }
	status, body := c.act("g1", "folder.trust", nil)
	if status != http.StatusAccepted {
		t.Fatalf("folder.trust = %d %s", status, body)
	}
	grantWait(t, "the dialog to be done", func() bool { return !c.app.grantOpen.Load() })
	if trusted := c.app.rt.cfg.TrustedFolderList(); len(trusted) != 0 {
		t.Fatalf("a cancelled picker trusted %v", trusted)
	}
	_ = dir
}

func TestDashboardGrantWithAnIDIsConfirmedWithTheHostsOwnWords(t *testing.T) {
	c, dir := grantDashboard(t)
	c.app.rt.selectFolder = func(string) (string, bool, error) { return dir, true, nil }
	if status, body := c.act("g1", "folder.trust", nil); status != http.StatusAccepted {
		t.Fatalf("folder.trust = %d %s", status, body)
	}
	grantWait(t, "the folder to be trusted", func() bool { return len(c.app.rt.cfg.TrustedFolderList()) == 1 })
	grantWait(t, "the dialog to be done", func() bool { return !c.app.grantOpen.Load() })
	var id string
	for _, f := range c.app.snapshot().Folders {
		if f.Path == dir {
			id = f.ID
		}
	}
	if id == "" {
		t.Fatal("the trusted folder is not in the snapshot")
	}

	type question struct{ title, message, affirmative string }
	asked := make(chan question, 2)
	yes := false
	c.app.rt.confirmTrust = func(title, message, affirmative string) (bool, error) {
		asked <- question{title, message, affirmative}
		return yes, nil
	}
	if status, body := c.act("s1", "folder.serve", map[string]any{"id": id}); status != http.StatusAccepted {
		t.Fatalf("folder.serve = %d %s", status, body)
	}
	q := <-asked
	if q.title != appcore.TextGrantTitle || !strings.Contains(q.message, dir) || q.affirmative != appcore.TextGrantServeYes {
		t.Fatalf("the question was %+v; want the folder's own path in it", q)
	}
	grantWait(t, "the dialog to be done", func() bool { return !c.app.grantOpen.Load() })
	if served := c.app.rt.cfg.ServedFolderList(); len(served) != 0 {
		t.Fatalf("the person said no and the folder is served: %v", served)
	}

	yes = true
	if status, body := c.act("s2", "folder.serve", map[string]any{"id": id}); status != http.StatusAccepted {
		t.Fatalf("folder.serve = %d %s", status, body)
	}
	<-asked
	grantWait(t, "the folder to be served", func() bool { return len(c.app.rt.cfg.ServedFolderList()) == 1 })

	// An id that names nothing opens no dialog.
	status, body := c.act("s3", "folder.serve", map[string]any{"id": "no-such-folder"})
	if code := refusalOf(t, status, body); code != appcore.RefuseBadArguments {
		t.Fatalf("folder.serve for an unknown id = %s", code)
	}
}

func TestDashboardGrantNeedsAProgramThatCanShowADialog(t *testing.T) {
	c, _ := grantDashboard(t)
	c.app.dialogAdvice = "Install zenity"
	c.app.rt.selectFolder = func(string) (string, bool, error) {
		t.Error("a picker was opened on a system with no dialog program")
		return "", false, nil
	}
	status, body := c.act("g1", "folder.trust", nil)
	if code := refusalOf(t, status, body); code != appcore.RefuseNoDialogs {
		t.Fatalf("folder.trust with no dialog program = %s", code)
	}
}

func TestDashboardSyncConnectTakesTheKeyFromTheHiddenPrompt(t *testing.T) {
	c, _ := grantDashboard(t)
	type prompt struct{ title, message string }
	prompts := make(chan prompt, 1)
	c.app.rt.promptSecret = func(title, message string) (string, bool, error) {
		prompts <- prompt{title, message}
		return "", false, nil
	}
	if status, body := c.act("k1", "sync.connect", nil); status != http.StatusAccepted {
		t.Fatalf("sync.connect = %d %s", status, body)
	}
	select {
	case p := <-prompts:
		if p.title != appcore.TextGrantConnectTitle || p.message != appcore.TextGrantConnectMessage {
			t.Fatalf("the key prompt was %+v", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no key prompt opened")
	}
	grantWait(t, "the dialog to be done", func() bool { return !c.app.grantOpen.Load() })
	if c.app.snapshot().Sync.Connected {
		t.Fatal("a cancelled key prompt connected sync")
	}
}

func TestDashboardOpenFileOpensWhatThePickerChose(t *testing.T) {
	c, dir := grantDashboard(t)
	prompts := make(chan string, 1)
	c.app.rt.selectFile = func(prompt string) (string, bool, error) {
		prompts <- prompt
		return "", false, nil
	}
	status, body := c.act("f0", "file.open", map[string]any{"path": filepath.Join(dir, "index.html")})
	if code := refusalOf(t, status, body); code != appcore.RefuseBadArguments {
		t.Fatalf("file.open with a path from the page = %s", code)
	}
	if status, body := c.act("f1", "file.open", nil); status != http.StatusAccepted {
		t.Fatalf("file.open = %d %s", status, body)
	}
	select {
	case prompt := <-prompts:
		if prompt != appcore.TextGrantOpenPick {
			t.Fatalf("the file picker asked %q", prompt)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no file picker opened")
	}
	grantWait(t, "the dialog to be done", func() bool { return !c.app.grantOpen.Load() })
}

func TestDashboardGrantStaysRefusedWhereTheAppsOwnWindowsGrant(t *testing.T) {
	c, _ := grantDashboard(t)
	c.app.nativeGrants = false
	c.app.rt.selectFolder = func(string) (string, bool, error) {
		t.Error("a picker opened on a host whose app shows its own windows")
		return "", false, nil
	}
	status, body := c.act("g1", "folder.trust", nil)
	if code := refusalOf(t, status, body); code != appcore.RefuseNeedsConfirmation {
		t.Fatalf("folder.trust on such a host = %s", code)
	}
	if c.snapshot().Host.Can["folder.trust"] {
		t.Fatal("the dashboard's snapshot offers folder.trust on such a host")
	}
}

func TestDashboardRelinkShowsTheHostsWarningBeforeItTakesADifferentFolder(t *testing.T) {
	c, dir := grantDashboard(t)
	other := filepath.Join(filepath.Dir(dir), "other")
	writeTestFile(t, filepath.Join(other, "index.html"), servedTestDocument)
	pick := dir
	c.app.rt.selectFolder = func(string) (string, bool, error) { return pick, true, nil }
	if status, body := c.act("t1", "folder.trust", nil); status != http.StatusAccepted {
		t.Fatalf("folder.trust = %d %s", status, body)
	}
	grantWait(t, "the folder to be trusted", func() bool { return len(c.app.rt.cfg.TrustedFolderList()) == 1 })
	grantWait(t, "the dialog to be done", func() bool { return !c.app.grantOpen.Load() })
	id := c.app.rt.cfg.TrustedFolderList()[0].ID

	asked := make(chan string, 2)
	yes := false
	c.app.rt.confirmTrust = func(title, message, affirmative string) (bool, error) {
		asked <- message
		return yes, nil
	}
	pick = other
	relink := func(action string) {
		t.Helper()
		if status, body := c.act(action, "folder.relink", map[string]any{"id": id}); status != http.StatusAccepted {
			t.Fatalf("folder.relink = %d %s", status, body)
		}
		select {
		case message := <-asked:
			if !strings.Contains(message, other) || !strings.Contains(message, appcore.TextWarnDifferentFolder) {
				t.Fatalf("the question was %q; want the new path and the host's warning", message)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("a different folder was taken with no question")
		}
		grantWait(t, "the dialog to be done", func() bool { return !c.app.grantOpen.Load() })
	}

	relink("r1")
	if got := c.app.rt.cfg.TrustedFolderList()[0].Path; got != dir {
		t.Fatalf("the person said no and the folder is now %s", got)
	}
	yes = true
	relink("r2")
	if got := c.app.rt.cfg.TrustedFolderList()[0].Path; got != other {
		t.Fatalf("the person said yes and the folder is still %s", got)
	}
}

func TestSyncConnectIsOfferedOnWindows(t *testing.T) {
	c, _ := grantDashboard(t)
	asked := make(chan struct{}, 1)
	c.app.rt.promptSecret = func(string, string) (string, bool, error) {
		asked <- struct{}{}
		return "", false, nil
	}
	serveOS = "windows"

	if !c.snapshot().Host.Can["sync.connect"] {
		t.Fatal("the snapshot does not offer sync.connect on Windows")
	}
	status, body := c.act("k1", "sync.connect", nil)
	if status != http.StatusAccepted {
		t.Fatalf("sync.connect on Windows = %d %s", status, body)
	}
	select {
	case <-asked:
	case <-time.After(5 * time.Second):
		t.Fatal("the key prompt never opened")
	}
}

func TestDashboardAddsAProgramFromThePickerAfterOneQuestion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a program file on Windows needs a program extension; programs_test covers that rule")
	}
	c, dir := grantDashboard(t)
	file := filepath.Join(dir, "tool")
	writeTestFile(t, file, "#!/bin/sh\necho hi\n")
	if err := os.Chmod(file, 0o755); err != nil {
		t.Fatal(err)
	}
	picked := 0
	c.app.rt.selectFile = func(string) (string, bool, error) { picked++; return file, true, nil }
	asked := make(chan string, 1)
	c.app.rt.confirmTrust = func(title, message, affirmative string) (bool, error) {
		asked <- message
		return affirmative == appcore.TextGrantAddProgramYes, nil
	}

	status, body := c.act("a1", "program.add", map[string]any{"name": "tool", "path": file})
	if code := refusalOf(t, status, body); code != appcore.RefuseBadArguments {
		t.Fatalf("a page that names the program file = %s, want bad-arguments", code)
	}
	if status, body := c.act("a3", "program.add", map[string]any{"name": "file-search"}); refusalOf(t, status, body) != appcore.RefuseNameTaken {
		t.Fatalf("a name a built-in program uses = %d %s", status, body)
	}
	if picked != 0 {
		t.Fatal("a name a built-in program uses opened the file picker first")
	}
	if status, body := c.act("a2", "program.add", map[string]any{"name": "tool"}); status != http.StatusAccepted {
		t.Fatalf("program.add = %d %s", status, body)
	}
	if message := <-asked; !strings.Contains(message, file) || !strings.Contains(message, "tool") {
		t.Fatalf("the question %q does not name the file and the name", message)
	}
	grantWait(t, "the program to be added", func() bool {
		for _, p := range c.app.snapshot().Programs {
			if p.Name == "tool" && p.Path == file {
				return true
			}
		}
		return false
	})
}

func TestDashboardCustomCommandIsCheckedBeforeItsQuestion(t *testing.T) {
	c, _ := grantDashboard(t)
	status, body := c.act("c1", "aiEdit.setCustom", map[string]any{"command": "no-such-program-htmlclay-test {prompt}"})
	if code := refusalOf(t, status, body); code != appcore.RefuseCommandNotFound {
		t.Fatalf("a command that is not installed = %s, want command-not-found", code)
	}
	asked := make(chan string, 1)
	c.app.rt.confirmTrust = func(title, message, affirmative string) (bool, error) {
		asked <- message
		return false, nil
	}
	if status, body := c.act("c2", "aiEdit.setCustom", map[string]any{"command": "go run {prompt}"}); status != http.StatusAccepted {
		t.Fatalf("aiEdit.setCustom = %d %s", status, body)
	}
	if message := <-asked; !strings.Contains(message, "go run {prompt}") {
		t.Fatalf("the question %q does not show the command", message)
	}
	grantWait(t, "the dialog to be done", func() bool { return !c.app.grantOpen.Load() })
	for _, tool := range c.app.snapshot().Preferences.AIEditTools {
		if tool.Custom && tool.Command != "" {
			t.Fatalf("the person said no and the command was saved: %+v", tool)
		}
	}
}

func TestDashboardBuiltinSwitchAsksBeforeItTurnsOn(t *testing.T) {
	c, _ := grantDashboard(t)
	c.app.rt.cfg.SetBuiltinOn("file-search", false)
	asked := make(chan string, 2)
	yes := false
	c.app.rt.confirmTrust = func(title, message, affirmative string) (bool, error) {
		asked <- affirmative
		return yes, nil
	}
	isOn := func() bool {
		for _, p := range c.app.snapshot().Programs {
			if p.ID == "file-search" {
				return p.On
			}
		}
		t.Fatal("file-search is not in the snapshot")
		return false
	}
	if status, body := c.act("b1", "program.setOn", map[string]any{"programID": "file-search", "on": true}); status != http.StatusAccepted {
		t.Fatalf("program.setOn = %d %s", status, body)
	}
	if got := <-asked; got != appcore.TextGrantBuiltinOnYes {
		t.Fatalf("the question's button was %q", got)
	}
	grantWait(t, "the dialog to be done", func() bool { return !c.app.grantOpen.Load() })
	if isOn() {
		t.Fatal("the person said no and the program is on")
	}
	yes = true
	if status, body := c.act("b2", "program.setOn", map[string]any{"programID": "file-search", "on": true}); status != http.StatusAccepted {
		t.Fatalf("program.setOn = %d %s", status, body)
	}
	<-asked
	grantWait(t, "the program to be on", isOn)
}

// Plan reads JSON names without regard to case, so a second spelling of an
// argument could carry a value the dialog never showed: a different command,
// another program, or a sync key for someone else's account.
func TestDashboardGrantRefusesASecondSpellingOfAnArgument(t *testing.T) {
	c, _ := grantDashboard(t)
	asked := make(chan string, 4)
	c.app.rt.confirmTrust = func(title, message, affirmative string) (bool, error) {
		asked <- message
		return true, nil
	}
	for i, tc := range []struct {
		typ  string
		args map[string]any
	}{
		{"aiEdit.setCustom", map[string]any{"command": "sh -c 'echo swapped'", "Command": "go run {prompt}"}},
		{"program.setOn", map[string]any{"programID": "file-search", "ProgramID": "file-convert", "on": true}},
		{"sync.connect", map[string]any{"\u212aey": "hcsk_" + strings.Repeat("5e", 32)}},
		{"folder.serve", map[string]any{"id": "notes", "name": "renamed"}},
	} {
		status, body := c.act(fmt.Sprintf("s%d", i), tc.typ, tc.args)
		if code := refusalOf(t, status, body); code != appcore.RefuseBadArguments {
			t.Fatalf("%s with %v = %s, want %s", tc.typ, tc.args, code, appcore.RefuseBadArguments)
		}
	}
	select {
	case message := <-asked:
		t.Fatalf("a refused grant still asked %q", message)
	case <-time.After(100 * time.Millisecond):
	}
}

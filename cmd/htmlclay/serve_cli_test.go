package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
)

// appSI is a SingleInstance whose primary is a test app: every frame goes to that
// app's handleInstanceCommand, exactly as the socket would carry it.
type appSI struct {
	app  *app
	sent []string
}

func (s *appSI) TryLock() (bool, error)        { return false, nil }
func (s *appSI) SendFilePath(string) error     { return errors.New("not a file") }
func (s *appSI) OnFileReceived(func(string))   {}
func (s *appSI) OnCommand(func(string) string) {}
func (s *appSI) Unlock() error                 { return nil }
func (s *appSI) SendCommand(cmd string) (string, error) {
	s.sent = append(s.sent, cmd)
	return s.app.handleInstanceCommand(cmd), nil
}

func newServeCLITestApp(t *testing.T) (*app, string) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := newServedTestApp(t, home, t.TempDir())
	// The command refuses on Windows (its own test below); the rest of these
	// run its logic on every system.
	previous := serveOS
	serveOS = "linux"
	t.Cleanup(func() { serveOS = previous })
	return a, home
}

func servedAddress(name string) string {
	return "http://" + servedHostName(name, servedTestPort) + "/"
}

func TestServeCommandUsesTheDefaultName(t *testing.T) {
	a, home := newServeCLITestApp(t)
	dir := filepath.Join(home, "My Notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	si := &appSI{app: a}
	var out, errOut bytes.Buffer
	code := runServe(si, []string{"serve", dir}, &out, &errOut)
	if code != serveExitOK {
		t.Fatalf("runServe serve = %d, stderr %q", code, errOut.String())
	}
	want := fmt.Sprintf(appcore.TextNowServed, "my-notes", servedAddress("my-notes"))
	if got := strings.TrimSpace(out.String()); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if len(si.sent) != 1 || !strings.HasPrefix(si.sent[0], "serve - ") {
		t.Fatalf("sent %q, want a serve frame with the default name", si.sent)
	}
	rec := servedRequest(t, a, http.MethodGet, servedHostName("my-notes", servedTestPort), "/index.html", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET the default address = %d, want 200", rec.Code)
	}
}

func TestServeCommandTakesAName(t *testing.T) {
	a, home := newServeCLITestApp(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	si := &appSI{app: a}
	var out, errOut bytes.Buffer
	if code := runServe(si, []string{"serve", dir, "--name", "docs"}, &out, &errOut); code != serveExitOK {
		t.Fatalf("runServe serve --name docs = %d, stderr %q", code, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); !strings.Contains(got, "docs") {
		t.Fatalf("stdout = %q, want the docs address", got)
	}
	if rec := servedRequest(t, a, http.MethodGet, servedHostName("docs", servedTestPort), "/index.html", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET docs = %d, want 200", rec.Code)
	}
}

func TestServeCommandCarriesASpacesAndNonASCIIPath(t *testing.T) {
	a, home := newServeCLITestApp(t)
	dir := filepath.Join(home, "Café Notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	si := &appSI{app: a}
	var out, errOut bytes.Buffer
	if code := runServe(si, []string{"serve", dir}, &out, &errOut); code != serveExitOK {
		t.Fatalf("runServe serve = %d, stderr %q", code, errOut.String())
	}
	if len(si.sent) != 1 {
		t.Fatalf("sent %q, want one frame", si.sent)
	}
	for _, r := range si.sent[0] {
		if r < 0x20 || r > 0x7e {
			t.Fatalf("frame %q holds a rune outside printable ASCII: %q", si.sent[0], r)
		}
	}
	if got := strings.TrimSpace(out.String()); !strings.Contains(got, "caf-notes") {
		t.Fatalf("stdout = %q, want the caf-notes address", got)
	}
	if rec := servedRequest(t, a, http.MethodGet, servedHostName("caf-notes", servedTestPort), "/index.html", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET caf-notes = %d, want 200", rec.Code)
	}
}

func TestServeCommandRefusesTheHomeFolder(t *testing.T) {
	a, home := newServeCLITestApp(t)
	si := &appSI{app: a}
	var out, errOut bytes.Buffer
	code := runServe(si, []string{"serve", home}, &out, &errOut)
	if code != serveExitRefused {
		t.Fatalf("runServe serve <home> = %d, want %d", code, serveExitRefused)
	}
	if got := out.String(); !strings.Contains(got, appcore.RefusalText[appcore.RefuseIsHome]) {
		t.Fatalf("stdout = %q, want the is-home text", got)
	}
	if len(a.rt.cfg.ServedFolderList()) != 0 {
		t.Fatal("a refused folder was recorded")
	}
}

func TestServeCommandStopServing(t *testing.T) {
	a, home := newServeCLITestApp(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	si := &appSI{app: a}
	var out, errOut bytes.Buffer
	if code := runServe(si, []string{"serve", dir, "--name", "docs"}, &out, &errOut); code != serveExitOK {
		t.Fatalf("serve = %d, stderr %q", code, errOut.String())
	}

	out.Reset()
	if code := runServe(si, []string{"stop-serving", "docs"}, &out, &errOut); code != serveExitOK {
		t.Fatalf("stop-serving docs = %d, stderr %q", code, errOut.String())
	}
	want := fmt.Sprintf(appcore.TextStopped, "docs")
	if got := strings.TrimSpace(out.String()); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if rec := servedRequest(t, a, http.MethodGet, servedHostName("docs", servedTestPort), "/index.html", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("GET docs after stopping = %d, want 404", rec.Code)
	}

	out.Reset()
	if code := runServe(si, []string{"stop-serving", "nope"}, &out, &errOut); code != serveExitRefused {
		t.Fatalf("stop-serving nope = %d, want %d", code, serveExitRefused)
	}
	if got := out.String(); !strings.Contains(got, fmt.Sprintf(appcore.TextNotServed, "nope")) {
		t.Fatalf("stdout = %q, want the not-served text", got)
	}
}

func TestServeCommandAcceptsALongPath(t *testing.T) {
	a, home := newServeCLITestApp(t)
	dir := filepath.Join(home, strings.Repeat("a", 90), strings.Repeat("b", 90), strings.Repeat("c", 90), "long-folder")
	if len(dir) < 300 {
		t.Fatalf("test path is %d characters, want at least 300", len(dir))
	}
	writeTestFile(t, filepath.Join(dir, "index.html"), servedTestDocument)

	si := &appSI{app: a}
	var out, errOut bytes.Buffer
	if code := runServe(si, []string{"serve", dir}, &out, &errOut); code != serveExitOK {
		t.Fatalf("serve a long path = %d, stderr %q, stdout %q", code, errOut.String(), out.String())
	}
	if len(si.sent) != 1 || len(si.sent[0]) <= 256 {
		t.Fatalf("frame %q is shorter than the 256 bytes an older socket took", si.sent)
	}
	if !strings.Contains(out.String(), "long-folder") {
		t.Fatalf("stdout = %q, want the long-folder address", out.String())
	}
}

func TestServeCommandLeavesUpdateNowAlone(t *testing.T) {
	orig := updateNowOS
	t.Cleanup(func() { updateNowOS = orig })
	updateNowOS = "darwin"
	a, _ := newServeCLITestApp(t)
	want := "already " + version
	if got := a.handleInstanceCommand("update-now " + version); got != want {
		t.Fatalf("handleInstanceCommand(update-now %s) = %q, want %q", version, got, want)
	}
	for _, cmd := range []string{"", "update-now", "serve", "serve notes", "stop-serving", "install 9.9.9"} {
		if got := a.handleInstanceCommand(cmd); got != "refused unknown command" {
			t.Errorf("handleInstanceCommand(%q) = %q, want refused unknown command", cmd, got)
		}
	}
	if got := a.handleInstanceCommand("serve notes not-base64!"); got != "refused bad path" {
		t.Errorf("handleInstanceCommand with a bad path = %q, want refused bad path", got)
	}
}

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/builtin"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// builtinStand is a host with one trusted folder holding one document.
type builtinStand struct {
	app   *app
	home  string
	dir   string
	doc   string
	other string
}

func newBuiltinStand(t *testing.T) builtinStand {
	t.Helper()
	return newBuiltinStandAt(t, t.TempDir())
}

func newBuiltinStandAt(t *testing.T, home string) builtinStand {
	t.Helper()
	a := newTestApp(t, home)
	dir := filepath.Join(t.TempDir(), "docs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := filepath.Join(dir, "page.htmlclay")
	if err := os.WriteFile(doc, []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.rt.cfg.AddTrustedFolder(real(t, dir), platform.DirIdentity(dir))
	loose := filepath.Join(t.TempDir(), "loose")
	if err := os.MkdirAll(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(loose, "page.htmlclay")
	if err := os.WriteFile(other, []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return builtinStand{app: a, dir: dir, doc: doc, other: other}
}

// real is a path with symlinks resolved, as the host reports the folder's root.
func real(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestBuiltinStartsOff(t *testing.T) {
	s := newBuiltinStand(t)
	if got := s.app.builtinFor(s.doc, builtin.FileSearch).State; got != builtin.Off {
		t.Fatalf("state = %q, want off", got)
	}
}

func TestBuiltinNeedsATrustedFolder(t *testing.T) {
	s := newBuiltinStand(t)
	s.app.rt.cfg.SetBuiltinOn(builtin.FileSearch, true)
	if got := s.app.builtinFor(s.other, builtin.FileSearch).State; got != builtin.Untrusted {
		t.Fatalf("a document outside every folder = %q, want untrusted", got)
	}
	if got := s.app.builtinFor(s.doc, builtin.FileSearch).State; got != builtin.Ask {
		t.Fatalf("a document in a trusted folder = %q, want ask", got)
	}
}

func TestBuiltinAllowAndRemove(t *testing.T) {
	s := newBuiltinStand(t)
	s.app.rt.cfg.SetBuiltinOn(builtin.FileSearch, true)
	s.app.rt.cfg.AllowBuiltin(builtin.FileSearch, s.doc)
	access := s.app.builtinFor(s.doc, builtin.FileSearch)
	if access.State != builtin.Ready {
		t.Fatalf("state = %q, want ready", access.State)
	}
	if got, want := real(t, access.Root), real(t, s.dir); got != want {
		t.Fatalf("Root = %q, want %q", got, want)
	}
	if refusal := s.app.runEffect(context.Background(), appcore.Effect{Type: "program.removeAccess", ProgramID: builtin.FileSearch, DocumentID: s.doc}); refusal != nil {
		t.Fatalf("removeAccess = %+v, want none", refusal)
	}
	if got := s.app.builtinFor(s.doc, builtin.FileSearch).State; got != builtin.Removed {
		t.Fatalf("state = %q, want removed", got)
	}
	if refusal := s.app.runEffect(context.Background(), appcore.Effect{Type: "program.askAgain", ProgramID: builtin.FileSearch, DocumentID: s.doc}); refusal != nil {
		t.Fatalf("askAgain = %+v, want none", refusal)
	}
	if got := s.app.builtinFor(s.doc, builtin.FileSearch).State; got != builtin.Ask {
		t.Fatalf("state = %q, want ask", got)
	}
}

func TestBuiltinRemoveAllTakesEveryDocumentAndAnEmptyDocumentTakesNothing(t *testing.T) {
	s := newBuiltinStand(t)
	s.app.rt.cfg.SetBuiltinOn(builtin.FileSearch, true)
	s.app.rt.cfg.AllowBuiltin(builtin.FileSearch, s.doc)
	s.app.rt.cfg.SetBuiltinAllTrusted(builtin.FileSearch, true)
	refusal := s.app.runEffect(context.Background(), appcore.Effect{Type: "program.removeAccess", ProgramID: builtin.FileSearch})
	if refusal == nil || refusal.Code != appcore.RefuseBadArguments {
		t.Fatalf("remove access with no document = %+v, want bad-arguments", refusal)
	}
	if got := s.app.builtinFor(s.doc, builtin.FileSearch).State; got != builtin.Ready {
		t.Fatalf("a remove with no document changed the document to %q", got)
	}
	if refusal := s.app.runEffect(context.Background(), appcore.Effect{Type: "program.removeAllAccess", ProgramID: builtin.FileSearch}); refusal != nil {
		t.Fatalf("removeAllAccess = %+v, want none", refusal)
	}
	if got := s.app.builtinFor(s.doc, builtin.FileSearch).State; got != builtin.Removed {
		t.Fatalf("state after remove all = %q, want removed", got)
	}
	if s.app.rt.cfg.BuiltinAllTrusted(builtin.FileSearch) {
		t.Fatal("remove all left All documents in my trusted folders on")
	}
}

func TestBuiltinRemoveAllChangesNothingWhenTheSettingsCannotSave(t *testing.T) {
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("a read-only folder stops neither root nor Windows")
	}
	cfgBase := t.TempDir()
	a := newTestAppWithConfigDir(t, t.TempDir(), cfgBase)
	a.rt.cfg.SetBuiltinOn(builtin.FileSearch, true)
	a.rt.cfg.AllowBuiltin(builtin.FileSearch, "/docs/a.htmlclay")
	a.rt.cfg.SetBuiltinAllTrusted(builtin.FileSearch, true)
	if err := a.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
	dir := config.DirFrom(cfgBase)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	refusal := a.runEffect(context.Background(), appcore.Effect{Type: "program.removeAllAccess", ProgramID: builtin.FileSearch})
	if refusal == nil || refusal.Code != appcore.RefuseFailed {
		t.Fatalf("remove all with a settings file that cannot save = %+v, want failed", refusal)
	}
	if !a.rt.cfg.BuiltinAllowed(builtin.FileSearch, "/docs/a.htmlclay") || !a.rt.cfg.BuiltinAllTrusted(builtin.FileSearch) {
		t.Fatal("a remove the settings file could not save still took effect in memory")
	}
}

func TestBuiltinAllTrustedSkipsTheQuestion(t *testing.T) {
	s := newBuiltinStand(t)
	s.app.rt.cfg.SetBuiltinOn(builtin.FileSearch, true)
	if refusal := s.app.runEffect(context.Background(), appcore.Effect{Type: "program.setAllTrusted", ProgramID: builtin.FileSearch, On: true}); refusal != nil {
		t.Fatalf("setAllTrusted = %+v, want none", refusal)
	}
	if got := s.app.builtinFor(s.doc, builtin.FileSearch).State; got != builtin.Ready {
		t.Fatalf("state = %q, want ready", got)
	}
}

func TestBuiltinRowsComeFirst(t *testing.T) {
	s := newBuiltinStand(t)
	rows := s.app.programFacts()
	if len(rows) < 3 {
		t.Fatalf("programs = %d, want the three built-in rows at least", len(rows))
	}
	want := []string{builtin.FileSearch, builtin.FileConvert, builtin.FolderWatch}
	for i, id := range want {
		if rows[i].ID != id {
			t.Fatalf("row %d = %q, want %q", i, rows[i].ID, id)
		}
		if !rows[i].Builtin {
			t.Fatalf("row %d (%s) is not marked built-in", i, id)
		}
	}
}

func TestOpenBuiltinExampleAllowsIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	s := newBuiltinStandAt(t, real(t, home))
	opened := ""
	s.app.rt.launchBrowser = func(target string) error {
		opened = target
		return nil
	}
	s.app.rt.cfg.SetBuiltinOn(builtin.FileConvert, true)
	if refusal := s.app.runEffect(context.Background(), appcore.Effect{Type: "program.openExample", ProgramID: builtin.FileConvert}); refusal != nil {
		t.Fatalf("openExample = %+v, want none", refusal)
	}
	dir := filepath.Join(home, "htmlclay", "examples")
	for _, name := range []string{"Image Converter.htmlclay", sampleImageName} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if opened == "" {
		t.Fatal("the example was not opened")
	}
	if got := s.app.builtinFor(filepath.Join(dir, "Image Converter.htmlclay"), builtin.FileConvert).State; got != builtin.Ready {
		t.Fatalf("the example's state = %q, want ready", got)
	}
}

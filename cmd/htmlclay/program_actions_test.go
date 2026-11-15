package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// m3ProgramDo sends one program action the way a shell sends it and answers why
// nothing happened, or nil.
func m3ProgramDo(t *testing.T, a *app, ch Channel, typ string, args map[string]any) *appcore.Refusal {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	_, refusal := a.do(context.Background(), appcore.Action{Type: typ, Args: raw}, ch)
	return refusal
}

// m3ProgramAction is one program action as the Mac app's sheet sends it.
func m3ProgramAction(t *testing.T, typ string, args map[string]any) appcore.Action {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return appcore.Action{Type: typ, Args: raw}
}

// m3ProgramSite is the live site the document is open in and its save token.
func m3ProgramSite(t *testing.T, a *app, document string) (*site, string) {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, candidate := range a.sites {
		if registration, ok := candidate.sessions.LookupByPath(document); ok {
			return candidate, registration.Token
		}
	}
	t.Fatalf("%s is open in no site", document)
	return nil, ""
}

// m3ProgramRun sends the program request a document's page sends and reports
// whether a process started, so the marker is the answer. Each request carries
// its own id: the dispatcher remembers the outcome of an id it has answered.
func m3ProgramRun(t *testing.T, a *app, document, id, marker string) bool {
	t.Helper()
	site, token := m3ProgramSite(t, a, document)
	wireHelperRequest(t, site, token, id, document)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// m3ProgramInHome writes a runnable program file in a folder of the test's home,
// which no risky-folder rule covers.
func m3ProgramInHome(t *testing.T, a *app, name string, mode os.FileMode) string {
	t.Helper()
	dir := filepath.Join(a.rt.home, "apps", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return writeTestProgram(t, dir, name, mode)
}

// blockConfigDir makes every Save fail, without changing anything in memory: the
// config directory's path becomes a regular file, so it cannot be created.
func blockConfigDir(t *testing.T, cfgBase string) {
	t.Helper()
	if err := os.RemoveAll(config.DirFrom(cfgBase)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.DirFrom(cfgBase), []byte("blocks the config directory"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestM3ProgramActions is step 7.10's host half: the Settings pane's program
// actions really change what runs, a grant the dashboard sends is refused, a file
// that changed since the sheet is never registered, and a failed save leaves the
// permissions in memory as they were.
func TestM3ProgramActions(t *testing.T) {
	t.Run("remove access empties the next resolve and stops a connected helper", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		f.allowForAnyDocument(t)
		f.attach(t)
		if !m3ProgramRun(t, f.app, f.document, "m3-program-before", f.marker) {
			t.Fatal("the broad grant did not start the program before the removal")
		}
		if err := os.Remove(f.marker); err != nil {
			t.Fatal(err)
		}
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "program.removeAccess", map[string]any{
			"programID": f.program.ID, "documentID": f.document,
		}); refusal != nil {
			t.Fatalf("program.removeAccess = %+v", refusal)
		}
		resolution, _ := f.app.rt.cfg.ResolveHelper(f.document, "search", true, f.app.scopeOf(f.document))
		if !resolution.Decided || resolution.Allowed {
			t.Fatalf("the next resolve = %+v, want a decided deny", resolution)
		}
		// The dispatcher that attached before the removal learns the denial, so the
		// page is told the helper is not granted instead of being started and
		// stopped at the last moment.
		site, token := m3ProgramSite(t, f.app, f.document)
		terminal := attachmentRequestTerminal(t, site, f.document, token, "m3-program-after")
		if terminal.Type != "wire/error" || terminal.Payload.Code != "helper_not_granted" {
			t.Fatalf("the page was told %s/%s, want wire/error helper_not_granted", terminal.Type, terminal.Payload.Code)
		}
		if _, err := os.Stat(f.marker); err == nil {
			t.Fatal("a document whose access was removed still started the program")
		}
	})

	t.Run("a removal in a team folder refuses at once", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		f.markTeam(t, "acme", "0123456789abcdef")
		f.requireTeam(t, "team:acme")
		f.allowDocument(t, f.scope(t))
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "program.removeAccess", map[string]any{
			"programID": f.program.ID, "documentID": f.document,
		}); refusal != nil {
			t.Fatalf("program.removeAccess = %+v", refusal)
		}
		resolution, _ := f.app.rt.cfg.ResolveHelper(f.document, "search", f.app.trustedLive(f.document), f.app.scopeOf(f.document))
		if !resolution.Decided || !resolution.Revoked || resolution.Allowed {
			t.Fatalf("the next resolve in the team folder = %+v, want a decided deny", resolution)
		}
	})

	t.Run("all trusted on never reaches a team folder", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		f.markTeam(t, "acme", "0123456789abcdef")
		f.requireTeam(t, "team:acme")
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "program.setAllTrusted", map[string]any{
			"programID": f.program.ID, "on": true,
		}); refusal != nil {
			t.Fatalf("program.setAllTrusted = %+v", refusal)
		}
		if program, _ := f.app.rt.cfg.LookupHelperProgram(f.program.ID); !program.AnyDocument {
			t.Fatal("the action did not widen the program")
		}
		f.attach(t)
		if m3ProgramRun(t, f.app, f.document, "m3-program-team", f.marker) {
			t.Fatal("the broad grant started the program in a team folder")
		}
	})

	t.Run("relink to a risky file with nothing acknowledged is refused", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		f.allowDocument(t, config.Scope{Kind: config.ScopeLocal})
		downloads := filepath.Join(f.app.rt.home, "Downloads")
		if err := os.MkdirAll(downloads, 0o755); err != nil {
			t.Fatal(err)
		}
		risky := writeTestProgram(t, downloads, "search", 0o755)
		before, _ := f.app.rt.cfg.LookupHelperProgram(f.program.ID)
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "program.relink", map[string]any{
			"programID": f.program.ID, "path": risky, "acknowledged": []string{},
		}); refusal == nil || refusal.Code != appcore.RefuseNeedsConfirmation {
			t.Fatalf("an unacknowledged relink = %+v, want needs-confirmation", refusal)
		}
		after, _ := f.app.rt.cfg.LookupHelperProgram(f.program.ID)
		if after.Path != before.Path || after.ID != before.ID {
			t.Fatalf("the refused relink stored %s, want %s", after.Path, before.Path)
		}
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "program.relink", map[string]any{
			"programID": f.program.ID, "path": risky, "acknowledged": []string{helperDetailInDownloads},
		}); refusal != nil {
			t.Fatalf("an acknowledged relink = %+v", refusal)
		}
		relinked, _ := f.app.rt.cfg.LookupHelperProgram(f.program.ID)
		if relinked.Path != risky || relinked.ID != f.program.ID {
			t.Fatalf("the relinked program = %+v, want %s at %s", relinked, f.program.ID, risky)
		}
		resolution, _ := f.app.rt.cfg.ResolveHelper(f.document, "search", true, f.app.scopeOf(f.document))
		if !resolution.Allowed || resolution.Program.ID != f.program.ID {
			t.Fatalf("the relink lost the document's decision: %+v", resolution)
		}
	})

	t.Run("add makes a file executable only when the sheet said so", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows has no executable bit: a program there runs by its extension, so this question is never asked")
		}
		f := newM3ProgramFixture(t)
		plain := m3ProgramInHome(t, f.app, "second", 0o644)
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "program.add", map[string]any{
			"name": "second", "path": plain, "acknowledged": []string{},
		}); refusal == nil || refusal.Code != appcore.RefuseNeedsConfirmation {
			t.Fatalf("an unacknowledged add = %+v, want needs-confirmation", refusal)
		}
		if info, err := os.Stat(plain); err != nil || info.Mode()&0o111 != 0 {
			t.Fatalf("the refused add changed the file's permissions: %v %v", info, err)
		}
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "program.add", map[string]any{
			"name": "second", "path": plain, "acknowledged": []string{helperDetailNotExecutable},
		}); refusal != nil {
			t.Fatalf("an acknowledged add = %+v", refusal)
		}
		if info, err := os.Stat(plain); err != nil || info.Mode()&0o111 == 0 {
			t.Fatalf("the acknowledged add did not make the file executable: %v %v", info, err)
		}
		found := false
		for _, program := range f.app.rt.cfg.HelperProgramList() {
			if program.Name == "second" && program.Path == plain {
				found = true
			}
		}
		if !found {
			t.Fatalf("the added program is not registered: %+v", f.app.rt.cfg.HelperProgramList())
		}
	})

	t.Run("ask again raises the question for that document", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		f.allowDocument(t, config.Scope{Kind: config.ScopeLocal})
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "program.removeAccess", map[string]any{
			"programID": f.program.ID, "documentID": f.document,
		}); refusal != nil {
			t.Fatalf("program.removeAccess = %+v", refusal)
		}
		var asked int32
		f.app.rt.chooseHelperSetup = func(string, string, string, string, string) (platform.Choice, error) {
			atomic.AddInt32(&asked, 1)
			return platform.ChoiceAffirm, nil
		}
		f.app.rt.prepareHelperProgram = func(string) (bool, error) { return true, nil }
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "program.askAgain", map[string]any{
			"programID": f.program.ID, "documentID": f.document,
		}); refusal != nil {
			t.Fatalf("program.askAgain = %+v", refusal)
		}
		if n := atomic.LoadInt32(&asked); n != 1 {
			t.Fatalf("ask again raised %d questions, want one", n)
		}
		resolution, _ := f.app.rt.cfg.ResolveHelper(f.document, "search", true, f.app.scopeOf(f.document))
		if !resolution.Allowed || resolution.Program.ID != f.program.ID {
			t.Fatalf("the answer to ask again = %+v, want the program allowed", resolution)
		}
	})

	t.Run("the dashboard may revoke but not grant", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.trust(t)
		f.allowForAnyDocument(t)
		clean := m3ProgramInHome(t, f.app, "second", 0o755)
		grants := []struct {
			typ  string
			args map[string]any
		}{
			{"program.add", map[string]any{"name": "second", "path": clean, "acknowledged": []string{}}},
			{"program.relink", map[string]any{"programID": f.program.ID, "path": clean, "acknowledged": []string{}}},
			{"program.askAgain", map[string]any{"programID": f.program.ID, "documentID": f.document}},
			{"program.setAllTrusted", map[string]any{"programID": f.program.ID, "on": true}},
		}
		for _, grant := range grants {
			if refusal := m3ProgramDo(t, f.app, ChannelDashboard, grant.typ, grant.args); refusal == nil || refusal.Code != appcore.RefuseNeedsConfirmation {
				t.Fatalf("%s from the dashboard = %+v, want needs-confirmation", grant.typ, refusal)
			}
		}
		if programs := f.app.rt.cfg.HelperProgramList(); len(programs) != 1 {
			t.Fatalf("the refused grants registered %+v", programs)
		}
		if program, _ := f.app.rt.cfg.LookupHelperProgram(f.program.ID); program.Path != f.program.Path {
			t.Fatalf("a refused relink moved the program to %s", program.Path)
		}
		if refusal := m3ProgramDo(t, f.app, ChannelDashboard, "program.setAllTrusted", map[string]any{
			"programID": f.program.ID, "on": false,
		}); refusal != nil {
			t.Fatalf("turning the broad grant off from the dashboard = %+v", refusal)
		}
		if program, _ := f.app.rt.cfg.LookupHelperProgram(f.program.ID); program.AnyDocument {
			t.Fatal("the dashboard could not turn the broad grant off")
		}
		if refusal := m3ProgramDo(t, f.app, ChannelDashboard, "program.removeAccess", map[string]any{
			"programID": f.program.ID, "documentID": f.document,
		}); refusal != nil {
			t.Fatalf("removing access from the dashboard = %+v", refusal)
		}
		resolution, _ := f.app.rt.cfg.ResolveHelper(f.document, "search", true, f.app.scopeOf(f.document))
		if !resolution.Decided || resolution.Allowed {
			t.Fatalf("the dashboard's removal = %+v, want a decided deny", resolution)
		}
	})

	t.Run("a failed config save changes nothing", func(t *testing.T) {
		cases := []struct {
			name string
			typ  string
			args func(t *testing.T, f *m3ProgramFixture) map[string]any
			gone bool
		}{
			{"add", "program.add", func(t *testing.T, f *m3ProgramFixture) map[string]any {
				return map[string]any{"name": "second", "path": m3ProgramInHome(t, f.app, "second", 0o755), "acknowledged": []string{}}
			}, false},
			{"relink", "program.relink", func(t *testing.T, f *m3ProgramFixture) map[string]any {
				return map[string]any{"programID": f.program.ID, "path": m3ProgramInHome(t, f.app, "search", 0o755), "acknowledged": []string{}}
			}, false},
			{"remove access", "program.removeAccess", func(t *testing.T, f *m3ProgramFixture) map[string]any {
				return map[string]any{"programID": f.program.ID, "documentID": f.document}
			}, false},
			{"all trusted", "program.setAllTrusted", func(t *testing.T, f *m3ProgramFixture) map[string]any {
				return map[string]any{"programID": f.program.ID, "on": true}
			}, false},
			{"remove", "program.remove", func(t *testing.T, f *m3ProgramFixture) map[string]any {
				return map[string]any{"programID": f.program.ID}
			}, true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				f := newM3ProgramFixture(t)
				f.trust(t)
				f.allowDocument(t, config.Scope{Kind: config.ScopeLocal})
				args := tc.args(t, f)
				if tc.gone {
					if err := os.Remove(f.program.Path); err != nil {
						t.Fatal(err)
					}
				}
				beforePrograms := f.app.rt.cfg.HelperProgramList()
				beforeDecisions := f.app.rt.cfg.HelperDecisionList()
				blockConfigDir(t, f.cfgBase)
				if refusal := m3ProgramDo(t, f.app, ChannelShell, tc.typ, args); refusal == nil || refusal.Code != appcore.RefuseFailed {
					t.Fatalf("%s with an unwritable config = %+v, want failed", tc.typ, refusal)
				}
				if got := f.app.rt.cfg.HelperProgramList(); !equalHelperPrograms(got, beforePrograms) {
					t.Fatalf("%s left the programs as %+v, want %+v", tc.typ, got, beforePrograms)
				}
				if got := f.app.rt.cfg.HelperDecisionList(); !equalHelperDecisions(got, beforeDecisions) {
					t.Fatalf("%s left the decisions as %+v, want %+v", tc.typ, got, beforeDecisions)
				}
			})
		}
	})

	t.Run("the sheets show the codes the effect checks again", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		downloads := filepath.Join(f.app.rt.home, "Downloads")
		if err := os.MkdirAll(downloads, 0o755); err != nil {
			t.Fatal(err)
		}
		risky := writeTestProgram(t, downloads, "search", 0o644)
		preview, refusal := shellBackend{f.app}.Check(m3ProgramAction(t, "program.relink", map[string]any{
			"programID": f.program.ID, "path": risky,
		}))
		if refusal != nil {
			t.Fatalf("the relink sheet = %+v", refusal)
		}
		var codes []string
		for _, warning := range preview.Warnings {
			codes = append(codes, warning.Code)
		}
		want := []string{helperDetailInDownloads, helperDetailNotExecutable}
		if runtime.GOOS == "windows" {
			// No executable bit there: a program runs by its extension.
			want = []string{helperDetailInDownloads}
		}
		if !slices.Equal(codes, want) {
			t.Fatalf("relink warnings = %v, want %v", codes, want)
		}
		if preview.Path != risky {
			t.Fatalf("the sheet shows %q, want the file %q", preview.Path, risky)
		}
		clean := m3ProgramInHome(t, f.app, "second", 0o755)
		preview, refusal = shellBackend{f.app}.Check(m3ProgramAction(t, "program.add", map[string]any{
			"name": "second", "path": clean,
		}))
		if refusal != nil || len(preview.Warnings) != 0 || preview.Path != clean {
			t.Fatalf("the add sheet = %+v, %+v, want no warnings for %s", preview, refusal, clean)
		}
		_, refusal = shellBackend{f.app}.Check(m3ProgramAction(t, "program.add", map[string]any{
			"name": "second", "path": filepath.Join(downloads, "gone"),
		}))
		if refusal == nil || refusal.Code != appcore.RefuseFileMissing {
			t.Fatalf("the sheet for a file that is not there = %+v, want file-missing", refusal)
		}
	})

	t.Run("the facts list who may run it", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		f.allowDocument(t, config.Scope{Kind: config.ScopeLocal})
		other := filepath.Join(f.folder, "other.htmlclay")
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "program.removeAccess", map[string]any{
			"programID": f.program.ID, "documentID": other,
		}); refusal != nil {
			t.Fatalf("program.removeAccess for a document that is not open = %+v", refusal)
		}
		var program appcore.Program
		for _, candidate := range f.app.facts().Programs {
			if candidate.ID == f.program.ID {
				program = candidate
			}
		}
		if !slices.Contains(program.AllowedDocuments, f.document) {
			t.Fatalf("allowed documents = %v, want %s", program.AllowedDocuments, f.document)
		}
		if !slices.Contains(program.RemovedDocuments, other) {
			t.Fatalf("removed documents = %v, want %s", program.RemovedDocuments, other)
		}
		if program.FileMissing || program.AllTrustedFolders {
			t.Fatalf("the facts report %+v, want the file present and no broad grant", program)
		}
	})

	t.Run("reveal shows the program's file", func(t *testing.T) {
		f := newM3ProgramFixture(t)
		var shown string
		f.app.rt.reveal = func(path string) error {
			shown = path
			return nil
		}
		if refusal := m3ProgramDo(t, f.app, ChannelShell, "program.reveal", map[string]any{
			"programID": f.program.ID,
		}); refusal != nil {
			t.Fatalf("program.reveal = %+v", refusal)
		}
		if shown != f.program.Path {
			t.Fatalf("revealed %q, want %q", shown, f.program.Path)
		}
	})
}

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// settledSetup makes the setup dialog judge the files a test just wrote as
// having sat unchanged long enough to offer, and moves the risky folders inside
// the test's home, since the real temp folder holds every test's files.
func settledSetup(t *testing.T) {
	t.Helper()
	previousNow, previousRisky := helperSuggestNow, helperSuggestRiskyFolders
	helperSuggestNow = func() time.Time { return time.Now().Add(time.Hour) }
	helperSuggestRiskyFolders = func(home string) []string {
		return []string{filepath.Join(home, "Downloads"), filepath.Join(home, "tmp")}
	}
	t.Cleanup(func() { helperSuggestNow, helperSuggestRiskyFolders = previousNow, previousRisky })
}

// setupRecorder answers setup dialogs from answers and records every title,
// message and button set it was asked with.
type setupRecorder struct {
	answers  []platform.Choice
	titles   []string
	messages []string
	buttons  [][3]string
}

func (r *setupRecorder) dialogs(t *testing.T) helperApprovalDialogs {
	t.Helper()
	return helperApprovalDialogs{
		setup: func(title, message, refuse, middle, affirm string) (platform.Choice, error) {
			r.titles = append(r.titles, title)
			r.messages = append(r.messages, message)
			r.buttons = append(r.buttons, [3]string{refuse, middle, affirm})
			if len(r.answers) == 0 {
				return platform.ChoiceNone, nil
			}
			answer := r.answers[0]
			r.answers = r.answers[1:]
			return answer, nil
		},
		choose: func(string, string) (string, bool, error) {
			t.Error("the setup dialog opened the picker")
			return "", false, nil
		},
		prepare: func(string) (bool, error) { return true, nil },
	}
}

// A file HTML Clay found leads the dialog: the buttons name it, the message
// shows where it is and the name the page uses, and no picker is needed.
func TestHelperSetupOffersFoundFile(t *testing.T) {
	a, home := newSuggestApp(t)
	settledSetup(t)
	bin := filepath.Join(home, "apps", "bin")
	addSuggestProgram(t, a, "other", writeSuggestFile(t, filepath.Join(bin, "other"), 0o755))
	found := writeSuggestFile(t, filepath.Join(bin, "clay-search"), 0o755)
	doc := writeSuggestDocument(t, filepath.Join(home, "apps", "search", "search.htmlclay"),
		`<meta name="htmlclay-helper" content="search" data-program-folder="~/apps/bin" data-program-file="clay-search">`)

	recorder := &setupRecorder{answers: []platform.Choice{platform.ChoiceAffirm}}
	dialogs := recorder.dialogs(t)
	chose := false
	dialogs.choose = func(string, string) (string, bool, error) {
		chose = true
		return "", false, nil
	}
	plan := a.helpersForUseWith(doc, "search", dialogs)

	if chose {
		t.Fatal("a found file still opened the picker")
	}
	if len(recorder.buttons) != 1 {
		t.Fatalf("setup dialogs = %d, want one", len(recorder.buttons))
	}
	if got, want := recorder.titles[0], "Allow a program for search.htmlclay?"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
	if got, want := recorder.buttons[0], [3]string{"Deny", "More Options\u2026", "Allow " + filepath.Base(found)}; got != want {
		t.Fatalf("buttons = %q, want %q", got, want)
	}
	for _, want := range []string{filepath.Join("~", "apps", "bin", filepath.Base(found)), "\u201csearch\u201d"} {
		if !strings.Contains(recorder.messages[0], want) {
			t.Errorf("the dialog does not state %q:\n%s", want, recorder.messages[0])
		}
	}
	if _, ok := plan.allowed["search"]; !ok {
		t.Fatalf("allowed plan = %+v, want search", plan.allowed)
	}
	resolution, _ := a.rt.cfg.ResolveHelper(doc, "search", false)
	if !resolution.Allowed || resolution.Program.Path != found {
		t.Fatalf("resolution = %+v, want the found file %s", resolution, found)
	}
}

// The person's bug: a program registered under one name, widened to every
// trusted document from a document that asks for another name, has to become a
// registration under the name the documents ask for.
func TestHelperSetupTrustedFoldersRegistersUnderRequestedName(t *testing.T) {
	home, err := resolveSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := buildTestApp(t, home, t.TempDir())
	folder := filepath.Join(home, "docs")
	document := filepath.Join(folder, "one.htmlclay")
	writeTestFile(t, document, `<!doctype html><head><meta name="htmlclay-helper" content="search" data-program-file="clay-search"></head>`)
	other := filepath.Join(folder, "two.htmlclay")
	writeTestFile(t, other, `<!doctype html><head><meta name="htmlclay-helper" content="search" data-program-file="clay-search"></head>`)
	path := writeSuggestFile(t, filepath.Join(home, "apps", "bin", "clay-search"), 0o755)
	registered, err := a.rt.cfg.AddHelperProgram("clay-search", path)
	if err != nil {
		t.Fatal(err)
	}
	a.rt.cfg.SetHelperAnyDocument(registered.ID, true)
	a.rt.cfg.AddTrustedFolder(folder, platform.DirIdentity(folder))

	if before, _ := a.rt.cfg.ResolveHelper(other, "search", true); before.Allowed {
		t.Fatalf("the requested name resolved before it was set up: %+v", before)
	}

	recorder := &setupRecorder{answers: []platform.Choice{platform.ChoiceMiddle, platform.ChoiceAffirm}}
	a.helpersForUseWith(document, "search", recorder.dialogs(t))

	var declared []config.HelperProgram
	for _, program := range a.rt.cfg.HelperProgramList() {
		if program.Name == "search" {
			declared = append(declared, program)
		}
	}
	if len(declared) != 1 || declared[0].Path != path || !declared[0].AnyDocument || declared[0].ID == registered.ID {
		t.Fatalf("registrations named search = %+v, want one any-document registration at %s", declared, path)
	}
	after, _ := a.rt.cfg.ResolveHelper(other, "search", true)
	if !after.Allowed || after.Program.ID != declared[0].ID {
		t.Fatalf("the other document resolves to %+v, want %s", after, declared[0].ID)
	}

	quiet := helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) {
			t.Error("a document covered by the wider grant raised the dialog")
			return platform.ChoiceNone, nil
		},
	}
	if plan := a.helpersForUseWith(other, "search", quiet); len(plan.allowed) == 0 {
		t.Fatalf("the other document is not allowed: %+v", plan)
	}
}

// A Deny lasts this open: the page cannot raise the dialog again until a direct
// open clears it.
func TestHelperSetupDenyBlocksPageUntilReopen(t *testing.T) {
	home := t.TempDir()
	a := newTestApp(t, home)
	document := writeHelperPage(t, home, "search")
	if _, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0755)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	dialogs := helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) {
			calls++
			return platform.ChoiceRefuse, nil
		},
	}

	if plan := a.helpersForUseWith(document, "search", dialogs); !slices.Contains(plan.denied, "search") {
		t.Fatalf("denied plan = %+v, want search denied", plan.denied)
	}
	if calls != 1 {
		t.Fatalf("dialog calls = %d, want 1", calls)
	}
	a.helpersForUseWith(document, "search", dialogs)
	if calls != 1 {
		t.Fatalf("the page raised the dialog past its own Deny: calls = %d", calls)
	}
	reopened := a.helpersForOpenWith(document, true, false, dialogs)
	if calls != 1 {
		t.Fatalf("a direct open raised the dialog: calls = %d", calls)
	}
	if len(reopened.denied) != 0 {
		t.Fatalf("the direct open kept the refusal: %+v", reopened.denied)
	}
	clearPageAskQuiet(t, a, document)
	a.helpersForUseWith(document, "search", dialogs)
	if calls != 2 {
		t.Fatalf("the page did not ask again after a direct open: calls = %d", calls)
	}
}

// Opening a document is not a question: an undecided name opens undecided and
// no dialog is shown.
func TestHelperOpenNeverPrompts(t *testing.T) {
	home := t.TempDir()
	a := newTestApp(t, home)
	document := writeHelperPage(t, home, "search")
	if _, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0755)); err != nil {
		t.Fatal(err)
	}
	plan := a.helpersForOpenWith(document, true, true, helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) {
			t.Error("opening a document raised the program dialog")
			return platform.ChoiceAffirm, nil
		},
		choose: func(string, string) (string, bool, error) {
			t.Error("opening a document opened the picker")
			return "", false, nil
		},
		prepare: func(string) (bool, error) {
			t.Error("opening a document prepared a program")
			return false, nil
		},
	})
	if len(plan.allowed) != 0 || len(plan.denied) != 0 {
		t.Fatalf("an undecided open produced a decision: allowed %+v, denied %+v", plan.allowed, plan.denied)
	}
}

// A name the person took away can be set up again from the page, and the answer
// is stored.
func TestHelperSetupAfterRevoke(t *testing.T) {
	home := t.TempDir()
	a := newTestApp(t, home)
	document := writeHelperPage(t, home, "search")
	program, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0755))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.rt.cfg.DecideHelper(config.HelperDecision{Document: document, Name: "search", Program: program.ID, Allowed: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.rt.cfg.RevokeHelper(document, "search"); err != nil {
		t.Fatal(err)
	}
	if revoked, _ := a.rt.cfg.ResolveHelper(document, "search", true); revoked.Allowed {
		t.Fatalf("a revoked name still resolved as allowed: %+v", revoked)
	}

	recorder := &setupRecorder{answers: []platform.Choice{platform.ChoiceAffirm}}
	plan := a.helpersForUseWith(document, "search", recorder.dialogs(t))
	if len(recorder.messages) != 1 {
		t.Fatalf("setup dialogs = %d, want one after a revocation", len(recorder.messages))
	}
	if _, ok := plan.allowed["search"]; !ok {
		t.Fatalf("allowed plan = %+v, want search", plan.allowed)
	}
	resolution, _ := a.rt.cfg.ResolveHelper(document, "search", true)
	if !resolution.Allowed || resolution.Program.ID != program.ID {
		t.Fatalf("resolution = %+v, want %s allowed again", resolution, program.ID)
	}
}

// An any-document registration covers only documents inside a trusted folder.
func TestAnyDocumentGrantSkipsUntrustedDocument(t *testing.T) {
	home, err := resolveSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := buildTestApp(t, home, t.TempDir())
	folder := filepath.Join(home, "notes")
	document := filepath.Join(folder, "page.htmlclay")
	writeTestFile(t, document, `<!doctype html><head><meta name="htmlclay-helper" content="search"></head>`)
	program, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0755))
	if err != nil {
		t.Fatal(err)
	}
	a.rt.cfg.SetHelperAnyDocument(program.ID, true)

	if allowed, _ := a.resolvedHelpers(document, []string{"search"}); len(allowed) != 0 {
		t.Fatalf("an untrusted document resolved from an any-document grant: %+v", allowed)
	}
	a.rt.cfg.AddTrustedFolder(folder, platform.DirIdentity(folder))
	allowed, _ := a.resolvedHelpers(document, []string{"search"})
	if got, ok := allowed["search"]; !ok || got.ID != program.ID {
		t.Fatalf("a trusted document resolves to %+v, want %s", allowed, program.ID)
	}
}

// A document that stops declaring the program while the dialog is open gets
// nothing stored.
func TestHelperSetupStoresNothingWhenTheDeclarationGoesAway(t *testing.T) {
	home := t.TempDir()
	a := newTestApp(t, home)
	document := writeHelperPage(t, home, "search")
	if _, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0755)); err != nil {
		t.Fatal(err)
	}
	dialogs := helperApprovalDialogs{
		setup: func(title, message, refuse, middle, affirm string) (platform.Choice, error) {
			if err := os.WriteFile(document, []byte("<!doctype html><html><head></head><body></body></html>"), 0o644); err != nil {
				t.Fatal(err)
			}
			return platform.ChoiceAffirm, nil
		},
		choose:  func(prompt, dir string) (string, bool, error) { t.Fatal("picker opened"); return "", false, nil },
		prepare: func(string) (bool, error) { return true, nil },
	}
	a.helpersForUseWith(document, "search", dialogs)
	for _, d := range a.rt.cfg.HelperDecisionList() {
		if d.Document == document {
			t.Fatalf("stored %+v for a document that no longer declares the program", d)
		}
	}
}

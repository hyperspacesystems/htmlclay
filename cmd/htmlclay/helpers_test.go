package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/session"
)

func writeHelperPage(t *testing.T, dir string, names ...string) string {
	t.Helper()
	var metas strings.Builder
	for _, name := range names {
		metas.WriteString("<meta name=\"htmlclay-helper\" content=\"" + name + "\">")
	}
	path := filepath.Join(dir, "page.htmlclay")
	if err := os.WriteFile(path, []byte("<!doctype html><head>"+metas.String()+"</head>"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// clearPageAskQuiet lifts the minute a page must wait after a dialog that
// granted nothing, so a test can prove the page may ask again without waiting.
func clearPageAskQuiet(t *testing.T, a *app, document string) {
	t.Helper()
	a.helperStateMu.Lock()
	defer a.helperStateMu.Unlock()
	delete(a.pageAskQuiet, document)
}

// A click on Deny is not remembered: it denies the helper for the rest of this
// open, stores nothing, and a new open may ask again, including after a restart.
func TestHelperDenialAppliesToThisOpenOnly(t *testing.T) {
	home := t.TempDir()
	cfgBase := t.TempDir()
	a := newTestAppWithConfigDir(t, home, cfgBase)
	document := writeHelperPage(t, home, "search", "ocr")
	for _, name := range []string{"search", "ocr"} {
		if _, err := a.rt.cfg.AddHelperProgram(name, writeTestProgram(t, t.TempDir(), name, 0755)); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
	calls := 0
	var message string
	dialogs := helperApprovalDialogs{
		setup: func(_, text, _, _, _ string) (platform.Choice, error) {
			calls++
			message = text
			return platform.ChoiceRefuse, nil
		},
		choose: func(string, string) (string, bool, error) {
			t.Fatal("denial opened a file picker")
			return "", false, nil
		},
		prepare: func(string) (bool, error) {
			t.Fatal("denial prepared a program")
			return false, nil
		},
	}
	plan := a.helpersForUseWith(document, "search", dialogs)
	if len(plan.allowed) != 0 || !slices.Contains(plan.denied, "search") {
		t.Fatalf("denied plan = allowed %+v, denied %+v, names %+v", plan.allowed, plan.denied, plan.names)
	}
	if !strings.Contains(message, "\u201csearch\u201d") {
		t.Fatalf("the dialog did not quote the name the page asks for: %q", message)
	}
	stopped := a.helpersForUseWith(document, "ocr", dialogs)
	if calls != 1 {
		t.Fatalf("dialog calls = %d, want a refusal to stop every page-raised dialog for the document", calls)
	}
	if len(stopped.allowed) != 0 || strings.Join(stopped.denied, ",") != "search" {
		t.Fatalf("the stopped plan = allowed %+v, denied %+v", stopped.allowed, stopped.denied)
	}
	for _, decision := range a.rt.cfg.HelperDecisionList() {
		if decision.Document == document {
			t.Fatalf("a denial was stored: %+v", decision)
		}
	}

	restarted := buildTestApp(t, home, cfgBase)
	asked := 0
	restartedPlan := restarted.helpersForUseWith(document, "search", helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) {
			asked++
			return platform.ChoiceRefuse, nil
		},
	})
	if asked != 1 {
		t.Fatalf("a setup request after a restart must ask again, asked %d times", asked)
	}
	if len(restartedPlan.allowed) != 0 || strings.Join(restartedPlan.denied, ",") != "search" {
		t.Fatalf("restarted denied plan = allowed %+v, denied %+v", restartedPlan.allowed, restartedPlan.denied)
	}
}

// Return, Escape, the close box, the timeout, a missing dialog tool and every
// error come back as no answer. None of them may be stored: the name stays
// undecided, and a later open asks again, including after a restart.
func TestHelperPromptClosedWithoutAnAnswerStoresNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"dismissed", nil},
		{"dialog failed", errors.New("no native dialog tool (zenity or kdialog) found")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			cfgBase := t.TempDir()
			a := newTestAppWithConfigDir(t, home, cfgBase)
			a.rt.notify = func(string, string) error { return nil }
			document := writeHelperPage(t, home, "search", "ocr")
			if _, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0755)); err != nil {
				t.Fatal(err)
			}
			calls := 0
			dialogs := helperApprovalDialogs{
				setup: func(string, string, string, string, string) (platform.Choice, error) {
					calls++
					return platform.ChoiceNone, tc.err
				},
				choose: func(string, string) (string, bool, error) {
					t.Fatal("a dialog closed without an answer opened the file picker")
					return "", false, nil
				},
				prepare: func(string) (bool, error) {
					t.Fatal("a dialog closed without an answer prepared a program")
					return false, nil
				},
			}
			plan := a.helpersForUseWith(document, "search", dialogs)
			if len(plan.allowed) != 0 || len(plan.denied) != 0 {
				t.Fatalf("no answer produced a decision: allowed %+v, denied %+v", plan.allowed, plan.denied)
			}
			if calls != 1 {
				t.Fatalf("dialog calls = %d, want 1", calls)
			}
			if got := a.rt.cfg.HelperDecisionList(); len(got) != 0 {
				t.Fatalf("no answer was stored: %+v", got)
			}
			restarted := buildTestApp(t, home, cfgBase)
			restarted.helpersForUseWith(document, "search", dialogs)
			if calls != 2 {
				t.Fatalf("after a restart the document must ask again, calls = %d", calls)
			}
		})
	}
}

// A program reached through a different path is the program that was
// registered. The registry keeps the path as selected, and a package manager
// often replaces the target behind that path, so picking the real file rather
// than the link must reuse the registration instead of adding a second one.
func TestRegisteredAtPathFollowsALinkToTheSameFile(t *testing.T) {
	dir := t.TempDir()
	real := writeTestProgram(t, dir, "search", 0755)
	link := filepath.Join(t.TempDir(), "search")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	program := config.HelperProgram{ID: "abc", Name: "search", Path: real}
	for _, path := range []string{real, link} {
		got, ok := registeredAtPath([]config.HelperProgram{program}, path)
		if !ok || got != program {
			t.Errorf("registeredAtPath(%q) = (%+v, %v), want the registration", path, got, ok)
		}
	}
	if got, ok := registeredAtPath([]config.HelperProgram{program}, filepath.Join(dir, "other")); ok {
		t.Errorf("an unrelated path returned %+v", got)
	}
}

// Several registrations under one name used to resolve to the earliest one
// without asking. Now HTML Clay offers no file it cannot choose, so an Allow
// opens the picker, and a file that is already registered under the name is
// reused rather than registered again.
func TestHelperApprovalWithASharedNameAsksWhichProgram(t *testing.T) {
	for _, tc := range []struct {
		name      string
		pick      int
		wantAdded bool
	}{
		{"picks the second registration", 1, false},
		{"picks a new file", 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			a := newTestApp(t, home)
			document := writeHelperPage(t, home, "search")
			document, err := resolveSymlinks(document)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			paths := []string{writeTestProgram(t, dir, "first", 0755), writeTestProgram(t, dir, "second", 0755), writeTestProgram(t, dir, "third", 0755)}
			var registered []config.HelperProgram
			for _, path := range paths[:2] {
				program, err := a.rt.cfg.AddHelperProgram("search", path)
				if err != nil {
					t.Fatal(err)
				}
				registered = append(registered, program)
			}
			chose := false
			plan := a.helpersForUseWith(document, "search", helperApprovalDialogs{
				setup: func(string, string, string, string, string) (platform.Choice, error) {
					return platform.ChoiceAffirm, nil
				},
				choose: func(string, string) (string, bool, error) {
					chose = true
					return paths[tc.pick], true, nil
				},
				prepare: func(string) (bool, error) { return true, nil },
			})
			if !chose {
				t.Fatal("an Allow for a shared name did not ask which program")
			}
			got, ok := plan.allowed["search"]
			if !ok || got.Path != paths[tc.pick] {
				t.Fatalf("allowed = %+v, %v, want %s", got, ok, paths[tc.pick])
			}
			programs := a.rt.cfg.HelperProgramList()
			if tc.wantAdded != (len(programs) == 3) {
				t.Fatalf("registrations = %d, want a new one: %v", len(programs), tc.wantAdded)
			}
			if !tc.wantAdded && got.ID != registered[tc.pick].ID {
				t.Fatalf("the picked file was registered again instead of reused: %s, want %s", got.ID, registered[tc.pick].ID)
			}
		})
	}
}

func TestHelperApprovalSelectsThenPersistsTheProgram(t *testing.T) {
	settledSetup(t)
	home := t.TempDir()
	a := newTestApp(t, home)
	document := writeHelperPage(t, home, "search")
	programPath := writeTestProgram(t, t.TempDir(), "search", 0755)
	var order []string
	plan := a.helpersForUseWith(document, "search", helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) {
			order = append(order, "setup")
			if len(order) == 1 {
				return platform.ChoiceMiddle, nil
			}
			return platform.ChoiceAffirm, nil
		},
		choose: func(prompt, dir string) (string, bool, error) {
			order = append(order, "choose")
			if !strings.Contains(prompt, "search") {
				t.Fatalf("picker prompt = %q", prompt)
			}
			return programPath, true, nil
		},
		prepare: func(path string) (bool, error) {
			order = append(order, "prepare")
			if path != programPath {
				t.Fatalf("prepared %q, want %q", path, programPath)
			}
			return true, nil
		},
	})
	if strings.Join(order, ",") != "setup,setup,choose,prepare" {
		t.Fatalf("dialog order = %v", order)
	}
	program, ok := plan.allowed["search"]
	if !ok || program.Path != programPath || !program.AnyDocument {
		t.Fatalf("allowed helper = %+v, %v", program, ok)
	}
	resolution, _ := a.rt.cfg.ResolveHelper(document, "search", true)
	if !resolution.Decided || !resolution.Allowed || resolution.Program.ID != program.ID {
		t.Fatalf("saved resolution = %+v", resolution)
	}
}

func TestHelperDecisionSaveFailureRollsBackExactly(t *testing.T) {
	tests := []struct {
		name       string
		choice     platform.ConfirmChoice
		registered bool
	}{
		{"new registration", platform.ConfirmAllowOnce, false},
		{"broad flag", platform.ConfirmAllowAlways, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfgBase := t.TempDir()
			a := newTestAppWithConfigDir(t, t.TempDir(), cfgBase)
			path := writeTestProgram(t, t.TempDir(), "search", 0755)
			candidate := helperCandidate{name: "search", path: path}
			if tc.registered {
				program, err := a.rt.cfg.AddHelperProgram("search", path)
				if err != nil {
					t.Fatal(err)
				}
				candidate.program = program
			}
			beforePrograms := a.rt.cfg.HelperProgramList()
			beforeDecisions := a.rt.cfg.HelperDecisionList()
			if err := os.RemoveAll(config.DirFrom(cfgBase)); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(config.DirFrom(cfgBase), []byte("blocks config directory"), 0600); err != nil {
				t.Fatal(err)
			}
			err := a.saveHelperDecisionSet("/docs/page.htmlclay", []helperCandidate{candidate}, tc.choice)
			if err == nil {
				t.Fatal("save failure returned nil")
			}
			if got := a.rt.cfg.HelperProgramList(); !equalHelperPrograms(got, beforePrograms) {
				t.Fatalf("program rollback = %+v, want %+v", got, beforePrograms)
			}
			if got := a.rt.cfg.HelperDecisionList(); !equalHelperDecisions(got, beforeDecisions) {
				t.Fatalf("decision rollback = %+v, want %+v", got, beforeDecisions)
			}
		})
	}
}

func TestStoredHelperDecisionRestoresWithoutDialogs(t *testing.T) {
	home := t.TempDir()
	a := newTestApp(t, home)
	document := writeHelperPage(t, home, "search")
	document, err := resolveSymlinks(document)
	if err != nil {
		t.Fatal(err)
	}
	program, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0755))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.rt.cfg.DecideHelper(config.HelperDecision{
		Document: document, Name: "search", Program: program.ID, Allowed: true,
	}); err != nil {
		t.Fatal(err)
	}
	fail := errors.New("dialog called")
	plan := a.helpersForOpenWith(document, false, true, helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) {
			return platform.ChoiceRefuse, fail
		},
		choose: func(string, string) (string, bool, error) {
			return "", false, fail
		},
		prepare: func(string) (bool, error) {
			return false, fail
		},
	})
	if got := plan.allowed["search"]; got.ID != program.ID {
		t.Fatalf("restored helper = %+v, want %s", got, program.ID)
	}
}

func TestTrustedNavigationRestoresHelpersWithoutDialogs(t *testing.T) {
	for _, entry := range []string{"startup", "banner"} {
		for _, permission := range []struct {
			name        string
			anyDocument bool
			decided     bool
			allowed     bool
			state       string
		}{
			{"any document", true, true, true, "ready"},
			{"document allow", false, true, true, "ready"},
			{"undecided", false, false, false, "unavailable"},
		} {
			t.Run(entry+"/"+permission.name, func(t *testing.T) {
				home, err := resolveSymlinks(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				folder := filepath.Join(home, "search")
				document := filepath.Join(folder, "sol.htmlclay")
				writeTestFile(t, document, `<!doctype html><html><head><meta name="htmlclay-helper" content="search"></head></html>`)
				cfgBase := t.TempDir()
				first := newTestAppWithConfigDir(t, home, cfgBase)
				program, err := first.rt.cfg.AddHelperProgram("search", writeTestProgram(t, home, "clay-search", 0755))
				if err != nil {
					t.Fatal(err)
				}
				first.rt.cfg.SetHelperAnyDocument(program.ID, permission.anyDocument)
				if permission.decided {
					decision := config.HelperDecision{Document: document, Name: "search", Allowed: permission.allowed}
					if permission.allowed {
						decision.Program = program.ID
						if permission.anyDocument {
							decision.Document = filepath.Join(folder, "astra.htmlclay")
						}
					}
					if _, _, err := first.rt.cfg.DecideHelper(decision); err != nil {
						t.Fatal(err)
					}
				}
				if entry == "startup" {
					first.rt.cfg.AddTrustedFolder(folder, platform.DirIdentity(folder))
				}
				if err := first.rt.cfg.Save(); err != nil {
					t.Fatal(err)
				}
				first.shutdown()
				a := newTestAppWithConfigDir(t, home, cfgBase)
				a.helperMu.Lock()
				release := sync.OnceFunc(a.helperMu.Unlock)
				defer release()
				timer := time.AfterFunc(5*time.Second, release)
				defer timer.Stop()
				a.rt.chooseHelperSetup = func(string, string, string, string, string) (platform.Choice, error) {
					t.Error("browser navigation raised a helper permission dialog")
					return platform.ChoiceNone, nil
				}
				resolution, _ := a.rt.cfg.ResolveHelper(document, "search", true)
				if resolution.Decided != permission.decided || resolution.Allowed != permission.allowed {
					t.Fatalf("stored resolution = %+v", resolution)
				}
				var s *site
				if entry == "startup" {
					a.startSites()
					a.mu.Lock()
					s = a.siteAtLocked(folder)
					a.mu.Unlock()
					if s == nil {
						t.Fatal("startup did not bind the trusted folder")
					}
				} else {
					index := filepath.Join(folder, "index.htmlclay")
					writeTestFile(t, index, "<html></html>")
					s, _ = a.openForTest(t, index)
					a.rt.confirmTrust = func(string, string, string) (bool, error) { return true, nil }
				}
				if _, ok := s.sessions.LookupByPath(document); ok {
					t.Fatal("the fixture registered the helper document before navigation")
				}
				target := s.url(filepath.Join("search", "sol.htmlclay"))
				if entry == "banner" {
					status, body := fetchNav(t, target)
					nonce := bannerNonceRe.FindStringSubmatch(body)
					if status != 200 || nonce == nil {
						t.Fatalf("read-only banner = %d: %s", status, body)
					}
					status, body = postSameOrigin(t, "http://"+s.host+"/_/open-request",
						"application/json", `{"nonce":"`+nonce[1]+`"}`)
					if status != 200 || !strings.Contains(body, `"ok":true`) {
						t.Fatalf("trust request = %d: %s", status, body)
					}
				}
				for visit := 1; visit <= 2; visit++ {
					status, body := fetchNav(t, target)
					token := tokenAttrRe.FindStringSubmatch(body)
					if status != 200 || token == nil {
						t.Fatalf("navigation = %d: %s", status, body)
					}
					status, body = fetch(t, "http://"+s.host+"/_/meta/"+token[1])
					var meta struct {
						Document struct {
							Helpers []struct{ Name, State string }
						}
					}
					if status != 200 {
						t.Fatalf("meta = %d: %s", status, body)
					}
					if err := json.Unmarshal([]byte(body), &meta); err != nil {
						t.Fatal(err)
					}
					helpers := meta.Document.Helpers
					if len(helpers) != 2 || helpers[0].Name != "search" || helpers[0].State != permission.state || helpers[1].Name != "ai-edit" || helpers[1].State != "ready" {
						t.Fatalf("visit %d: discovery helpers = %+v, want search:%s", visit, helpers, permission.state)
					}
					t.Logf("visit %d: discovery search=%s", visit, helpers[0].State)
				}
				if via := s.sessions.Via(document); via != session.ViaTrusted {
					t.Fatalf("document provenance = %v, want only ViaTrusted", via)
				}
				if !timer.Stop() {
					t.Fatal("navigation waited for the helper dialog lock to be released")
				}
				t.Log("navigation and discovery completed while the helper dialog lock was held")
			})
		}
	}
}

// A navigation and a tray action both resolve without prompting, but only one
// of them has a user waiting on it. Reporting a bad declaration is keyed on
// that, not on whether the pass prompts, or a tray action on an unreadable file
// would fail with nothing on screen anywhere.
func TestOnlyAUserActionReportsADeclarationItCannotRead(t *testing.T) {
	missing := func(a *app) string { return filepath.Join(a.rt.home, "missing.htmlclay") }

	t.Run("navigation stays silent", func(t *testing.T) {
		a := newTestApp(t, t.TempDir())
		a.rt.notify = func(string, string) error {
			t.Error("an HTTP navigation raised a native notification")
			return nil
		}
		plan := a.helpersForNavigation(missing(a))
		if len(plan.names) != 0 || len(plan.allowed) != 0 {
			t.Fatalf("failed declaration read returned a plan: %+v", plan)
		}
	})

	t.Run("a tray action reports", func(t *testing.T) {
		a := newTestApp(t, t.TempDir())
		reported := 0
		a.rt.notify = func(string, string) error {
			reported++
			return nil
		}
		a.helpersForOpen(missing(a), false)
		if reported != 1 {
			t.Fatalf("notifications raised = %d, want 1", reported)
		}
	})
}

func TestRefreshingOpenDispatchersAppliesRevocationWithoutPrompt(t *testing.T) {
	home := t.TempDir()
	a := newTestApp(t, home)
	document := writeHelperPage(t, home, "search")
	document, err := resolveSymlinks(document)
	if err != nil {
		t.Fatal(err)
	}
	program, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0755))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.rt.cfg.DecideHelper(config.HelperDecision{
		Document: document, Name: "search", Program: program.ID, Allowed: true,
	}); err != nil {
		t.Fatal(err)
	}
	s, _ := a.openForTest(t, document)
	a.applyHelperPlan(s, document, a.helpersForOpenWith(document, false, false, helperApprovalDialogs{}))
	before := s.srv.HelperGeneration(document)
	if before == 0 || !s.srv.HelpersBound(document) {
		t.Fatal("allowed helper did not attach")
	}
	if removed := a.rt.cfg.ForgetHelperProgramDecisions(program.ID); len(removed) != 1 {
		t.Fatalf("removed decisions = %+v", removed)
	}

	a.refreshHelperDispatchers()

	after := s.srv.HelperGeneration(document)
	if after != before || !s.srv.HelpersBound(document) {
		t.Fatalf("revocation refresh = generation %d, bound %v; before %d", after, s.srv.HelpersBound(document), before)
	}
	file, ok := s.sessions.LookupByPath(document)
	if !ok {
		t.Fatal("reopened document is not registered")
	}
	status, raw := fetch(t, "http://"+s.host+"/_/meta/"+file.Token)
	if status != 200 {
		t.Fatalf("meta status = %d: %s", status, raw)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatal(err)
	}
	block := meta["document"].(map[string]any)
	helpers := block["helpers"].([]any)
	// Forgetting a document permission is not a refusal. The dispatcher stops
	// allowing the name, and discovery has to say so without telling the page
	// the user said no: the next direct open asks again.
	state := helpers[0].(map[string]any)["state"]
	if state != "unavailable" {
		t.Fatalf("helper state after revocation = %v, want unavailable", state)
	}
}

func equalHelperPrograms(a, b []config.HelperProgram) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalHelperDecisions(a, b []config.HelperDecision) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The document-programs prompt must not run through the read prompt's seam. It
// did, and inherited that dialog's buttons: "Allow Once" over a decision that
// stands until the tray changes it, and "Trust this folder" over a grant that
// covers a PROGRAM, in documents anywhere, and no folder at all.
// The document-programs prompt must not run through the read prompt's seam. It
// did, and inherited that dialog's buttons: "Allow Once" over a decision that
// stands until the tray changes it, and "Trust this folder" over a grant that
// covers a PROGRAM, in documents anywhere, and no folder at all. The first
// dialog now names the file it will run, and the folder grant sits behind More
// Options.
func TestTheDocumentProgramsPromptHasItsOwnSeamAndButtons(t *testing.T) {
	a := &app{rt: &appRuntime{}}
	a.rt.confirm = func(string, string, bool) (platform.ConfirmChoice, error) {
		t.Error("the document-programs prompt used the read prompt's seam, so it also used its buttons")
		return platform.ConfirmDeny, nil
	}
	calls := 0
	a.rt.chooseHelperSetup = func(string, string, string, string, string) (platform.Choice, error) {
		calls++
		return platform.ChoiceRefuse, nil
	}
	if _, err := a.systemHelperApprovalDialogs().setup("Allow a program for page.htmlclay?", "message", helperSetupDeny, helperSetupMoreOptions, helperSetupChooseFile); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("chooseHelperSetup called %d times, want 1", calls)
	}

	if helperSetupMoreOptions == helperSetupChooseFile || helperSetupMoreOptions == helperSetupDeny {
		t.Fatalf("the dialog repeats a button: %q %q %q", helperSetupDeny, helperSetupMoreOptions, helperSetupChooseFile)
	}
	for _, label := range []string{helperSetupDeny, helperSetupMoreOptions, helperSetupChooseFile, helperAllowLabel(filepath.Join("apps", "bin", "clay-search"))} {
		if strings.Contains(label, "Once") {
			t.Fatalf("the document-programs dialog still offers the read prompt's Allow Once: %q", label)
		}
	}
	if got := helperAllowLabel(filepath.Join("apps", "bin", "clay-search")); got != "Allow clay-search" {
		t.Fatalf("the first dialog's affirmative = %q, want the file it will run", got)
	}
	if strings.Contains(helperSetupChooseFile, "Folder") || strings.Contains(strings.ToLower(helperSetupChooseFile), "trust") {
		t.Fatalf("the first dialog's affirmative grants a folder: %q", helperSetupChooseFile)
	}
	if !strings.Contains(helperSetupTrusted, "Trusted Folders") || !strings.Contains(helperSetupChooseOther, "Choose Another File") {
		t.Fatalf("the More Options dialog does not offer the wider grant and another file: %q %q", helperSetupTrusted, helperSetupChooseOther)
	}
}

func writeHelperPageIn(t *testing.T, dir, folder string, names ...string) string {
	t.Helper()
	var metas strings.Builder
	for _, name := range names {
		metas.WriteString("<meta name=\"htmlclay-helper\" content=\"" + name + "\"")
		if folder != "" {
			metas.WriteString(" data-program-folder=\"" + folder + "\"")
		}
		metas.WriteString(">")
	}
	path := filepath.Join(dir, "page.htmlclay")
	if err := os.WriteFile(path, []byte("<!doctype html><head>"+metas.String()+"</head>"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHelperPickerFolderStartsInTheDocumentFolderUnlessTheHintIsUsable(t *testing.T) {
	home := t.TempDir()
	a := newTestApp(t, home)
	docs := filepath.Join(home, "docs")
	if err := os.MkdirAll(docs, 0755); err != nil {
		t.Fatal(err)
	}
	hinted := filepath.Join(home, "var", "bin")
	if err := os.MkdirAll(hinted, 0755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		folder string
		want   string
	}{
		{"hinted folder inside home", "../var/bin", hinted},
		{"home-relative hint", "~/var/bin", hinted},
		{"home-relative hint that leaves home", "~/../elsewhere", docs},
		{"hint that leaves home", "../../elsewhere", docs},
		{"hinted folder that does not exist", "../var/missing", docs},
		{"no hint", "", docs},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := writeHelperPageIn(t, docs, tc.folder, "search")
			if got := a.helperPickerFolder(document, "search"); got != tc.want {
				t.Fatalf("helperPickerFolder = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHelperApprovalPickerStartsInTheDeclaredFolder(t *testing.T) {
	home := t.TempDir()
	a := newTestApp(t, home)
	docs := filepath.Join(home, "docs")
	if err := os.MkdirAll(docs, 0755); err != nil {
		t.Fatal(err)
	}
	hinted := filepath.Join(home, "var", "bin")
	if err := os.MkdirAll(hinted, 0755); err != nil {
		t.Fatal(err)
	}
	document := writeHelperPageIn(t, docs, "../var/bin", "search")
	programPath := writeTestProgram(t, hinted, "search", 0755)
	plan := a.helpersForUseWith(document, "search", helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) {
			return platform.ChoiceAffirm, nil
		},
		choose: func(prompt, dir string) (string, bool, error) {
			if dir != hinted {
				t.Fatalf("picker started in %q, want %q", dir, hinted)
			}
			return programPath, true, nil
		},
		prepare: func(string) (bool, error) { return true, nil },
	})
	if program, ok := plan.allowed["search"]; !ok || program.Path != programPath {
		t.Fatalf("allowed helper = %+v, %v", program, ok)
	}
}

func TestHelperUseAsksOnlyAboutTheHelperThatWasUsed(t *testing.T) {
	home := t.TempDir()
	a := newTestApp(t, home)
	document := writeHelperPage(t, home, "search", "ocr")
	for _, name := range []string{"search", "ocr"} {
		if _, err := a.rt.cfg.AddHelperProgram(name, writeTestProgram(t, t.TempDir(), name, 0755)); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
	calls := 0
	var message string
	plan := a.helpersForUseWith(document, "search", helperApprovalDialogs{
		setup: func(_, text, _, _, _ string) (platform.Choice, error) {
			calls++
			message = text
			return platform.ChoiceAffirm, nil
		},
		choose: func(string, string) (string, bool, error) {
			t.Fatal("a registered program opened the picker")
			return "", false, nil
		},
		prepare: func(path string) (bool, error) {
			if filepath.Base(path) != "search" {
				t.Fatalf("prepared %q, want the registered program", path)
			}
			return true, nil
		},
	})
	if calls != 1 {
		t.Fatalf("dialog calls = %d, want one for the used helper only", calls)
	}
	if !strings.Contains(message, "search") || strings.Contains(message, "ocr") {
		t.Fatalf("dialog named more than the used helper: %q", message)
	}
	if _, ok := plan.allowed["search"]; !ok {
		t.Fatalf("allowed plan = %+v, want search", plan.allowed)
	}
}

func TestHelperUseDenialAppliesToThisOpenAndKeepsEarlierDenials(t *testing.T) {
	home := t.TempDir()
	a := newTestApp(t, home)
	document := writeHelperPage(t, home, "search", "ocr")
	for _, name := range []string{"search", "ocr"} {
		if _, err := a.rt.cfg.AddHelperProgram(name, writeTestProgram(t, t.TempDir(), name, 0755)); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
	calls := 0
	dialogs := helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) {
			calls++
			return platform.ChoiceRefuse, nil
		},
		choose: func(string, string) (string, bool, error) {
			t.Fatal("denial opened the picker")
			return "", false, nil
		},
		prepare: func(string) (bool, error) {
			t.Fatal("denial prepared a program")
			return false, nil
		},
	}
	plan := a.helpersForUseWith(document, "search", dialogs)
	if !slices.Contains(plan.denied, "search") {
		t.Fatalf("denied plan = %+v, want search denied", plan.denied)
	}
	if calls != 1 {
		t.Fatalf("dialog calls = %d, want 1", calls)
	}
	again := a.helpersForUseWith(document, "search", dialogs)
	if calls != 1 {
		t.Fatalf("a denied name raised the dialog again: calls = %d", calls)
	}
	if !slices.Contains(again.denied, "search") {
		t.Fatalf("second denied plan = %+v, want search still denied", again.denied)
	}
	second := a.helpersForUseWith(document, "ocr", dialogs)
	if calls != 1 {
		t.Fatalf("dialog calls = %d, want a refusal to stop every page-raised dialog for the document", calls)
	}
	if !slices.Contains(second.denied, "search") {
		t.Fatalf("second plan dropped the first denial: %+v", second.denied)
	}
	if _, ok := second.allowed["ocr"]; ok {
		t.Fatalf("a stopped dialog granted ocr: %+v", second.allowed)
	}
}

func TestHelperUseNeverAsksAboutAStoredDecision(t *testing.T) {
	home := t.TempDir()
	a := newTestApp(t, home)
	document := writeHelperPage(t, home, "search", "ocr")
	allowed, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, t.TempDir(), "search", 0755))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.rt.cfg.AddHelperProgram("ocr", writeTestProgram(t, t.TempDir(), "ocr", 0755)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.rt.cfg.DecideHelper(config.HelperDecision{Document: document, Name: "search", Program: allowed.ID, Allowed: true}); err != nil {
		t.Fatal(err)
	}
	if err := a.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
	dialogs := helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) {
			t.Fatal("a stored decision raised the dialog")
			return platform.ChoiceRefuse, nil
		},
	}
	plan := a.helpersForUseWith(document, "search", dialogs)
	if got := plan.allowed["search"]; got.ID != allowed.ID {
		t.Fatalf("allowed helper = %+v, want %s", got, allowed.ID)
	}
}

func TestARefusalStopsEveryPageRaisedDialogForTheDocument(t *testing.T) {
	home := t.TempDir()
	a := newTestApp(t, home)
	document := writeHelperPage(t, home, "a", "b")
	for _, name := range []string{"a", "b"} {
		if _, err := a.rt.cfg.AddHelperProgram(name, writeTestProgram(t, t.TempDir(), name, 0755)); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.rt.cfg.Save(); err != nil {
		t.Fatal(err)
	}
	calls := 0
	dialogs := helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) {
			calls++
			return platform.ChoiceRefuse, nil
		},
		choose: func(string, string) (string, bool, error) {
			t.Fatal("denial opened the picker")
			return "", false, nil
		},
		prepare: func(string) (bool, error) {
			t.Fatal("denial prepared a program")
			return false, nil
		},
	}
	a.helpersForUseWith(document, "a", dialogs)
	if calls != 1 {
		t.Fatalf("dialog calls = %d, want 1", calls)
	}
	plan := a.helpersForUseWith(document, "b", dialogs)
	if calls != 1 {
		t.Fatalf("a second page-raised ask raised the dialog again: calls = %d", calls)
	}
	if _, ok := plan.allowed["b"]; ok {
		t.Fatalf("a stopped dialog granted b: %+v", plan.allowed)
	}
	reopened := a.helpersForOpenWith(document, true, true, dialogs)
	if calls != 1 {
		t.Fatalf("a direct open raised the helper dialog: calls = %d", calls)
	}
	if len(reopened.denied) != 0 {
		t.Fatalf("a direct open kept this open's refusals: %+v", reopened.denied)
	}
	clearPageAskQuiet(t, a, document)
	a.helpersForUseWith(document, "b", dialogs)
	if calls != 2 {
		t.Fatalf("the page did not ask again after a direct open: calls = %d", calls)
	}
}

func TestAPageWaitsAfterADialogThatGrantedNothing(t *testing.T) {
	home := t.TempDir()
	a := newTestApp(t, home)
	document := writeHelperPage(t, home, "search")
	calls := 0
	dialogs := helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) {
			calls++
			return platform.ChoiceNone, nil
		},
	}
	a.helpersForUseWith(document, "search", dialogs)
	if calls != 1 {
		t.Fatalf("dialog calls = %d, want 1", calls)
	}
	a.helpersForUseWith(document, "search", dialogs)
	if calls != 1 {
		t.Fatalf("the page raised the dialog again inside the quiet period: calls = %d", calls)
	}
	a.helperStateMu.Lock()
	a.pageAskQuiet[document] = time.Now().Add(-time.Minute)
	a.helperStateMu.Unlock()
	a.helpersForUseWith(document, "search", dialogs)
	if calls != 2 {
		t.Fatalf("the page did not ask again after the quiet period: calls = %d", calls)
	}
}

func TestAPageRaisedDialogFailureShowsNoBanner(t *testing.T) {
	home := t.TempDir()
	a := newTestApp(t, home)
	banners := 0
	a.rt.notify = func(string, string) error {
		banners++
		return nil
	}
	document := writeHelperPage(t, home, "search")
	dialogs := helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) {
			return platform.ChoiceNone, errors.New("no native dialog tool (zenity or kdialog) found")
		},
	}
	a.helpersForUseWith(document, "search", dialogs)
	if banners != 0 {
		t.Fatalf("banners = %d, want none for a page-raised ask", banners)
	}
}

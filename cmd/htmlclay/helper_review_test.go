package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// Regressions found in the 2026-10-31 code review of program setup.

func TestReviewChangedFileDuringApproval(t *testing.T) {
	a, home := newSuggestApp(t)
	path := writeSuggestFile(t, filepath.Join(home, "bin", "search"), 0755)
	addSuggestProgram(t, a, "search", path)
	doc := writeSuggestDocument(t, filepath.Join(home, "docs", "page.htmlclay"), `<meta name="htmlclay-helper" content="search">`)
	var before, after os.FileInfo
	before, _ = os.Stat(path)
	dialogs := helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) {
			replacement := writeSuggestFile(t, filepath.Join(home, "bin", "replacement"), 0755)
			if err := os.WriteFile(replacement, []byte("#!/bin/sh\n# harmless changed review fixture\n"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(replacement, path); err != nil {
				t.Fatal(err)
			}
			after, _ = os.Stat(path)
			return platform.ChoiceAffirm, nil
		},
		choose: func(string, string) (string, bool, error) { t.Fatal("unexpected picker"); return "", false, nil },
		prepare: func(path string) (bool, error) {
			return prepareHelperProgram(path, func(string, string, string) (bool, error) { t.Fatal("unexpected chmod prompt"); return false, nil })
		},
	}
	plan := a.helpersForUseWith(doc, "search", dialogs)
	if os.SameFile(before, after) {
		t.Fatal("fixture failed to replace inode")
	}
	if p, ok := plan.allowed["search"]; ok {
		t.Errorf("replacement file granted without reapproval: %s", p.Path)
	}
}

func TestReviewRemoveMissingDoesNotSwitchProgram(t *testing.T) {
	a, home := newSuggestApp(t)
	doc := writeSuggestDocument(t, filepath.Join(home, "docs", "page.htmlclay"), `<meta name="htmlclay-helper" content="search">`)
	a.rt.cfg.AddTrustedFolder(filepath.Dir(doc), platform.DirIdentity(filepath.Dir(doc)))
	missing, err := a.rt.cfg.AddHelperProgram("search", filepath.Join(home, "missing", "search"))
	if err != nil {
		t.Fatal(err)
	}
	live, err := a.rt.cfg.AddHelperProgram("search", writeSuggestFile(t, filepath.Join(home, "bin", "search"), 0755))
	if err != nil {
		t.Fatal(err)
	}
	a.rt.cfg.SetHelperAnyDocument(live.ID, true)
	if _, _, err := a.rt.cfg.DecideHelper(config.HelperDecision{Document: doc, Name: "search", Program: missing.ID, Allowed: true}); err != nil {
		t.Fatal(err)
	}
	before, _ := a.rt.cfg.ResolveHelper(doc, "search", true)
	if before.Program.ID != missing.ID {
		t.Fatal("fixture did not bind missing program")
	}
	if outcome := a.programsAction("remove", missing.ID, "", ""); outcome != "removed" {
		t.Fatal(outcome)
	}
	after, _ := a.rt.cfg.ResolveHelper(doc, "search", true)
	if after.Allowed {
		t.Errorf("Remove from List silently granted another executable: before=%s after=%s", before.Program.ID, after.Program.ID)
	}
}

func TestReviewUntrustParentRevokesExistingChildHelper(t *testing.T) {
	home, err := resolveSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := newTestApp(t, home)
	parent := filepath.Join(home, "projects")
	doc := filepath.Join(parent, "child", "page.htmlclay")
	writeTestFile(t, doc, `<head><meta name="htmlclay-helper" content="search"></head><body>fixture</body>`)
	s, _ := a.openForTest(t, doc)
	path, marker := attachmentProgram(t, "search")
	p, err := a.rt.cfg.AddHelperProgram("search", path)
	if err != nil {
		t.Fatal(err)
	}
	a.rt.cfg.SetHelperAnyDocument(p.ID, true)
	if err := a.trustFolder(parent); err != nil {
		t.Fatal(err)
	}
	if !a.trustedLive(doc) {
		t.Fatal("fixture never became trusted")
	}
	if err := a.untrustFolder(parent); err != nil {
		t.Fatal(err)
	}
	if a.trustedLive(doc) {
		t.Fatal("fixture still trusted")
	}
	registration, ok := s.sessions.LookupByPath(doc)
	if !ok {
		t.Fatal("direct-open child vanished")
	}
	if got := helperDiscoveryState(t, s, doc); got != "ready" {
		t.Logf("post-untrust state %s", got)
		return
	}
	terminal := attachmentRequestTerminal(t, s, doc, registration.Token, "after-untrust")
	if _, err := os.Stat(marker); err == nil {
		t.Errorf("helper executed after folder trust was removed: terminal=%+v", terminal)
	}
}

func TestReviewProgramsSaveFailureRollback(t *testing.T) {
	for _, action := range []string{"revoke-document", "revoke-trusted", "revoke-all", "remove", "relink", "widen"} {
		t.Run(action, func(t *testing.T) {
			home, cfgBase := t.TempDir(), t.TempDir()
			a := buildTestApp(t, home, cfgBase)
			doc := writeSuggestDocument(t, filepath.Join(home, "docs", "page.htmlclay"), `<meta name="htmlclay-helper" content="search">`)
			path := filepath.Join(home, "missing", "search")
			if action != "remove" && action != "relink" {
				writeSuggestFile(t, path, 0755)
			}
			p, err := a.rt.cfg.AddHelperProgram("search", path)
			if err != nil {
				t.Fatal(err)
			}
			a.rt.cfg.SetHelperAnyDocument(p.ID, action != "widen")
			if _, _, err := a.rt.cfg.DecideHelper(config.HelperDecision{Document: doc, Name: "search", Program: p.ID, Allowed: true}); err != nil {
				t.Fatal(err)
			}
			if _, _, err := a.rt.cfg.RevokeHelper(filepath.Join(home, "other.htmlclay"), "search"); err != nil {
				t.Fatal(err)
			}
			beforePrograms, beforeDecisions := a.rt.cfg.HelperProgramList(), a.rt.cfg.HelperDecisionList()
			target := writeSuggestFile(t, filepath.Join(home, "new", "search"), 0755)
			a.rt.chooseHelperSetup = func(string, string, string, string, string) (platform.Choice, error) {
				return platform.ChoiceAffirm, nil
			}
			a.rt.chooseHelperProgram = func(string, string) (string, bool, error) { return target, true, nil }
			a.rt.prepareHelperProgram = func(string) (bool, error) { return true, nil }
			dir := config.DirFrom(cfgBase)
			if err := os.Rename(dir, dir+".backup"); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if err := os.WriteFile(dir, []byte("block save in fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			if got := a.programsAction(action, p.ID, doc, "search"); got != "failed" {
				t.Fatalf("action %s reported %s", action, got)
			}
			if !reflect.DeepEqual(beforePrograms, a.rt.cfg.HelperProgramList()) || !reflect.DeepEqual(beforeDecisions, a.rt.cfg.HelperDecisionList()) {
				t.Fatal("state changed after save failure")
			}
		})
	}
}

func TestReviewProgramsNativeDialogExtendsHTTPDeadline(t *testing.T) {
	a := newProgramsApp(t)
	p, err := a.rt.cfg.AddHelperProgram("search", writeSuggestFile(t, filepath.Join(a.rt.home, "bin", "search"), 0755))
	if err != nil {
		t.Fatal(err)
	}
	a.rt.chooseHelperSetup = func(string, string, string, string, string) (platform.Choice, error) {
		time.Sleep(80 * time.Millisecond)
		return platform.ChoiceAffirm, nil
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(a.serveHost))
	server.Config.WriteTimeout = 30 * time.Millisecond
	server.Start()
	defer server.Close()
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	a.rt.port, _ = strconv.Atoi(port)
	c := signIn(t, a)
	form := url.Values{"csrf": {c.csrf()}, "action": {"widen"}, "program": {p.ID}}
	request, err := http.NewRequest("POST", server.URL+"/programs/action", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", c.origin)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(c.cookie)
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		after, _ := a.rt.cfg.LookupHelperProgram(p.ID)
		t.Fatalf("native action saved AnyDocument=%v but browser received %v", after.AnyDocument, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", response.StatusCode)
	}
}

func TestReviewSuggestedSymlinkRetarget(t *testing.T) {
	a, home := newSuggestApp(t)
	settledSetup(t)
	bin := filepath.Join(home, "bin")
	addSuggestProgram(t, a, "other", writeSuggestFile(t, filepath.Join(bin, "other"), 0755))
	first := writeSuggestFile(t, filepath.Join(home, "versions", "first"), 0755)
	second := writeSuggestFile(t, filepath.Join(home, "versions", "second"), 0755)
	path := filepath.Join(bin, "search")
	if err := os.Symlink(first, path); err != nil {
		t.Fatal(err)
	}
	doc := writeSuggestDocument(t, filepath.Join(home, "docs", "page.htmlclay"), `<meta name="htmlclay-helper" content="search">`)
	original, _ := os.Stat(path)
	dialogs := helperApprovalDialogs{
		setup: func(string, string, string, string, string) (platform.Choice, error) {
			next := filepath.Join(bin, "next-link")
			if err := os.Symlink(second, next); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(next, path); err != nil {
				t.Fatal(err)
			}
			return platform.ChoiceAffirm, nil
		},
		choose: func(string, string) (string, bool, error) { t.Fatal("unexpected picker"); return "", false, nil },
		prepare: func(path string) (bool, error) {
			return prepareHelperProgram(path, func(string, string, string) (bool, error) { t.Fatal("unexpected chmod prompt"); return false, nil })
		},
	}
	plan := a.helpersForUseWith(doc, "search", dialogs)
	after, _ := os.Stat(path)
	if os.SameFile(original, after) {
		t.Fatal("fixture did not replace symlink target")
	}
	if _, ok := plan.allowed["search"]; ok {
		t.Error("suggested link changed to another settled executable while dialog was open, but permission was saved")
	}
}

func TestReviewPageCannotSteerSuggestionToAShell(t *testing.T) {
	a, home := newSuggestApp(t)
	bin := filepath.Join(home, "opt", "bin")
	addSuggestProgram(t, a, "other", writeSuggestFile(t, filepath.Join(bin, "other"), 0o755))
	writeSuggestFile(t, filepath.Join(bin, "bash"), 0o755)
	writeSuggestFile(t, filepath.Join(bin, "zsh"), 0o755)
	doc := writeSuggestDocument(t, filepath.Join(home, "docs", "page.htmlclay"),
		`<meta name="htmlclay-helper" content="search" data-program-file="bash"><meta name="htmlclay-helper" content="zsh">`)
	if got := a.suggestHelperProgram(doc, "search", settled()); got.path != "" || got.program.ID != "" {
		t.Errorf("data-program-file=bash suggested %+v", got)
	}
	if got := a.suggestHelperProgram(doc, "zsh", settled()); got.path != "" || got.program.ID != "" {
		t.Errorf("helper named zsh suggested %+v", got)
	}
}

func TestScriptInterpreter(t *testing.T) {
	for _, path := range []string{"/bin/sh", "/usr/bin/bash", "/opt/homebrew/bin/python3.12", "cmd.exe", "pwsh.exe", "/usr/bin/env", "Node"} {
		if !scriptInterpreter(path) {
			t.Errorf("scriptInterpreter(%q) = false", path)
		}
	}
	for _, path := range []string{"/apps/bin/clay-search", "/bin/bashful", "/bin/search.sh", "/bin/node-helper", "/bin/sh2x"} {
		if scriptInterpreter(path) {
			t.Errorf("scriptInterpreter(%q) = true", path)
		}
	}
}

func TestReviewPickedFileBesideDocumentGetsWarning(t *testing.T) {
	a, home := newSuggestApp(t)
	settledSetup(t)
	doc := writeSuggestDocument(t, filepath.Join(home, "pack", "pack.htmlclay"), `<meta name="htmlclay-helper" content="search">`)
	bundled := writeSuggestFile(t, filepath.Join(home, "pack", "search"), 0o755)
	elsewhere := writeSuggestFile(t, filepath.Join(home, "apps", "bin", "search"), 0o755)
	if got := a.helperFileRisk(doc, bundled, settled()); !strings.Contains(got, "document's own folder") {
		t.Errorf("risk beside document = %q", got)
	}
	if got := a.helperFileRisk(doc, filepath.Join(home, "Downloads", "x"), settled()); !strings.Contains(got, "downloads or temporary") {
		t.Errorf("risk in Downloads = %q", got)
	}
	if got := a.helperFileRisk(doc, "/bin/sh", settled()); !strings.Contains(got, "interpreter") {
		t.Errorf("risk for a shell = %q", got)
	}
	if got := a.helperFileRisk(doc, elsewhere, settled()); got != "" {
		t.Errorf("risk elsewhere = %q, want none", got)
	}
	if got := a.helperFileRisk("", bundled, settled()); got != "" {
		t.Errorf("risk with no document = %q, want none", got)
	}

	var warned []string
	dialogs := helperApprovalDialogs{
		setup: func(title, message, refuse, middle, affirm string) (platform.Choice, error) {
			warned = append(warned, message)
			return platform.ChoiceRefuse, nil
		},
		choose:  func(string, string) (string, bool, error) { return bundled, true, nil },
		prepare: func(string) (bool, error) { return true, nil },
	}
	if picked, ok, err := a.chooseHelperFile(dialogs, doc, "Choose", filepath.Dir(doc)); err != nil || ok || picked != "" {
		t.Fatalf("chooseHelperFile = %q %v %v, want cancelled", picked, ok, err)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "document's own folder") {
		t.Errorf("warnings = %q", warned)
	}
}

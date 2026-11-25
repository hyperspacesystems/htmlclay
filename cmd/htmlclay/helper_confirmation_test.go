package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/testutil"
)

// waitForSetupRequest waits for the program question of one kind to reach the
// snapshot, the way the request window sees it.
func waitForSetupRequest(t *testing.T, a *app, kind string) appcore.Request {
	t.Helper()
	var found appcore.Request
	waitFor(t, "the "+kind+" program question to reach the snapshot", func() bool {
		for _, r := range a.snapshot().Requests {
			if r.Kind == kind {
				found = r
				return true
			}
		}
		return false
	})
	return found
}

// answerSetup sends the answer the Mac app sends for one request, with the path
// a choose-file answer carries.
func answerSetup(t *testing.T, a *app, actionID string, request appcore.Request, answer, path string) error {
	t.Helper()
	args := map[string]any{"id": request.ID, "revision": request.Revision, "answer": answer}
	if path != "" {
		args["path"] = path
	}
	_, refusal := a.do(context.Background(), coreAction(t, actionID, "request.answer", args), ChannelShell)
	if refusal != nil {
		return fmt.Errorf("answer %s for %s was refused: %+v", answer, request.ID, refusal)
	}
	return nil
}

// startSetup runs the setup flow the way a person's click does, in the
// background because it waits for an answer.
func startSetup(t *testing.T, a *app, document, name string) <-chan helperOpenPlan {
	t.Helper()
	plan := make(chan helperOpenPlan, 1)
	go func() {
		plan <- a.helpersForUseWith(document, name, a.systemHelperApprovalDialogs())
	}()
	return plan
}

// newSetupConfirmationApp is an app started the way the Mac app starts it, with
// one document that declares search and one program registered for the name.
func newSetupConfirmationApp(t *testing.T) (*app, string, string) {
	t.Helper()
	home := t.TempDir()
	a := newShellTestApp(t, home)
	t.Cleanup(func() {
		for _, p := range a.registry().Pending() {
			a.registry().Close(p.ID)
		}
	})
	document := writeHelperPage(t, home, "search")
	program := writeTestProgram(t, t.TempDir(), "search", 0o755)
	if _, err := a.rt.cfg.AddHelperProgram("search", program); err != nil {
		t.Fatal(err)
	}
	return a, document, program
}

// TestM3HelperConfirmation proves the program setup question goes through one
// flow and two presenters: the registry the Mac app answers when it runs this
// host, and today's dialogs otherwise.
func TestM3HelperConfirmation(t *testing.T) {
	t.Run("allow-document grants one document", func(t *testing.T) {
		a, document, program := newSetupConfirmationApp(t)
		plan := startSetup(t, a, document, "search")
		request := waitForSetupRequest(t, a, appcore.RequestFound)
		if request.Path != program {
			t.Fatalf("the question names %q, want the suggestion %q", request.Path, program)
		}
		if want := []string{"deny", "allow-document", "allow-all-trusted", "choose-file"}; !slices.Equal(request.Answers, want) {
			t.Fatalf("the found question offered %v, want %v", request.Answers, want)
		}
		if request.Document != document || request.DocumentID == nil || *request.DocumentID == "" {
			t.Fatalf("the question does not name its document: %+v", request)
		}
		if err := answerSetup(t, a, "a11", request, "allow-document", ""); err != nil {
			t.Fatal(err)
		}
		got := testutil.Receive(t, 5*time.Second, "the setup flow", plan)
		allowed, ok := got.allowed["search"]
		if !ok || allowed.Path != program || allowed.AnyDocument {
			t.Fatalf("allowed = %+v, %v, want one document at %s", allowed, ok, program)
		}
		decisions := a.rt.cfg.HelperDecisionList()
		if len(decisions) != 1 || decisions[0].Document != document || decisions[0].Scope.Kind != config.ScopeLocal {
			t.Fatalf("decisions = %+v, want one local decision for %s", decisions, document)
		}
		if pending := a.registry().Pending(); len(pending) != 0 {
			t.Fatalf("%d question(s) still open after the answer", len(pending))
		}
	})

	t.Run("allow-all-trusted sets AnyDocument", func(t *testing.T) {
		a, document, _ := newSetupConfirmationApp(t)
		plan := startSetup(t, a, document, "search")
		request := waitForSetupRequest(t, a, appcore.RequestFound)
		if err := answerSetup(t, a, "a12", request, "allow-all-trusted", ""); err != nil {
			t.Fatal(err)
		}
		got := testutil.Receive(t, 5*time.Second, "the setup flow", plan)
		allowed, ok := got.allowed["search"]
		if !ok || !allowed.AnyDocument {
			t.Fatalf("allowed = %+v, %v, want any-document", allowed, ok)
		}
	})

	t.Run("a picked file in Downloads becomes riskyFile and grants one document", func(t *testing.T) {
		settledSetup(t)
		a, document, _ := newSetupConfirmationApp(t)
		downloads := filepath.Join(a.rt.home, "Downloads")
		if err := os.MkdirAll(downloads, 0o755); err != nil {
			t.Fatal(err)
		}
		picked := writeTestProgram(t, downloads, "picked", 0o755)

		plan := startSetup(t, a, document, "search")
		found := waitForSetupRequest(t, a, appcore.RequestFound)
		if err := answerSetup(t, a, "a13", found, "choose-file", picked); err != nil {
			t.Fatal(err)
		}
		risky := waitForSetupRequest(t, a, appcore.RequestRiskyFile)
		if risky.ID != found.ID || risky.Revision != 2 {
			t.Fatalf("risky question = %s at revision %d, want %s at revision 2", risky.ID, risky.Revision, found.ID)
		}
		if risky.Path != picked || risky.Detail != "in-downloads" {
			t.Fatalf("risky question = path %q detail %q, want %q and in-downloads", risky.Path, risky.Detail, picked)
		}
		if want := []string{"deny", "choose-file", "allow-anyway"}; !slices.Equal(risky.Answers, want) {
			t.Fatalf("the risky question offered %v, want %v", risky.Answers, want)
		}
		if err := answerSetup(t, a, "a14", risky, "allow-anyway", ""); err != nil {
			t.Fatal(err)
		}
		got := testutil.Receive(t, 5*time.Second, "the setup flow", plan)
		allowed, ok := got.allowed["search"]
		if !ok || allowed.Path != picked || allowed.AnyDocument {
			t.Fatalf("allowed = %+v, %v, want one document at %s", allowed, ok, picked)
		}
	})

	t.Run("a file swapped between ask and answer stores nothing", func(t *testing.T) {
		a, document, program := newSetupConfirmationApp(t)
		plan := startSetup(t, a, document, "search")
		request := waitForSetupRequest(t, a, appcore.RequestFound)
		if err := os.WriteFile(program, []byte("#!/bin/sh\necho swapped in\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := answerSetup(t, a, "a15", request, "allow-document", ""); err != nil {
			t.Fatal(err)
		}
		got := testutil.Receive(t, 5*time.Second, "the setup flow", plan)
		if _, ok := got.allowed["search"]; ok {
			t.Fatalf("a swapped file was allowed: %+v", got.allowed)
		}
		if decisions := a.rt.cfg.HelperDecisionList(); len(decisions) != 0 {
			t.Fatalf("a swapped file stored %+v", decisions)
		}
	})

	t.Run("a document that stops declaring the program stores nothing", func(t *testing.T) {
		a, document, _ := newSetupConfirmationApp(t)
		plan := startSetup(t, a, document, "search")
		request := waitForSetupRequest(t, a, appcore.RequestFound)
		if err := os.WriteFile(document, []byte("<!doctype html><html><head></head><body></body></html>"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := answerSetup(t, a, "a16", request, "allow-document", ""); err != nil {
			t.Fatal(err)
		}
		got := testutil.Receive(t, 5*time.Second, "the setup flow", plan)
		if _, ok := got.allowed["search"]; ok {
			t.Fatalf("a document that stopped declaring the program was allowed: %+v", got.allowed)
		}
		if decisions := a.rt.cfg.HelperDecisionList(); len(decisions) != 0 {
			t.Fatalf("a document that stopped declaring the program stored %+v", decisions)
		}
	})

	t.Run("a team document never reaches always", func(t *testing.T) {
		home := t.TempDir()
		a := newShellTestApp(t, home)
		t.Cleanup(func() {
			for _, p := range a.registry().Pending() {
				a.registry().Close(p.ID)
			}
		})
		folder := filepath.Join(home, "shared")
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		document := writeHelperPageIn(t, folder, "", "search")
		program := writeTestProgram(t, t.TempDir(), "search", 0o755)
		if _, err := a.rt.cfg.AddHelperProgram("search", program); err != nil {
			t.Fatal(err)
		}
		if err := a.rt.cfg.MarkTeamFolder(config.TeamFolder{
			Path:     folder,
			Identity: platform.DirIdentity(folder),
			Kind:     config.TeamFolderKindTeam,
			Key:      "acme",
		}); err != nil {
			t.Fatal(err)
		}
		if scope := a.scopeOf(document); scope.Kind != config.ScopeTeam {
			t.Fatalf("fixture scope = %+v, want a team folder", scope)
		}

		plan := startSetup(t, a, document, "search")
		request := waitForSetupRequest(t, a, appcore.RequestTeamDocument)
		if slices.Contains(request.Answers, "allow-all-trusted") {
			t.Fatalf("a team document offered allow-all-trusted: %v", request.Answers)
		}
		if want := []string{"deny", "allow-document", "choose-file"}; !slices.Equal(request.Answers, want) {
			t.Fatalf("the team question offered %v, want %v", request.Answers, want)
		}
		_, refusal := a.do(context.Background(), coreAction(t, "a17", "request.answer", map[string]any{
			"id":       request.ID,
			"revision": request.Revision,
			"answer":   "allow-all-trusted",
		}), ChannelShell)
		if refusal == nil || refusal.Code != appcore.RefuseAnswerNotOffered {
			t.Fatalf("a forged allow-all-trusted = %+v, want answer-not-offered", refusal)
		}
		if decisions := a.rt.cfg.HelperDecisionList(); len(decisions) != 0 {
			t.Fatalf("a forged answer stored %+v", decisions)
		}
		pending := a.registry().Pending()
		if len(pending) != 1 || pending[0].Revision != request.Revision {
			t.Fatalf("pending after a refused answer = %+v, want the same question", pending)
		}
		if err := answerSetup(t, a, "a18", request, "deny", ""); err != nil {
			t.Fatal(err)
		}
		testutil.Receive(t, 5*time.Second, "the setup flow", plan)
	})

	t.Run("a second document's setup and Remove Access run while a question is open", func(t *testing.T) {
		home := t.TempDir()
		a := newShellTestApp(t, home)
		t.Cleanup(func() {
			for _, p := range a.registry().Pending() {
				a.registry().Close(p.ID)
			}
		})
		if err := os.MkdirAll(filepath.Join(home, "one"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(home, "two"), 0o755); err != nil {
			t.Fatal(err)
		}
		first := writeHelperPageIn(t, filepath.Join(home, "one"), "", "search")
		second := writeHelperPageIn(t, filepath.Join(home, "two"), "", "search")
		program := writeTestProgram(t, t.TempDir(), "search", 0o755)
		registered, err := a.rt.cfg.AddHelperProgram("search", program)
		if err != nil {
			t.Fatal(err)
		}

		firstPlan := startSetup(t, a, first, "search")
		waitForSetupRequest(t, a, appcore.RequestFound)
		secondPlan := startSetup(t, a, second, "search")
		waitFor(t, "the second document's program question", func() bool {
			return len(a.snapshot().Requests) == 2
		})

		removed := make(chan string, 1)
		go func() {
			removed <- a.programsAction("revoke-document", registered.ID, first, "search")
		}()
		if outcome := testutil.Receive(t, 5*time.Second, "Remove Access", removed); outcome != "revoked" {
			t.Fatalf("Remove Access = %q, want revoked", outcome)
		}

		for _, p := range a.registry().Pending() {
			a.registry().Close(p.ID)
		}
		testutil.Receive(t, 5*time.Second, "the first setup", firstPlan)
		testutil.Receive(t, 5*time.Second, "the second setup", secondPlan)
	})

	t.Run("dialog presenter's pick covers one document", func(t *testing.T) {
		settledSetup(t)
		home := t.TempDir()
		a := newTestApp(t, home)
		document := writeHelperPage(t, home, "search")
		programPath := writeTestProgram(t, t.TempDir(), "search", 0o755)
		var order []string
		plan := a.helpersForUseWith(document, "search", helperApprovalDialogs{
			setup: func(string, string, string, string, string) (platform.Choice, error) {
				order = append(order, "setup")
				if len(order) == 1 {
					return platform.ChoiceMiddle, nil
				}
				return platform.ChoiceAffirm, nil
			},
			choose: func(string, string) (string, bool, error) {
				order = append(order, "choose")
				return programPath, true, nil
			},
			prepare: func(string) (bool, error) {
				order = append(order, "prepare")
				return true, nil
			},
		})
		if got := strings.Join(order, ","); got != "setup,choose,prepare" {
			t.Fatalf("dialog order = %s", got)
		}
		program, ok := plan.allowed["search"]
		if !ok || program.Path != programPath || program.AnyDocument {
			t.Fatalf("allowed = %+v, %v, want this document only at %s", program, ok, programPath)
		}
	})
}

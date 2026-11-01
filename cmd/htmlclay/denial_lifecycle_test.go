package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

func helperDiscoveryState(t *testing.T, s *site, document string) string {
	t.Helper()
	f, ok := s.sessions.LookupByPath(document)
	if !ok {
		t.Fatal("missing document registration")
	}
	status, body := fetch(t, "http://"+s.host+"/_/meta/"+f.Token)
	var meta struct {
		Document struct {
			Helpers []struct{ Name, State string }
		}
	}
	if status != 200 {
		t.Fatalf("meta status %d: %s", status, body)
	}
	if err := json.Unmarshal([]byte(body), &meta); err != nil {
		t.Fatal(err)
	}
	// The declared helper, then the built-in ai-edit, which is on by default.
	if len(meta.Document.Helpers) != 2 || meta.Document.Helpers[1].Name != "ai-edit" || meta.Document.Helpers[1].State != "ready" {
		t.Fatalf("helpers: %+v", meta.Document.Helpers)
	}
	return meta.Document.Helpers[0].State
}

func TestADenialLastsForTheRestOfThatOpen(t *testing.T) {
	for _, scenario := range []string{"cancel", "revoke one shared global", "navigation reattach", "ordinary reload"} {
		t.Run(scenario, func(t *testing.T) {
			home, err := resolveSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			a := newTestApp(t, home)
			document := filepath.Join(home, "docs", "page.htmlclay")
			writeTestFile(t, document, `<html><head><meta name="htmlclay-helper" content="search"></head></html>`)
			first, err := a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, home, "first", 0755))
			if err != nil {
				t.Fatal(err)
			}
			var second config.HelperProgram
			if scenario == "revoke one shared global" {
				second, err = a.rt.cfg.AddHelperProgram("search", writeTestProgram(t, home, "second", 0755))
				if err != nil {
					t.Fatal(err)
				}
				a.rt.cfg.SetHelperAnyDocument(first.ID, true)
				a.rt.cfg.SetHelperAnyDocument(second.ID, true)
			}
			calls := 0
			plan := a.helpersForUseWith(document, "search", helperApprovalDialogs{
				setup: func(string, string, string, string, string) (platform.Choice, error) {
					calls++
					return platform.ChoiceRefuse, nil
				},
			})
			if calls != 1 || len(plan.denied) != 1 || len(plan.allowed) != 0 {
				t.Fatalf("initial plan %+v, calls %d", plan, calls)
			}
			s, rel := a.openForTest(t, document)
			a.applyHelperPlan(s, document, plan)
			if state := helperDiscoveryState(t, s, document); state != "denied" {
				t.Fatalf("initial state %q", state)
			}
			switch scenario {
			case "cancel":
				a.refreshHelperDispatchers()
			case "revoke one shared global":
				if outcome := a.programsAction("revoke-trusted", second.ID, "", ""); outcome != "revoked" {
					t.Fatalf("revoke-trusted = %q", outcome)
				}
				a.refreshHelperDispatchers()
			case "navigation reattach":
				if _, ok := a.serveURL(document); !ok {
					t.Fatal("serveURL failed")
				}
			case "ordinary reload":
				if status, _ := fetchNav(t, s.url(rel)); status != 200 {
					t.Fatalf("reload status %d", status)
				}
			}
			state := helperDiscoveryState(t, s, document)
			if state != "denied" {
				t.Errorf("this open's refusal was lost: want denied, got %s", state)
			}
			reopened := 0
			dialogs := helperApprovalDialogs{
				setup: func(string, string, string, string, string) (platform.Choice, error) {
					reopened++
					return platform.ChoiceNone, nil
				},
			}
			plan = a.helpersForOpenWith(document, true, false, dialogs)
			if reopened != 0 {
				t.Errorf("a direct open raised the helper dialog: %d prompts", reopened)
			}
			if len(plan.denied) != 0 {
				t.Errorf("the next direct open inherited this open's refusal: %+v", plan.denied)
			}
			if _, covered := plan.allowed["search"]; covered {
				t.Fatalf("a refused name resolved as allowed on the next open: %+v", plan.allowed)
			}
			clearPageAskQuiet(t, a, document)
			a.helpersForUseWith(document, "search", dialogs)
			if reopened != 1 {
				t.Errorf("the page did not ask again after a direct open: %d prompts", reopened)
			}
		})
	}
}

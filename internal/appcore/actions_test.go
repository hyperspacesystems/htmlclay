package appcore

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func fixtureFacts(t *testing.T, name string) Facts {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "v1", name+".facts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f Facts
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// TestPlanEveryTypeHasAClass reads Plan's own switch, so a case added there without
// a class in ActionClasses fails here, and every class entry reaches a case.
func TestPlanEveryTypeHasAClass(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "actions.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Plan" {
			return true
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if cc, ok := n.(*ast.CaseClause); ok {
				for _, e := range cc.List {
					if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						v, _ := strconv.Unquote(lit.Value)
						cases[v] = true
					}
				}
			}
			return true
		})
		return false
	})
	if len(cases) == 0 {
		t.Fatal("found no case in Plan")
	}
	for typ := range cases {
		if _, ok := ActionClasses[typ]; !ok {
			t.Errorf("Plan handles %q, which has no class", typ)
		}
	}
	for typ := range ActionClasses {
		if !cases[typ] {
			t.Errorf("%q has a class but no case in Plan", typ)
		}
	}
}

func TestPlanClassOfReadsOn(t *testing.T) {
	for _, typ := range []string{"folder.setSynced", "program.setAllTrusted"} {
		if c := ClassOf(Action{Type: typ, Args: json.RawMessage(`{"on":true}`)}); c != ClassGrant {
			t.Errorf("%s on = %v, want grant", typ, c)
		}
		if c := ClassOf(Action{Type: typ, Args: json.RawMessage(`{"on":false}`)}); c != ClassRevoke {
			t.Errorf("%s off = %v, want revoke", typ, c)
		}
	}
	if c := ClassOf(Action{Type: "folder.serve"}); c != ClassGrant {
		t.Errorf("folder.serve = %v, want grant", c)
	}
	if c := ClassOf(Action{Type: "nope"}); c != 0 {
		t.Errorf("unknown type = %v, want 0", c)
	}
}

func TestPlan(t *testing.T) {
	synced := fixtureFacts(t, "syncedWithTeams")
	trusted := fixtureFacts(t, "trustedFolder")
	local := fixtureFacts(t, "localUser")
	newPerson := fixtureFacts(t, "newPerson")
	ready := fixtureFacts(t, "newPerson")
	version := "2.0.4"
	ready.Update.Ready = &version

	cases := []struct {
		name   string
		facts  Facts
		typ    string
		args   string
		effect string // the effect type when accepted
		refuse string // the refusal code when refused
	}{
		{"unknown type", synced, "folder.paint", `{}`, "", RefuseUnknownAction},
		{"unknown argument", synced, "folder.reveal", `{"id":"notes","colour":"red"}`, "", RefuseBadArguments},

		{"open a document", synced, "document.open", `{"id":"todo"}`, "document.open", ""},
		{"open a missing document id", synced, "document.open", `{"id":"nope"}`, "", RefuseNoSuchDocument},
		{"open with no id", synced, "document.open", `{}`, "", RefuseBadArguments},
		{"reveal a document", synced, "document.reveal", `{"id":"todo"}`, "document.reveal", ""},
		{"forget a document", synced, "document.forget", `{"id":"todo"}`, "document.forget", ""},
		{"open the dashboard", synced, "dashboard.open", `{"route":"/folders"}`, "dashboard.open", ""},
		{"reveal a folder", synced, "folder.reveal", `{"id":"notes"}`, "folder.reveal", ""},
		{"reveal an unknown folder", synced, "folder.reveal", `{"id":"nope"}`, "", RefuseNoSuchFolder},
		{"copy a served folder's address", synced, "folder.copyAddress", `{"id":"prototypes"}`, "folder.copyAddress", ""},
		{"copy the address of a folder not served", synced, "folder.copyAddress", `{"id":"notes"}`, "", RefuseNotAllowedNow},
		{"mark seen", synced, "waiting.markSeen", `{"ids":["w-req-todo"]}`, "waiting.markSeen", ""},
		{"mark nothing seen", synced, "waiting.markSeen", `{"ids":[]}`, "", RefuseBadArguments},
		{"set a preference", synced, "prefs.set", `{"key":"aiEditing","value":false}`, "prefs.set", ""},
		{"set appearance", synced, "prefs.set", `{"key":"appearance","value":"dark"}`, "prefs.set", ""},
		{"set appearance to nonsense", synced, "prefs.set", `{"key":"appearance","value":"purple"}`, "", RefuseBadArguments},
		{"set a preference with the wrong type", synced, "prefs.set", `{"key":"aiEditing","value":"yes"}`, "", RefuseBadArguments},
		{"set an unknown preference", synced, "prefs.set", `{"key":"theme","value":true}`, "", RefuseUnknownPreference},
		{"pause sync", synced, "sync.setPaused", `{"paused":true}`, "sync.setPaused", ""},
		{"pause sync with no account", trusted, "sync.setPaused", `{"paused":true}`, "", RefuseNotConnected},
		{"pause with no value", synced, "sync.setPaused", `{}`, "", RefuseBadArguments},
		{"start a ready update", ready, "update.start", `{}`, "update.start", ""},
		{"start an update when none is ready", newPerson, "update.start", `{}`, "", RefuseNothingReady},
		{"restart the service", synced, "service.restart", `{}`, "service.restart", ""},
		{"quit", synced, "host.quit", `{"force":true}`, "host.quit", ""},

		{"stop serving a served folder", synced, "folder.stopServing", `{"id":"prototypes"}`, "folder.stopServing", ""},
		{"stop serving a folder not served", synced, "folder.stopServing", `{"id":"notes"}`, "", RefuseNotAllowedNow},
		{"stop trusting a trusted folder", synced, "folder.stopTrusting", `{"id":"notes"}`, "folder.stopTrusting", ""},
		{"stop trusting a served folder", synced, "folder.stopTrusting", `{"id":"prototypes"}`, "", WhyStopServingFirst},
		{"merge a folder into its parent", trusted, "folder.merge", `{"id":"journal"}`, "folder.merge", ""},
		{"merge a folder with no parent", trusted, "folder.merge", `{"id":"notes"}`, "", RefuseNotAllowedNow},
		{"stop syncing a synced folder", synced, "folder.setSynced", `{"id":"panphora","on":false}`, "folder.setSynced", ""},
		{"stop syncing a folder that does not sync", synced, "folder.setSynced", `{"id":"notes","on":false}`, "", RefuseNotAllowedNow},
		{"sync a local folder", synced, "folder.setSynced", `{"id":"prototypes","on":true}`, "", WhyNotFromAccount},
		{"sync with no on", synced, "folder.setSynced", `{"id":"panphora"}`, "", RefuseBadArguments},
		{"remove a program's access", synced, "program.removeAccess", `{"programID":"todo-search","documentID":"todo"}`, "program.removeAccess", ""},
		{"remove access for an unknown program", synced, "program.removeAccess", `{"programID":"nope","documentID":"todo"}`, "", RefuseNoSuchProgram},
		{"remove access for every document", synced, "program.removeAccess", `{"programID":"todo-search"}`, "program.removeAccess", ""},
		{"turn off all trusted folders for a program", synced, "program.setAllTrusted", `{"programID":"todo-search","on":false}`, "program.setAllTrusted", ""},
		{"set all trusted for an unknown program", synced, "program.setAllTrusted", `{"programID":"nope","on":true}`, "", RefuseNoSuchProgram},
		{"disconnect", synced, "sync.disconnect", `{}`, "sync.disconnect", ""},
		{"disconnect with no account", trusted, "sync.disconnect", `{}`, "", RefuseNotConnected},

		{"trust a folder", synced, "folder.trust", `{"path":"/Users/sam/Recipes"}`, "folder.trust", ""},
		{"trust with no path", synced, "folder.trust", `{}`, "", RefuseBadArguments},
		{"serve a trusted folder by id", synced, "folder.serve", `{"id":"notes","name":"notes"}`, "folder.serve", ""},
		{"serve a folder already served", synced, "folder.serve", `{"id":"prototypes","name":"p"}`, "", RefuseNotAllowedNow},
		{"serve a team folder not on this computer", synced, "folder.serve", `{"id":"marketing"}`, "", WhyNotOnComputer},
		{"serve a new path", synced, "folder.serve", `{"path":"/Users/sam/Sites/blog","name":"blog"}`, "folder.serve", ""},
		{"serve with neither id nor path", synced, "folder.serve", `{"name":"x"}`, "", RefuseBadArguments},
		{"answer a request", synced, "request.answer", `{"id":"req-todo","revision":1,"answer":"allow-document"}`, "request.answer", ""},
		{"answer a request at an old revision", synced, "request.answer", `{"id":"req-todo","revision":0,"answer":"deny"}`, "", RefuseRequestChanged},
		{"answer with an answer not offered", synced, "request.answer", `{"id":"req-todo","revision":1,"answer":"allow"}`, "", RefuseAnswerNotOffered},
		{"answer an ended request", synced, "request.answer", `{"id":"req-gone","revision":1,"answer":"deny"}`, "", RefuseNoSuchRequest},
		{"add a program", synced, "program.add", `{"name":"x","path":"/Users/sam/bin/x"}`, "program.add", ""},
		{"add a program with no path", synced, "program.add", `{"name":"x"}`, "", RefuseBadArguments},
		{"turn on all trusted folders for a program", synced, "program.setAllTrusted", `{"programID":"todo-search","on":true}`, "program.setAllTrusted", ""},
		{"connect", trusted, "sync.connect", `{"username":"sam","key":"hcsk_x"}`, "sync.connect", ""},
		{"connect with no key", trusted, "sync.connect", `{"username":"sam"}`, "", RefuseBadArguments},
		{"connect with no username takes the key's account", trusted, "sync.connect", `{"key":"hcsk_x"}`, "sync.connect", ""},
		{"sync a served account folder", synced, "folder.setSynced", `{"id":"panphora","on":true}`, "", RefuseNotAllowedNow},
		{"set up a team folder", synced, "folder.setUpTeam", `{"id":"marketing","path":"/Users/sam/hyperclay/Marketing"}`, "folder.setUpTeam", ""},
		{"set up a team folder already here", synced, "folder.setUpTeam", `{"id":"panphora"}`, "", RefuseNotAllowedNow},
		{"start the move", local, "move.start", `{"folderIDs":["panphora"]}`, "move.start", ""},
		{"start a move with nothing offered", synced, "move.start", `{}`, "", RefuseNothingToMove},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			effects, refusal := Plan(c.facts, Action{ID: "a1", Type: c.typ, Args: json.RawMessage(c.args)})
			if c.refuse != "" {
				if refusal == nil {
					t.Fatalf("accepted with %+v, want refusal %q", effects, c.refuse)
				}
				if refusal.Code != c.refuse {
					t.Fatalf("refused %q, want %q", refusal.Code, c.refuse)
				}
				if effects != nil {
					t.Errorf("a refusal also returned effects %+v", effects)
				}
				if msg := RefusalMessage(refusal); msg == refusal.Code {
					t.Errorf("refusal %q has no sentence in copy.go", refusal.Code)
				}
				return
			}
			if refusal != nil {
				t.Fatalf("refused %+v, want effect %q", refusal, c.effect)
			}
			if len(effects) != 1 || effects[0].Type != c.effect {
				t.Fatalf("effects = %+v, want one %q", effects, c.effect)
			}
		})
	}
}

func TestPlanRequestAnswersMatchTheSnapshot(t *testing.T) {
	f := fixtureFacts(t, "syncedWithTeams")
	s := Build(f)
	for _, r := range s.Requests {
		if len(r.Answers) == 0 {
			t.Fatalf("request %s offers no answer", r.ID)
		}
		for _, answer := range r.Answers {
			args, _ := json.Marshal(map[string]any{"id": r.ID, "revision": r.Revision, "answer": answer})
			if _, refusal := Plan(f, Action{Type: "request.answer", Args: args}); refusal != nil {
				t.Errorf("answer %q the snapshot offers is refused: %+v", answer, refusal)
			}
		}
	}
	for kind := range requestAnswers {
		if AnswersFor(kind)[0] != "deny" {
			t.Errorf("%s does not offer deny first", kind)
		}
	}
}

// TestPlanRefusalFromAFolderReasonCarriesItsText serves a folder inside a served one:
// the refusal is the rule's own sentence, naming the served folder, not a sentence
// filled in with an empty name.
func TestPlanRefusalFromAFolderReasonCarriesItsText(t *testing.T) {
	f := Facts{Folders: []FolderFact{
		{ID: "work", Name: "work", Path: filepath.FromSlash("/Users/sam/Work"), Origin: Origin{Kind: OriginLocal}, Trusted: true, Served: true, ServedName: "work", OnThisComputer: true},
		{ID: "inner", Name: "inner", Path: filepath.FromSlash("/Users/sam/Work/inner"), Origin: Origin{Kind: OriginLocal}, Trusted: true, OnThisComputer: true},
	}}
	_, refusal := Plan(f, Action{ID: "a1", Type: "folder.serve", Args: json.RawMessage(`{"id":"inner"}`)})
	if refusal == nil || refusal.Code != RefuseOverlapsServed {
		t.Fatalf("Plan = %+v, want an %s refusal", refusal, RefuseOverlapsServed)
	}
	msg := RefusalMessage(refusal)
	if !strings.Contains(msg, "work") || strings.Contains(msg, "overlaps ,") {
		t.Errorf("RefusalMessage = %q, want the reason's sentence naming work", msg)
	}
}

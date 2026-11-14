package appcore

import (
	"encoding/json"
	"slices"
	"testing"
)

// programFactsForPlan is one registered program and one open document, which is
// what the programs actions of the Settings pane read.
func programFactsForPlan() Facts {
	return Facts{
		Programs: []Program{{ID: "p1", Name: "search", Path: "/Users/x/bin/search"}},
		Documents: []DocumentFact{{
			ID:   "d1",
			Name: "one.htmlclay",
			Path: "/Users/x/docs/one.htmlclay",
		}},
	}
}

func planOne(t *testing.T, f Facts, typ, args string) (Effect, *Refusal) {
	t.Helper()
	effects, refusal := Plan(f, Action{Type: typ, Args: json.RawMessage(args)})
	if refusal != nil {
		return Effect{}, refusal
	}
	if len(effects) != 1 {
		t.Fatalf("%s planned %d effects, want one", typ, len(effects))
	}
	return effects[0], nil
}

// TestM3ProgramPlan is step 7.10's pure half: every program action of the Settings
// pane carries what the effect needs, the grants are grants and the removals are
// revokes, and the file warnings a sheet acknowledged travel with the two actions
// that register a program file.
func TestM3ProgramPlan(t *testing.T) {
	t.Run("the classes keep the dashboard from granting", func(t *testing.T) {
		cases := map[string]Class{
			"program.add":           ClassGrant,
			"program.relink":        ClassGrant,
			"program.askAgain":      ClassGrant,
			"program.setAllTrusted": ClassGrant,
			"program.removeAccess":  ClassRevoke,
			"program.remove":        ClassRevoke,
			"program.reveal":        ClassSafe,
		}
		for typ, want := range cases {
			if got := ClassOf(Action{Type: typ, Args: json.RawMessage(`{"on":true}`)}); got != want {
				t.Errorf("%s = %v, want %v", typ, got, want)
			}
		}
		if got := ClassOf(Action{Type: "program.setAllTrusted", Args: json.RawMessage(`{"on":false}`)}); got != ClassRevoke {
			t.Errorf("program.setAllTrusted off = %v, want revoke", got)
		}
	})

	t.Run("add carries the file, the name and what the sheet showed", func(t *testing.T) {
		effect, refusal := planOne(t, programFactsForPlan(), "program.add",
			`{"name":"search","path":"/Users/x/bin/search","acknowledged":["in-downloads"]}`)
		if refusal != nil {
			t.Fatalf("program.add = %+v", refusal)
		}
		if effect.Type != "program.add" || effect.Name != "search" || effect.Path != "/Users/x/bin/search" {
			t.Fatalf("effect = %+v, want the file and name", effect)
		}
		if !slices.Equal(effect.Acknowledged, []string{"in-downloads"}) {
			t.Fatalf("acknowledged = %v, want the code the sheet showed", effect.Acknowledged)
		}
	})

	t.Run("add needs a file and a name", func(t *testing.T) {
		for _, row := range []struct{ args, field string }{
			{`{"name":"search"}`, "path"},
			{`{"path":"/Users/x/bin/search"}`, "name"},
			{`{"name":"Search Me","path":"/Users/x/bin/search"}`, "name"},
		} {
			_, refusal := Plan(programFactsForPlan(), Action{Type: "program.add", Args: json.RawMessage(row.args)})
			if refusal == nil || refusal.Code != RefuseBadArguments || refusal.Field != row.field {
				t.Errorf("program.add %s = %+v, want bad-arguments on %s", row.args, refusal, row.field)
			}
		}
	})

	t.Run("relink keeps the program id and its acknowledged codes", func(t *testing.T) {
		effect, refusal := planOne(t, programFactsForPlan(), "program.relink",
			`{"programID":"p1","path":"/Users/x/bin/search2","acknowledged":["not-executable"]}`)
		if refusal != nil {
			t.Fatalf("program.relink = %+v", refusal)
		}
		if effect.ProgramID != "p1" || effect.Path != "/Users/x/bin/search2" {
			t.Fatalf("effect = %+v, want p1 at the new file", effect)
		}
		if !slices.Equal(effect.Acknowledged, []string{"not-executable"}) {
			t.Fatalf("acknowledged = %v, want not-executable", effect.Acknowledged)
		}
	})

	t.Run("relink names a program that still exists", func(t *testing.T) {
		_, refusal := Plan(programFactsForPlan(), Action{Type: "program.relink", Args: json.RawMessage(`{"programID":"nope","path":"/Users/x/bin/search2"}`)})
		if refusal == nil || refusal.Code != RefuseNoSuchProgram {
			t.Fatalf("relink of a missing program = %+v, want no-such-program", refusal)
		}
		_, refusal = Plan(programFactsForPlan(), Action{Type: "program.relink", Args: json.RawMessage(`{"programID":"p1"}`)})
		if refusal == nil || refusal.Code != RefuseBadArguments || refusal.Field != "path" {
			t.Fatalf("relink with no path = %+v, want bad-arguments on path", refusal)
		}
	})

	t.Run("ask again names the program and the document's path", func(t *testing.T) {
		effect, refusal := planOne(t, programFactsForPlan(), "program.askAgain", `{"programID":"p1","documentID":"/Users/x/docs/one.htmlclay"}`)
		if refusal != nil {
			t.Fatalf("program.askAgain = %+v", refusal)
		}
		if effect.ProgramID != "p1" || effect.DocumentID != "/Users/x/docs/one.htmlclay" || effect.Name != "search" {
			t.Fatalf("effect = %+v, want p1 for the document's path", effect)
		}
		if _, refusal := Plan(programFactsForPlan(), Action{Type: "program.askAgain", Args: json.RawMessage(`{"programID":"p1"}`)}); refusal == nil || refusal.Code != RefuseBadArguments {
			t.Fatalf("ask again with no document = %+v, want bad-arguments", refusal)
		}
		if _, refusal := Plan(programFactsForPlan(), Action{Type: "program.askAgain", Args: json.RawMessage(`{"programID":"nope","documentID":"/Users/x/docs/one.htmlclay"}`)}); refusal == nil || refusal.Code != RefuseNoSuchProgram {
			t.Fatalf("ask again for a missing program = %+v, want no-such-program", refusal)
		}
	})

	t.Run("remove access names one document or every document", func(t *testing.T) {
		one, refusal := planOne(t, programFactsForPlan(), "program.removeAccess", `{"programID":"p1","documentID":"/Users/x/docs/one.htmlclay"}`)
		if refusal != nil || one.DocumentID != "/Users/x/docs/one.htmlclay" || one.ProgramID != "p1" {
			t.Fatalf("one document = %+v, %+v", one, refusal)
		}
		all, refusal := planOne(t, programFactsForPlan(), "program.removeAccess", `{"programID":"p1"}`)
		if refusal != nil || all.DocumentID != "" || all.ProgramID != "p1" {
			t.Fatalf("every document = %+v, %+v", all, refusal)
		}
	})

	t.Run("remove and reveal name one program", func(t *testing.T) {
		for typ, want := range map[string]Class{"program.remove": ClassRevoke, "program.reveal": ClassSafe} {
			effect, refusal := planOne(t, programFactsForPlan(), typ, `{"programID":"p1"}`)
			if refusal != nil || effect.ProgramID != "p1" || effect.Type != typ {
				t.Fatalf("%s = %+v, %+v", typ, effect, refusal)
			}
			if ClassOf(Action{Type: typ}) != want {
				t.Fatalf("%s is not %v", typ, want)
			}
			if _, refusal := Plan(programFactsForPlan(), Action{Type: typ, Args: json.RawMessage(`{"programID":"nope"}`)}); refusal == nil || refusal.Code != RefuseNoSuchProgram {
				t.Fatalf("%s of a missing program = %+v, want no-such-program", typ, refusal)
			}
		}
	})
}

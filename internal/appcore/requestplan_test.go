package appcore

import (
	"encoding/json"
	"slices"
	"testing"
)

// TestM3RequestPlan proves an answer is planned against the answers the request
// itself offers, that the effect carries the revision and the path the registry
// checks, and that the snapshot shows the request's own answers.
func TestM3RequestPlan(t *testing.T) {
	teamDocument := func() Facts {
		return Facts{Requests: []RequestFact{{
			ID:         "r3f09a1c22b7e",
			Kind:       RequestTeamDocument,
			Revision:   4,
			DocumentID: "d41",
			Document:   "notes.htmlclay",
			Program:    "resize",
			Folder:     "Team notes",
			Path:       "/Users/x/bin/resize",
			Answers:    []string{"deny", "allow-document", "choose-file"},
		}}}
	}
	onlyAllowDocument := func() Facts {
		return Facts{Requests: []RequestFact{{
			ID:         "r3f09a1c22b7e",
			Kind:       RequestTeamDocument,
			Revision:   1,
			DocumentID: "d41",
			Document:   "notes.htmlclay",
			Program:    "resize",
			Answers:    []string{"allow-document"},
		}}}
	}

	t.Run("the request's own answers reach the snapshot", func(t *testing.T) {
		snapshot := Build(teamDocument())
		if len(snapshot.Requests) != 1 {
			t.Fatalf("snapshot lists %d requests, want 1", len(snapshot.Requests))
		}
		request := snapshot.Requests[0]
		if want := []string{"deny", "allow-document", "choose-file"}; !slices.Equal(request.Answers, want) {
			t.Fatalf("request answers = %v, want %v", request.Answers, want)
		}
		if request.DocumentID == nil || *request.DocumentID != "d41" || request.Document != "notes.htmlclay" {
			t.Fatalf("request = %+v, want document d41/notes.htmlclay", request)
		}
	})

	t.Run("the kind's own list offers choose-file", func(t *testing.T) {
		for _, kind := range []string{RequestFound, RequestTeamDocument} {
			if !slices.Contains(AnswersFor(kind), "choose-file") {
				t.Errorf("%s offers no choose-file: %v", kind, AnswersFor(kind))
			}
			if AnswersFor(kind)[0] != "deny" {
				t.Errorf("%s does not offer deny first: %v", kind, AnswersFor(kind))
			}
		}
	})

	t.Run("an offered answer plans one effect with its revision", func(t *testing.T) {
		effects, refusal := Plan(teamDocument(), Action{ID: "a17", Type: "request.answer", Args: json.RawMessage(`{"id":"r3f09a1c22b7e","revision":4,"answer":"allow-document"}`)})
		if refusal != nil {
			t.Fatalf("Plan refused an offered answer: %+v", refusal)
		}
		if len(effects) != 1 {
			t.Fatalf("effects = %+v, want one", effects)
		}
		effect := effects[0]
		if effect.Type != "request.answer" || effect.RequestID != "r3f09a1c22b7e" || effect.Revision != 4 || effect.Answer != "allow-document" || effect.Path != "" {
			t.Fatalf("effect = %+v, want request.answer for r3f09a1c22b7e revision 4", effect)
		}
	})

	t.Run("choose-file carries its path", func(t *testing.T) {
		effects, refusal := Plan(teamDocument(), Action{ID: "a18", Type: "request.answer", Args: json.RawMessage(`{"id":"r3f09a1c22b7e","revision":4,"answer":"choose-file","path":"/Users/x/bin/resize"}`)})
		if refusal != nil {
			t.Fatalf("Plan refused choose-file with a path: %+v", refusal)
		}
		if len(effects) != 1 || effects[0].Path != "/Users/x/bin/resize" || effects[0].Answer != "choose-file" {
			t.Fatalf("effects = %+v, want one choose-file with the path", effects)
		}
	})

	t.Run("a forged answer is refused", func(t *testing.T) {
		effects, refusal := Plan(onlyAllowDocument(), Action{ID: "a19", Type: "request.answer", Args: json.RawMessage(`{"id":"r3f09a1c22b7e","revision":1,"answer":"deny"}`)})
		if refusal == nil || refusal.Code != RefuseAnswerNotOffered {
			t.Fatalf("Plan of a forged answer = %+v %+v, want %s", effects, refusal, RefuseAnswerNotOffered)
		}
	})

	t.Run("a request that changed is refused", func(t *testing.T) {
		_, refusal := Plan(teamDocument(), Action{ID: "a20", Type: "request.answer", Args: json.RawMessage(`{"id":"r3f09a1c22b7e","revision":3,"answer":"deny"}`)})
		if refusal == nil || refusal.Code != RefuseRequestChanged {
			t.Fatalf("Plan of an old revision = %+v, want %s", refusal, RefuseRequestChanged)
		}
	})

	t.Run("an ended request is refused", func(t *testing.T) {
		_, refusal := Plan(teamDocument(), Action{ID: "a21", Type: "request.answer", Args: json.RawMessage(`{"id":"r-gone","revision":4,"answer":"deny"}`)})
		if refusal == nil || refusal.Code != RefuseNoSuchRequest {
			t.Fatalf("Plan of an ended request = %+v, want %s", refusal, RefuseNoSuchRequest)
		}
	})
}

package appcore

import (
	"encoding/json"
	"testing"
)

// mergeFacts is one home with a folder and a folder inside it that keeps its own
// address.
func mergeFacts() Facts {
	local := Origin{Kind: OriginLocal}
	return Facts{Folders: []FolderFact{
		{ID: "notes", Name: "Notes", Path: "/Users/sam/Documents/Notes", Origin: local, Trusted: true, OnThisComputer: true},
		{ID: "journal", Name: "Journal", Path: "/Users/sam/Documents/Notes/Journal", Origin: local, Trusted: true, OnThisComputer: true, ParentID: "notes", OwnAddress: true},
	}}
}

func TestM3MergePlan(t *testing.T) {
	t.Run("a folder that keeps its own address merges into its parent", func(t *testing.T) {
		f := mergeFacts()
		effects, refusal := Plan(f, Action{ID: "a1", Type: "folder.merge", Args: json.RawMessage(`{"id":"journal"}`)})
		if refusal != nil {
			t.Fatalf("merge refused: %+v", refusal)
		}
		if len(effects) != 1 || effects[0].Type != "folder.merge" || effects[0].FolderID != "journal" || effects[0].ParentID != "notes" {
			t.Fatalf("effects = %+v, want one folder.merge of journal into notes", effects)
		}
		if got := ClassOf(Action{Type: "folder.merge"}); got != ClassGrant {
			t.Fatalf("folder.merge class = %v, want grant: the dashboard may not send it", got)
		}
	})

	t.Run("a folder that uses its parent's address cannot merge", func(t *testing.T) {
		f := mergeFacts()
		f.Folders[1].OwnAddress = false
		f.Folders[1].ParentID = ""
		_, refusal := Plan(f, Action{ID: "a1", Type: "folder.merge", Args: json.RawMessage(`{"id":"journal"}`)})
		if refusal == nil || refusal.Code != RefuseNotAllowedNow {
			t.Fatalf("merge = %+v, want %s without a parent", refusal, RefuseNotAllowedNow)
		}
		// A parent that is still recorded, but the child keeps no address of its own.
		f.Folders[1].ParentID = "notes"
		_, refusal = Plan(f, Action{ID: "a2", Type: "folder.merge", Args: json.RawMessage(`{"id":"journal"}`)})
		if refusal == nil || refusal.Code != WhyNotOwnAddress {
			t.Fatalf("merge = %+v, want %s", refusal, WhyNotOwnAddress)
		}
		if refusal.Text == "" {
			t.Fatal("the refusal carries no sentence")
		}
	})

	t.Run("a team folder is refused on either side", func(t *testing.T) {
		f := mergeFacts()
		f.Folders[1].Team = true
		if _, refusal := Plan(f, Action{ID: "a1", Type: "folder.merge", Args: json.RawMessage(`{"id":"journal"}`)}); refusal == nil || refusal.Code != WhyNotLocal {
			t.Fatalf("merging a team folder = %+v, want %s", refusal, WhyNotLocal)
		}
		f = mergeFacts()
		f.Folders[0].Team = true
		if _, refusal := Plan(f, Action{ID: "a2", Type: "folder.merge", Args: json.RawMessage(`{"id":"journal"}`)}); refusal == nil || refusal.Code != WhyNotLocal {
			t.Fatalf("merging into a team folder = %+v, want %s", refusal, WhyNotLocal)
		}
		f = mergeFacts()
		f.Folders[1].Origin = Origin{Kind: OriginTeam, Team: "Design"}
		if _, refusal := Plan(f, Action{ID: "a3", Type: "folder.merge", Args: json.RawMessage(`{"id":"journal"}`)}); refusal == nil || refusal.Code != WhyNotLocal {
			t.Fatalf("merging a folder from a team = %+v, want %s", refusal, WhyNotLocal)
		}
	})

	t.Run("a served folder cannot merge", func(t *testing.T) {
		f := mergeFacts()
		f.Folders[1].Served = true
		_, refusal := Plan(f, Action{ID: "a1", Type: "folder.merge", Args: json.RawMessage(`{"id":"journal"}`)})
		if refusal == nil || refusal.Code != WhyStopServingFirst {
			t.Fatalf("merging a served folder = %+v, want %s", refusal, WhyStopServingFirst)
		}
	})

	t.Run("the rules say every reason it can give", func(t *testing.T) {
		f := mergeFacts()
		can, why := folderRules(f, f.Folders[1], folderState(f.Folders[1]))
		if !can.Merge || why.Merge != nil {
			t.Fatalf("can.Merge = %v why = %+v, want true and no reason", can.Merge, why.Merge)
		}
		f.Folders[1].OwnAddress = false
		can, why = folderRules(f, f.Folders[1], folderState(f.Folders[1]))
		if can.Merge || why.Merge == nil || why.Merge.Code != WhyNotOwnAddress || why.Merge.Text == "" {
			t.Fatalf("can.Merge = %v why = %+v, want false with a %s reason that has text", can.Merge, why.Merge, WhyNotOwnAddress)
		}
	})
}

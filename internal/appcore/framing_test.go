package appcore

import (
	"encoding/json"
	"testing"
)

func framingFacts(ff FolderFact) Facts {
	if ff.ID == "" {
		ff.ID = "notes"
	}
	ff.Trusted = true
	return Facts{Folders: []FolderFact{ff}}
}

func localServedFraming() FolderFact {
	return FolderFact{ID: "notes", Name: "Notes", Path: "/Users/sam/Notes", Origin: Origin{Kind: OriginLocal},
		Served: true, ServedName: "notes", OnThisComputer: true}
}

// TestM3FramingPlan is step 7.14's pure half: allowing frames is a grant the Mac
// app sends and refusing them again is a revoke, a folder the host serves on this
// computer may be switched either way, and a team folder may not allow frames.
func TestM3FramingPlan(t *testing.T) {
	t.Run("allowing frames is a grant and refusing them is a revoke", func(t *testing.T) {
		if got := ClassOf(Action{Type: "folder.setFraming", Args: json.RawMessage(`{"id":"notes","allow":true}`)}); got != ClassGrant {
			t.Errorf("folder.setFraming allow = %v, want grant", got)
		}
		if got := ClassOf(Action{Type: "folder.setFraming", Args: json.RawMessage(`{"id":"notes","allow":false}`)}); got != ClassRevoke {
			t.Errorf("folder.setFraming refuse = %v, want revoke", got)
		}
	})

	t.Run("a served folder on this computer carries the choice", func(t *testing.T) {
		effect, refusal := planOne(t, framingFacts(localServedFraming()), "folder.setFraming", `{"id":"notes","allow":true}`)
		if refusal != nil {
			t.Fatalf("allowing frames = %+v", refusal)
		}
		if effect.Type != "folder.setFraming" || effect.FolderID != "notes" || !effect.On {
			t.Errorf("effect = %+v, want folder.setFraming for notes with On", effect)
		}
		effect, refusal = planOne(t, framingFacts(localServedFraming()), "folder.setFraming", `{"id":"notes","allow":false}`)
		if refusal != nil {
			t.Fatalf("refusing frames = %+v", refusal)
		}
		if effect.Type != "folder.setFraming" || effect.FolderID != "notes" || effect.On {
			t.Errorf("effect = %+v, want folder.setFraming for notes without On", effect)
		}
	})

	t.Run("the switch says whether the folder may allow frames", func(t *testing.T) {
		served := localServedFraming()
		can, _ := folderRules(framingFacts(served), served, folderState(served))
		if !can.SetFraming {
			t.Errorf("a served folder on this computer cannot set framing: %+v", can)
		}
		stopped := served
		stopped.Served = false
		can, _ = folderRules(framingFacts(stopped), stopped, folderState(stopped))
		if can.SetFraming {
			t.Errorf("a folder that is not served can set framing: %+v", can)
		}
		team := served
		team.Team = true
		can, _ = folderRules(framingFacts(team), team, folderState(team))
		if can.SetFraming {
			t.Errorf("a team folder can set framing: %+v", can)
		}
	})

	t.Run("a team folder is refused", func(t *testing.T) {
		team := localServedFraming()
		team.Team = true
		if _, refusal := planOne(t, framingFacts(team), "folder.setFraming", `{"id":"notes","allow":true}`); refusal == nil || refusal.Code != RefuseNotAllowedNow {
			t.Fatalf("allowing frames for a team folder = %+v, want not-allowed-now", refusal)
		}
		teamOnly := localServedFraming()
		teamOnly.Origin = Origin{Kind: OriginTeam, Team: "Design"}
		if _, refusal := planOne(t, framingFacts(teamOnly), "folder.setFraming", `{"id":"notes","allow":true}`); refusal == nil || refusal.Code != RefuseNotAllowedNow {
			t.Fatalf("allowing frames for a team folder by origin = %+v, want not-allowed-now", refusal)
		}
	})

	t.Run("a folder that is not served is refused", func(t *testing.T) {
		stopped := localServedFraming()
		stopped.Served = false
		if _, refusal := planOne(t, framingFacts(stopped), "folder.setFraming", `{"id":"notes","allow":true}`); refusal == nil || refusal.Code != RefuseNotAllowedNow {
			t.Fatalf("allowing frames for a folder that is not served = %+v, want not-allowed-now", refusal)
		}
	})

	t.Run("a missing choice and an unknown folder are refused", func(t *testing.T) {
		f := framingFacts(localServedFraming())
		if _, refusal := planOne(t, f, "folder.setFraming", `{"id":"notes"}`); refusal == nil || refusal.Code != RefuseBadArguments || refusal.Field != "allow" {
			t.Fatalf("folder.setFraming with no allow = %+v, want bad arguments on allow", refusal)
		}
		if _, refusal := planOne(t, f, "folder.setFraming", `{"id":"gone","allow":true}`); refusal == nil || refusal.Code != RefuseNoSuchFolder {
			t.Fatalf("folder.setFraming for an unknown folder = %+v, want no-such-folder", refusal)
		}
	})

	t.Run("the snapshot shows the stored choice and the switch", func(t *testing.T) {
		facts := framingFacts(localServedFraming())
		facts.Folders[0].AllowsFraming = true
		snapshot := Build(facts)
		if len(snapshot.Folders) != 1 {
			t.Fatalf("the snapshot lists %d folders, want one", len(snapshot.Folders))
		}
		if !snapshot.Folders[0].AllowsFraming || !snapshot.Folders[0].Can.SetFraming {
			t.Fatalf("the folder = %+v, want framing allowed and settable", snapshot.Folders[0])
		}
	})
}

package appcore

import (
	"slices"
	"testing"
)

// TestM3TrustPlan is step 7.45's pure half: folder.trust carries what the Trust
// sheet showed, so the effect can tell an accepted warning from none.
func TestM3TrustPlan(t *testing.T) {
	effect, refusal := planOne(t, Facts{}, "folder.trust", `{"path":"/Users/x/notes/drafts","acknowledged":["inside-trusted"]}`)
	if refusal != nil {
		t.Fatalf("folder.trust = %+v", refusal)
	}
	if effect.Type != "folder.trust" || effect.Path != "/Users/x/notes/drafts" {
		t.Fatalf("effect = %+v, want the folder", effect)
	}
	if !slices.Equal(effect.Acknowledged, []string{"inside-trusted"}) {
		t.Fatalf("acknowledged = %v, want the code the sheet showed", effect.Acknowledged)
	}

	plain, refusal := planOne(t, Facts{}, "folder.trust", `{"path":"/Users/x/notes"}`)
	if refusal != nil {
		t.Fatalf("folder.trust with nothing acknowledged = %+v", refusal)
	}
	if len(plain.Acknowledged) != 0 {
		t.Fatalf("acknowledged = %v, want none", plain.Acknowledged)
	}
}

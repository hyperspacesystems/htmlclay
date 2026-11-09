package dataguard

import "github.com/panphora/htmlclay/internal/dataapi"

type Provenance string

const (
	External     Provenance = "external"
	UIBackground Provenance = "ui-background"
	UIGestured   Provenance = "ui-gestured"
	UIUnknown    Provenance = "ui-unknown"
)

type LossSummary struct {
	Provenance     Provenance `json:"provenance"`
	DestroyedAtoms int        `json:"destroyedAtoms"`
	RemovedItems   int        `json:"removedItems"`
	ModifiedAtoms  int        `json:"modifiedAtoms"`
	Paths          []string   `json:"paths"`
}

func ShouldFire(prov Provenance, uiWorkPending bool, base, inc dataapi.Value, d *Destruction) bool {
	if base == nil || IsEmptyIsland(base) {
		return false
	}
	if inc == nil {
		return true
	}

	if prov == External && d.AnyDestruction {
		return true
	}
	if prov == External && d.ModifiedAtoms >= 1 && uiWorkPending {
		return true
	}

	if prov == UIBackground && d.AnyDestruction {
		return true
	}

	if d.RemovedItems >= 2 {
		return true
	}
	if d.ItemWipes >= 1 {
		return true
	}
	if d.HugeWipes >= 1 {
		return true
	}
	if d.DestroyedLeaves >= 3 && d.LeafFrac >= 0.30 && d.MassFrac >= 0.25 {
		return true
	}
	if d.PlaceholderResets >= 3 && d.ResetFrac >= 0.30 {
		return true
	}
	if d.LeafCount <= 3 && d.DestroyedLeaves > 0 && d.DestroyedLeaves == d.MeaningfulCount {
		return true
	}

	return false
}

func UIProvenanceOf(userDriven *bool) Provenance {
	if userDriven != nil {
		if *userDriven {
			return UIGestured
		}
		return UIBackground
	}
	return UIUnknown
}

func LossSummaryOf(prov Provenance, d *Destruction) LossSummary {
	paths := []string{}
	if d != nil {
		n := len(d.Paths)
		if n > 12 {
			n = 12
		}
		paths = append(paths, d.Paths[:n]...)
	}
	summary := LossSummary{Provenance: prov, Paths: paths}
	if d != nil {
		summary.DestroyedAtoms = d.DestroyedLeaves + d.RemovedItems + d.ItemWipes + d.HugeWipes
		summary.RemovedItems = d.RemovedItems
		summary.ModifiedAtoms = d.ModifiedAtoms
	}
	return summary
}

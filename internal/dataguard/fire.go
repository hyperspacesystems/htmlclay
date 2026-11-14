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

func ShouldFire(prov Provenance, uiWorkPending bool, base, inc dataapi.Value, d *Destruction, provenReader bool) bool {
	if base == nil || IsEmptyIsland(base) {
		return false
	}
	if inc == nil {
		return true
	}

	// A program that named the exact version it read meant its change. Only a
	// near-total wipe still speaks (David, 2026-11-13): 90% or more of the data's
	// values gone, counted across the whole data so removed list items count.
	if provenReader {
		return d.AnyDestruction && nearTotalWipe(base, inc)
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
	if d.LeafCount <= 3 && d.DestroyedLeaves-d.CodecDestroyedLeaves > 0 && d.DestroyedLeaves == d.LeafCount {
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

// nearTotalWipe reports whether the write kept no more than a tenth of the values
// that were there, counting every value inside lists and objects, so removed list
// items count as much as cleared fields.
func nearTotalWipe(base, inc dataapi.Value) bool {
	before := dataValueCount(base)
	if before == 0 {
		return false
	}
	removed := before - dataValueCount(inc)
	return float64(removed)/float64(before) >= NearTotalWipe
}

// dataValueCount counts an island's values, leaving out the guard's own top-level
// bookkeeping (the codec declarations and the stored-form marker).
func dataValueCount(island dataapi.Value) int {
	obj, ok := island.(*dataapi.Object)
	if !ok {
		return valueCount(island)
	}
	n := 0
	for _, k := range jsKeys(obj) {
		if k == CodecsKey || k == FormKey {
			continue
		}
		val, _ := obj.Get(k)
		n += valueCount(val)
	}
	return n
}

// valueCount is the number of values a person could have typed: every non-blank
// scalar, at any depth.
func valueCount(v dataapi.Value) int {
	switch t := v.(type) {
	case nil:
		return 0
	case string:
		if Meaningful(t) {
			return 1
		}
		return 0
	case []dataapi.Value:
		n := 0
		for _, item := range t {
			n += valueCount(item)
		}
		return n
	case *dataapi.Object:
		n := 0
		for _, k := range jsKeys(t) {
			val, _ := t.Get(k)
			n += valueCount(val)
		}
		return n
	}
	return 1
}

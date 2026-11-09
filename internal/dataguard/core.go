package dataguard

import (
	"strings"

	"github.com/panphora/htmlclay/internal/dataapi"
)

const (
	FuzzyMatchThreshold = 0.45
	ItemWipeFraction    = 0.8
	HugeScalarBytes     = 1024

	hugeScalarKeepFraction = 0.2
	hugeScalarSim          = 0.4
	fuzzyPairBudget        = 2000
)

var placeholderValues = map[string]bool{
	"untitled":       true,
	"untitled draft": true,
	"new item":       true,
	"new":            true,
	"todo":           true,
	"to do":          true,
	"click to edit":  true,
	"enter text":     true,
	"placeholder":    true,
	"example":        true,
}

func IsPlaceholder(v dataapi.Value) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	t := jsLower(jsTrim(s))
	if t == "" {
		return false
	}
	if placeholderValues[t] {
		return true
	}
	return strings.HasPrefix(t, "lorem ipsum")
}

func Meaningful(v dataapi.Value) bool {
	if v == nil {
		return false
	}
	switch t := v.(type) {
	case string:
		return jsLen(jsTrim(t)) > 0
	case []dataapi.Value:
		return len(t) > 0
	case *dataapi.Object:
		return len(jsKeys(t)) > 0
	}
	return true
}

func Mass(v dataapi.Value) int {
	switch t := v.(type) {
	case nil:
		return 0
	case string:
		return jsLen(jsTrim(t))
	case []dataapi.Value:
		total := 0
		for _, item := range t {
			total += Mass(item)
		}
		return total
	case *dataapi.Object:
		total := 0
		for _, k := range jsKeys(t) {
			val, _ := t.Get(k)
			total += Mass(val)
		}
		return total
	}
	return 0
}

func isScalar(v dataapi.Value) bool {
	if v == nil {
		return true
	}
	_, ok := v.(string)
	return ok
}

func isPlainObject(v dataapi.Value) bool {
	_, ok := v.(*dataapi.Object)
	return ok
}

// An island is empty when it is null/absent or carries no meaningful value.
func IsEmptyIsland(island dataapi.Value) bool {
	switch t := island.(type) {
	case []dataapi.Value:
		for _, item := range t {
			if Meaningful(item) {
				return false
			}
		}
		return true
	case *dataapi.Object:
		for _, k := range jsKeys(t) {
			val, _ := t.Get(k)
			if Meaningful(val) {
				return false
			}
		}
		return true
	}
	return true
}

func bigrams(str string) map[[2]uint16]int {
	s := codeUnits(str)
	out := map[[2]uint16]int{}
	for i := 0; i < len(s)-1; i++ {
		out[[2]uint16{s[i], s[i+1]}]++
	}
	return out
}

func DiceCoefficient(a, b string) float64 {
	if a == b {
		return 1
	}
	la, lb := jsLen(a), jsLen(b)
	if la < 2 || lb < 2 {
		return 0
	}
	ba := bigrams(a)
	bb := bigrams(b)
	intersection := 0
	for g, count := range ba {
		if other, ok := bb[g]; ok {
			if count < other {
				intersection += count
			} else {
				intersection += other
			}
		}
	}
	return float64(2*intersection) / float64(la-1+lb-1)
}

// Concatenated, lowercased token string of an item's scalar leaves (recursive).
func tokenString(item dataapi.Value) string {
	var parts []string
	var collect func(v dataapi.Value)
	collect = func(v dataapi.Value) {
		switch t := v.(type) {
		case nil:
			return
		case string:
			parts = append(parts, t)
		case []dataapi.Value:
			for _, item := range t {
				collect(item)
			}
		case *dataapi.Object:
			for _, k := range jsKeys(t) {
				val, _ := t.Get(k)
				collect(val)
			}
		}
	}
	collect(item)
	return jsTrim(jsLower(strings.Join(parts, " ")))
}

func ItemSimilarity(a, b dataapi.Value) float64 {
	ta := tokenString(a)
	tb := tokenString(b)
	if ta == tb {
		return 1
	}
	tokensA := jsFields(ta)
	tokensB := jsFields(tb)
	biA := tokenBigrams(tokensA)
	biB := tokenBigrams(tokensB)
	if len(biA) < 1 || len(biB) < 1 {
		return DiceCoefficient(ta, tb)
	}
	intersection := 0
	for g := range biA {
		if _, ok := biB[g]; ok {
			intersection++
		}
	}
	return float64(2*intersection) / float64(len(biA)+len(biB))
}

func tokenBigrams(tokens []string) map[string]struct{} {
	set := map[string]struct{}{}
	for i := 0; i < len(tokens)-1; i++ {
		set[tokens[i]+"\x00"+tokens[i+1]] = struct{}{}
	}
	return set
}

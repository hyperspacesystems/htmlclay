package dataguard

import (
	"sort"
	"strconv"
	"strings"

	"github.com/panphora/htmlclay/internal/dataapi"
)

type Destruction struct {
	DestroyedLeaves      int      `json:"destroyedLeaves"`
	RemovedItems         int      `json:"removedItems"`
	ItemWipes            int      `json:"itemWipes"`
	HugeWipes            int      `json:"hugeWipes"`
	PlaceholderResets    int      `json:"placeholderResets"`
	ModifiedAtoms        int      `json:"modifiedAtoms"`
	LeafCount            int      `json:"leafCount"`
	CodecLeaves          int      `json:"codecLeaves"`
	CodecDestroyedLeaves int      `json:"codecDestroyedLeaves"`
	MeaningfulCount      int      `json:"meaningfulCount"`
	DestroyedMass        int      `json:"destroyedMass"`
	TotalMass            int      `json:"totalMass"`
	LeafFrac             float64  `json:"leafFrac"`
	MassFrac             float64  `json:"massFrac"`
	ResetFrac            float64  `json:"resetFrac"`
	AnyDestruction       bool     `json:"anyDestruction"`
	FuzzyFallback        bool     `json:"fuzzyFallback"`
	Paths                []string `json:"paths"`
}

var idKeys = []string{"id", "_id", "key", "uid", "slug"}

type pathSeg struct {
	key     string
	index   int
	isIndex bool
}

type itemPair struct {
	base int
	inc  int
}

type scoredPair struct {
	sim  float64
	base int
	inc  int
}

func ClassifyDestruction(base, inc dataapi.Value) *Destruction {
	D := &Destruction{Paths: []string{}}
	for _, key := range objectKeyList(base) {
		if key == CodecsKey || key == FormKey {
			continue
		}
		baseVal, _ := objectLookup(base, key)
		incVal, _ := objectLookup(inc, key)
		was, now := codecDeclarations(base, key), codecDeclarations(inc, key)
		if (was != nil || now != nil) && isPlainObject(baseVal) {
			walkData(baseVal, incVal, D, []pathSeg{{key: key}}, was, now)
		} else {
			walk(baseVal, incVal, D, []pathSeg{{key: key}})
		}
	}
	if D.LeafCount > 0 {
		D.LeafFrac = float64(D.DestroyedLeaves) / float64(D.LeafCount)
		D.ResetFrac = float64(D.PlaceholderResets) / float64(D.LeafCount)
	}
	if D.TotalMass > 0 {
		D.MassFrac = float64(D.DestroyedMass) / float64(D.TotalMass)
	}
	D.MeaningfulCount = D.LeafCount
	D.AnyDestruction = D.DestroyedLeaves+D.RemovedItems+D.ItemWipes+D.HugeWipes > 0
	return D
}

func objectKeyList(v dataapi.Value) []string {
	switch t := v.(type) {
	case []dataapi.Value:
		keys := make([]string, len(t))
		for i := range t {
			keys[i] = strconv.Itoa(i)
		}
		return keys
	case *dataapi.Object:
		return jsKeys(t)
	}
	return nil
}

func arrayIndexKey(key string) (int, bool) {
	if key == "" || (len(key) > 1 && key[0] == '0') {
		return 0, false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < '0' || key[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(key)
	if err != nil {
		return 0, false
	}
	return n, true
}

func objectLookup(v dataapi.Value, key string) (dataapi.Value, bool) {
	switch t := v.(type) {
	case []dataapi.Value:
		i, ok := arrayIndexKey(key)
		if !ok || i >= len(t) {
			return nil, false
		}
		return t[i], true
	case *dataapi.Object:
		val, ok := t.Get(key)
		return val, ok
	}
	return nil, false
}

func pathWith(path []pathSeg, seg pathSeg) []pathSeg {
	out := make([]pathSeg, len(path)+1)
	copy(out, path)
	out[len(path)] = seg
	return out
}

func keySeg(key string) pathSeg { return pathSeg{key: key} }

func indexSeg(i int) pathSeg { return pathSeg{index: i, isIndex: true} }

func pathStr(path []pathSeg) string {
	parts := make([]string, len(path))
	for i, seg := range path {
		if seg.isIndex {
			parts[i] = "[" + strconv.Itoa(seg.index) + "]"
		} else {
			parts[i] = seg.key
		}
	}
	return strings.ReplaceAll(strings.Join(parts, "."), ".[", "[")
}

func recordLeaf(D *Destruction, path []pathSeg) {
	if len(D.Paths) < 24 {
		D.Paths = append(D.Paths, pathStr(path))
	}
}

func walk(baseVal, incVal dataapi.Value, D *Destruction, path []pathSeg) {
	if !Meaningful(baseVal) {
		return
	}

	if isScalar(baseVal) {
		m := Mass(baseVal)
		D.LeafCount++
		D.TotalMass += m

		if !Meaningful(incVal) {
			D.DestroyedLeaves++
			D.DestroyedMass += m
			recordLeaf(D, path)
			return
		}

		baseStr, baseIsStr := baseVal.(string)
		incStr, incIsStr := incVal.(string)
		baseTrimLen := 0
		if baseIsStr {
			baseTrimLen = jsLen(jsTrim(baseStr))
		}
		if baseIsStr && incIsStr &&
			baseTrimLen >= HugeScalarBytes &&
			float64(jsLen(jsTrim(incStr))) < float64(baseTrimLen)*hugeScalarKeepFraction &&
			DiceCoefficient(jsTrim(baseStr), jsTrim(incStr)) < hugeScalarSim {
			D.HugeWipes++
			destroyed := m - Mass(incVal)
			if destroyed > 0 {
				D.DestroyedMass += destroyed
			}
			recordLeaf(D, path)
			return
		}

		if IsPlaceholder(incVal) && !IsPlaceholder(baseVal) {
			D.PlaceholderResets++
			recordLeaf(D, path)
			return
		}

		if !sameScalar(baseVal, incVal) {
			D.ModifiedAtoms++
		}
		return
	}

	if baseArr, ok := baseVal.([]dataapi.Value); ok {
		incArr, _ := incVal.([]dataapi.Value)
		if incArr == nil {
			incArr = []dataapi.Value{}
		}
		if looksLikeItemList(baseArr) {
			walkItemList(baseArr, incArr, D, path)
		} else {
			walkScalarArray(baseArr, incArr, D, path)
		}
		return
	}

	if baseObj, ok := baseVal.(*dataapi.Object); ok {
		incObj, _ := incVal.(*dataapi.Object)
		for _, key := range jsKeys(baseObj) {
			baseChild, _ := baseObj.Get(key)
			var incChild dataapi.Value
			if incObj != nil {
				incChild, _ = incObj.Get(key)
			}
			walk(baseChild, incChild, D, pathWith(path, keySeg(key)))
		}
	}
}

func sameScalar(a, b dataapi.Value) bool {
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		return jsTrim(as) == jsTrim(bs)
	}
	return a == nil && b == nil
}

func walkScalarArray(baseArr, incArr []dataapi.Value, D *Destruction, path []pathSeg) {
	incCounts := map[string]int{}
	for _, v := range incArr {
		if !Meaningful(v) {
			continue
		}
		incCounts[scalarKey(v)]++
	}
	for i, v := range baseArr {
		if !Meaningful(v) {
			continue
		}
		m := Mass(v)
		D.LeafCount++
		D.TotalMass += m
		k := scalarKey(v)
		if incCounts[k] > 0 {
			incCounts[k]--
		} else {
			D.DestroyedLeaves++
			D.DestroyedMass += m
			recordLeaf(D, pathWith(path, indexSeg(i)))
		}
	}
}

func scalarKey(v dataapi.Value) string {
	if s, ok := v.(string); ok {
		return "s:" + jsTrim(s)
	}
	return "object:" + jsToString(v)
}

func jsToString(v dataapi.Value) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	case []dataapi.Value:
		parts := make([]string, len(t))
		for i, item := range t {
			if item == nil {
				continue
			}
			parts[i] = jsToString(item)
		}
		return strings.Join(parts, ",")
	}
	return "[object Object]"
}

func looksLikeItemList(arr []dataapi.Value) bool {
	if len(arr) == 0 {
		return false
	}
	for _, item := range arr {
		if !isPlainObject(item) {
			return false
		}
	}
	return true
}

func itemID(item dataapi.Value) (string, bool) {
	obj, ok := item.(*dataapi.Object)
	if !ok {
		return "", false
	}
	for _, k := range idKeys {
		v, seen := obj.Get(k)
		if !seen {
			continue
		}
		switch n := v.(type) {
		case string:
			if jsTrim(n) != "" {
				return k + ":" + n, true
			}
		case float64:
			return k + ":" + strconv.FormatFloat(n, 'f', -1, 64), true
		case int:
			return k + ":" + strconv.FormatFloat(float64(n), 'f', -1, 64), true
		}
	}
	return "", false
}

func walkItemList(baseArr, incArr []dataapi.Value, D *Destruction, path []pathSeg) {
	n := len(baseArr)
	m := len(incArr)

	baseMatched := make([]bool, n)
	incMatched := make([]bool, m)
	var pairs []itemPair

	incByID := map[string]int{}
	for j := 0; j < m; j++ {
		if id, ok := itemID(incArr[j]); ok {
			incByID[id] = j
		}
	}
	for i := 0; i < n; i++ {
		id, ok := itemID(baseArr[i])
		if !ok {
			continue
		}
		j, ok := incByID[id]
		if ok && !incMatched[j] {
			baseMatched[i] = true
			incMatched[j] = true
			pairs = append(pairs, itemPair{i, j})
		}
	}

	incByDigest := map[string][]int{}
	for j := 0; j < m; j++ {
		if incMatched[j] {
			continue
		}
		d := stableDigest(incArr[j])
		incByDigest[d] = append(incByDigest[d], j)
	}
	for i := 0; i < n; i++ {
		if baseMatched[i] {
			continue
		}
		d := stableDigest(baseArr[i])
		bucket := incByDigest[d]
		if len(bucket) > 0 {
			j := bucket[0]
			incByDigest[d] = bucket[1:]
			baseMatched[i] = true
			incMatched[j] = true
			pairs = append(pairs, itemPair{i, j})
		}
	}

	var baseLeft, incLeft []int
	for i := range baseArr {
		if !baseMatched[i] {
			baseLeft = append(baseLeft, i)
		}
	}
	for i := range incArr {
		if !incMatched[i] {
			incLeft = append(incLeft, i)
		}
	}
	if len(baseLeft)*len(incLeft) > fuzzyPairBudget {
		D.FuzzyFallback = true
		for k := 0; k < min(len(baseLeft), len(incLeft)); k++ {
			i, j := baseLeft[k], incLeft[k]
			baseMatched[i], incMatched[j] = true, true
			pairs = append(pairs, itemPair{i, j})
		}
	} else {

		var candidates []scoredPair
		for i := 0; i < n; i++ {
			if baseMatched[i] {
				continue
			}
			for j := 0; j < m; j++ {
				if incMatched[j] {
					continue
				}
				sim := ItemSimilarity(baseArr[i], incArr[j])
				if sim >= FuzzyMatchThreshold {
					candidates = append(candidates, scoredPair{sim, i, j})
				}
			}
		}
		sort.SliceStable(candidates, func(a, b int) bool { return candidates[a].sim > candidates[b].sim })
		for _, c := range candidates {
			if baseMatched[c.base] || incMatched[c.inc] {
				continue
			}
			baseMatched[c.base] = true
			incMatched[c.inc] = true
			pairs = append(pairs, itemPair{c.base, c.inc})
		}

		var leftover []scoredPair
		for i := 0; i < n; i++ {
			if baseMatched[i] {
				continue
			}
			if _, ok := itemID(baseArr[i]); ok {
				continue
			}
			for j := 0; j < m; j++ {
				if incMatched[j] {
					continue
				}
				if _, ok := itemID(incArr[j]); ok {
					continue
				}
				leftover = append(leftover, scoredPair{ItemSimilarity(baseArr[i], incArr[j]), i, j})
			}
		}
		sort.SliceStable(leftover, func(a, b int) bool { return leftover[a].sim > leftover[b].sim })
		for _, c := range leftover {
			if baseMatched[c.base] || incMatched[c.inc] {
				continue
			}
			baseMatched[c.base] = true
			incMatched[c.inc] = true
			pairs = append(pairs, itemPair{c.base, c.inc})
		}

	}

	for _, p := range pairs {
		classifyMatchedItem(baseArr[p.base], incArr[p.inc], D, pathWith(path, indexSeg(p.base)))
	}

	unmatchedBase := 0
	for i := 0; i < n; i++ {
		if !baseMatched[i] {
			unmatchedBase++
		}
	}
	D.RemovedItems += unmatchedBase
}

func classifyMatchedItem(baseItem, incItem dataapi.Value, D *Destruction, path []pathSeg) {
	baseMass := Mass(baseItem)
	if baseMass > 0 {
		destroyed := destroyedMassWithin(baseItem, incItem)
		if float64(destroyed)/float64(baseMass) >= ItemWipeFraction {
			D.ItemWipes++
			recordLeaf(D, path)
			return
		}
	}
	walk(baseItem, incItem, D, path)
}

func destroyedMassWithin(baseVal, incVal dataapi.Value) int {
	if !Meaningful(baseVal) {
		return 0
	}
	if isScalar(baseVal) {
		if Meaningful(incVal) {
			return 0
		}
		return Mass(baseVal)
	}
	if baseArr, ok := baseVal.([]dataapi.Value); ok {
		total := 0
		for i, v := range baseArr {
			total += destroyedMassWithin(v, arrayAt(incVal, i))
		}
		return total
	}
	if baseObj, ok := baseVal.(*dataapi.Object); ok {
		incObj, _ := incVal.(*dataapi.Object)
		total := 0
		for _, k := range jsKeys(baseObj) {
			baseChild, _ := baseObj.Get(k)
			var incChild dataapi.Value
			if incObj != nil {
				incChild, _ = incObj.Get(k)
			}
			total += destroyedMassWithin(baseChild, incChild)
		}
		return total
	}
	return 0
}

func arrayAt(v dataapi.Value, i int) dataapi.Value {
	if arr, ok := v.([]dataapi.Value); ok && i < len(arr) {
		return arr[i]
	}
	return nil
}

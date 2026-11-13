package dataguard

import (
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/panphora/htmlclay/internal/dataapi"
)

func stableDigest(v dataapi.Value) string { return Stringify(sortKeys(v)) }

func sortKeys(v dataapi.Value) dataapi.Value {
	switch t := v.(type) {
	case []dataapi.Value:
		out := make([]dataapi.Value, len(t))
		for i, item := range t {
			out[i] = sortKeys(item)
		}
		return out
	case *dataapi.Object:
		keys := jsKeys(t)
		sort.SliceStable(keys, func(i, j int) bool { return jsLess(keys[i], keys[j]) })
		out := dataapi.NewObject()
		for _, k := range keys {
			val, _ := t.Get(k)
			out.Set(k, sortKeys(val))
		}
		return out
	}
	return v
}

func islandHash(island dataapi.Value) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(stableDigest(islandData(island)))))
}

func islandsEqual(a, b dataapi.Value) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return stableDigest(islandData(a)) == stableDigest(islandData(b))
}

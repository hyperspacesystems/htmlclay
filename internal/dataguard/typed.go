package dataguard

import (
	"sort"
	"strings"

	"github.com/panphora/htmlclay/internal/dataapi"
)

// TypedValue is one value a person typed that is still in the document.
type TypedValue struct {
	Path    string `json:"path"`           // identity path, e.g. api.items[id:5].note or api.tags[*]
	Digest  string `json:"digest"`         // stableDigest of the value
	Preview string `json:"preview"`        // the value as text, at most 80 UTF-16 units
	At      int64  `json:"at"`             // unix milliseconds when it was typed
	Prev    string `json:"prev,omitempty"` // digest this value replaced when it was typed
}

// OutsideVerdict is the judge's answer for an outside write with no proof of reading.
type OutsideVerdict struct {
	Fire   bool
	Reason string       // "recent", "all-typed", "emptied", "unreadable", or "" when silent
	Gone   []TypedValue // typed values the writer shows no sign of having read
}

const (
	TypedRecentWindow = 10 * 60 * 1000 // ms: typing this recent is protected
	TypedAllMin       = 3              // "every typed value removed" needs at least this many
	TypedMax          = 500            // the record keeps the newest this many
)

// typedLeaves lists the values a person could have typed, each with the identity
// path it sits at. The guard's own bookkeeping is left out.
func typedLeaves(island dataapi.Value) []TypedValue {
	out := []TypedValue{}
	collectTypedLeaves(island, "", true, &out)
	return out
}

func collectTypedLeaves(v dataapi.Value, path string, top bool, out *[]TypedValue) {
	switch t := v.(type) {
	case *dataapi.Object:
		for _, k := range jsKeys(t) {
			if top && (k == CodecsKey || k == FormKey) {
				continue
			}
			val, _ := t.Get(k)
			collectTypedLeaves(val, typedJoin(path, k), false, out)
		}
	case []dataapi.Value:
		itemList := looksLikeItemList(t)
		for _, item := range t {
			seg := "[*]"
			if itemList {
				if id, ok := itemID(item); ok {
					// The identity field names the item; an app writes it, a person
					// does not type it, so it is left out of the record.
					collectItemLeaves(item.(*dataapi.Object), path+"["+id+"]", id[:strings.IndexByte(id, ':')], out)
					continue
				}
			}
			collectTypedLeaves(item, path+seg, false, out)
		}
	default:
		if Meaningful(v) {
			*out = append(*out, TypedValue{
				Path:    path,
				Digest:  stableDigest(v),
				Preview: truncatePreview(jsStringOf(v)),
			})
		}
	}
}

func collectItemLeaves(item *dataapi.Object, path, idKey string, out *[]TypedValue) {
	for _, k := range jsKeys(item) {
		if k == idKey {
			continue
		}
		val, _ := item.Get(k)
		collectTypedLeaves(val, typedJoin(path, k), false, out)
	}
}

func typedJoin(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func typedKey(v TypedValue) string { return v.Path + "\x00" + v.Digest }

// typedBuckets maps each leaf key to the leaves that hold it, so one leaf can
// satisfy one entry and no more.
func typedBuckets(leaves []TypedValue) map[string][]int {
	buckets := map[string][]int{}
	for i, leaf := range leaves {
		k := typedKey(leaf)
		buckets[k] = append(buckets[k], i)
	}
	return buckets
}

func typedPresent(typed []TypedValue, island dataapi.Value) []bool {
	leaves := typedLeaves(island)
	buckets := typedBuckets(leaves)
	consumed := make([]bool, len(leaves))
	present := make([]bool, len(typed))
	for i, e := range typed {
		k := typedKey(e)
		if window := buckets[k]; len(window) > 0 {
			present[i] = true
			consumed[window[0]] = true
			buckets[k] = window[1:]
		}
	}
	loose := map[string][]int{}
	for i, leaf := range leaves {
		if consumed[i] || !strings.HasPrefix(leaf.Digest, "\"") {
			continue
		}
		k := typedLooseKey(leaf.Path, leaf.Digest)
		loose[k] = append(loose[k], i)
	}
	for i, e := range typed {
		if present[i] || !strings.HasPrefix(e.Digest, "\"") {
			continue
		}
		k := typedLooseKey(e.Path, e.Digest)
		if window := loose[k]; len(window) > 0 {
			present[i] = true
			consumed[window[0]] = true
			loose[k] = window[1:]
		}
	}
	return present
}

// typedLooseKey identifies a text leaf by digest alone across a list that gained
// or lost identities: an answer typed at items[*].answer is still that answer at
// items[id:3].answer.
func typedLooseKey(path, digest string) string {
	return typedNormalPath(path) + "\x00" + digest
}

// typedNormalPath replaces every bracket segment with [*], so a path keeps its
// shape when the items along it are re-keyed.
func typedNormalPath(path string) string {
	segs := typedSegments(path)
	for i, seg := range segs {
		if strings.HasPrefix(seg, "[") {
			segs[i] = "[*]"
		}
	}
	return strings.Join(segs, ".")
}

// UpdateTyped records a save made by a user gesture: the typed values still in the
// document keep their time, values gone from it are dropped, and values that were
// not there before are recorded as typed now.
func UpdateTyped(prev, inc dataapi.Value, typed []TypedValue, now int64) []TypedValue {
	if IsSheet(prev) || IsSheet(inc) {
		return typed
	}
	leaves := typedLeaves(inc)
	buckets := typedBuckets(leaves)
	consumed := make([]bool, len(leaves))
	out := make([]TypedValue, 0, len(typed)+len(leaves))
	for _, e := range typed {
		k := typedKey(e)
		if window := buckets[k]; len(window) > 0 {
			consumed[window[0]] = true
			buckets[k] = window[1:]
			out = append(out, e)
		}
	}
	prevAt := map[string]string{}
	if prev != nil {
		for _, leaf := range typedLeaves(prev) {
			if strings.Contains(leaf.Path, "[*]") {
				continue
			}
			if _, seen := prevAt[leaf.Path]; !seen {
				prevAt[leaf.Path] = leaf.Digest
			}
		}
	}
	known := map[string]int{}
	if prev != nil {
		for _, leaf := range typedLeaves(prev) {
			known[typedKey(leaf)]++
		}
	}
	for i, leaf := range leaves {
		if consumed[i] {
			continue
		}
		k := typedKey(leaf)
		if known[k] > 0 {
			known[k]--
			continue
		}
		leaf.At = now
		if p, ok := prevAt[leaf.Path]; ok && p != leaf.Digest {
			leaf.Prev = p
		}
		out = append(out, leaf)
	}
	return capTyped(out)
}

// DropGone keeps only the typed values an accepted outside write left in place.
func DropGone(typed []TypedValue, inc dataapi.Value) []TypedValue {
	present := typedPresent(typed, inc)
	out := make([]TypedValue, 0, len(typed))
	for i, e := range typed {
		if present[i] {
			out = append(out, e)
		}
	}
	return out
}

// unionTyped merges newly lost typed values into a pending event's rows, keeping
// the entry that arrived first when both describe the same value.
func unionTyped(have, add []TypedValue) []TypedValue {
	seen := map[string]bool{}
	out := make([]TypedValue, 0, len(have)+len(add))
	for _, e := range have {
		seen[typedKey(e)] = true
		out = append(out, e)
	}
	for _, e := range add {
		k := typedKey(e)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	return out
}

func capTyped(typed []TypedValue) []TypedValue {
	if len(typed) <= TypedMax {
		return typed
	}
	order := make([]int, len(typed))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return typed[order[a]].At < typed[order[b]].At })
	drop := make([]bool, len(typed))
	for _, i := range order[:len(typed)-TypedMax] {
		drop[i] = true
	}
	out := make([]TypedValue, 0, TypedMax)
	for i, e := range typed {
		if !drop[i] {
			out = append(out, e)
		}
	}
	return out
}

// JudgeOutside asks whether an outside write that showed no proof of reading took
// values a person typed (David, 2026-11-13).
func JudgeOutside(base, inc dataapi.Value, d *Destruction, typed []TypedValue, now int64) OutsideVerdict {
	if inc == nil {
		if dataValueCount(base) == 0 {
			return OutsideVerdict{Fire: false, Reason: "", Gone: []TypedValue{}}
		}
		return OutsideVerdict{Fire: true, Reason: "unreadable", Gone: []TypedValue{}}
	}
	if d == nil || (!d.AnyDestruction && d.ModifiedAtoms == 0) {
		return OutsideVerdict{Fire: false, Reason: "", Gone: []TypedValue{}}
	}
	present := typedPresent(typed, inc)
	carried := []TypedValue{}
	gone := []TypedValue{}
	for i, e := range typed {
		if present[i] {
			carried = append(carried, e)
		} else {
			gone = append(gone, e)
		}
	}
	newestCarried := int64(0)
	for _, e := range carried {
		if strings.Contains(e.Path, "[*]") || !strings.HasPrefix(e.Digest, "\"") {
			continue
		}
		if e.At > newestCarried {
			newestCarried = e.At
		}
	}
	incDigestAt := map[string]string{}
	for _, leaf := range typedLeaves(inc) {
		incDigestAt[leaf.Path] = leaf.Digest
	}
	unread := []TypedValue{}
	for _, e := range gone {
		if !strings.Contains(e.Path, "[*]") {
			if digest, ok := incDigestAt[e.Path]; ok {
				if digest == e.Prev {
					unread = append(unread, e)
				}
				continue
			}
		}
		if e.At < newestCarried {
			continue
		}
		unread = append(unread, e)
	}
	for _, e := range unread {
		if now-e.At <= TypedRecentWindow {
			return OutsideVerdict{Fire: true, Reason: "recent", Gone: unread}
		}
	}
	if len(typed) >= TypedAllMin && len(carried) == 0 && len(unread) == len(typed) && (d.AnyDestruction || d.ModifiedAtoms > 0) {
		return OutsideVerdict{Fire: true, Reason: "all-typed", Gone: unread}
	}
	if d.AnyDestruction && nearTotalWipe(base, inc) {
		return OutsideVerdict{Fire: true, Reason: "emptied", Gone: unread}
	}
	return OutsideVerdict{Fire: false, Reason: "", Gone: unread}
}

// PutBack writes the typed values a loss took back into the places the current
// page still has, and reports how many landed. It returns a deep copy of current,
// so the writer's other changes stay exactly as they are.
func PutBack(current dataapi.Value, gone []TypedValue, recoverable dataapi.Value) (dataapi.Value, int) {
	merged := copyIsland(current)
	placed := 0
	for _, e := range gone {
		if putBackOne(merged, e, recoverable) {
			placed++
		}
	}
	return merged, placed
}

// putBackOne places one typed value: an object key or an identified item's key
// along a path that still exists, or an appended entry on a scalar list.
func putBackOne(merged dataapi.Value, e TypedValue, recoverable dataapi.Value) bool {
	segs := typedSegments(e.Path)
	if len(segs) == 0 {
		return false
	}
	if segs[len(segs)-1] == "[*]" {
		return putBackAppend(merged, e, recoverable, segs[:len(segs)-1])
	}
	if strings.HasPrefix(segs[len(segs)-1], "[") {
		return false
	}
	for _, seg := range segs {
		if seg == "[*]" {
			return false
		}
	}
	value, ok := typedValueAt(recoverable, segs, e.Digest)
	if !ok {
		return false
	}
	parent, ok := walkExisting(merged, segs[:len(segs)-1]).(*dataapi.Object)
	if !ok {
		return false
	}
	parent.Set(segs[len(segs)-1], value)
	return true
}

// putBackAppend puts one scalar back on a list the writer emptied of it, when the
// list is still there and no entry holds that value any more.
func putBackAppend(merged dataapi.Value, e TypedValue, recoverable dataapi.Value, parentSegs []string) bool {
	if len(parentSegs) == 0 || strings.HasPrefix(parentSegs[len(parentSegs)-1], "[") {
		return false
	}
	list, ok := walkExisting(recoverable, parentSegs).([]dataapi.Value)
	if !ok {
		return false
	}
	var value dataapi.Value
	found := false
	for _, item := range list {
		if isScalar(item) && stableDigest(item) == e.Digest {
			value = item
			found = true
			break
		}
	}
	if !found {
		return false
	}
	holder, ok := walkExisting(merged, parentSegs[:len(parentSegs)-1]).(*dataapi.Object)
	if !ok {
		return false
	}
	key := parentSegs[len(parentSegs)-1]
	current, ok := holder.Get(key)
	if !ok {
		return false
	}
	arr, ok := current.([]dataapi.Value)
	if !ok {
		return false
	}
	for _, item := range arr {
		if stableDigest(item) == e.Digest {
			return false
		}
	}
	appended := make([]dataapi.Value, len(arr), len(arr)+1)
	copy(appended, arr)
	holder.Set(key, append(appended, copyIsland(value)))
	return true
}

// typedValueAt reads the original value a typed entry recorded, which must still
// be exactly that value in the pinned data.
func typedValueAt(recoverable dataapi.Value, segs []string, digest string) (dataapi.Value, bool) {
	value := walkExisting(recoverable, segs)
	if value == nil || stableDigest(value) != digest {
		return nil, false
	}
	return copyIsland(value), true
}

// walkExisting follows identity-path segments through an island, failing when an
// object key or an identified item is no longer there.
func walkExisting(v dataapi.Value, segs []string) dataapi.Value {
	for _, seg := range segs {
		if strings.HasPrefix(seg, "[") && strings.HasSuffix(seg, "]") {
			arr, ok := v.([]dataapi.Value)
			if !ok {
				return nil
			}
			item, found := typedItemByID(arr, seg[1:len(seg)-1])
			if !found {
				return nil
			}
			v = item
			continue
		}
		obj, ok := v.(*dataapi.Object)
		if !ok {
			return nil
		}
		next, ok := obj.Get(seg)
		if !ok {
			return nil
		}
		v = next
	}
	return v
}

// typedItemByID finds the item an identity path names, by the same identity match
// typedLeaves records one with.
func typedItemByID(arr []dataapi.Value, id string) (dataapi.Value, bool) {
	for _, item := range arr {
		if got, ok := itemID(item); ok && got == id {
			return item, true
		}
	}
	return nil, false
}

// typedSegments splits an identity path into its keys and its item segments, which
// keep their brackets: api.items[id:5].note -> api, items, [id:5], note.
func typedSegments(path string) []string {
	segs := []string{}
	start := 0
	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '.':
			if i > start {
				segs = append(segs, path[start:i])
			}
			start = i + 1
		case '[':
			if i > start {
				segs = append(segs, path[start:i])
			}
			end := strings.IndexByte(path[i:], ']')
			if end < 0 {
				return append(segs, path[i:])
			}
			segs = append(segs, path[i:i+end+1])
			i += end
			start = i + 1
		}
	}
	if start < len(path) {
		segs = append(segs, path[start:])
	}
	return segs
}

// copyIsland deep-copies an island, so a put-back never writes through the value
// it was handed.
func copyIsland(v dataapi.Value) dataapi.Value {
	switch t := v.(type) {
	case *dataapi.Object:
		out := dataapi.NewObject()
		for _, k := range t.Keys() {
			child, _ := t.Get(k)
			out.Define(k, copyIsland(child))
		}
		return out
	case []dataapi.Value:
		out := make([]dataapi.Value, len(t))
		for i, child := range t {
			out[i] = copyIsland(child)
		}
		return out
	}
	return v
}

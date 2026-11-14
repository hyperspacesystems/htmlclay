package dataguard

import (
	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/gridcodec"
	"strconv"
)

const CodecsKey = "@codecs"
const FormKey = "@form"

func islandData(island dataapi.Value) dataapi.Value {
	obj, ok := island.(*dataapi.Object)
	if !ok {
		return island
	}
	out := dataapi.NewObject()
	for _, key := range obj.Keys() {
		if key != CodecsKey && key != FormKey {
			value, _ := obj.Get(key)
			out.Define(key, value)
		}
	}
	return out
}

func codecDeclarations(island dataapi.Value, name string) *dataapi.Object {
	all, _ := objectLookup(island, CodecsKey)
	value, _ := objectLookup(all, name)
	obj, _ := value.(*dataapi.Object)
	return obj
}

func declared(island dataapi.Value, name, key string) string {
	return textField(codecDeclarations(island, name), key)
}

func textField(obj *dataapi.Object, key string) string {
	if obj == nil {
		return ""
	}
	value, _ := obj.Get(key)
	text, _ := value.(string)
	return text
}

// IsSheet reports whether an island carries a non-empty top-level codecs map, the
// mark of a sheet page.
func IsSheet(island dataapi.Value) bool {
	obj, ok := island.(*dataapi.Object)
	if !ok {
		return false
	}
	all, ok := obj.Get(CodecsKey)
	if !ok {
		return false
	}
	codecs, ok := all.(*dataapi.Object)
	return ok && codecs.Len() > 0
}

func HasCodecs(island dataapi.Value) bool {
	all, _ := objectLookup(island, CodecsKey)
	for _, name := range objectKeyList(all) {
		if d := codecDeclarations(island, name); d != nil && d.Len() > 0 {
			return true
		}
	}
	return false
}

func walkData(base, inc dataapi.Value, d *Destruction, path []pathSeg, was, now *dataapi.Object) {
	for _, key := range objectKeyList(base) {
		before, _ := objectLookup(base, key)
		after, _ := objectLookup(inc, key)
		prior, next := textField(was, key), textField(now, key)
		childPath := pathWith(path, keySeg(key))
		if prior != "" {
			walkCodecValue(before, after, d, childPath, prior == "sheet" && next == prior)
		} else {
			if next != "" {
				after = nil
			}
			walk(before, after, d, childPath)
		}
	}
}

type matchKey struct{ id, content string }

func matchEntries(base, inc []matchKey) []int {
	partners := make([]int, len(base))
	for i := range partners {
		partners[i] = -1
	}
	taken := make([]bool, len(inc))
	pairBy := func(keyOf func(matchKey) (string, bool)) {
		buckets := map[string][]int{}
		for i, entry := range inc {
			if taken[i] {
				continue
			}
			if key, ok := keyOf(entry); ok {
				buckets[key] = append(buckets[key], i)
			}
		}
		for i, entry := range base {
			if partners[i] != -1 {
				continue
			}
			key, ok := keyOf(entry)
			if !ok || len(buckets[key]) == 0 {
				continue
			}
			j := buckets[key][0]
			buckets[key] = buckets[key][1:]
			partners[i], taken[j] = j, true
		}
	}
	pairBy(func(e matchKey) (string, bool) { return e.id + "\x00" + e.content, e.id != "" })
	pairBy(func(e matchKey) (string, bool) { return e.id, e.id != "" })
	pairBy(func(e matchKey) (string, bool) { return e.content, true })
	pairBy(func(e matchKey) (string, bool) { return "", e.id == "" })
	return partners
}

func codecTabs(view dataapi.Value) []*dataapi.Object {
	tabs := []*dataapi.Object{}
	list, _ := view.([]dataapi.Value)
	for _, value := range list {
		if obj, ok := value.(*dataapi.Object); ok {
			rows, _ := obj.Get("rows")
			if _, ok := rows.([]dataapi.Value); ok {
				tabs = append(tabs, obj)
			}
		}
	}
	return tabs
}

func tabColumns(tab *dataapi.Object) []string {
	if tab == nil {
		return nil
	}
	value, _ := tab.Get("cols")
	list, _ := value.([]dataapi.Value)
	cols := make([]string, len(list))
	for i, value := range list {
		cols[i], _ = value.(string)
	}
	return cols
}

func tabRows(tab *dataapi.Object) [][]dataapi.Value {
	if tab == nil {
		return nil
	}
	value, _ := tab.Get("rows")
	list, _ := value.([]dataapi.Value)
	rows := [][]dataapi.Value{}
	for _, value := range list {
		if row, ok := value.([]dataapi.Value); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func entryID(value dataapi.Value) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return Stringify(value)
}

func tabKeys(tabs []*dataapi.Object) []matchKey {
	keys := make([]matchKey, len(tabs))
	for i, tab := range tabs {
		id, _ := tab.Get("id")
		cols, _ := tab.Get("cols")
		rows, _ := tab.Get("rows")
		keys[i] = matchKey{entryID(id), stableDigest([]dataapi.Value{cols, rows})}
	}
	return keys
}

func rowKeys(rows [][]dataapi.Value, named bool) []matchKey {
	keys := make([]matchKey, len(rows))
	for i, row := range rows {
		content := entryID(row[3]) + "\x00"
		if named {
			content += entryID(row[4]) + "\x00"
		}
		keys[i] = matchKey{entryID(row[0]), content + entryID(row[5])}
	}
	return keys
}

type codecRowChange struct {
	tab                *dataapi.Object
	tabIndex, rowIndex int
	before, now        []dataapi.Value
	change             string
}

func codecRowChanges(baseView, incView dataapi.Value, codec bool) []codecRowChange {
	base, inc := codecTabs(baseView), codecTabs(incView)
	partners := matchEntries(tabKeys(base), tabKeys(inc))
	changes := []codecRowChange{}
	for ti, tab := range base {
		var next *dataapi.Object
		if codec && partners[ti] != -1 {
			next = inc[partners[ti]]
		}
		beforeRows, nowRows := tabRows(tab), tabRows(next)
		beforeCols, afterCols := tabColumns(tab), tabColumns(next)
		named := false
		for i, id := range beforeCols {
			if id != "" && id != "#"+strconv.Itoa(i) {
				named = true
			}
		}
		rowPartners := matchEntries(rowKeys(beforeRows, named), rowKeys(nowRows, named))
		for ri, before := range beforeRows {
			var now []dataapi.Value
			if rowPartners[ri] != -1 {
				now = nowRows[rowPartners[ri]]
			}
			change := "destruction"
			if codec {
				change = gridcodec.RowChange(digestArgs(before), digestArgs(now), beforeCols, afterCols)
			}
			changes = append(changes, codecRowChange{tab, ti, ri, before, now, change})
		}
	}
	return changes
}

func digestCount(value dataapi.Value) int {
	switch v := value.(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

func entrySeg(value dataapi.Value, fallback int) pathSeg {
	if id := entryID(value); id != "" {
		return keySeg(id)
	}
	return indexSeg(fallback)
}

func walkCodecValue(base, inc dataapi.Value, d *Destruction, path []pathSeg, codec bool) {
	for _, row := range codecRowChanges(base, inc, codec) {
		filled, mass := digestCount(row.before[1]), digestCount(row.before[2])
		d.LeafCount += filled
		d.CodecLeaves += filled
		d.TotalMass += mass
		if row.change == "modification" {
			d.ModifiedAtoms++
		} else if row.change == "destruction" {
			destroyed, lostMass := filled, mass
			if row.now != nil {
				destroyed = max(1, filled-digestCount(row.now[1]))
				lostMass = max(0, mass-digestCount(row.now[2]))
			}
			d.DestroyedLeaves += destroyed
			d.CodecDestroyedLeaves += destroyed
			d.DestroyedMass += lostMass
			id, _ := row.tab.Get("id")
			recordLeaf(d, pathWith(pathWith(path, entrySeg(id, row.tabIndex)), entrySeg(row.before[0], row.rowIndex)))
		}
	}
}

func digestArgs(row []dataapi.Value) []any {
	if row == nil {
		return nil
	}
	out := make([]any, len(row))
	for i, v := range row {
		out[i] = v
	}
	return out
}

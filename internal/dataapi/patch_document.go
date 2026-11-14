package dataapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/panphora/htmlclay/internal/gridcodec"
)

// PatchOptions are patchDocument's options. Token defaults to "api"; NewID nil means random ids.
type PatchOptions struct {
	Token string
	NewID func() string
}

// PatchResult is patchDocument's { html, changed, stale }. A host cannot recalculate, so patched
// tables and formula-bearing survivors of a deletion carry `stale` until a browser saves them.
type PatchResult struct {
	HTML    []byte
	Changed gridcodec.Changed
	Stale   bool
}

// NotCanonicalMessage is patch-document.js NOT_CANONICAL_MESSAGE.
const NotCanonicalMessage = "This sheet was edited outside the sheet editor, so the API cannot patch it: open the sheet in the editor and save once."

// PatchDocument applies a patch list to a sheet document's source, rewriting only touched rows,
// deleted tab sections and needed stale attributes. Port of hyper-html-api
// src/patch-document.js patchDocument. Every check runs in the reference's order, so a document
// or patch the reference refuses is refused with the same type, code and message before output.
func PatchDocument(src []byte, patch Value, opts PatchOptions) (*PatchResult, error) {
	token := opts.Token
	if token == "" {
		token = "api"
	}
	ops, ok := patch.([]Value)
	if !ok || len(ops) == 0 {
		return nil, &gridcodec.QueryRefused{Code: "bad_patch", Message: "patch is a non-empty list of operations"}
	}

	// Offsets index the bytes after the BOM, as ReadDocument reads them; the mark goes back on the
	// output, which is what a host that keeps the file's bytes expects.
	var bom []byte
	body := src
	if bytes.HasPrefix(src, utf8BOM) {
		bom = src[:len(utf8BOM)]
		body = src[len(utf8BOM):]
	}
	source := string(body)

	deleting := false
	for _, raw := range ops {
		if op, isObject := raw.(*Object); isObject {
			if _, hasDelete := op.Get("delete"); hasDelete {
				deleting = true
				break
			}
		}
	}
	tabs, err := locateTabs(source, token, deleting)
	if err != nil {
		return nil, err
	}

	order := []*sheetTab{}
	groups := map[*sheetTab][]any{}
	for _, raw := range ops {
		op, isObject := raw.(*Object)
		if !isObject {
			return nil, &gridcodec.QueryRefused{Code: "bad_patch", Message: "an operation is { sheet, range, values }, { sheet, append }, or { sheet, delete: true }"}
		}
		if deleteValue, hasDelete := op.Get("delete"); hasDelete {
			deleteTrue, isTrue := deleteValue.(bool)
			if !isTrue || !deleteTrue {
				return nil, &gridcodec.QueryRefused{Code: "bad_patch", Message: "delete must be exactly true"}
			}
			if _, hasSheet := op.Get("sheet"); !hasSheet {
				return nil, &gridcodec.QueryRefused{Code: "bad_patch", Message: "delete requires a sheet name or id"}
			}
			for _, key := range []string{"range", "append", "values"} {
				if _, present := op.Get(key); present {
					return nil, &gridcodec.QueryRefused{Code: "bad_patch", Message: "delete cannot include range, append, or values; send the delete on its own"}
				}
			}
		}
		wanted, hasSheet := op.Get("sheet")
		tab, err := pickTab(tabs, wanted, hasSheet)
		if err != nil {
			return nil, err
		}
		gridOp := map[string]any{}
		for _, key := range op.Keys() {
			if key == "sheet" {
				continue
			}
			sub, _ := op.Get(key)
			gridOp[key] = plainJSON(sub)
		}
		if _, seen := groups[tab]; !seen {
			order = append(order, tab)
		}
		groups[tab] = append(groups[tab], gridOp)
	}
	deleted := map[*sheetTab]bool{}
	for _, tab := range order {
		deleteGroup := false
		for _, raw := range groups[tab] {
			op := raw.(map[string]any)
			deleteTrue, isTrue := op["delete"].(bool)
			if isTrue && deleteTrue {
				deleteGroup = true
				break
			}
		}
		if !deleteGroup {
			continue
		}
		if len(groups[tab]) != 1 {
			return nil, &gridcodec.QueryRefused{Code: "bad_patch", Message: fmt.Sprintf("another operation names sheet %s; send the delete on its own", jsJSON(tab.name))}
		}
		deleted[tab] = true
	}
	if len(deleted) > 0 && len(deleted) == len(tabs) {
		return nil, &gridcodec.QueryRefused{Code: "last_sheet", Message: "cannot delete the last sheet; keep at least one tab"}
	}

	type pendingEdit struct {
		start, end int
		bytes      string
	}
	edits := []pendingEdit{}
	cells, rows := 0, 0
	stale := false
	for _, tab := range order {
		if deleted[tab] {
			if tab.sectionRegion == nil {
				return nil, &CodecRefused{Code: "sheet-not-canonical", Message: NotCanonicalMessage}
			}
			edits = append(edits, pendingEdit{start: tab.sectionRegion.Start, end: tab.sectionRegion.End})
			continue
		}
		if tab.region == nil {
			return nil, &CodecRefused{Code: "sheet-not-canonical", Message: NotCanonicalMessage}
		}
		out, changed, err := gridcodec.PatchGrid(source[tab.region.Start:tab.region.End], groups[tab], opts.NewID)
		if err != nil {
			return nil, err
		}
		edits = append(edits, pendingEdit{start: tab.region.Start, end: tab.region.End, bytes: out})
		cells += changed.Cells
		rows += changed.Rows
		stale = true
	}
	if len(deleted) > 0 {
		for i := range tabs {
			tab := &tabs[i]
			if _, grouped := groups[tab]; grouped || tab.table == nil || !hasFormulaDescendant(tab.table) {
				continue
			}
			stale = true
			if hasAttribute(tab.table, "stale") {
				continue
			}
			if tab.staleAt < 0 {
				return nil, &CodecRefused{Code: "sheet-not-canonical", Message: NotCanonicalMessage}
			}
			edits = append(edits, pendingEdit{start: tab.staleAt, end: tab.staleAt, bytes: " stale"})
		}
	}

	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	html := source
	for _, edit := range edits {
		html = html[:edit.start] + edit.bytes + html[edit.end:]
	}
	result := []byte(html)
	if len(bom) > 0 {
		result = append(append([]byte{}, bom...), result...)
	}
	return &PatchResult{
		HTML:    result,
		Changed: gridcodec.Changed{Cells: cells, Rows: rows, Sheets: len(deleted)},
		Stale:   stale,
	}, nil
}

// hasFormulaDescendant matches cheerio table.find("[f]"): the table itself is excluded and
// detached template content is not traversed.
func hasFormulaDescendant(table *html.Node) bool {
	var walk func(*html.Node) bool
	walk = func(node *html.Node) bool {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && hasAttribute(child, "f") {
				return true
			}
			if walk(child) {
				return true
			}
		}
		return false
	}
	return walk(table)
}

// plainJSON converts a decoded patch value into the plain JSON shapes gridcodec.PatchGrid takes:
// ordered objects become maps, a json.Number becomes the float64 JSON.parse produces (a range
// error keeps the returned ±Inf, exactly as JSON.parse gives Infinity), and everything else stays.
func plainJSON(value Value) any {
	switch v := value.(type) {
	case *Object:
		out := make(map[string]any, v.Len())
		for _, key := range v.Keys() {
			sub, _ := v.Get(key)
			out[key] = plainJSON(sub)
		}
		return out
	case []Value:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = plainJSON(item)
		}
		return out
	case json.Number:
		number, _ := strconv.ParseFloat(string(v), 64)
		return number
	}
	return value
}

// DocumentRules is read.js documentRules: the rules tag's rules and version without parsing the
// canonical tables (same cut and checks as ReadDocument; anything they reject parses the whole
// source). found is false when no tag carries the token.
func DocumentRules(src []byte, token string) (rules Value, version string, found bool, err error) {
	if token == "" {
		token = "api"
	}
	body := string(trimBOM(src))

	marker := gridcodec.HoleMarker(body)
	rest, regions, err := gridcodec.CutGrids(body, marker)
	if err != nil {
		return nil, "", false, err
	}
	if len(regions) > 0 {
		d, err := ParseBytes([]byte(rest))
		if err != nil {
			return nil, "", false, err
		}
		found, err := d.FindRulesIn(token)
		if err != nil {
			return nil, "", false, err
		}
		if found == nil {
			return nil, "", false, nil
		}
		if holesHold(d, found.Rules, "sheet", marker, len(regions)) {
			return found.Rules, found.Version, true, nil
		}
	}

	d, err := ParseBytes([]byte(body))
	if err != nil {
		return nil, "", false, err
	}
	tag, err := d.FindRulesIn(token)
	if err != nil {
		return nil, "", false, err
	}
	if tag == nil {
		return nil, "", false, nil
	}
	return tag.Rules, tag.Version, true, nil
}

// IsPatchBody is write.js isPatchBody: body is an object whose "patch" is an array, whose only
// other key is "operationId", and rules (when an object) has no top-level "patch" key.
func IsPatchBody(body, rules Value) bool {
	obj, ok := body.(*Object)
	if !ok {
		return false
	}
	patch, has := obj.Get("patch")
	if !has {
		return false
	}
	if _, isList := patch.([]Value); !isList {
		return false
	}
	for _, key := range obj.Keys() {
		if key != "patch" && key != "operationId" {
			return false
		}
	}
	if ruleObj, ok := rules.(*Object); ok {
		if _, has := ruleObj.Get("patch"); has {
			return false
		}
	}
	return true
}

// locateTabs is patch-document.js locateTabs: each tab with the byte range of its canonical table,
// or no region when the table is missing or not canonical. The cut route first, as ReadDocument
// reads; when its holes do not verify, one parse with the tables' starts marked in the source, then
// a strict scan at each mark.
func locateTabs(body, token string, deleting bool) ([]sheetTab, error) {
	if deleting {
		return locateDeleteTabs(body, token)
	}
	marker := gridcodec.HoleMarker(body)
	rest, regions, err := gridcodec.CutGrids(body, marker)
	if err != nil {
		return nil, err
	}
	if len(regions) > 0 {
		d, err := ParseBytes([]byte(rest))
		if err != nil {
			return nil, err
		}
		found, err := d.FindRulesIn(token)
		if err != nil {
			return nil, err
		}
		if found == nil {
			return nil, &NoRulesTag{Token: token}
		}
		container, err := sheetContainer(d, found.Rules, found.Version)
		if err != nil {
			return nil, err
		}
		if holesHold(d, found.Rules, "sheet", marker, len(regions)) {
			tabs := sheetTabNodes(container)
			for i := range tabs {
				if tabs[i].table == nil {
					continue
				}
				if region, ok := holeRegion(tabs[i].table, marker, regions); ok {
					tabs[i].region = &region
				}
			}
			return tabs, nil
		}
	}

	d, err := ParseBytes([]byte(markGridStarts(body, marker)))
	if err != nil {
		return nil, err
	}
	found, err := d.FindRulesIn(token)
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, &NoRulesTag{Token: token}
	}
	container, err := sheetContainer(d, found.Rules, found.Version)
	if err != nil {
		return nil, err
	}
	tabs := sheetTabNodes(container)
	for i := range tabs {
		if tabs[i].table == nil {
			continue
		}
		if region, ok := scannedRegion(tabs[i].table, marker, body); ok {
			tabs[i].region = &region
		}
	}
	return tabs, nil
}

// sheetContainer is patch-document.js sheetContainer: the element the document's single top level
// @codec(sheet) rule selects, refusing anything that is not exactly one such rule.
func sheetContainer(d *Document, rules Value, version string) (*html.Node, error) {
	if err := checkCodecRules(rules, version, map[string]Codec{"sheet": SheetCodec}); err != nil {
		return nil, err
	}
	selectors := []string{}
	if obj, ok := rules.(*Object); ok {
		for _, key := range obj.Keys() {
			v, _ := obj.Get(key)
			rule, ok := v.(string)
			if !ok {
				continue
			}
			selector, name, isCodec, err := codecOf(rule)
			if err != nil || !isCodec || name != "sheet" {
				continue
			}
			selectors = append(selectors, selector)
		}
	}
	if len(selectors) != 1 {
		return nil, &CodecRefused{Code: "no-sheet", Message: "this document has no single @codec(sheet) rule to patch"}
	}
	if selectors[0] == "" {
		return d.Root, nil
	}
	matches, err := Find(d.Root, selectors[0], FindOpts{})
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, &CodecRefused{Code: "no-sheet", Message: "the document's @codec(sheet) rule matches no element"}
	}
	return matches[0], nil
}

// holeRegion is patch-document.js holeRegion: the region the table's data-hole names, if any.
func holeRegion(table *html.Node, marker string, regions []gridcodec.Region) (gridcodec.Region, bool) {
	hole, ok := attrValue(table, "data-hole")
	if !ok || !strings.HasPrefix(hole, marker+"-") {
		return gridcodec.Region{}, false
	}
	index, err := strconv.Atoi(hole[len(marker)+1:])
	if err != nil || index < 0 || index >= len(regions) {
		return gridcodec.Region{}, false
	}
	return regions[index], true
}

// scannedRegion is patch-document.js scannedRegion: the range the table's marked start names, then
// a strict scan from that offset. A *gridcodec.Miss, or a table with no mark (an uppercase or
// otherwise non-canonical start tag), means no region.
func scannedRegion(table *html.Node, marker, body string) (gridcodec.Region, bool) {
	at, ok := attrValue(table, "data-"+marker)
	if !ok {
		return gridcodec.Region{}, false
	}
	start, err := strconv.Atoi(at)
	if err != nil {
		return gridcodec.Region{}, false
	}
	end, err := gridcodec.ScanGrid(body, start, nil)
	if err != nil {
		return gridcodec.Region{}, false
	}
	return gridcodec.Region{Start: start, End: end}, true
}

// markGridStarts is the tree route's stand-in for the source offsets the reference's parser
// records: every "<table grid" in body gets " data-<marker>=\"<offset>\"" right after "<table",
// so the parsed table carries where it started. The attribute is first, so the parser keeps it over
// any duplicate; an insertion inside a comment, a script or an attribute value never becomes an
// element, so it is harmless.
func markGridStarts(body, marker string) string {
	var out strings.Builder
	from := 0
	for {
		k := strings.Index(body[from:], gridOpen)
		if k < 0 {
			break
		}
		at := from + k
		out.WriteString(body[from:at])
		out.WriteString("<table data-")
		out.WriteString(marker)
		out.WriteString("=\"")
		out.WriteString(strconv.Itoa(at))
		out.WriteString("\"")
		from = at + len("<table")
	}
	out.WriteString(body[from:])
	return out.String()
}

package dataapi

import (
	"strings"

	"golang.org/x/net/html"
)

// hasComputedRules reports whether the document's rules read a computed value: one a page's
// script writes, which a write applied without running the page cannot bring up to date. It is
// hyper-html-api's src/computed.js line for line, and the conformance corpus's write-stale cases
// pin it. A value is computed when the node a rule reads sits inside an <output>, a
// clay="no-dirty", no-dirty or save-ignore region; when sapjs paints what the rule reads; or when
// a sheet rule's table holds a formula or a stale mark. Nothing runs: only rules and markup are
// read.
func hasComputedRules(root *html.Node, rules Value) bool {
	return computedIn(root, rules, true)
}

// hasComputedRulesBesideSheets is hasComputedRules with sheet tables left out. The stale mark
// uses it: a sheet engine clears its own table's stale attribute, never this mark, so a value only
// a sheet computes is never marked (hyper-html-api: hasComputedRules with opts.sheets === false).
func hasComputedRulesBesideSheets(root *html.Node, rules Value) bool {
	return computedIn(root, rules, false)
}

func computedIn(root *html.Node, rules Value, sheets bool) bool {
	if !holdsMarker(root) {
		return false
	}
	return readsComputed(root, rules, sheets)
}

// The reference's internal lookups pass templateAttr:null, so they see [cms-template] seeds but,
// like every adapter find, never a no-data subtree or a rules tag.
func descendantsWhere(n *html.Node, pred func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(p *html.Node) {
		for c := p.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && pred(c) && !isRulesTag(c) && !inNoData(c) {
				out = append(out, c)
			}
			walk(c)
		}
	}
	if n != nil {
		walk(n)
	}
	return out
}

func anyDescendant(n *html.Node, pred func(*html.Node) bool) bool {
	return len(descendantsWhere(n, pred)) > 0
}

func selfOrAncestor(n *html.Node, pred func(*html.Node) bool) bool {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur.Type == html.ElementNode && pred(cur) {
			return true
		}
	}
	return false
}

func hasAttr(n *html.Node, name string) bool {
	_, ok := attrValue(n, name)
	return ok
}

// isComputedRegion is the selector `output, [clay~="no-dirty"], [no-dirty], [save-ignore]`.
func isComputedRegion(n *html.Node) bool {
	if n.Data == "output" || hasAttr(n, "no-dirty") || hasAttr(n, "save-ignore") {
		return true
	}
	clay, _ := attrValue(n, "clay")
	for _, f := range strings.Fields(clay) {
		if f == "no-dirty" {
			return true
		}
	}
	return false
}

// isInert is `template, [template], [cms-template], [sap-ignore]`.
func isInert(n *html.Node) bool {
	return n.Data == "template" || hasAttr(n, "template") || hasAttr(n, cmsTemplateAttr) || hasAttr(n, "sap-ignore")
}

func isGridTable(n *html.Node) bool { return n.Data == "table" && hasAttr(n, "grid") }

func isMarker(n *html.Node) bool { return hasAttr(n, "sap") || isGridTable(n) || isComputedRegion(n) }

// Most pages hold nothing that computes; they answer without walking any rule.
func holdsMarker(root *html.Node) bool {
	return selfOrAncestor(root, isMarker) || anyDescendant(root, isMarker)
}

// findRule runs a rule's selector the way the write ran it. A selector that cannot run reads
// nothing, computed or not; it must not fail a write that does not name it.
func findRule(ctx *html.Node, selector string) []*html.Node {
	nodes, err := Find(ctx, selector, FindOpts{})
	if err != nil {
		return nil
	}
	return nodes
}

func firstOnly(nodes []*html.Node) []*html.Node {
	if len(nodes) > 1 {
		return nodes[:1]
	}
	return nodes
}

func readsComputed(ctx *html.Node, rule Value, sheets bool) bool {
	switch r := rule.(type) {
	case string:
		return scalarComputed(ctx, r, sheets)
	case []Value:
		if len(r) == 0 {
			return false
		}
		selector, ok := r[0].(string)
		if !ok {
			return false
		}
		var shape Value
		if len(r) > 1 {
			shape = r[1]
		}
		for _, node := range findRule(ctx, selector) {
			if readsComputed(node, shape, sheets) {
				return true
			}
		}
	case *Object:
		for _, key := range r.Keys() {
			sub, _ := r.Get(key)
			if readsComputed(ctx, sub, sheets) {
				return true
			}
		}
	}
	return false
}

// A scalar rule reads its first match only, unless it is a [*] rule.
func scalarComputed(ctx *html.Node, rule string, sheets bool) bool {
	if strings.HasSuffix(rule, "[]") {
		for _, node := range findRule(ctx, strings.TrimSuffix(rule, "[]")) {
			if textComputed(node) {
				return true
			}
		}
		return false
	}
	if selector, name, ok, err := codecOf(rule); err != nil || ok {
		if err != nil || name != "sheet" {
			return false
		}
		nodes := []*html.Node{ctx}
		if selector != "" {
			nodes = firstOnly(findRule(ctx, selector))
		}
		for _, node := range nodes {
			if sheets && sheetComputed(node) {
				return true
			}
		}
		return false
	}
	if strings.HasPrefix(rule, "@") {
		return attrComputed(ctx, rule[1:])
	}
	if rule == "." {
		return textComputed(ctx)
	}
	selector, prop, hasProp, all := splitRule(rule)
	nodes := []*html.Node{ctx}
	if selector != "" {
		nodes = findRule(ctx, selector)
		if !all {
			nodes = firstOnly(nodes)
		}
	}
	for _, node := range nodes {
		if hasProp && attrComputed(node, prop) || !hasProp && textComputed(node) {
			return true
		}
	}
	return false
}

// Text is computed when the node is in a computed region, or it or anything under it holds a
// computed region or painted text.
func textComputed(n *html.Node) bool {
	return selfOrAncestor(n, isComputedRegion) ||
		anyDescendant(n, isComputedRegion) ||
		paintsText(n) ||
		anyDescendant(n, paintsText)
}

var textProps = map[string]bool{"textContent": true, "innerText": true, "innerHTML": true, "outerHTML": true}

var neverPainted = map[string]bool{
	"id": true, "data-id": true, "sap": true, "scope": true, "items": true, "item": true, "bind": true,
	"state": true, "template": true, "cms-template": true, "sap-ignore": true,
}

var formState = map[string]bool{"value": true, "checked": true, "selected": true}

// Attribute names are lower case in HTML, so a property rule (@readOnly) reads the attribute sapjs
// paints under the lower case name. sapjs's css:name paints --name.
func attrComputed(n *html.Node, prop string) bool {
	if textProps[prop] {
		return textComputed(n)
	}
	if selfOrAncestor(n, isComputedRegion) {
		return true
	}
	if prop == "" || !appOf(n) {
		return false
	}
	if isCustomPropertyName(prop) {
		return hasAttr(n, "css:"+strings.ToLower(prop[2:]))
	}
	name := strings.ToLower(prop)
	if prop == "className" || prop == "classList" {
		name = "class"
	}
	switch {
	case name == "hidden":
		return attrNameWhere(n, func(a string) bool {
			return a == "show" || strings.HasPrefix(a, "show-when:") || strings.HasPrefix(a, "hide-when:") ||
				strings.HasPrefix(a, "option:") || strings.HasPrefix(a, "option-not:")
		})
	case name == "class":
		return attrNameWhere(n, func(a string) bool { return strings.HasPrefix(a, "class:") })
	case name == "style":
		return attrNameWhere(n, func(a string) bool { return strings.HasPrefix(a, "css:") })
	case neverPainted[name]:
		return false
	case formState[name] && ownsFormState(n):
		return false
	}
	return hasAttr(n, "attr:"+name) && !declares(n, name)
}

func attrNameWhere(n *html.Node, pred func(string) bool) bool {
	for _, a := range n.Attr {
		if pred(a.Key) {
			return true
		}
	}
	return false
}

func paintsText(n *html.Node) bool {
	if hasAttr(n, "bind") {
		return false
	}
	if !attrNameWhere(n, func(a string) bool { return a == "text" || strings.HasPrefix(a, "text:") }) {
		return false
	}
	return appOf(n)
}

func sheetComputed(n *html.Node) bool {
	tables := []*html.Node{n}
	if !isGridTable(n) {
		tables = findRule(n, "table[grid]")
	}
	for _, table := range tables {
		if selfOrAncestor(table, isInert) {
			continue
		}
		if hasAttr(table, "stale") || len(findRule(table, "td[f], th[f]")) > 0 {
			return true
		}
	}
	return false
}

// appOf reports whether n sits in a [sap] app that paints it: no inert subtree on the way up.
func appOf(n *html.Node) bool {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur.Type != html.ElementNode {
			continue
		}
		if isInert(cur) {
			return false
		}
		if hasAttr(cur, "sap") {
			return true
		}
	}
	return false
}

func ownsFormState(n *html.Node) bool {
	if hasAttr(n, "bind") || hasAttr(n, "persist") {
		return true
	}
	return selfOrAncestor(n, func(c *html.Node) bool { return c.Data == "select" && hasAttr(c, "bind") })
}

func declares(n *html.Node, name string) bool {
	decl, _ := attrValue(n, "state")
	for _, token := range strings.Fields(decl) {
		head, _, _ := strings.Cut(token, "=")
		head, _, _ = strings.Cut(head, ":")
		if head == name {
			return true
		}
	}
	return false
}

// withoutMeta drops a top level __meta__ key from a write body unless the rules define it: a host
// answers a write with that key, and an agent may post the whole object back.
func withoutMeta(rules, data Value) Value {
	obj, ok := data.(*Object)
	if !ok {
		return data
	}
	if _, has := obj.Get(MetaKey); !has {
		return data
	}
	if r, ok := rules.(*Object); ok {
		if _, defined := r.Get(MetaKey); defined {
			return data
		}
	}
	out := NewObject()
	for _, key := range obj.Keys() {
		if key != MetaKey {
			v, _ := obj.Get(key)
			out.Define(key, v)
		}
	}
	return out
}

// MetaKey is the key a host adds to a write answer, and drops from a write body.
const MetaKey = "__meta__"

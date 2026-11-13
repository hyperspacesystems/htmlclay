package dataapi

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/panphora/htmlclay/internal/gridcodec"
)

// ReadOptions are readDocument's options. Token defaults to "api"; Codecs defaults to the sheet
// codec.
type ReadOptions struct {
	Token             string
	Query             Value
	View              string
	Codecs            map[string]Codec
	guardDeclarations *Object
}

// ReadResult is readDocument's { data, version, fast }. Found is false when no rules tag carries
// the token (JavaScript's data: null). Fast is false whenever a <table grid was read through the
// tree.
type ReadResult struct {
	Data    Value
	Version string
	Fast    bool
	Found   bool
}

const gridOpen = "<table grid"

// ReadDocument reads a document's data through its rules tag, from source. Port of hyper-html-api
// src/read.js readDocument: when the sheet codec is in use, every canonical grid is cut out, only
// the rest is parsed, and each hole hands the codec its region's bytes. The holes are verified
// first (each hole is a real element, each index appears once and in order, and every hole lies
// under the element a rule naming the sheet codec selected); anything else reads the whole source
// through the tree.
func ReadDocument(src []byte, opts ReadOptions) (*ReadResult, error) {
	token := opts.Token
	if token == "" {
		token = "api"
	}
	codecs := opts.Codecs
	if codecs == nil {
		codecs = map[string]Codec{"sheet": SheetCodec}
	}
	body := string(trimBOM(src))

	if _, cutting := codecs["sheet"].(sheetCodec); cutting {
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
			fast := strings.Count(rest, gridOpen) == len(regions)
			if found == nil {
				return &ReadResult{Fast: fast}, nil
			}
			if readHolesHold(d, found.Rules, "sheet", marker, len(regions)) {
				region := func(table *html.Node) (string, bool) { return regionBytes(table, body, marker, regions) }
				tree := d
				if readHasOrdinaryRules(found.Rules) {
					tree, err = ParseBytes([]byte(body))
					if err != nil {
						return nil, err
					}
				}
				data, err := extractReadData(d, tree, found, opts, codecs, region)
				if err != nil {
					return nil, err
				}
				return &ReadResult{Data: data, Version: found.Version, Fast: fast, Found: true}, nil
			}
		}
	}

	fast := !strings.Contains(body, gridOpen)
	d, err := ParseBytes([]byte(body))
	if err != nil {
		return nil, err
	}
	found, err := d.FindRulesIn(token)
	if err != nil {
		return nil, err
	}
	if found == nil {
		return &ReadResult{Fast: fast}, nil
	}
	data, err := extractReadData(d, d, found, opts, codecs, nil)
	if err != nil {
		return nil, err
	}
	return &ReadResult{Data: data, Version: found.Version, Fast: fast, Found: true}, nil
}

// holesHold is read.js holesHold. Holes are found by walking the whole tree (as cheerio's
// $('table[data-hole]') does), not through Find, whose results skip some regions.
func holesHold(d *Document, rules Value, name, marker string, count int) bool {
	var holes []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "table" && hasAttribute(c, "data-hole") {
				holes = append(holes, c)
			}
			walk(c)
		}
	}
	walk(d.Root)
	if len(holes) != count {
		return false
	}
	containers := codecContainers(d, rules, name)
	if len(containers) == 0 {
		return false
	}
	for n, hole := range holes {
		if v, _ := attrValue(hole, "data-hole"); v != marker+"-"+strconv.Itoa(n) {
			return false
		}
		inside := false
		for _, c := range containers {
			for p := hole; p != nil; p = p.Parent {
				if p == c {
					inside = true
					break
				}
			}
			if inside {
				break
			}
		}
		if !inside {
			return false
		}
	}
	return true
}

var sheetContainerSelector = regexp.MustCompile(`^\[items\s*=\s*(?:sheets|"sheets"|'sheets')\]$`)

func readHolesHold(d *Document, rules Value, name, marker string, count int) bool {
	obj, ok := rules.(*Object)
	if !ok {
		return false
	}
	for _, key := range obj.Keys() {
		value, _ := obj.Get(key)
		rule, _ := value.(string)
		selector, codec, isCodec, err := codecOf(rule)
		if err != nil || isCodec && (codec != name || !sheetContainerSelector.MatchString(selector)) {
			return false
		}
	}
	if !holesHold(d, rules, name, marker, count) {
		return false
	}
	containers := make(map[*html.Node]bool)
	for _, container := range codecContainers(d, rules, name) {
		containers[container] = true
	}
	grids := 0
	valid := true
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" && hasAttribute(n, "grid") {
			grids++
			if n.Parent == nil || n.Parent.Data != "section" || !hasAttribute(n.Parent, "item") || !containers[n.Parent.Parent] || !hasAttribute(n, "data-hole") {
				valid = false
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(d.Root)
	return valid && grids == count
}

// codecContainers is the element each top level selector@codec(name) rule selects, as the engine
// reads it: the first match, or the document root for a bare "@codec(name)".
func codecContainers(d *Document, rules Value, name string) []*html.Node {
	obj, ok := rules.(*Object)
	if !ok {
		return nil
	}
	var out []*html.Node
	for _, key := range obj.Keys() {
		v, _ := obj.Get(key)
		rule, ok := v.(string)
		if !ok {
			continue
		}
		selector, codec, isCodec, err := codecOf(rule)
		if err != nil || !isCodec || codec != name {
			continue
		}
		if selector == "" {
			out = append(out, d.Root)
			continue
		}
		matches, err := Find(d.Root, selector, FindOpts{})
		if err == nil && len(matches) > 0 {
			out = append(out, matches[0])
		}
	}
	return out
}

func regionBytes(table *html.Node, src, marker string, regions []gridcodec.Region) (string, bool) {
	hole, ok := attrValue(table, "data-hole")
	if !ok || !strings.HasPrefix(hole, marker+"-") {
		return "", false
	}
	n, err := strconv.Atoi(hole[len(marker)+1:])
	if err != nil || n < 0 || n >= len(regions) {
		return "", false
	}
	return src[regions[n].Start:regions[n].End], true
}

func readHasOrdinaryRules(rules Value) bool {
	obj, ok := rules.(*Object)
	if !ok {
		return true
	}
	for _, key := range obj.Keys() {
		value, _ := obj.Get(key)
		rule, _ := value.(string)
		_, _, codec, err := codecOf(rule)
		if err != nil || !codec {
			return true
		}
	}
	return false
}

func ReadGuardDocument(src []byte) (*ReadResult, *Object, error) {
	declarations := NewObject()
	result, err := ReadDocument(src, ReadOptions{View: "digest", guardDeclarations: declarations})
	return result, declarations, err
}

func extractReadData(d, tree *Document, tag *RulesTag, opts ReadOptions, codecs map[string]Codec, region func(*html.Node) (string, bool)) (Value, error) {
	options := ExtractOptions{Version: tag.Version, Codecs: codecs, Query: opts.Query, View: opts.View, Region: region}
	rules, object := tag.Rules.(*Object)
	if !object || tree == d && opts.guardDeclarations == nil {
		return d.ExtractWith(tag.Rules, options)
	}
	if opts.guardDeclarations == nil {
		if err := checkCodecRules(tag.Rules, tag.Version, codecs); err != nil {
			return nil, err
		}
		data := NewObject()
		for _, key := range rules.Keys() {
			rule, _ := rules.Get(key)
			text, _ := rule.(string)
			_, _, codec, _ := codecOf(text)
			context := tree
			if codec {
				context = d
			}
			one := NewObject()
			one.Define(key, rule)
			part, err := context.ExtractWith(one, options)
			if err != nil {
				return nil, err
			}
			value, _ := part.(*Object).Get(key)
			data.Define(key, value)
		}
		return data, nil
	}
	ordinary := NewObject()
	for _, key := range rules.Keys() {
		rule, _ := rules.Get(key)
		text, _ := rule.(string)
		_, name, codec, err := codecOf(text)
		if err != nil {
			return nil, err
		}
		if codec {
			opts.guardDeclarations.Define(key, name)
		} else {
			ordinary.Define(key, rule)
		}
	}
	if opts.guardDeclarations.Len() == 0 {
		return d.ExtractWith(tag.Rules, options)
	}
	value, err := tree.ExtractWith(ordinary, options)
	if err != nil {
		return nil, err
	}
	data, ok := value.(*Object)
	if !ok {
		data = NewObject()
	}
	for _, key := range opts.guardDeclarations.Keys() {
		one := NewObject()
		rule, _ := rules.Get(key)
		one.Define(key, rule)
		part, err := d.ExtractWith(one, options)
		if err != nil {
			continue
		}
		if obj, ok := part.(*Object); ok {
			if value, found := obj.Get(key); found {
				data.Define(key, value)
			}
		}
	}
	return data, nil
}

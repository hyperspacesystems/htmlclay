package dataapi

import (
	"regexp"

	"golang.org/x/net/html"
)

// A Codec reads one element's whole value as JSON, for content the rule grammar cannot describe
// (a sheet's grid). It is named by the rule form "selector@codec(name)", valid only under
// data-rules-version="2" and only as the value of a top level key. Ported from codecs.js.
type Codec interface {
	Read(d *Document, node *html.Node, ctx CodecContext) (Value, error)
	Write(d *Document, node *html.Node, value Value, ctx CodecContext) error
}

// CodecContext is what a read passes through to the codec: the caller's query and view.
type CodecContext struct {
	Query  Value
	View   string
	Region func(table *html.Node) (string, bool) // Region returns a cut table's canonical bytes (the fast route); nil on the tree route.
}

// ExtractOptions configures ExtractWith. An empty Version means "1". Codecs maps a codec name to
// its implementation; a codec rule naming anything else refuses with UnknownCodec.
type ExtractOptions struct {
	Version string
	Codecs  map[string]Codec
	Query   Value
	View    string
	Region  func(table *html.Node) (string, bool) // Region returns a cut table's canonical bytes (the fast route); nil on the tree route.
}

const (
	needsVersion2 = `The @codec(name) rule form needs a rules tag with data-rules-version="2".`
	needsTopLevel = "A @codec(name) rule must be the value of a top level key."
	codecNoStar   = "A @codec(name) rule cannot use [*]."
)

var codecProp = regexp.MustCompile(`^codec\(([a-z][a-z0-9-]*)\)$`)

// codecOf reports whether a scalar rule calls a codec, and which. A codec rule with [*] is a
// parse error: a codec owns one element's whole value.
func codecOf(rule string) (selector, name string, ok bool, err error) {
	at := ruleAttrIndex(rule)
	if at == -1 {
		return "", "", false, nil
	}
	m := codecProp.FindStringSubmatch(rule[at+1:])
	if m == nil {
		return "", "", false, nil
	}
	selector, _, _, all := splitRule(rule)
	if all {
		return "", "", false, &RulesParseError{Message: codecNoStar}
	}
	return selector, m[1], true, nil
}

// checkCodecRules walks the whole rule tree once, before anything is read or written, so a rule
// tree the call cannot honor refuses without a partial result. Order of checks matches the
// reference: version, then position, then the codec's name.
func checkCodecRules(rules Value, version string, codecs map[string]Codec) error {
	if version == "" {
		version = "1"
	}
	var walk func(rule Value, depth int) error
	walk = func(rule Value, depth int) error {
		if depth > maxRuleDepth {
			return nil
		}
		switch r := rule.(type) {
		case string:
			_, name, ok, err := codecOf(r)
			if err != nil || !ok {
				return err
			}
			if version != "2" {
				return &RulesParseError{Message: needsVersion2}
			}
			if depth != 1 {
				return &RulesParseError{Message: needsTopLevel}
			}
			if _, found := codecs[name]; !found {
				return &UnknownCodec{Name: name}
			}
		case []Value:
			if len(r) > 1 {
				return walk(r[1], depth+1)
			}
		case *Object:
			for _, key := range r.Keys() {
				sub, _ := r.Get(key)
				if err := walk(sub, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(rules, 0)
}

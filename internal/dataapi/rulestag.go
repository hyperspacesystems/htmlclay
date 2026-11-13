package dataapi

import (
	"regexp"

	"golang.org/x/net/html"
)

// SupportedRulesVersions are the data-rules-version values this engine accepts. Version 2 adds
// the selector@codec(name) rule form; ordinary rules read the same under both.
var SupportedRulesVersions = []string{"1", "2"}

func supportedRulesVersion(version string) bool {
	for _, v := range SupportedRulesVersions {
		if v == version {
			return true
		}
	}
	return false
}

// rulesToken is validated BEFORE it is interpolated into a selector, and rejected rather than
// sanitised. The charset is also what forecloses selector injection through the ~= match.
var rulesToken = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// RulesTag is a page's own published endpoint definition.
type RulesTag struct {
	Rules   Value
	Node    *html.Node
	Version string
}

// FindRulesIn returns the first script[data-rules-name~="token"] in root, with its body parsed.
// A missing tag is (nil, nil) — not an error — because the HTTP status for "this page publishes
// no such endpoint" is the server's call, not the engine's.
//
// The JS also has a no-token form that takes the first data-rules-name script of any name. That is
// a tooling path with no caller on the server side, so it is not ported.
func (d *Document) FindRulesIn(token string) (*RulesTag, error) {
	if !rulesToken.MatchString(token) {
		return nil, &InvalidRulesToken{Token: token}
	}

	// includeRulesTag is REQUIRED here: isRulesTag matches any data-rules-name script, so without
	// it Find would filter out the very tag being selected.
	candidates, err := Find(d.Root, `script[data-rules-name~="`+token+`"]`, FindOpts{IncludeRulesTag: true})
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// More than one match is not an error; first wins. The JS logs a warning here, which is a
	// library talking to a developer's console and has no equivalent in a Go server.
	tagNode := candidates[0]

	version, _ := attrValue(tagNode, "data-rules-version")
	if !supportedRulesVersion(version) {
		return nil, &UnknownRulesVersion{Version: version}
	}

	// Script bodies take the same relaxed syntax as ?data=. The body is TRIMMED first, because
	// the JS reads it through adapter.text().
	rules, err := ParseRelaxed(trimmedText(tagNode))
	if err != nil {
		return nil, err
	}
	return &RulesTag{Rules: rules, Node: tagNode, Version: version}, nil
}

package dataapi

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/panphora/htmlclay/internal/gridcodec"
)

// Temporary markers recover original offsets while letting the HTML parser decide which section
// closing tags it honors. A close inside an open table or template can be ignored by the parser.
func locateDeleteTabs(body, token string) ([]sheetTab, error) {
	tokens := tokenizeSource([]byte(body))
	marker := gridcodec.HoleMarker(body)
	d, err := ParseBytes([]byte(markSourceTags(body, tokens, marker)))
	if err != nil {
		return nil, err
	}
	starts := map[*html.Node]sourceToken{}
	comments := map[string]*html.Node{}
	owners := map[*html.Node]*html.Node{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if raw, ok := removeAttribute(n, "data-"+marker); ok {
				index, err := strconv.Atoi(raw)
				if err == nil && index >= 0 && index < len(tokens) {
					starts[n] = tokens[index]
				}
			}
		} else if n.Type == html.CommentNode && strings.HasPrefix(n.Data, marker) {
			comments[n.Data] = n
		}
		if kids := d.templateChildren(n); kids != nil {
			for _, child := range kids {
				owners[child] = n
				walk(child)
			}
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(d.Root)
	parent := func(n *html.Node) *html.Node {
		if n.Parent != nil {
			return n.Parent
		}
		return owners[n]
	}
	ends := map[*html.Node]int{}
	for index, tok := range tokens {
		if tok.kind != html.EndTagToken || tok.name != "section" {
			continue
		}
		before := comments[marker+"-before-"+strconv.Itoa(index)]
		after := comments[marker+"-after-"+strconv.Itoa(index)]
		if before == nil || after == nil {
			continue
		}
		var section *html.Node
		for n := parent(before); n != nil; n = parent(n) {
			if n.Type == html.ElementNode && n.Data == "section" {
				section = n
				break
			}
		}
		if section == nil {
			continue
		}
		inside := false
		for n := parent(after); n != nil; n = parent(n) {
			if n == section {
				inside = true
				break
			}
		}
		if !inside {
			ends[section] = tok.end
		}
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
		tab := &tabs[i]
		// The tokenizer and the parser can disagree inside foreign content (an unclosed
		// <style> in an inline SVG). A section or table without a start token is then
		// left unknown, and the delete or stale insertion refuses as not canonical.
		sectionStart, startKnown := starts[tab.section]
		if end, explicit := ends[tab.section]; explicit && startKnown {
			region := gridcodec.Region{Start: sectionStart.start, End: end}
			tab.sectionRegion = &region
		}
		if tab.table == nil {
			continue
		}
		start, tableKnown := starts[tab.table]
		if !tableKnown {
			tab.staleAt = -1
			continue
		}
		tab.staleAt = start.end - 1
		if start.kind == html.SelfClosingTagToken {
			tab.staleAt--
		}
		if end, scanErr := gridcodec.ScanGrid(body, start.start, nil); scanErr == nil {
			region := gridcodec.Region{Start: start.start, End: end}
			tab.region = &region
		}
	}
	return tabs, nil
}

func markSourceTags(body string, tokens []sourceToken, marker string) string {
	var out strings.Builder
	from := 0
	for index, token := range tokens {
		if token.kind == html.EndTagToken && token.name == "section" {
			out.WriteString(body[from:token.start])
			out.WriteString("<!--" + marker + "-before-" + strconv.Itoa(index) + "-->")
			out.WriteString(body[token.start:token.end])
			out.WriteString("<!--" + marker + "-after-" + strconv.Itoa(index) + "-->")
			from = token.end
			continue
		}
		if token.kind != html.StartTagToken && token.kind != html.SelfClosingTagToken {
			continue
		}
		end := token.start + 1
		for end < token.end && !isHTMLSpace(body[end]) && body[end] != '/' && body[end] != '>' {
			end++
		}
		out.WriteString(body[from:end])
		out.WriteString(" data-" + marker + `="` + strconv.Itoa(index) + `"`)
		from = end
	}
	out.WriteString(body[from:])
	return out.String()
}

func isHTMLSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\f', '\r':
		return true
	}
	return false
}

func removeAttribute(n *html.Node, name string) (string, bool) {
	for i, attr := range n.Attr {
		if attr.Namespace == "" && attr.Key == name {
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			return attr.Val, true
		}
	}
	return "", false
}

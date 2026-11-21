package main

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/codeset"
	"github.com/panphora/htmlclay/internal/tier"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const pendingKeepTTL = 30 * time.Minute

// pendingKeepCode is the code an AI edit on a trusted document would leave
// after Keep: each code-set hash the edit's result can produce in the file.
type pendingKeepCode struct {
	hashes  []string
	expires time.Time
}

// noteAIEditResult precomputes the code-set hash that saving resultHTML into
// document produces, for a document that was already trusted when the AI edit
// started. Only a save matching that exact code-set hash trusts the new code.
func (a *app) noteAIEditResult(document, elementHTML, tag, resultHTML string) {
	path := closedPath(document)
	if !a.tierOf(path).Trusted() {
		return
	}
	if store := a.tierStore(); store != nil {
		if _, ok := store.PathTrust(path); ok {
			return
		}
	}
	origBytes, err := os.ReadFile(path)
	if err != nil {
		return
	}
	hashes := expectedKeepCodeHashes(origBytes, elementHTML, tag, resultHTML)
	if current, ok := a.codeSetOf(path); ok {
		hashes = slices.DeleteFunc(hashes, func(h string) bool { return h == current.Hash })
	}
	if len(hashes) == 0 {
		return
	}
	a.initCore()
	a.core.mu.Lock()
	if a.core.pendingKeep == nil {
		a.core.pendingKeep = make(map[string]pendingKeepCode)
	}
	a.core.pendingKeep[path] = pendingKeepCode{
		hashes:  hashes,
		expires: time.Now().Add(pendingKeepTTL),
	}
	a.core.mu.Unlock()
}

// noteDocumentSaved trusts the code-set hash of savedBytes when it matches a
// pending AI edit result for that document, as a save after Keep does. Nothing
// else is trusted here, and the path itself never is.
func (a *app) noteDocumentSaved(document string, savedBytes []byte) {
	path := closedPath(document)
	a.initCore()
	now := time.Now()
	a.core.mu.Lock()
	pending, ok := a.core.pendingKeep[path]
	if ok && now.After(pending.expires) {
		delete(a.core.pendingKeep, path)
		ok = false
	}
	a.core.mu.Unlock()
	if !ok {
		return
	}

	savedSet, err := codeset.Of(savedBytes)
	if err != nil || savedSet.Parts == 0 || !slices.Contains(pending.hashes, savedSet.Hash) {
		return
	}

	a.core.mu.Lock()
	delete(a.core.pendingKeep, path)
	delete(a.core.codeSets, path)
	a.core.mu.Unlock()

	store := a.tierStore()
	if store == nil {
		return
	}
	if err := store.TrustCodeViaPath(savedSet.Hash, path, tier.HowAIEdit); err != nil {
		a.rt.logger.Printf("Could not record AI-edited code trust for %s: %v", path, err)
	}
}

func expectedKeepCodeHashes(origBytes []byte, elementHTML, tag, resultHTML string) []string {
	var hashes []string
	addHash := func(candidate []byte) {
		set, err := codeset.Of(candidate)
		if err != nil || set.Parts == 0 || set.Hash == "" {
			return
		}
		if !slices.Contains(hashes, set.Hash) {
			hashes = append(hashes, set.Hash)
		}
	}

	if elementHTML != "" {
		origStr := string(origBytes)
		searchFrom := 0
		for {
			idx := strings.Index(origStr[searchFrom:], elementHTML)
			if idx < 0 {
				break
			}
			pos := searchFrom + idx
			spliced := origStr[:pos] + resultHTML + origStr[pos+len(elementHTML):]
			addHash([]byte(spliced))
			searchFrom = pos + len(elementHTML)
		}
	}

	// A browser's outerHTML can order attributes or space text differently
	// from the file, so the element is also found and replaced as a tree.
	doc, err := html.Parse(bytes.NewReader(origBytes))
	if err != nil {
		return hashes
	}
	lowerTag := strings.ToLower(strings.TrimSpace(tag))
	if lowerTag == "body" {
		if bodyNode := findFirstTag(doc, "body"); bodyNode != nil {
			if patched, ok := replaceElementWithHTML(doc, bodyNode, resultHTML); ok {
				addHash(patched)
			}
		}
		return hashes
	}

	targetElem := parseSingleElement(elementHTML)
	if targetElem == nil {
		return hashes
	}
	var matches []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && elementsEquivalent(n, targetElem) {
			matches = append(matches, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	for _, m := range matches {
		if patched, ok := replaceElementWithHTML(doc, m, resultHTML); ok {
			addHash(patched)
		}
	}
	return hashes
}

func findFirstTag(n *html.Node, name string) *html.Node {
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, name) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findFirstTag(c, name); found != nil {
			return found
		}
	}
	return nil
}

func parseSingleElement(fragment string) *html.Node {
	ctx := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
	nodes, err := html.ParseFragment(strings.NewReader(strings.TrimSpace(fragment)), ctx)
	if err != nil {
		return nil
	}
	for _, n := range nodes {
		if n.Type == html.ElementNode {
			return n
		}
	}
	return nil
}

func elementsEquivalent(a, b *html.Node) bool {
	if !strings.EqualFold(a.Data, b.Data) || len(a.Attr) != len(b.Attr) {
		return false
	}
	bAttrs := make(map[string]string, len(b.Attr))
	for _, at := range b.Attr {
		bAttrs[strings.ToLower(at.Key)] = at.Val
	}
	for _, at := range a.Attr {
		if v, ok := bAttrs[strings.ToLower(at.Key)]; !ok || v != at.Val {
			return false
		}
	}
	return strings.TrimSpace(nodeText(a)) == strings.TrimSpace(nodeText(b))
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(cur *html.Node) {
		if cur.Type == html.TextNode {
			b.WriteString(cur.Data)
		}
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func replaceElementWithHTML(doc *html.Node, target *html.Node, replacementHTML string) ([]byte, bool) {
	if strings.EqualFold(target.Data, "body") {
		// html.ParseFragment on <html> ignores <body> tags in some parser modes,
		// so parse a minimal document and extract its <body>.
		parsedBodyDoc, err := html.Parse(strings.NewReader("<!doctype html><html>" + replacementHTML + "</html>"))
		if err != nil {
			return nil, false
		}
		newBody := findFirstTag(parsedBodyDoc, "body")
		if newBody == nil {
			return nil, false
		}
		newBody.Parent, newBody.PrevSibling, newBody.NextSibling = nil, nil, nil
		return renderWithReplacedNode(doc, target, []*html.Node{newBody})
	}
	parentTag := "body"
	parentAtom := atom.Body
	if target.Parent != nil && target.Parent.Type == html.ElementNode {
		parentTag = strings.ToLower(target.Parent.Data)
		parentAtom = target.Parent.DataAtom
	}
	ctx := &html.Node{Type: html.ElementNode, DataAtom: parentAtom, Data: parentTag}
	nodes, err := html.ParseFragment(strings.NewReader(replacementHTML), ctx)
	if err != nil || len(nodes) == 0 {
		return nil, false
	}
	return renderWithReplacedNode(doc, target, nodes)
}

func renderWithReplacedNode(doc *html.Node, target *html.Node, replacements []*html.Node) ([]byte, bool) {
	parent := target.Parent
	if parent == nil {
		return nil, false
	}
	next := target.NextSibling
	for _, r := range replacements {
		r.Parent, r.PrevSibling, r.NextSibling = nil, nil, nil
		parent.InsertBefore(r, target)
	}
	parent.RemoveChild(target)

	var buf bytes.Buffer
	err := html.Render(&buf, doc)

	// Restore doc so other match candidates see the original tree.
	for _, r := range replacements {
		parent.RemoveChild(r)
	}
	if next != nil {
		parent.InsertBefore(target, next)
	} else {
		parent.AppendChild(target)
	}
	if err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

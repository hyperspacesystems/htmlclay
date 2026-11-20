// Package codeset finds the code in an HTML document and hashes it.
//
// The code set is every part of a page that runs a program or steers one:
// scripts that are not JSON data (program and instruction tags included),
// event handler and framework attributes, script URLs, srcdoc, and the tags
// that load or frame other content. Everything else is data, and editing data
// never changes the hash. HTML Clay's trust list and the ecosystem signature
// both key on this hash, and ClayJS computes the same one from the same
// definition, pinned by the vectors in testdata/vectors.json.
package codeset

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Version names the definition below. It is the first field hashed, so a
// future definition can never collide with this one.
const Version = "clay-codeset-v1"

// SignatureMeta is the name of the meta tag that carries an ecosystem
// signature. It is never part of the code set.
const SignatureMeta = "clay-signature"

// MaxDocument is the largest document Of reads, HTML Clay's own save limit.
const MaxDocument = 50 << 20

var (
	activeAttr  = regexp.MustCompile(`^(on|hx-|x-|@|:|v-on:|v-bind:|v-|data-hx-|data-init|data-effect|data-on[-:])`)
	activeTags  = map[string]bool{"iframe": true, "frame": true, "object": true, "embed": true, "base": true, "link": true}
	dataScripts = map[string]bool{"application/json": true, "application/ld+json": true}
	sriToken    = regexp.MustCompile(`^(sha256|sha384|sha512)-[A-Za-z0-9+/_-]+={0,2}(\?\S*)?$`)
	// scriptHosts serve code a signed document may load without integrity:
	// HTML Clay's own sites, which serve ClayJS.
	scriptHosts = map[string]bool{"clayjs.com": true, "hyperclay.com": true}
)

// Set is what Of found in one document.
type Set struct {
	// Hash is the lowercase hex SHA-256 of the code set.
	Hash string
	// Parts is how many code parts the hash covers.
	Parts int
	// Signatures are the content values of the clay-signature meta tags in the
	// document's head, in document order. A tag anywhere else is ignored.
	Signatures []string
	// Unpinned are the URLs of scripts that load code the hash cannot cover:
	// no integrity a browser checks, and not from https://clayjs.com or
	// https://hyperclay.com. A signature never counts while there are any.
	Unpinned []string
}

// Of reads data as HTML the way a browser does and returns its code set.
func Of(data []byte) (Set, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return Set{}, err
	}
	w := walker{}
	w.walk(doc, false)
	h := sha256.New()
	h.Write(field(Version))
	for _, p := range w.parts {
		h.Write(field(string(p)))
	}
	return Set{Hash: hex.EncodeToString(h.Sum(nil)), Parts: len(w.parts), Signatures: w.signatures, Unpinned: w.unpinned}, nil
}

type walker struct {
	parts      [][]byte
	signatures []string
	unpinned   []string
}

func (w *walker) walk(n *html.Node, inHead bool) {
	if n.Type == html.ElementNode {
		tag := tagName(n)
		if tag == "noscript" && n.Namespace == "" {
			return
		}
		if isSignatureMeta(n, tag) {
			if inHead {
				w.signatures = append(w.signatures, attr(n, "content"))
			}
			return
		}
		w.element(n, tag)
		if n.DataAtom == atom.Head && n.Namespace == "" {
			inHead = true
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.walk(c, inHead)
	}
}

// isSignatureMeta reports whether n is a clay-signature meta with nothing but a
// name and a content. Any other attribute, such as http-equiv, could act, so
// such a tag is code like any other and never a signature.
func isSignatureMeta(n *html.Node, tag string) bool {
	if tag != "meta" || !strings.EqualFold(strings.TrimSpace(attr(n, "name")), SignatureMeta) {
		return false
	}
	for _, a := range n.Attr {
		if name := attrName(a); name != "name" && name != "content" {
			return false
		}
	}
	return true
}

func (w *walker) element(n *html.Node, tag string) {
	attrs := sortedAttrs(n)
	switch {
	case tag == "script":
		if !dataScripts[mediaType(attr(n, "type"))] {
			w.add(append([]string{"script", textContent(n)}, attrs...)...)
			w.unpinned = append(w.unpinned, unpinnedSources(n)...)
		}
	case activeTags[tag], tag == "meta" && hasAttr(n, "http-equiv"),
		(tag == "animate" || tag == "set") && strings.Contains(strings.ToLower(attr(n, "attributename")), "href"):
		w.add(append([]string{"tag", tag}, attrs...)...)
	}
	for i := 0; i < len(attrs); i += 2 {
		name, value := attrs[i], attrs[i+1]
		if activeAttr.MatchString(name) || name == "srcdoc" || IsScriptURL(value) {
			w.add("attr", tag, name, value)
		}
	}
}

// unpinnedSources returns the script's source URLs that load code the hash
// cannot cover. An HTML script loads src; an SVG script loads href or
// xlink:href. A browser ignores an integrity value it cannot read and loads the
// script unchecked, so only a well-formed sha256, sha384 or sha512 value pins.
func unpinnedSources(n *html.Node) []string {
	for _, token := range strings.Fields(attr(n, "integrity")) {
		if sriToken.MatchString(token) {
			return nil
		}
	}
	var out []string
	for _, name := range []string{"src", "href", "xlink:href"} {
		if src, ok := attrValue(n, name); ok && !fromScriptHost(src) {
			out = append(out, src)
		}
	}
	return out
}

func fromScriptHost(src string) bool {
	u, err := url.Parse(strings.TrimSpace(src))
	return err == nil && u.Scheme == "https" && u.User == nil && u.Port() == "" && scriptHosts[strings.ToLower(u.Hostname())]
}

func (w *walker) add(fields ...string) {
	var b bytes.Buffer
	for _, f := range fields {
		b.Write(field(f))
	}
	w.parts = append(w.parts, b.Bytes())
}

// field writes s as its byte length, a colon, its bytes and a comma, so no
// sequence of fields can be read two ways.
func field(s string) []byte {
	return []byte(strconv.Itoa(len(s)) + ":" + s + ",")
}

// IsScriptURL reports whether value runs as a script when used as a URL. URL
// parsing drops tab, CR and LF anywhere and control characters and spaces at
// the start, so "java&#9;script:" still counts.
func IsScriptURL(value string) bool {
	v := strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, value)
	v = strings.ToLower(strings.TrimLeft(v, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x0b\x0c\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f "))
	return strings.HasPrefix(v, "javascript:") || strings.HasPrefix(v, "vbscript:") || strings.HasPrefix(v, "data:text/html")
}

func tagName(n *html.Node) string {
	return strings.ToLower(n.Data)
}

func attrName(a html.Attribute) string {
	if a.Namespace != "" {
		return strings.ToLower(a.Namespace + ":" + a.Key)
	}
	return strings.ToLower(a.Key)
}

func attr(n *html.Node, name string) string {
	v, _ := attrValue(n, name)
	return v
}

func attrValue(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if attrName(a) == name {
			return a.Val, true
		}
	}
	return "", false
}

func hasAttr(n *html.Node, name string) bool {
	for _, a := range n.Attr {
		if attrName(a) == name {
			return true
		}
	}
	return false
}

// sortedAttrs returns name, value pairs sorted by name as raw bytes.
func sortedAttrs(n *html.Node) []string {
	type kv struct{ k, v string }
	list := make([]kv, 0, len(n.Attr))
	for _, a := range n.Attr {
		list = append(list, kv{attrName(a), a.Val})
	}
	slices.SortStableFunc(list, func(a, b kv) int { return strings.Compare(a.k, b.k) })
	out := make([]string, 0, 2*len(list))
	for _, e := range list {
		out = append(out, e.k, e.v)
	}
	return out
}

// mediaType is a script type without parameters, trimmed and lowercased.
func mediaType(t string) string {
	t, _, _ = strings.Cut(t, ";")
	return strings.ToLower(strings.TrimSpace(t))
}

// textContent is the DOM's textContent: every descendant text node in order.
func textContent(n *html.Node) string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.TextNode:
				b.WriteString(c.Data)
			case html.ElementNode:
				visit(c)
			}
		}
	}
	visit(n)
	return b.String()
}

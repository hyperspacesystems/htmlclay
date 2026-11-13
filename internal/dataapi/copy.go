package dataapi

import (
	"bytes"
	"errors"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// errUnlocatableCopy is the export's refusal when a marked section cannot be cut with certainty.
var errUnlocatableCopy = errors.New("Cannot export this copy because a no-copy section could not be located safely. Check that section's HTML and try again.")

// StripCopyContent removes the copy-only sections of a document, leaving every other byte alone.
//
// A section is copy-only when its `clay` attribute holds an exact `no-copy` or
// `no-copy-unless:paid-forms` token, or when it carries `data-paid-only`. An offline copy carries
// neither a tracker nor a paid-only form, so both kinds of section go. The kept bytes are slices of
// the input: nothing is re-encoded, so encoding, line endings and even invalid UTF-8 survive.
//
// When a marked section's byte range cannot be recovered -- misnested markup, foster parenting, a
// self-closing non-void tag whose slash HTML ignores -- the whole strip is refused with an error and
// no bytes. Half a strip would leak what was marked; a re-render would lose the rest of the file.
func StripCopyContent(data []byte) ([]byte, error) {
	lower := bytes.ToLower(data)
	if !bytes.Contains(lower, []byte("clay")) && !bytes.Contains(lower, []byte("data-paid-only")) {
		return data, nil
	}

	src := trimBOM(data)
	prefix := len(data) - len(src)
	d, err := ParseBytes(src)
	if err != nil {
		return nil, err
	}
	var selected []*html.Node
	collectCopyNodes(d, d.Root, &selected)
	if len(selected) == 0 {
		return data, nil
	}

	spans, _ := sourceSpans(d, src)
	cuts := make([]span, 0, len(selected))
	for _, n := range selected {
		sp, ok := spans[n]
		if !ok || sp.start < 0 || sp.end <= sp.start || sp.end > len(src) {
			return nil, errUnlocatableCopy
		}
		if n.Namespace == "" && !voidElements[n.Data] {
			tokenizer := html.NewTokenizer(bytes.NewReader(src[sp.start:sp.end]))
			if tokenizer.Next() == html.SelfClosingTagToken {
				return nil, errUnlocatableCopy
			}
		}
		cuts = append(cuts, span{start: sp.start + prefix, end: sp.end + prefix})
	}
	sort.Slice(cuts, func(i, j int) bool {
		if cuts[i].start != cuts[j].start {
			return cuts[i].start < cuts[j].start
		}
		return cuts[i].end > cuts[j].end
	})

	out := make([]byte, 0, len(data))
	at := 0
	for _, c := range cuts {
		if c.start < at || c.end > len(data) || c.end <= c.start {
			return nil, errUnlocatableCopy
		}
		start := copyCutStart(data, c.start)
		if start < at {
			start = at
		}
		out = append(out, data[at:start]...)
		at = c.end
	}
	out = append(out, data[at:]...)
	return out, nil
}

// collectCopyNodes gathers the outermost copy-only elements in document order. A selected element's
// descendants are left out: the outer cut already removes them, and cutting twice would add a
// refusal for markup the outer range alone can safely cover.
func collectCopyNodes(d *Document, n *html.Node, out *[]*html.Node) {
	if n.Type == html.ElementNode && isCopyOnly(n) {
		*out = append(*out, n)
		return
	}
	if kids := d.templateChildren(n); kids != nil {
		for _, c := range kids {
			collectCopyNodes(d, c, out)
		}
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectCopyNodes(d, c, out)
	}
}

// isCopyOnly reports whether an element is marked for removal. Attribute names fold; the token
// values do not, because they are this document format's own vocabulary.
func isCopyOnly(n *html.Node) bool {
	clay := ""
	hasClay := false
	for _, a := range n.Attr {
		if a.Namespace != "" {
			continue
		}
		if !hasClay && strings.EqualFold(a.Key, "clay") {
			clay = a.Val
			hasClay = true
			continue
		}
		if strings.EqualFold(a.Key, "data-paid-only") {
			return true
		}
	}
	if !hasClay {
		return false
	}
	for _, tok := range strings.FieldsFunc(clay, isCopySpace) {
		if tok == "no-copy" || tok == "no-copy-unless:paid-forms" {
			return true
		}
	}
	return false
}

// isCopySpace is the HTML whitespace set, so a token split never follows Unicode's wider rules.
func isCopySpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
}

// copyCutStart moves a cut back over the indentation in front of it, to just after the newline, so
// removing an element leaves no blank line. It only crosses whitespace: anything else on the way
// stops it at the element's own start tag.
func copyCutStart(data []byte, start int) int {
	nl := bytes.LastIndexByte(data[:start], '\n')
	if nl < 0 {
		return start
	}
	for _, c := range data[nl+1 : start] {
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' && c != '\f' {
			return start
		}
	}
	if nl > 0 && data[nl-1] == '\r' {
		return nl - 1
	}
	return nl
}

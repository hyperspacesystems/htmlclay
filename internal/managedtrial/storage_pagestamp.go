//go:build managedtrial

package managedtrial

import (
	"strings"

	"github.com/panphora/htmlclay/internal/pageadmission"
)

// The restore's stamp, ported from ClayJS's src/server/rooms/page-rebase.js:
// stampPageRev puts a stored page record's revision onto a version's bytes, so a
// restored version of a blockeditor-merge document is a page an open tab can merge onto
// the page it holds, and declares the floor the restore writes at. The article it
// stamps is the one non-inert article[data-blockeditor] storage_pagesplit.go already
// locates, with the same template/noscript rule page-rebase.js filters by.
func stampPageRev(source, rev string, floor int) (string, bool) {
	if rev == "" {
		return "", false
	}
	found := locatePageSplit(source)
	if found.articles != 1 || found.articleStart < 0 {
		return "", false
	}
	end := pageStartTagEnd(source, found.articleStart)
	if end < 0 {
		return "", false
	}
	return pageadmission.WithPageFormatFloor(pageStampOpen(source, found.articleStart, end, rev), floor), true
}

// pageStartTagEnd is where the start tag beginning at start ends: the first '>'
// outside a quoted attribute value.
func pageStartTagEnd(source string, start int) int {
	quote := byte(0)
	for i := start; i < len(source); i++ {
		ch := source[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case '>':
			return i + 1
		}
	}
	return -1
}

// pageStartTagAttr is one attribute of a start tag: its lowercased name and the
// span parse5's source locations give it, from the name to the end of the value.
type pageStartTagAttr struct {
	Name       string
	Start, End int
}

func pageStartTagSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f'
}

// pageStartTagAttributes reads a start tag's attributes with their spans, the way
// the HTML tokenizer does: a name ends at whitespace, '/' or '=', a quoted value
// at its closing quote, an unquoted one at whitespace or '>'.
func pageStartTagAttributes(source string, start, end int) []pageStartTagAttr {
	var attrs []pageStartTagAttr
	i := start + 1
	for i < end && !pageStartTagSpace(source[i]) && source[i] != '/' && source[i] != '>' {
		i++
	}
	for i < end {
		for i < end && pageStartTagSpace(source[i]) {
			i++
		}
		if i >= end || source[i] == '>' || source[i] == '/' {
			return attrs
		}
		nameStart := i
		for i < end && !pageStartTagSpace(source[i]) && source[i] != '/' && source[i] != '=' && source[i] != '>' {
			i++
		}
		name := strings.ToLower(source[nameStart:i])
		valueEnd := i
		at := i
		for at < end && pageStartTagSpace(source[at]) {
			at++
		}
		if at < end && source[at] == '=' {
			at++
			for at < end && pageStartTagSpace(source[at]) {
				at++
			}
			if at < end && (source[at] == '"' || source[at] == '\'') {
				quote := source[at]
				at++
				for at < end && source[at] != quote {
					at++
				}
				if at < end {
					at++
				}
			} else {
				for at < end && !pageStartTagSpace(source[at]) && source[at] != '>' {
					at++
				}
			}
			valueEnd = at
		}
		attrs = append(attrs, pageStartTagAttr{Name: name, Start: nameStart, End: valueEnd})
		i = valueEnd
	}
	return attrs
}

// pageStampOpen is ClayJS's stampOpen over the whole document: the start tag with
// its data-rev replaced or added, using the attribute's own span so a data-rev
// inside another value is left alone, and the first of duplicate names is the one
// replaced, as the parser keeps the first.
func pageStampOpen(source string, start, end int, rev string) string {
	for _, attr := range pageStartTagAttributes(source, start, end) {
		if attr.Name == "data-rev" {
			return source[:attr.Start] + `data-rev="` + rev + `"` + source[attr.End:]
		}
	}
	return source[:end-1] + ` data-rev="` + rev + `">` + source[end:]
}

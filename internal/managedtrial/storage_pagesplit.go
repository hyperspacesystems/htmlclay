//go:build managedtrial

package managedtrial

import (
	"strings"

	"github.com/panphora/htmlclay/internal/pageadmission"
	"golang.org/x/net/html"
)

// The page tag split and compose, ported from ClayJS's src/server/rooms/page-state.js.
// The managed trial host keeps a page editor's merge state out of the person's
// file: on save the one Yjs page tag goes into a sidecar record, and on read it
// is put back before the article the page belongs to.

// PageRecord is the page editor's merge state, the part of a page document the
// host holds beside the person's file.
type PageRecord struct {
	V   int    `json:"v"`
	Rev string `json:"rev"`
	Tag string `json:"tag"`
}

// pageLocations is what one pass over a document finds: the non-inert
// article[data-blockeditor] count and the span of the first, plus the live page tag
// count and the span of the first.
type pageLocations struct {
	articles     int
	articleStart int
	articleEnd   int
	rev          string
	tags         int
	tagStart     int
	tagEnd       int
}

func locatePageSplit(source string) pageLocations {
	found := pageLocations{articleStart: -1, tagStart: -1}
	z := html.NewTokenizer(strings.NewReader(source))
	offset := 0
	var inertTags []string
	articleDepth := 0
	watched := -1
	pendingTag := false
	for {
		token := z.Next()
		at := offset
		offset += len(z.Raw())
		switch token {
		case html.ErrorToken:
			if watched >= 0 {
				found.articleEnd = len(source)
			}
			if pendingTag {
				found.tagEnd = len(source)
			}
			return found
		case html.StartTagToken, html.SelfClosingTagToken:
			name, attrs := pageTokenAttributes(z)
			switch name {
			case "template", "noscript":
				if token == html.StartTagToken {
					inertTags = append(inertTags, name)
				}
			case "article":
				if len(inertTags) == 0 {
					if _, ok := attrs["data-blockeditor"]; ok {
						found.articles++
						if found.articles == 1 {
							found.articleStart = at
							found.rev = attrs["data-rev"]
							if token == html.SelfClosingTagToken {
								found.articleEnd = offset
							} else {
								watched = articleDepth
							}
						}
					}
				}
				if token == html.StartTagToken {
					articleDepth++
				}
			case "script":
				if token == html.StartTagToken && len(inertTags) == 0 && !pendingTag && pageLiveTag(attrs) {
					found.tags++
					if found.tags == 1 {
						found.tagStart = at
						pendingTag = true
					}
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "template", "noscript":
				for i := len(inertTags) - 1; i >= 0; i-- {
					if inertTags[i] == string(name) {
						inertTags = inertTags[:i]
						break
					}
				}
			case "article":
				if articleDepth > 0 {
					articleDepth--
				}
				if watched >= 0 && articleDepth <= watched {
					found.articleEnd = offset
					watched = -1
				}
			case "script":
				if pendingTag {
					found.tagEnd = offset
					pendingTag = false
				}
			}
		}
	}
}

// pageLiveTag reports ClayJS's locate selector, script[merge="blockeditor"] carrying
// the clay token live outside template and noscript.
func pageLiveTag(attrs map[string]string) bool {
	return attrs["merge"] == "blockeditor" && pageHasToken(attrs["clay"], "live")
}

func pageHasToken(source, token string) bool {
	for _, part := range strings.Fields(source) {
		if part == token {
			return true
		}
	}
	return false
}

func pageTokenAttributes(z *html.Tokenizer) (string, map[string]string) {
	name, _ := z.TagName()
	attrs := map[string]string{}
	for {
		key, value, more := z.TagAttr()
		attrs[string(key)] = string(value)
		if !more {
			break
		}
	}
	return string(name), attrs
}

func pageLineStart(source string, at int) int {
	for i := at - 1; i >= 0; i-- {
		if source[i] == '\n' || source[i] == '\r' {
			return i + 1
		}
	}
	return 0
}

func pageLineEnd(source string, at int) int {
	for i := at; i < len(source); i++ {
		if source[i] == '\n' {
			return i + 1
		}
		if source[i] == '\r' {
			if i+1 < len(source) && source[i+1] == '\n' {
				return i + 2
			}
			return i + 1
		}
	}
	return len(source)
}

func pageNewlineOf(source string, from int) string {
	lo := from - 2
	if lo < 0 {
		lo = 0
	}
	before := source[lo:from]
	switch {
	case strings.HasSuffix(before, "\r\n"):
		return "\r\n"
	case strings.HasSuffix(before, "\n"):
		return "\n"
	case strings.HasSuffix(before, "\r"):
		return "\r"
	}
	for i := 0; i < len(source); i++ {
		switch source[i] {
		case '\r':
			if i+1 < len(source) && source[i+1] == '\n' {
				return "\r\n"
			}
			return "\r"
		case '\n':
			return "\n"
		}
	}
	return "\n"
}

// pageRev is ClayJS's pageRev: the revision of the one non-inert
// article[data-blockeditor] in a document, or nothing when it has no such single
// article or the article carries no data-rev.
func pageRev(source string) string {
	found := locatePageSplit(source)
	if found.articles != 1 {
		return ""
	}
	return found.rev
}

// splitPage removes the page editor's merge state from a page document and
// returns it as a record: the file without the tag, and the tag with the
// revision of the article it belongs to. A document that is not exactly one
// live Yjs page tag beside exactly one revisioned article comes back unchanged
// with no record, so a tag that is the only copy of the page stays in the file.
func splitPage(source string) (string, *PageRecord) {
	if pageadmission.Inspect(source).Kind != "yjs" {
		return source, nil
	}
	found := locatePageSplit(source)
	if found.articles != 1 || found.tags != 1 || found.rev == "" {
		return source, nil
	}
	if found.tagStart < found.articleEnd && found.tagEnd > found.articleStart {
		return source, nil
	}
	start, end := pageTagSpan(source, found.tagStart, found.tagEnd)
	return source[:start] + source[end:], &PageRecord{V: 1, Rev: found.rev, Tag: source[found.tagStart:found.tagEnd]}
}

// composePage is the document an editor sees: the file with the stored page tag
// put back before its article, when the record still belongs to that article.
func composePage(file string, record *PageRecord) string {
	if record == nil || record.V != 1 || record.Rev == "" {
		return file
	}
	if pageadmission.Inspect(file).Kind != "none" {
		return file
	}
	found := locatePageSplit(file)
	if found.articles != 1 || found.rev != record.Rev || found.articleStart < 0 {
		return file
	}
	at := found.articleStart
	from := pageLineStart(file, at)
	indent := file[from:at]
	insert := record.Tag
	if strings.TrimSpace(indent) == "" {
		insert = record.Tag + pageNewlineOf(file, from) + indent
	}
	return file[:at] + insert + file[at:]
}

// pageTagSpan widens the tag's span to its whole line when nothing but
// whitespace shares that line with it, the way ClayJS's spanOf does.
func pageTagSpan(source string, start, end int) (int, int) {
	from := pageLineStart(source, start)
	to := pageLineEnd(source, end)
	if strings.TrimSpace(source[from:start]) == "" && strings.TrimSpace(source[end:to]) == "" {
		return from, to
	}
	return start, end
}

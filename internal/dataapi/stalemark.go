package dataapi

import (
	"bytes"

	"golang.org/x/net/html"
)

// StaleMark is the element a data write leaves in the head of a document whose computed values
// it could not recalculate (calculator documents plan, M1b). It is hyper-html-api's
// src/stale-mark.js byte for byte. Every ClayJS save strips no-save, so the first page that
// recalculates and saves removes it.
var StaleMark = []byte(`<meta name="clay-stale" clay="no-save">`)

// HasStaleMark reports whether src carries the stale mark.
func HasStaleMark(src []byte) bool {
	return bytes.Contains(src, StaleMark)
}

// withStaleMark returns src with the mark inserted: before </head>; else just after an unclosed
// <head ...>; else, wrapped in a head of its own, before <body ...>. A document with none of these
// tags, or one that already carries the mark, comes back unchanged.
func withStaleMark(src []byte) []byte {
	if HasStaleMark(src) {
		return src
	}
	at, wrap := staleMarkAt(src)
	if at < 0 {
		return src
	}
	mark := StaleMark
	if wrap {
		mark = append(append([]byte("<head>"), StaleMark...), "</head>"...)
	}
	out := make([]byte, 0, len(src)+len(mark))
	out = append(out, src[:at]...)
	out = append(out, mark...)
	return append(out, src[at:]...)
}

// staleMarkAt finds where the mark goes, scanning tags only up to the first <body>. The tokenizer
// treats script, style, title and textarea contents as text, so a </head> inside them is skipped.
func staleMarkAt(src []byte) (at int, wrap bool) {
	z := html.NewTokenizer(bytes.NewReader(src))
	offset, afterHeadOpen := 0, -1
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		start := offset
		offset += len(z.Raw())
		if tt != html.StartTagToken && tt != html.EndTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		name, _ := z.TagName()
		switch string(name) {
		case "head":
			if tt == html.EndTagToken {
				return start, false
			}
			if afterHeadOpen < 0 {
				afterHeadOpen = offset
			}
		case "body":
			if tt != html.EndTagToken {
				if afterHeadOpen >= 0 {
					return afterHeadOpen, false
				}
				return start, true
			}
		}
	}
	if afterHeadOpen >= 0 {
		return afterHeadOpen, false
	}
	return -1, false
}

package gridcodec

import "unicode/utf8"

func jsSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ',
		0x00a0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// jsTrim is String.prototype.trim: it strips exactly the characters JavaScript
// counts as whitespace, which is not unicode.IsSpace (that has U+0085 and lacks
// U+FEFF).
func jsTrim(s string) string {
	start := 0
	for start < len(s) {
		r, size := utf8.DecodeRuneInString(s[start:])
		if !jsSpace(r) {
			break
		}
		start += size
	}
	end := len(s)
	for end > start {
		r, size := utf8.DecodeLastRuneInString(s[:end])
		if !jsSpace(r) {
			break
		}
		end -= size
	}
	return s[start:end]
}

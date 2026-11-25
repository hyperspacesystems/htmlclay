package appcore

import (
	"strconv"
	"strings"
)

// servedNameMax is the longest served name config.ValidServedName accepts.
const servedNameMax = 40

// DefaultServedName is the folder's name made into a served name: lowercase,
// every run of characters other than a-z and 0-9 becomes one hyphen, leading and
// trailing hyphens dropped, cut to 40 characters (then trailing hyphens dropped
// again). An empty result becomes "folder". When available refuses it, "-2", "-3"
// and so on are appended until one is accepted, up to 99; past that it answers "".
func DefaultServedName(folderBase string, available func(name string) error) string {
	base := servedNameFrom(folderBase, servedNameMax)
	if base == "" {
		base = "folder"
	}
	if available(base) == nil {
		return base
	}
	for n := 2; n <= 99; n++ {
		suffix := "-" + strconv.Itoa(n)
		stem := servedNameFrom(folderBase, servedNameMax-len(suffix))
		if stem == "" {
			stem = "folder"
		}
		if available(stem+suffix) == nil {
			return stem + suffix
		}
	}
	return ""
}

// servedNameFrom keeps the lowercase a-z and 0-9 in s, turns every other run into
// one hyphen, drops leading and trailing hyphens, and cuts the result to limit
// characters (dropping trailing hyphens again).
func servedNameFrom(s string, limit int) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if hyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			hyphen = false
			b.WriteRune(r)
			continue
		}
		hyphen = true
	}
	name := strings.TrimRight(b.String(), "-")
	if len(name) > limit {
		name = strings.TrimRight(name[:limit], "-")
	}
	return name
}

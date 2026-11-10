//go:build managedtrial

package managedtrial

import (
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// pageMergePattern is the merge value Local's hasPageStore accepts.
var pageMergePattern = regexp.MustCompile(`^page(?:\.[0-9]+)?$`)

// hasPageStore is Local's hasPageStore (utils/page-api.js): the composed
// document has a <script> whose clay attribute holds the token `live` and whose
// merge attribute is `page` or `page.<digits>`. A repeated attribute counts by
// its first occurrence, as the parser Local uses keeps it.
func hasPageStore(document string) bool {
	tokenizer := html.NewTokenizer(strings.NewReader(document))
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return false
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			if token.Data != "script" {
				continue
			}
			var merge, clay string
			var hasMerge, hasClay bool
			for _, attr := range token.Attr {
				switch {
				case attr.Key == "merge" && !hasMerge:
					merge, hasMerge = attr.Val, true
				case attr.Key == "clay" && !hasClay:
					clay, hasClay = attr.Val, true
				}
			}
			if hasMerge && pageMergePattern.MatchString(merge) && slices.Contains(strings.Fields(clay), "live") {
				return true
			}
		}
	}
}

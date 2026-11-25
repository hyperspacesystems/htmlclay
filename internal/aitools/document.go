package aitools

import (
	"bytes"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/panphora/htmlclay/internal/htmlutil"
	"golang.org/x/net/html"
)

// DocumentCarriesAI reports whether an HTML document on disk loads the ClayJS
// "ai" or "programs" plugin or carries a <script type="text/x-clay-instructions">
// or "text/x-clay-program" tag. It reads as far as carried programs are read
// from, 2.5 MiB. Live sync relays refuse updates to such documents so a
// same-origin sibling cannot push a script into an open AI-enabled tab.
func DocumentCarriesAI(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, htmlutil.ProgramScanLimit))
	if err != nil || len(data) == 0 {
		return false
	}
	return HTMLCarriesAI(data)
}

// HTMLCarriesAI reports whether HTML bytes load ClayJS with plugins=...ai... or
// plugins=...programs..., or declare a text/x-clay-instructions or
// text/x-clay-program script tag. It reads the document as a browser parses it,
// so a script an svg or math namespace holds counts: a browser runs it.
func HTMLCarriesAI(data []byte) bool {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return false
	}
	var walk func(n *html.Node) bool
	walk = func(n *html.Node) bool {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			if tag == "noscript" && n.Namespace == "" {
				return false
			}
			if tag == "script" {
				for _, a := range n.Attr {
					switch a.Key {
					case "type":
						typ := strings.ToLower(strings.TrimSpace(a.Val))
						if typ == "text/x-clay-instructions" || typ == "text/x-clay-program" {
							return true
						}
					case "src", "href":
						if scriptURLHasAIPlugin(strings.TrimSpace(a.Val)) {
							return true
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if walk(c) {
				return true
			}
		}
		return false
	}
	return walk(doc)
}

func scriptURLHasAIPlugin(src string) bool {
	u, err := url.Parse(src)
	if err != nil {
		return false
	}
	for _, raw := range u.Query()["plugins"] {
		for _, token := range strings.Split(raw, ",") {
			if t := strings.TrimSpace(token); t == "ai" || t == "programs" {
				return true
			}
		}
	}
	return false
}

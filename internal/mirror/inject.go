package mirror

import (
	_ "embed"

	"github.com/panphora/htmlclay/internal/htmlutil"
)

//go:embed boot.js
var bootSource []byte

// Inject returns a new slice holding html with the boot script tag right after its
// <head> start tag. html itself is never written to: a caller may hash the stored bytes
// after this call. A document with no <html> start tag is returned unchanged.
func Inject(html []byte) []byte {
	return htmlutil.InsertAtHeadStart(html, htmlutil.OfflineBootTag)
}

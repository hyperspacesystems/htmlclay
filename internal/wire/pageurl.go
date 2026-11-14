package wire

import "net/http"

// PageURL reads the address of the document a page's wire request is for.
// Both hosts resolve it against their own folders.
func PageURL(r *http.Request) string {
	// Document-URL first, Page-URL after it, and the query last. §3 names the
	// first spelling and §11's channel addresses a document the same way the save
	// route does, so a client written against the spec sends only that one and was
	// getting no target at all here. The older spelling stays because a document
	// that opened a wire before the rename hardcoded it in its own inline script.
	pageURL := r.Header.Get("Document-URL")
	if pageURL == "" {
		pageURL = r.Header.Get("Page-URL")
	}
	if pageURL == "" {
		pageURL = r.URL.Query().Get("document-url")
	}
	if pageURL == "" {
		pageURL = r.URL.Query().Get("page-url")
	}
	return pageURL
}

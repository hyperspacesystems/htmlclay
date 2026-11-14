package served

import (
	"net/http"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/htmlutil"
	"github.com/panphora/htmlclay/internal/wire"
)

// serveHelperSetup is the setup bar's call: the person clicked Set Up for a
// program the page declares. Only the page itself may raise the question; a
// local process can never make the app ask.
func (s *rootServer) serveHelperSetup(w http.ResponseWriter, r *http.Request) {
	if s.wire == nil {
		sendText(w, r, http.StatusNotFound, "File not found")
		return
	}
	docs := &wireDocuments{root: s, internalDir: s.internalDir}
	isBrowser, ok := docs.Caller(r)
	if !ok || !isBrowser {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	doc, ok := docs.Target(r, true, "")
	if !ok {
		sendText(w, r, http.StatusNotFound, "File not found")
		return
	}
	s.wire.HandleSetup(w, r, doc)
}

// setupBar answers the bar for a page load of filePath, or nil when there is
// none to show: no wire, a policy that cannot ask, a source read, a request
// that is not a document load, or nothing to set up.
func (s *rootServer) setupBar(r *http.Request, filePath string) []byte {
	if s.wire == nil || s.wirePolicy == nil || !s.wirePolicy.CanAskProgram() {
		return nil
	}
	if r.Header.Get("Clay-Source") == "1" {
		return nil
	}
	if dest := r.Header.Get("Sec-Fetch-Dest"); dest != "" && dest != "document" {
		return nil
	}
	names := s.wire.SetupBarNames(filePath)
	if len(names) == 0 {
		return nil
	}
	return htmlutil.WrapBanner(wire.SetupBarHTML(names, false))
}

func realOrSame(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return p
}

package served

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/unsavedreport"
)

// serveUnsavedReport is POST /_/unsaved: an open tab of the document its
// Document-URL names says whether it holds edits not saved yet, so Quit can
// ask. Only a served folder with a public host takes it, the hosts that
// announce unsaved-report-v1.
func (s *rootServer) serveUnsavedReport(w http.ResponseWriter, r *http.Request) error {
	rep, err := unsavedreport.Read(w, r)
	if err != nil {
		sendText(w, r, http.StatusBadRequest, err.Error())
		return nil
	}
	href := documentURLHeader(r)
	if href == "" {
		sendText(w, r, http.StatusBadRequest, "Document-URL required")
		return nil
	}
	filePath, err := resolveWriteTarget(s.registry, resolveResourceFromHref(href))
	if err != nil {
		sendText(w, r, http.StatusNotFound, "File not found")
		return nil
	}
	// "Saved" is taken for a document that is gone: the tab of a file deleted or
	// renamed while it was open says so as it closes.
	if *rep.Unsaved {
		if info, statErr := os.Stat(filePath); statErr != nil || !info.Mode().IsRegular() {
			sendText(w, r, http.StatusNotFound, "File not found")
			return nil
		}
	}
	if s.onUnsavedReport != nil {
		// The path as the system writes it, as the app keys every document.
		s.onUnsavedReport(filepath.FromSlash(filePath), rep.Tab, rep.Seq, *rep.Unsaved, rep.Frozen)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

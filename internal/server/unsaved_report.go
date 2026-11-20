package server

import (
	"net/http"

	"github.com/panphora/htmlclay/internal/unsavedreport"
)

// handleUnsavedReport is POST /_/unsaved/{token}: an open tab of the token's
// document says whether it holds edits not saved yet, so Quit can ask.
func (s *Server) handleUnsavedReport(w http.ResponseWriter, r *http.Request) {
	f, ok := s.lookupSession(w, r)
	if !ok {
		return
	}
	rep, err := unsavedreport.Read(w, r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.hooks.UnsavedReport != nil {
		s.hooks.UnsavedReport(f.AbsPath, rep.Tab, rep.Seq, *rep.Unsaved, rep.Frozen)
	}
	w.WriteHeader(http.StatusNoContent)
}

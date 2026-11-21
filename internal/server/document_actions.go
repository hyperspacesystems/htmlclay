package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/panphora/htmlclay/internal/htmlutil"
	"github.com/panphora/htmlclay/internal/session"
)

// The page menu's file actions. Each is a POST carrying the document's session
// token, gated by sameOrigin, and each answers with JSON rather than a page: the
// page names what it wants and the HOST does the work, so a failure is a sentence
// the page can show instead of a navigation that leaves it.

// documentActions names the actions this host offers for one document, or nil
// when the app wired no hook that could serve any of them.
func (s *Server) documentActions(token string) *documentActions {
	if s.hooks.Reveal == nil && s.hooks.OpenFolder == nil && s.hooks.ExportDocument == nil && s.hooks.NewDocument == nil {
		return nil
	}
	var actions documentActions
	if s.hooks.Reveal != nil {
		actions.Reveal = &postAction{Post: "/_/reveal/" + token}
	}
	if s.hooks.OpenFolder != nil {
		actions.Versions = &postAction{Post: "/_/versions-folder/" + token}
	}
	if s.hooks.ExportDocument != nil {
		actions.Export = &postAction{Post: "/_/export/" + token}
	}
	if s.hooks.NewDocument != nil {
		actions.NewDocument = &postAction{Post: "/_/new-document/" + token}
	}
	return &actions
}

// actionDocument resolves the token and refuses a trusted write that has been
// revoked, which is the same admission every other route on a document applies.
func (s *Server) actionDocument(w http.ResponseWriter, r *http.Request) (*session.File, bool) {
	if s.isHiddenRecalcRequest(r) {
		write403(w)
		return nil, false
	}
	f, ok := s.lookupSession(w, r)
	if !ok {
		return nil, false
	}
	if s.trustedWriteRevoked(f) {
		s.writeError(w, http.StatusUnauthorized, "invalid token")
		return nil, false
	}
	return f, true
}

// writeActionReply answers a file action. A refused action names a plain sentence
// rather than an error code: the page shows it as written, to a person.
func writeActionReply(w http.ResponseWriter, status int, message string) {
	noStoreJSON(w)
	w.WriteHeader(status)
	if message == "" {
		fmt.Fprint(w, `{"ok":true}`)
		return
	}
	fmt.Fprintf(w, `{"ok":false,"message":%q}`, message)
}

func (s *Server) handleReveal(w http.ResponseWriter, r *http.Request) {
	f, ok := s.actionDocument(w, r)
	if !ok {
		return
	}
	hook := s.hooks.Reveal
	if hook == nil {
		writeActionReply(w, http.StatusNotFound, "Couldn't show the file.")
		return
	}
	if err := hook(f.AbsPath); err != nil {
		s.Logger.Printf("Error revealing %s: %v", f.AbsPath, err)
		writeActionReply(w, http.StatusInternalServerError, "Couldn't show the file.")
		return
	}
	writeActionReply(w, http.StatusOK, "")
}

func (s *Server) handleVersionsFolder(w http.ResponseWriter, r *http.Request) {
	f, ok := s.actionDocument(w, r)
	if !ok {
		return
	}
	hook := s.hooks.OpenFolder
	if hook == nil {
		writeActionReply(w, http.StatusNotFound, "Couldn't open the versions folder.")
		return
	}
	f.Lock()
	dir, err := s.versions.Folder(historyKey(f), f.AbsPath)
	f.Unlock()
	if err == nil {
		err = hook(dir)
	}
	if err != nil {
		s.Logger.Printf("Error opening the versions folder for %s: %v", f.AbsPath, err)
		writeActionReply(w, http.StatusInternalServerError, "Couldn't open the versions folder.")
		return
	}
	writeActionReply(w, http.StatusOK, "")
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	f, ok := s.actionDocument(w, r)
	if !ok {
		return
	}
	hook := s.hooks.ExportDocument
	if hook == nil {
		writeActionReply(w, http.StatusNotFound, "Couldn't export the document.")
		return
	}
	if err := hook(f.AbsPath); err != nil {
		s.Logger.Printf("Error exporting %s: %v", f.AbsPath, err)
		writeActionReply(w, http.StatusInternalServerError, "Couldn't export the document.")
		return
	}
	writeActionReply(w, http.StatusOK, "")
}

// ActionRefusal is a hook's refusal that the page shows as written.
type ActionRefusal struct{ Message string }

func (r ActionRefusal) Error() string { return r.Message }

// newDocumentGap is the shortest time between two new documents from one site,
// as on Hyperclay, so a page's script cannot fill a folder with files.
const newDocumentGap = 5 * time.Second

// newDocumentFailed is the page's own sentence for a new document it could not
// make, so both answers read the same.
const newDocumentFailed = "Couldn't make a new document. Try again."

// handleNewDocument is the page menu's "New blank document": the page builds the
// blank copy, and the host writes it beside this document and answers where it
// serves. The body is the document as text, as a save is.
func (s *Server) handleNewDocument(w http.ResponseWriter, r *http.Request) {
	f, ok := s.actionDocument(w, r)
	if !ok {
		return
	}
	hook := s.hooks.NewDocument
	if hook == nil {
		writeActionReply(w, http.StatusNotFound, newDocumentFailed)
		return
	}
	if isJSONContentType(r.Header.Get("Content-Type")) {
		writeActionReply(w, http.StatusUnsupportedMediaType, newDocumentFailed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSaveSize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeActionReply(w, http.StatusRequestEntityTooLarge, "This document is too large to copy.")
			return
		}
		writeActionReply(w, http.StatusBadRequest, newDocumentFailed)
		return
	}
	if len(bytes.TrimSpace(body)) == 0 {
		writeActionReply(w, http.StatusBadRequest, newDocumentFailed)
		return
	}
	// The copy is written as a save writes a document: no session token, no
	// banner, no host runtime addresses, and not the source's document id.
	body = htmlutil.StripHTMLClayID(canonicalForWrite(r.Host, htmlutil.StripBanner(htmlutil.StripToken(body))))
	if !htmlutil.HasHTMLTag(body) {
		writeActionReply(w, http.StatusBadRequest, newDocumentFailed)
		return
	}
	if !s.takeNewDocumentTurn(time.Now()) {
		writeActionReply(w, http.StatusTooManyRequests, "Wait a few seconds, then try again.")
		return
	}
	url, err := hook(f.AbsPath, s.sessions.Via(f.AbsPath), body)
	var refusal ActionRefusal
	if errors.As(err, &refusal) {
		writeActionReply(w, http.StatusForbidden, refusal.Message)
		return
	}
	if err != nil {
		s.Logger.Printf("Error making a new document from %s: %v", f.AbsPath, err)
		writeActionReply(w, http.StatusInternalServerError, newDocumentFailed)
		return
	}
	noStoreJSON(w)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(struct {
		OK   bool   `json:"ok"`
		Href string `json:"href"`
	}{true, url})
}

// takeNewDocumentTurn reports whether this site may make a new document now,
// and if so starts the gap before the next one.
func (s *Server) takeNewDocumentTurn(now time.Time) bool {
	s.newDocumentMu.Lock()
	defer s.newDocumentMu.Unlock()
	if !s.newDocumentAt.IsZero() && now.Sub(s.newDocumentAt) < newDocumentGap {
		return false
	}
	s.newDocumentAt = now
	return true
}

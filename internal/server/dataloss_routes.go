package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/panphora/htmlclay/internal/dataguard"
	"github.com/panphora/htmlclay/internal/specwire"
	"github.com/panphora/htmlclay/internal/versions"
)

const maxDataLossBody = 1 << 20

type dataLossEventBody struct {
	Event *dataguard.ClientEvent `json:"event"`
}

type dataLossResolveRequest struct {
	ID     string `json:"id"`
	Choice string `json:"choice"`
}

func writeDataLossJSON(w http.ResponseWriter, status int, v interface{}) {
	noStoreJSON(w)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeDataLossError(w http.ResponseWriter, status int, msg string) {
	writeDataLossJSON(w, status, map[string]string{"error": msg})
}

// handleDataLossGet returns the pending data-loss event for this document, or
// null. It catches up an outside edit made while no tab was open before it
// answers, so the first read after such a write already carries the notice.
func (s *Server) handleDataLossGet(w http.ResponseWriter, r *http.Request) {
	f, ok := s.lookupSession(w, r)
	if !ok {
		return
	}

	f.Lock()
	current, err := s.readRegisteredFile(f.AbsPath)
	key := f.HistoryKey()
	f.Unlock()
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if key != "" && s.guardSeenHash(key, f.AbsPath) != versions.Hash(current) {
		s.catchUpGuard(f, key)
	}

	event, err := s.guard.CurrentEvent(key, f.AbsPath, func() ([]byte, error) {
		return s.readRegisteredFile(f.AbsPath)
	})
	if err != nil {
		s.logger.Printf("Data guard read of %s failed: %v", f.RelPath, err)
		writeDataLossError(w, http.StatusInternalServerError, "The data guard could not read this document.")
		return
	}
	writeDataLossJSON(w, http.StatusOK, dataLossEventBody{Event: event})
}

// handleDataLossResolve answers a pending event with a person's choice, commits
// the bytes that choice implies through the same path a write takes, and tells
// the document's live lane the event is resolved.
func (s *Server) handleDataLossResolve(w http.ResponseWriter, r *http.Request) {
	f, ok := s.lookupSession(w, r)
	if !ok {
		return
	}

	var req dataLossResolveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDataLossBody)).Decode(&req); err != nil {
		writeDataLossError(w, http.StatusBadRequest, "A resolve takes a JSON body with an id and a choice.")
		return
	}
	switch req.Choice {
	case "restore", "put-back", "revert", "dismiss":
	default:
		writeDataLossError(w, http.StatusBadRequest, "Unknown choice. Use restore, put-back, revert or dismiss.")
		return
	}

	f.Lock()
	if s.trustedWriteRevoked(f) {
		f.Unlock()
		writeDataLossError(w, http.StatusUnauthorized, "This folder is no longer trusted.")
		return
	}
	current, err := s.readRegisteredFile(f.AbsPath)
	if err != nil {
		f.Unlock()
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	key := f.HistoryKey()
	var written []byte
	res, err := s.guard.Resolve(key, f.AbsPath, req.ID, req.Choice, current, func(html []byte) error {
		if _, bErr := s.versions.Backup(key, f.AbsPath, current); bErr != nil {
			return fmt.Errorf("back up the current document: %w", bErr)
		}
		out, _, err := s.commitDataEdit(f, current, html, r.Host, false, false)
		if err != nil {
			return err
		}
		written = out
		return nil
	})
	if err == nil {
		if pErr := s.versions.SetProvisional(key, f.AbsPath, false); pErr != nil {
			s.logger.Printf("Could not clear provisional flag for %s: %v", f.RelPath, pErr)
		}
	}
	if err == nil && written != nil {
		f.NoteWriteFromOutsideTabs()
		s.coord.acceptServerReplacement(f)
		s.coord.publishExternalChange(f, f.Name+" was restored", forBrowser(written, key, s.documentURLFor(f)), specwire.Etag(written))
	}
	f.Unlock()

	if err != nil {
		switch {
		case errors.Is(err, dataguard.ErrUnknownChoice):
			writeDataLossError(w, http.StatusBadRequest, "Unknown choice. Use restore, put-back, revert or dismiss.")
		case errors.Is(err, dataguard.ErrNoEvent):
			writeDataLossError(w, http.StatusNotFound, "There is no pending data-loss event for this document.")
		case errors.Is(err, dataguard.ErrEventMismatch):
			writeDataLossError(w, http.StatusConflict, "That event id does not match the pending event, which may already be resolved.")
		case errors.Is(err, dataguard.ErrNotRestorable), errors.Is(err, dataguard.ErrNoRecoverCopy):
			writeDataLossError(w, http.StatusUnprocessableEntity, "The data cannot be restored onto the current page. Revert instead.")
		default:
			s.logger.Printf("Data guard resolve of %s failed: %v", f.RelPath, err)
			writeDataLossError(w, http.StatusInternalServerError, "The data guard could not resolve this event.")
		}
		return
	}

	s.coord.evict(f, s.hub.notifyGuard(f.AbsPath, "resolved", "Data guard resolved", map[string]string{"id": res.ID}))
	writeDataLossJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "choice": res.Choice, "status": res.Status})
}

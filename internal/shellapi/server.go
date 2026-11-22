// Package shellapi is the private listener the Mac app talks to: loopback only, one
// secret handed over stdin, and no request a browser could make. The app holds no
// rule of its own; it reads the snapshot, sends actions, and follows the stream.
package shellapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/synchistory"
)

// Backend is what the listener asks of the app. The app's core (cmd/htmlclay
// core.go) implements it; tests use a fake.
type Backend interface {
	Snapshot() appcore.Snapshot
	// Do runs an action from the Mac app, answering a repeated id with its first answer.
	Do(ctx context.Context, a appcore.Action) (appcore.Snapshot, *appcore.Refusal)
	// Check decides an action without running it: a refusal, or a preview.
	Check(a appcore.Action) (Preview, *appcore.Refusal)
	// Answer is the remembered outcome of an action id in this epoch.
	Answer(id string) (appcore.Snapshot, *appcore.Refusal, bool)
	Subscribe() (<-chan appcore.Snapshot, func())
}

// Notice is contract C6's frame on the shell stream: a new conflict, a key the
// server stopped accepting, or a team the person was added to, with Type
// "notice". Type FrameRestartToUpdate, with no other field, asks the Mac app to
// restart into the update its updater holds. A client ignores a type it does
// not know.
type Notice struct {
	Type     string  `json:"type"`
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	FolderID *string `json:"folderID"`
	Name     string  `json:"name"`
	Detail   string  `json:"detail"`
}

// FrameRestartToUpdate is the shell stream frame for Update on a browser page.
const FrameRestartToUpdate = "restartToUpdate"

// FrameOpenSettings is the shell stream frame asking the Mac app to bring its
// Settings window to the front, optionally selecting the pane named in Detail.
const FrameOpenSettings = "openSettings"

// NoticeSource is the backend's notices, for a stream to pass on (08-sync.md
// 8.40). A backend counts a notice delivered when a stream is subscribed.
type NoticeSource interface {
	SubscribeNotices() (<-chan Notice, func())
}

// HistoryReader is the backend's read of a synced folder's history (08-sync.md
// 8.39): one page, and false for a folder no sync session keeps.
type HistoryReader interface {
	FolderHistory(folderID, before string) (synchistory.Page, bool)
}

// Preview is what an action would do, for a sheet to show before the person
// confirms (the address a folder would be served at).
type Preview struct {
	Address string `json:"address,omitempty"`
	Name    string `json:"name,omitempty"`
	// Path is the folder as the host will store it (folder.trust).
	Path string `json:"path,omitempty"`
	// Warnings are things the person should know before going ahead; none refuses.
	Warnings []Warning `json:"warnings,omitempty"`
}

// Warning is one thing a sheet shows before the person confirms.
type Warning struct {
	Code  string   `json:"code"`
	Text  string   `json:"text"`
	Names []string `json:"names,omitempty"`
}

const (
	maxBody      = 64 << 10
	maxActionID  = 100
	pingInterval = 15 * time.Second
)

// Server is the listener's handler. Port is the port it is bound to, so the Host
// check can be exact.
type Server struct {
	backend Backend
	digest  [32]byte
	host    string
	done    <-chan struct{}
	ping    time.Duration
}

// New makes the handler for a listener bound to 127.0.0.1:port. done closes when
// the app is shutting down, which ends every event stream with a bye line.
func New(backend Backend, secret string, port int, done <-chan struct{}) *Server {
	return &Server{
		backend: backend,
		digest:  sha256.Sum256([]byte(secret)),
		host:    "127.0.0.1:" + strconv.Itoa(port),
		done:    done,
		ping:    pingInterval,
	}
}

// browserHeaders are sent by every browser on every request a page can make; the
// Mac app's client sends none of them. One present means a page is calling.
var browserHeaders = []string{"Origin", "Sec-Fetch-Site", "Sec-Fetch-Mode", "Sec-Fetch-Dest", "Cookie"}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Host != s.host {
		writeStatus(w, http.StatusForbidden)
		return
	}
	for _, h := range browserHeaders {
		if _, ok := r.Header[http.CanonicalHeaderKey(h)]; ok {
			writeStatus(w, http.StatusForbidden)
			return
		}
	}
	if !s.authorized(r) {
		writeStatus(w, http.StatusUnauthorized)
		return
	}

	path := r.URL.Path
	switch {
	case path == "/shell/v1/snapshot" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, s.backend.Snapshot())
	case path == "/shell/v1/actions" && r.Method == http.MethodPost:
		s.handleAction(w, r)
	case path == "/shell/v1/check" && r.Method == http.MethodPost:
		s.handleCheck(w, r)
	case strings.HasPrefix(path, "/shell/v1/actions/") && r.Method == http.MethodGet:
		s.handleLookup(w, strings.TrimPrefix(path, "/shell/v1/actions/"))
	case path == "/shell/v1/events" && r.Method == http.MethodGet:
		s.handleEvents(w, r)
	case strings.HasPrefix(path, "/shell/v1/folders/") && strings.HasSuffix(path, "/history") && r.Method == http.MethodGet:
		s.handleHistory(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/shell/v1/folders/"), "/history"))
	default:
		writeStatus(w, http.StatusNotFound)
	}
}

// authorized compares the bearer secret in constant time, over digests so the
// comparison does not depend on the length of what was sent.
func (s *Server) authorized(r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return false
	}
	got := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(got[:], s.digest[:]) == 1
}

func (s *Server) readAction(w http.ResponseWriter, r *http.Request) (appcore.Action, bool) {
	var a appcore.Action
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil || a.Type == "" || a.ID == "" || len(a.ID) > maxActionID {
		writeStatus(w, http.StatusBadRequest)
		return appcore.Action{}, false
	}
	return a, true
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	a, ok := s.readAction(w, r)
	if !ok {
		return
	}
	snapshot, refusal := s.backend.Do(r.Context(), a)
	writeOutcome(w, snapshot, refusal)
}

func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	a, ok := s.readAction(w, r)
	if !ok {
		return
	}
	preview, refusal := s.backend.Check(a)
	if refusal != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": errorBody(refusal)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "preview": preview})
}

// handleLookup answers a dropped reply: the outcome of an action id in this epoch.
// A 404 means the action never arrived (or the epoch changed), so the app reloads
// the snapshot before deciding to send it again.
func (s *Server) handleLookup(w http.ResponseWriter, id string) {
	if id == "" || len(id) > maxActionID {
		writeStatus(w, http.StatusBadRequest)
		return
	}
	snapshot, refusal, ok := s.backend.Answer(id)
	if !ok {
		writeStatus(w, http.StatusNotFound)
		return
	}
	writeOutcome(w, snapshot, refusal)
}

// handleEvents streams newline-delimited JSON: the current snapshot first when its
// rev is past since, then one line per change, a ping every 15 seconds, and bye
// when the app shuts down.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeStatus(w, http.StatusInternalServerError)
		return
	}
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	updates, unsubscribe := s.backend.Subscribe()
	defer unsubscribe()
	var notices <-chan Notice
	if source, ok := s.backend.(NoticeSource); ok {
		ch, stop := source.SubscribeNotices()
		defer stop()
		notices = ch
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	send := func(v any) bool {
		if err := enc.Encode(v); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if current := s.backend.Snapshot(); current.Rev > since || since == 0 {
		if !send(snapshotEvent(current)) {
			return
		}
	}
	ticker := time.NewTicker(s.ping)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.done:
			send(map[string]string{"type": "bye", "reason": "quit"})
			return
		case snapshot, open := <-updates:
			if !open {
				send(map[string]string{"type": "bye", "reason": "quit"})
				return
			}
			if !send(snapshotEvent(snapshot)) {
				return
			}
		case notice, open := <-notices:
			if !open {
				notices = nil
				continue
			}
			if !send(notice) {
				return
			}
		case <-ticker.C:
			if !send(map[string]string{"type": "ping"}) {
				return
			}
		}
	}
}

func snapshotEvent(s appcore.Snapshot) map[string]any {
	return map[string]any{"type": "snapshot", "rev": s.Rev, "snapshot": s}
}

func writeOutcome(w http.ResponseWriter, snapshot appcore.Snapshot, refusal *appcore.Refusal) {
	if refusal != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "rev": snapshot.Rev, "error": errorBody(refusal)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "rev": snapshot.Rev, "snapshot": snapshot})
}

// errorBody is a refusal as the app reads it: a code, the field and subject it is
// about, whether trying again may work, and the sentence. It never carries an
// error's own text, which could hold a path or a key.
func errorBody(r *appcore.Refusal) map[string]any {
	body := map[string]any{
		"code":      r.Code,
		"retryable": r.Code == appcore.RefuseFailed || r.Code == appcore.RefuseRequestChanged || r.Code == appcore.RefuseStillRunning,
		"text":      appcore.RefusalMessage(r),
	}
	if r.Field != "" {
		body["field"] = r.Field
	}
	if r.SubjectID != "" {
		body["subjectID"] = r.SubjectID
	}
	if len(r.Documents) > 0 {
		body["documents"] = r.Documents
	}
	if r.Counts != nil {
		body["counts"] = r.Counts
	}
	if len(r.Running) > 0 {
		body["running"] = r.Running
	}
	return body
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeStatus(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

// ErrBadBootstrap is a bootstrap line that is not {"v":1,"secret":"<64 hex>"}.
var ErrBadBootstrap = errors.New("shellapi: the bootstrap line must be {\"v\":1,\"secret\":\"<64 lowercase hex>\"}")

// handleHistory answers one page of a synced folder's history, as the
// dashboard's GET /api/app/folders/<id>/history does.
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request, folderID string) {
	reader, ok := s.backend.(HistoryReader)
	if !ok || folderID == "" || strings.Contains(folderID, "/") {
		writeStatus(w, http.StatusNotFound)
		return
	}
	page, ok := reader.FolderHistory(folderID, r.URL.Query().Get("before"))
	if !ok {
		writeStatus(w, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

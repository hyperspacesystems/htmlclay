package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/recalc"
)

// A data write with ?recalculate=wait parks here until an open page recalculates the document it
// wrote (calculator documents plan, M2). The write names itself in its disk frame. A page answers
// with a save built on the written version (its If-Match names the written ETag), or with a note
// saying it cannot help.

const (
	// recalcTabWait is how long a waited write waits for open tabs.
	recalcTabWait = 3 * time.Second
	// recalcBusyGrace is how long a waited write still waits for a save once every tab said busy.
	// The host counts browsers, not tabs: on HTML Clay a browser's tabs share one subscription
	// through the sync worker, so one tab's busy note can arrive while a sibling tab, or the busy
	// tab's own merge save, is still saving.
	recalcBusyGrace = 750 * time.Millisecond
	// recalcDefaultTotal is a waited write's limit when the caller names no timeout.
	recalcDefaultTotal = 8 * time.Second
)

// recalcRequest is a write's ?recalculate=wait, with its limit.
type recalcRequest struct {
	total time.Duration
}

// recalcRequestFor reads ?recalculate and ?timeout. A malformed pair is refused before any file
// is touched.
func recalcRequestFor(r *http.Request, mode dataMode) (*recalcRequest, *writeErrorBody) {
	q := r.URL.Query()
	if !q.Has("recalculate") {
		return nil, nil
	}
	if q.Get("recalculate") != "wait" {
		return nil, &writeErrorBody{Error: "Invalid recalculate", Message: "recalculate takes one value: wait."}
	}
	if mode.face != faceAPI {
		return nil, &writeErrorBody{Error: "Invalid recalculate", Message: "recalculate=wait works only on a write through the document's own rules, not ?data= rules."}
	}
	total := recalcDefaultTotal
	if q.Has("timeout") {
		ms, err := strconv.Atoi(q.Get("timeout"))
		if err != nil || ms < 500 || ms > 15000 {
			return nil, &writeErrorBody{Error: "Invalid timeout", Message: "timeout is a whole number of milliseconds from 500 to 15000."}
		}
		total = time.Duration(ms) * time.Millisecond
	}
	return &recalcRequest{total: total}, nil
}

// recalcFrame is the field a waited write adds to its disk frame.
type recalcFrame struct {
	ID   string `json:"id"`
	Etag string `json:"etag"`
}

// recalcOutcome is what a waited write learned: a page recalculated, or the code saying why not.
type recalcOutcome struct {
	recalculated bool
	code         string
}

// recalcWhy is the sentence for each code a waited write can answer.
var recalcWhy = map[string]string{
	"no-browser":   "The write succeeded, but no open tab or hidden browser could recalculate it. Open the document to refresh its computed values.",
	"save-refused": "A page recalculated this write, but the document changed again before it could save. Read the document again for current values.",
	"page-error":   "The write succeeded, but the page failed while recalculating. Open the document to see the error.",
	"timeout":      "The write succeeded, but recalculation timed out. Open the document to refresh computed values, or retry with ?timeout= up to 15000 ms.",
}

// RecalcRunner recalculates a document in a hidden browser when no open edit tab can.
type RecalcRunner interface {
	Recalculate(ctx context.Context, docURL, expectedEtag string) error
}

type recalcSecretProvider interface {
	HeaderSecret() string
}

// isHiddenRecalcRequest reports whether r came from this server's hidden-browser runner.
// Real runners supply a per-process 128-bit secret in Clay-Recalc-Secret so an open page
// cannot forge the header; fake test runners without a secret fall back to Clay-Recalc-Source.
func (s *Server) isHiddenRecalcRequest(r *http.Request) bool {
	if s.recalcRunner == nil {
		return false
	}
	if sp, ok := s.recalcRunner.(recalcSecretProvider); ok {
		secret := sp.HeaderSecret()
		if secret == "" {
			return false
		}
		got := r.Header.Get(recalc.HiddenSecretHeader)
		return subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
	}
	return r.Header.Get(recalc.HiddenSourceHeader) == "hidden"
}

type recalcWait struct {
	id, key, etag string
	wake          chan struct{}

	mu            sync.Mutex
	hidden        bool
	saved         bool
	savedByHidden bool
	refused       bool
	failed        bool
	closed        bool
	counted       bool
	expect        int
	answered      int
}

func (w *recalcWait) setHidden(on bool) {
	w.mu.Lock()
	w.hidden = on
	w.mu.Unlock()
}

func (w *recalcWait) poke() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// expectTabs records how many edit tabs took the frame. Zero means no tab will answer.
func (w *recalcWait) expectTabs(n int) {
	w.mu.Lock()
	w.expect, w.counted = n, true
	w.mu.Unlock()
	w.poke()
}

type recalcState struct {
	saved, savedByHidden, refused, failed, closed, tabsDone bool
	tabs                                                    int
}

func (w *recalcWait) state() recalcState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return recalcState{saved: w.saved, savedByHidden: w.savedByHidden, refused: w.refused, failed: w.failed, closed: w.closed,
		tabsDone: w.counted && w.answered >= w.expect, tabs: w.expect}
}

type recalcWaits struct {
	mu     sync.Mutex
	byID   map[string]*recalcWait
	closed bool
}

func newRecalcWaits() *recalcWaits {
	return &recalcWaits{byID: map[string]*recalcWait{}}
}

// park registers a wait for key at etag. The caller parks before it publishes the frame, so no
// answer can arrive before anyone listens.
func (ws *recalcWaits) park(key, etag string) (*recalcWait, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	w := &recalcWait{id: base64.RawURLEncoding.EncodeToString(b), key: key, etag: etag, wake: make(chan struct{}, 1)}
	ws.mu.Lock()
	w.closed = ws.closed
	ws.byID[w.id] = w
	ws.mu.Unlock()
	return w, nil
}

func (ws *recalcWaits) drop(w *recalcWait) {
	ws.mu.Lock()
	delete(ws.byID, w.id)
	ws.mu.Unlock()
}

func (ws *recalcWaits) matching(key, ifMatch string) []*recalcWait {
	ifMatch = strings.TrimSpace(ifMatch)
	ws.mu.Lock()
	defer ws.mu.Unlock()
	var hit []*recalcWait
	for _, w := range ws.byID {
		if w.key == key && w.etag == ifMatch {
			hit = append(hit, w)
		}
	}
	return hit
}

// waitingOn reports whether a save to key with this If-Match could answer a wait.
func (ws *recalcWaits) waitingOn(key, ifMatch string) bool {
	return len(ws.matching(key, ifMatch)) > 0
}

// isHiddenWaiting reports whether a hidden browser recalculation is active for key at ifMatch.
func (ws *recalcWaits) isHiddenWaiting(key, ifMatch string) bool {
	for _, w := range ws.matching(key, ifMatch) {
		w.mu.Lock()
		h := w.hidden
		w.mu.Unlock()
		if h {
			return true
		}
	}
	return false
}

// saveFinished wakes every wait on key at the version a save was built on. A save that landed
// recalculated the document; a refused one counts as a tab that could not.
func (ws *recalcWaits) saveFinished(key, ifMatch string, landed, fromHidden bool) {
	for _, w := range ws.matching(key, ifMatch) {
		w.mu.Lock()
		if landed {
			w.saved = true
			if fromHidden {
				w.savedByHidden = true
			}
		} else {
			w.refused = true
			w.answered++
		}
		w.mu.Unlock()
		w.poke()
	}
}

// note takes a tab's answer that it cannot recalculate ("busy") or failed ("error").
func (ws *recalcWaits) note(key, id, etag, status string) bool {
	if status != "busy" && status != "error" {
		return false
	}
	ws.mu.Lock()
	w := ws.byID[id]
	ws.mu.Unlock()
	if w == nil || w.key != key || w.etag != etag {
		return false
	}
	w.mu.Lock()
	w.answered++
	if status == "error" {
		w.failed = true
	}
	w.mu.Unlock()
	w.poke()
	return true
}

// shutdown answers every parked wait, so a quitting server never holds a write open.
func (ws *recalcWaits) shutdown() {
	ws.mu.Lock()
	ws.closed = true
	all := make([]*recalcWait, 0, len(ws.byID))
	for _, w := range ws.byID {
		all = append(all, w)
	}
	ws.mu.Unlock()
	for _, w := range all {
		w.mu.Lock()
		w.closed = true
		w.mu.Unlock()
		w.poke()
	}
}

// awaitRecalc waits for a page to recalculate the written version: open tabs get
// recalcTabWait, the whole wait gets total.
func (s *Server) awaitRecalc(ctx context.Context, w *recalcWait, docURL string, total time.Duration) recalcOutcome {
	defer s.recalcs.drop(w)
	deadline := time.Now().Add(total)
	totalTimer := time.NewTimer(total)
	defer totalTimer.Stop()
	tabTimer := time.NewTimer(recalcTabWait)
	defer tabTimer.Stop()
	tabsOut, grace := false, false
	for {
		st := w.state()
		if st.tabsDone && st.tabs > 0 && !st.refused && !st.failed && !tabsOut && !grace {
			grace = true
			tabTimer.Stop()
			graceWait := recalcBusyGrace
			if s.recalcRunner != nil {
				if share := time.Until(deadline) / 3; share < graceWait {
					graceWait = share
				}
				if graceWait < 50*time.Millisecond {
					graceWait = 50 * time.Millisecond
				}
			}
			tabTimer.Reset(graceWait)
			st.tabsDone = false
		} else if grace && !tabsOut {
			st.tabsDone = false
		}
		switch {
		case st.saved:
			return recalcOutcome{recalculated: true}
		case st.closed:
			return recalcOutcome{code: "no-browser"}
		case st.tabsDone || tabsOut:
			switch {
			case st.refused:
				return recalcOutcome{code: "save-refused"}
			case st.failed:
				return recalcOutcome{code: "page-error"}
			}
			if s.recalcRunner == nil {
				return recalcOutcome{code: "no-browser"}
			}
			return s.runHiddenRecalc(ctx, w, docURL, deadline)
		}
		select {
		case <-w.wake:
		case <-tabTimer.C:
			tabsOut = true
		case <-totalTimer.C:
			return recalcOutcome{code: "timeout"}
		case <-ctx.Done():
			return recalcOutcome{code: "timeout"}
		}
	}
}

func (s *Server) runHiddenRecalc(ctx context.Context, w *recalcWait, docURL string, deadline time.Time) recalcOutcome {
	if time.Until(deadline) <= 0 {
		return recalcOutcome{code: "timeout"}
	}
	runCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	stopWatch := make(chan struct{})
	defer close(stopWatch)
	go func() {
		for {
			select {
			case <-stopWatch:
				return
			case <-runCtx.Done():
				return
			case <-w.wake:
				st := w.state()
				// Cancel immediately on shutdown or if an open tab beat the hidden browser to a save.
				// When the hidden browser itself saved, let Runtime.evaluate finish cleanly so the
				// warm browser stays alive instead of being killed mid-reply.
				if st.closed || (st.saved && !st.savedByHidden) {
					cancel()
					return
				}
			}
		}
	}()

	w.setHidden(true)
	err := s.recalcRunner.Recalculate(runCtx, docURL, w.etag)
	w.setHidden(false)

	st := w.state()
	if st.saved {
		return recalcOutcome{recalculated: true}
	}
	if st.closed {
		return recalcOutcome{code: "no-browser"}
	}
	if errors.Is(err, recalc.ErrNoBrowser) {
		return recalcOutcome{code: "no-browser"}
	}
	if errors.Is(err, recalc.ErrSaveRefused) || st.refused {
		return recalcOutcome{code: "save-refused"}
	}
	if errors.Is(err, recalc.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || time.Now().After(deadline) {
		return recalcOutcome{code: "timeout"}
	}
	if err != nil {
		return recalcOutcome{code: "page-error"}
	}
	return recalcOutcome{code: "page-error"}
}

// handleRecalcNote takes an open page's answer to a waited write it cannot recalculate.
func (s *Server) handleRecalcNote(w http.ResponseWriter, r *http.Request) {
	f, ok := s.lookupSession(w, r)
	if !ok {
		return
	}
	var note struct {
		ID     string `json:"id"`
		Etag   string `json:"etag"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&note); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid note")
		return
	}
	if !s.recalcs.note(f.AbsPath, note.ID, note.Etag, note.Status) {
		s.writeError(w, http.StatusNotFound, "no write is waiting")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// saveStatus records how a save answered, for a data write waiting on it.
type saveStatus struct {
	http.ResponseWriter
	status int
}

func (s *saveStatus) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *saveStatus) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *saveStatus) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func (s *saveStatus) landed() bool { return s.status >= 200 && s.status < 300 }

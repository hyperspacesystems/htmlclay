package wire

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/pathrel"
)

// Run files carry the large text a run produced (a diff, lines of a file) to
// the page that asked for it, so that text never rides a wire frame. They live
// in memory for WireTerminalTTL after the run ends, and only the document that
// started the run may read them.
const (
	MaxRunFileBytes = 4 << 20
	MaxRunBytes     = 8 << 20
	// All runs together. Past it the oldest run is dropped whole, like the
	// terminal-frame cache.
	MaxRunFilesBytes = 32 << 20
)

var (
	ErrRunFileTooLarge = errors.New("run file is too large")
	ErrRunTooLarge     = errors.New("run has too much output")
	errRunFileName     = errors.New("invalid run or file id")
	errRunOtherDoc     = errors.New("run belongs to another document")

	runIDPattern     = regexp.MustCompile(`^[0-9a-f]{32}$`)
	runFileIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

type RunFileRef struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type RunFiles struct {
	mu    sync.Mutex
	runs  map[string]*runFileSet
	order []string
	total int
	now   func() time.Time
}

type runFileSet struct {
	doc     string
	files   map[string][]byte
	bytes   int
	expires time.Time
}

func NewRunID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func RunFileURL(run, id string) string {
	return "/_/wire/runs/" + run + "/files/" + id
}

func (r *RunFiles) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

// Put stores body as file id of run, for document doc. Replacing a file
// counts only the new size.
func (r *RunFiles) Put(doc, run, id string, body []byte) (RunFileRef, error) {
	if !runIDPattern.MatchString(run) || !runFileIDPattern.MatchString(id) {
		return RunFileRef{}, errRunFileName
	}
	if len(body) > MaxRunFileBytes {
		return RunFileRef{}, ErrRunFileTooLarge
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.clock()
	r.sweep(now)
	if r.runs == nil {
		r.runs = map[string]*runFileSet{}
	}
	set := r.runs[run]
	if set == nil {
		set = &runFileSet{doc: doc, files: map[string][]byte{}}
		r.runs[run] = set
		r.order = append(r.order, run)
	}
	if set.doc != doc {
		return RunFileRef{}, errRunOtherDoc
	}
	old := len(set.files[id])
	if set.bytes-old+len(body) > MaxRunBytes {
		return RunFileRef{}, ErrRunTooLarge
	}
	set.files[id] = append([]byte(nil), body...)
	set.bytes += len(body) - old
	r.total += len(body) - old
	r.active(run)
	for r.total > MaxRunFilesBytes {
		oldest := ""
		for _, id := range r.order {
			if id != run {
				oldest = id
				break
			}
		}
		if oldest == "" {
			break
		}
		r.drop(oldest)
	}
	sum := sha256.Sum256(body)
	return RunFileRef{ID: id, URL: RunFileURL(run, id), Bytes: len(body), SHA256: hex.EncodeToString(sum[:])}, nil
}

// Finish starts the WireTerminalTTL clock for run, which Put does not. M2
// calls it when it sends wire/done or the run is cancelled; until then a run
// that writes files stays readable however long it takes.
func (r *RunFiles) Finish(run string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if set := r.runs[run]; set != nil {
		set.expires = r.clock().Add(WireTerminalTTL)
	}
}

// Get returns the file only to the document that owns the run.
func (r *RunFiles) Get(doc, run, id string) ([]byte, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweep(r.clock())
	set := r.runs[run]
	if set == nil || set.doc != doc {
		return nil, false
	}
	body, ok := set.files[id]
	return body, ok
}

// Drop removes a run's files at once, as a cancel does.
func (r *RunFiles) Drop(run string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drop(run)
}

// Forget removes every run of doc, as Core.Forget does when the document is
// closed, moved or no longer trusted.
func (r *RunFiles) Forget(doc string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, run := range append([]string(nil), r.order...) {
		if set := r.runs[run]; set != nil && set.doc == doc {
			r.drop(run)
		}
	}
}

// Clear removes every run, as a shutting down server does.
func (r *RunFiles) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, run := range append([]string(nil), r.order...) {
		r.drop(run)
	}
}

// ForgetUnder removes every run of a document at or under dir, as Core does
// for a folder that was renamed or trashed.
func (r *RunFiles) ForgetUnder(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, run := range append([]string(nil), r.order...) {
		if set := r.runs[run]; set != nil && pathrel.SameOrBelow(set.doc, dir) {
			r.drop(run)
		}
	}
}

// active puts run at the back of the order, so the run that last wrote is not
// the one the eviction below drops first.
func (r *RunFiles) active(run string) {
	for i, id := range r.order {
		if id != run {
			continue
		}
		copy(r.order[i:], r.order[i+1:])
		r.order[len(r.order)-1] = run
		return
	}
}

func (r *RunFiles) sweep(now time.Time) {
	for _, run := range append([]string(nil), r.order...) {
		if set := r.runs[run]; set != nil && !set.expires.IsZero() && !now.Before(set.expires) {
			r.drop(run)
		}
	}
}

func (r *RunFiles) drop(run string) {
	set := r.runs[run]
	if set == nil {
		return
	}
	r.total -= set.bytes
	delete(r.runs, run)
	for i, id := range r.order {
		if id == run {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

// handleRunFile answers the page or process that may use programs on the
// document, with the same Target and token check as a wire send.
func (s *Core) handleRunFile(w http.ResponseWriter, r *http.Request) {
	isBrowser, _ := s.docs.Caller(r)
	doc, ok := s.docs.Target(r, isBrowser, r.URL.Query().Get("file"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "unknown file")
		return
	}
	if !s.docs.MayUsePrograms(doc, isBrowser, r.Header.Get(TokenHeader)) {
		s.writeError(w, http.StatusForbidden, "invalid token")
		return
	}
	body, ok := s.RunFiles.Get(doc.Path, r.PathValue("run"), r.PathValue("id"))
	if !ok {
		s.writeError(w, http.StatusNotFound, "no such run file")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Write(body)
}

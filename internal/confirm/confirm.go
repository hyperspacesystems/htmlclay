// Package confirm holds the one question a flow asks the person and the
// registry of the questions still waiting for an answer. A flow calls Ask and
// blocks; the request window the person sees is Pending, and the answer they
// give comes back through Answer. Nothing here draws anything or writes config:
// a later step connects the window and the dialogs to this package.
package confirm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"slices"
	"sync"
)

// Answer errors. Each of them means nothing was decided: the request is still
// what it was. An answer for a request that is already settled is the one case
// where there is nothing left to decide.
var (
	// ErrNoSuchRequest is an answer for an id the registry does not hold: never
	// asked, or already removed by an answer or Close.
	ErrNoSuchRequest = errors.New("confirm: no such request")
	// ErrRequestChanged is an answer for a revision the person did not see, or
	// for a request that was already answered.
	ErrRequestChanged = errors.New("confirm: the request changed or was already answered")
	// ErrAnswerNotOffered is an answer that is not one of the choices this
	// request offers.
	ErrAnswerNotOffered = errors.New("confirm: the answer is not one of the offered choices")
	// ErrBadPath is choose-file without an absolute path, or a path on a choice
	// that takes none.
	ErrBadPath = errors.New("confirm: bad path for this choice")
)

// ChoiceDeny refuses the request. It is the reply to a cancelled or stopped
// ask, and to an ask that arrives with the registry already full.
const ChoiceDeny = "deny"

// ChoiceChooseFile is the one answer that carries a path, and the one answer
// that leaves the request listed until the flow asks again or closes it.
const ChoiceChooseFile = "choose-file"

// ChoiceChooseFolder answers with a folder the person picked. It carries a
// path like choose-file, and settles the request like every other answer.
const ChoiceChooseFolder = "choose-folder"

// maxPending is how many questions the window can show at once. The next ask
// beyond it is refused rather than queued: a request nobody can see is one
// nobody can answer.
const maxPending = 16

// Ask is one question for the person. ID, Path, Identity, Scope and Detail are
// the flow's own; Answers is what the window offers, in button order.
type Ask struct {
	ID       string
	Kind     string
	Document string
	Program  string
	Folder   string
	Path     string
	Identity string
	Scope    string
	Detail   string
	Answers  []string
	// Raise brings the request window to the front for this question: a new
	// request starts raised, and asking an open one again raises it again.
	Raise bool
}

// Reply is the person's answer to one Ask.
type Reply struct {
	ID     string
	Choice string
	Path   string
}

// Confirmer is how a flow asks the person a question and waits for it.
type Confirmer interface {
	Ask(ctx context.Context, ask Ask) (Reply, error)
	Close(id string)
}

// Pending is one question the window shows: its id, the revision of the
// question the person is looking at, how many times it was raised in the
// window, and the question itself.
type Pending struct {
	ID        string
	Revision  uint64
	Attention uint64
	Ask       Ask
}

// askKey is what makes two asks the same request. Detail is left out on
// purpose: it is prose that can change under an open request without making it
// a different question.
type askKey struct {
	kind     string
	document string
	program  string
	folder   string
	path     string
	scope    string
}

func askKeyOf(ask Ask) askKey {
	return askKey{
		kind:     ask.Kind,
		document: ask.Document,
		program:  ask.Program,
		folder:   ask.Folder,
		path:     ask.Path,
		scope:    ask.Scope,
	}
}

// waiter is one caller waiting on a request. Its reply is written once, under
// the lock, just before done is closed, so a caller that re-asks the same
// request cannot overwrite the answer somebody already received.
type waiter struct {
	done  chan struct{}
	reply Reply
}

// request is one question, the callers waiting on it, and how many times
// somebody raised it in the window.
type request struct {
	ask       Ask
	revision  uint64
	attention uint64
	settled   bool
	reply     Reply
	waiters   map[*waiter]struct{}
}

// Registry is the Confirmer the app uses: it mints ids, keeps each open
// question until it is answered, and hands one answer to every caller that
// asked the same question.
type Registry struct {
	mu       sync.Mutex
	changed  func()
	requests map[string]*request
	byKey    map[askKey]*request
	order    []*request
}

// New returns an empty registry. changed is called with the lock released
// every time the registry changes, so it may read Pending; it may be nil.
func New(changed func()) *Registry {
	return &Registry{
		changed:  changed,
		requests: make(map[string]*request),
		byKey:    make(map[askKey]*request),
	}
}

// Pending returns the open questions in ask order.
func (r *Registry) Pending() []Pending {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Pending, 0, len(r.order))
	for _, req := range r.order {
		ask := req.ask
		ask.Answers = slices.Clone(ask.Answers)
		out = append(out, Pending{ID: req.ask.ID, Revision: req.revision, Attention: req.attention, Ask: ask})
	}
	return out
}

// Ask shows a question and blocks until the person answers it, the context is
// cancelled, or the registry stops. An Ask with no ID gets a new one; an Ask
// with the ID of an open request replaces what that request shows and bumps
// its revision; an Ask equal to an open one shares it, and both callers get
// the same reply. A cancelled ask, a stopped registry and a full registry
// answer deny with a nil error.
func (r *Registry) Ask(ctx context.Context, ask Ask) (Reply, error) {
	key := askKeyOf(ask)

	r.mu.Lock()
	req := r.requests[ask.ID]
	same := req != nil
	if req == nil {
		req = r.byKey[key]
	}
	existed := req != nil
	switch {
	case req == nil && len(r.requests) >= maxPending:
		id := ask.ID
		if id == "" {
			id = newID()
		}
		r.mu.Unlock()
		return Reply{ID: id, Choice: ChoiceDeny}, nil
	case req == nil:
		id := ask.ID
		if id == "" {
			id = newID()
		}
		ask.ID = id
		req = &request{ask: ask, revision: 1, waiters: map[*waiter]struct{}{}}
		if ask.Raise {
			req.attention = 1
		}
		r.requests[id] = req
		r.byKey[key] = req
		r.order = append(r.order, req)
	case same:
		r.replace(req, ask, key)
	}
	if existed && ask.Raise {
		req.attention++
	}
	w := r.wait(req)
	r.mu.Unlock()

	r.notify()

	select {
	case <-w.done:
		return w.reply, nil
	case <-ctx.Done():
	}

	r.mu.Lock()
	select {
	case <-w.done:
		reply := w.reply
		r.mu.Unlock()
		return reply, nil
	default:
	}
	delete(req.waiters, w)
	id := req.ask.ID
	last := len(req.waiters) == 0
	if last {
		req.settled = true
		req.reply = Reply{ID: id, Choice: ChoiceDeny}
		r.remove(req)
	}
	r.mu.Unlock()
	if last {
		r.notify()
	}
	return Reply{ID: id, Choice: ChoiceDeny}, nil
}

// wait registers a waiter on req, or hands it the reply of a request that is
// already settled. The caller holds the lock.
func (r *Registry) wait(req *request) *waiter {
	w := &waiter{done: make(chan struct{})}
	if req.settled {
		w.reply = req.reply
		close(w.done)
		return w
	}
	req.waiters[w] = struct{}{}
	return w
}

// Answer decides one request, as the person did in the window. A choose-file
// answer leaves the request listed; every other choice removes it before
// Answer returns.
func (r *Registry) Answer(id string, revision uint64, choice, path string) error {
	r.mu.Lock()
	req := r.requests[id]
	if req == nil {
		r.mu.Unlock()
		return ErrNoSuchRequest
	}
	if req.revision != revision || req.settled {
		r.mu.Unlock()
		return ErrRequestChanged
	}
	if !slices.Contains(req.ask.Answers, choice) {
		r.mu.Unlock()
		return ErrAnswerNotOffered
	}
	if choice == ChoiceChooseFile || choice == ChoiceChooseFolder {
		if !filepath.IsAbs(path) {
			r.mu.Unlock()
			return ErrBadPath
		}
	} else if path != "" {
		r.mu.Unlock()
		return ErrBadPath
	}
	r.settle(req, Reply{ID: req.ask.ID, Choice: choice, Path: path})
	if choice != ChoiceChooseFile {
		r.remove(req)
	}
	r.mu.Unlock()
	r.notify()
	return nil
}

// Show raises one open request in the window the person is looking at: it adds
// one to that request's attention, which the Mac app watches, and changes
// nothing else about it. An id the registry does not hold is ErrNoSuchRequest.
func (r *Registry) Show(id string) error {
	r.mu.Lock()
	req := r.requests[id]
	if req == nil {
		r.mu.Unlock()
		return ErrNoSuchRequest
	}
	req.attention++
	r.mu.Unlock()
	r.notify()
	return nil
}

// Close dismisses one request, whether or not it was answered, and denies a
// caller still waiting on it.
func (r *Registry) Close(id string) {
	r.mu.Lock()
	req := r.requests[id]
	if req == nil {
		r.mu.Unlock()
		return
	}
	r.settle(req, Reply{ID: req.ask.ID, Choice: ChoiceDeny})
	r.remove(req)
	r.mu.Unlock()
	r.notify()
}

// CloseDocument dismisses every open question about document, whether or not
// it was answered, and denies each caller still waiting on one, so a document
// the person refused leaves nothing behind in the window or in a call.
func (r *Registry) CloseDocument(document string) {
	r.mu.Lock()
	var closing []*request
	for _, req := range r.order {
		if req.ask.Document == document {
			closing = append(closing, req)
		}
	}
	for _, req := range closing {
		r.settle(req, Reply{ID: req.ask.ID, Choice: ChoiceDeny})
		r.remove(req)
	}
	r.mu.Unlock()
	if len(closing) > 0 {
		r.notify()
	}
}

// Stop denies every open question and empties the registry.
func (r *Registry) Stop() {
	r.mu.Lock()
	open := r.order
	r.requests = make(map[string]*request)
	r.byKey = make(map[askKey]*request)
	r.order = nil
	for _, req := range open {
		r.settle(req, Reply{ID: req.ask.ID, Choice: ChoiceDeny})
	}
	r.mu.Unlock()
	r.notify()
}

// settle answers req and releases everyone waiting on it. An already settled
// request is left alone, because an idempotent Close must not overwrite the
// answer a caller already received. The caller holds the lock.
func (r *Registry) settle(req *request, reply Reply) {
	if req.settled {
		return
	}
	req.settled = true
	req.reply = reply
	for w := range req.waiters {
		w.reply = reply
		close(w.done)
	}
	req.waiters = nil
}

// replace makes ask the new revision of an open request. The caller holds the
// lock.
func (r *Registry) replace(req *request, ask Ask, key askKey) {
	old := askKeyOf(req.ask)
	if old != key {
		if r.byKey[old] == req {
			delete(r.byKey, old)
		}
		r.byKey[key] = req
	}
	ask.ID = req.ask.ID
	req.ask = ask
	req.revision++
	if req.settled {
		req.settled = false
		req.reply = Reply{}
		req.waiters = map[*waiter]struct{}{}
	}
}

// remove takes a request out of the registry. The caller holds the lock.
func (r *Registry) remove(req *request) {
	if r.requests[req.ask.ID] != req {
		return
	}
	delete(r.requests, req.ask.ID)
	key := askKeyOf(req.ask)
	if r.byKey[key] == req {
		delete(r.byKey, key)
	}
	for i, other := range r.order {
		if other == req {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

// notify runs the change callback. It is only ever called with the lock
// released, so the callback may read Pending.
func (r *Registry) notify() {
	if r.changed != nil {
		r.changed()
	}
}

// newID mints a request id: r and twelve hex digits.
func newID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return "r" + hex.EncodeToString(b[:])
}

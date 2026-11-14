package wire

import (
	"encoding/json"
	"errors"
	"github.com/panphora/htmlclay/internal/sse"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/aiedit"
)

// builtinHelpers is the helpers this server answers itself: the name a page asks
// for and the run it starts. Both share one per-document slot and one token bar.
var builtinHelpers = map[string]func(*Core, Document, Envelope){
	aiedit.HelperName:         (*Core).startAIEdit,
	aiedit.PageToolHelperName: (*Core).startPageTool,
}

// The wire is a per-file control channel between a page and a local process. A
// page sends a request ("rewrite the section I circled"), a process answers with
// progress and a terminal frame, and the process edits the FILE. HTML never rides
// the wire: the edit reaches the page as an ordinary external change, through the
// watcher and the live-sync lane that already exist. That split is the whole
// design, and it is why this file has no morphing, no content lane, and no
// interest in what a payload contains.
//
// Everything here is bounded. The wire hub's lock is a LEAF: nothing is called
// while it is held except channel sends with a default case, so it joins the
// documented order (document, coordinator, watcher, hub) without adding a
// cycle. Network writes always happen on the connection's own goroutine, never
// under the hub lock, exactly as the live-sync hub does it.
const (
	// A control frame is not a document. Anything approaching a document belongs
	// in the file, which is the wire's premise; a cap this low makes "HTML never
	// rides the wire" fail loudly in development instead of drifting in quietly.
	maxWireBody = 1 << 20
	// Free-form progress text. Bounded separately so a chatty handler cannot
	// flood observers' queues into eviction.
	maxWireText = 4 << 10
	// Request ids are opaque to the router. It validates only that one exists and
	// is not unbounded; it never parses, namespaces or rewrites one.
	maxWireIDLen = 128

	// One handler, a page or two, and a couple of tails. Past this something is
	// wrong and growing memory is not the answer. The live-sync hub has no
	// equivalent cap to inherit: its bounds are per-stream queue size, the
	// subscriptions one shared stream may carry, and the replay caps.
	MaxWireSubs = 8
	// Retained terminal frames per file. See wireChannel.terminal.
	MaxWireTerminals = 32
	WireTerminalTTL  = 5 * time.Minute
	WireQueueSize    = 32

	// The per-file caps bound one channel; this bounds all of them together. A
	// page may send on any registered path on its origin, so without a global
	// ceiling a loop over a trusted tree pins maxWireTerminals frames per file for
	// the whole TTL, and the per-file limits never fire. Oldest-first eviction,
	// like the live-sync replay cache.
	WireGlobalMaxTerminalBytes = 8 << 20
	TokenHeader                = "Save-Token"
	TokenQuery                 = "token"
)

var (
	ErrWireHandlerTaken = errors.New("a handler is already attached")
	ErrWireBusy         = errors.New("too many wire subscribers")
	errWireClosed       = errors.New("shutting down")
)

// Envelope is the whole protocol. The router validates the routing fields
// and treats Payload as opaque JSON.
//
// File and From are stamped by the SERVER and any client-supplied value is
// discarded. File, because a page that could name its own path would launder a
// write through the agent's OS authority into a file the page can never touch.
// From, because it identifies a connection, and a value the sender chooses is a
// value a hostile sender can choose to be someone else's.
type Envelope struct {
	V        int             `json:"v"`
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	From     string          `json:"from,omitempty"`
	File     string          `json:"file"`
	Helper   string          `json:"helper,omitempty"`
	Document string          `json:"document,omitempty"`
	Text     string          `json:"text,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

// IsTerminal reports whether this frame ends a request. Only terminal frames are
// retained, because a lost status frame is repaired by the next one and a lost
// terminal frame is repaired by nothing.
func (e Envelope) IsTerminal() bool {
	return e.Type == "wire/done" || e.Type == "wire/error"
}

type Sub struct {
	Key     string
	Handler bool
	Mode    string
	Ch      chan []byte
	Done    chan struct{}
	once    sync.Once
	// Removed makes removal idempotent, so an eviction and the handler's own
	// defer cannot both tear down one subscriber. Guarded by wireHub.mu.
	Removed bool
}

func WireHandlerMode(mode string) (string, bool) {
	if mode == "" {
		return "raw", true
	}
	if mode == "raw" || mode == "jsonl" {
		return mode, true
	}
	return "", false
}

func (sub *Sub) stop() { sub.once.Do(func() { close(sub.Done) }) }

type helperBinding struct {
	bound      bool
	generation uint64
}

type helperBindingRegistry struct {
	mu     sync.RWMutex
	byFile map[string]helperBinding
}

// HelpersBound reports whether file currently has an in-process helper handler.
func (s *Core) HelpersBound(file string) bool {
	s.HelperBindings.mu.RLock()
	defer s.HelperBindings.mu.RUnlock()
	return s.HelperBindings.byFile[file].bound
}

// HelperGeneration identifies file's current binding lifetime. It changes once
// per real bind or unbind transition and stays stable across idempotent calls.
func (s *Core) HelperGeneration(file string) uint64 {
	s.HelperBindings.mu.RLock()
	defer s.HelperBindings.mu.RUnlock()
	return s.HelperBindings.byFile[file].generation
}

// BindHelpers activates helper-channel admission after the dispatcher has won
// the handler slot. The existing handler stays, while pre-bind observer queues
// and retained outcomes are removed in the same locked transition.
func (s *Core) BindHelpers(file string) uint64 {
	s.HelperBindings.mu.Lock()
	state := s.HelperBindings.byFile[file]
	if state.bound {
		s.HelperBindings.mu.Unlock()
		return state.generation
	}
	if s.HelperBindings.byFile == nil {
		s.HelperBindings.byFile = make(map[string]helperBinding)
	}
	state.bound = true
	state.generation++
	s.HelperBindings.byFile[file] = state
	evicted := s.Hub.transitionBinding(file, true)
	s.HelperBindings.mu.Unlock()
	stopWireSubs(evicted)
	return state.generation
}

// UnbindHelpers revokes the helper channel, disconnects every subscriber and
// removes retained outcomes before tokenless plain-wire admission resumes.
func (s *Core) UnbindHelpers(file string) uint64 {
	s.HelperBindings.mu.Lock()
	state := s.HelperBindings.byFile[file]
	if !state.bound {
		s.HelperBindings.mu.Unlock()
		return state.generation
	}
	state.bound = false
	state.generation++
	s.HelperBindings.byFile[file] = state
	evicted := s.Hub.transitionBinding(file, false)
	s.HelperBindings.mu.Unlock()
	stopWireSubs(evicted)
	return state.generation
}

// BoundFiles lists the documents whose program channel is bound now, sorted.
func (s *Core) BoundFiles() []string {
	s.HelperBindings.mu.RLock()
	defer s.HelperBindings.mu.RUnlock()
	var files []string
	for file, state := range s.HelperBindings.byFile {
		if state.bound {
			files = append(files, file)
		}
	}
	sort.Strings(files)
	return files
}

// Forget ends everything the wire holds for path: a running AI edit is
// cancelled, its programs are detached, its channel is unbound, and every
// stream on it is closed. The binding's
// generation keeps counting, so a dispatcher from before cannot match a later
// binding of the same path. A renamed or removed document is forgotten under
// its old path.
func (s *Core) Forget(path string) {
	s.CancelAIEditFor(path)
	s.DetachHelpers(path)
	s.HelperBindings.mu.Lock()
	if state, ok := s.HelperBindings.byFile[path]; ok && state.bound {
		state.bound = false
		state.generation++
		s.HelperBindings.byFile[path] = state
	}
	evicted := s.Hub.closeChannel(path)
	s.HelperBindings.mu.Unlock()
	stopWireSubs(evicted)
}

type Terminal struct {
	seq   int64
	Frame []byte
	At    time.Time
}

// Channel is one file's wire: at most one handler, capped observers, and the
// terminal frame of each recent request.
//
// Retention is keyed by request id rather than kept as an anonymous ring so that
// a reconnecting subscriber recovers the OUTCOME of its requests, which is the
// only question it ever asks, and so a request that terminates twice keeps the
// first answer rather than the last.
type Channel struct {
	subs     map[*Sub]struct{}
	handler  *Sub
	Terminal map[string]Terminal
}

// Hub lives on the Server, not on the process-wide runtime, because a site is
// the security and lifetime boundary for an instruction channel. untrustFolder
// already calls s.close() on the site and drops it (cmd/htmlclay/folders.go), so an untrusted
// folder cannot keep a live wire into itself, and graceful shutdown closes these
// streams before the HTTP drain instead of holding it to ShutdownBudget. A
// process-wide hub would need a new bookkeeping call at both sites, and the one
// that gets forgotten is the one that matters.
type Hub struct {
	Mu      sync.Mutex
	Chans   map[string]*Channel
	seq     int64
	closing chan struct{}
	closed  bool

	// TerminalBytes is the running total across every channel, so the global cap
	// costs no walk to check. Every path that adds or drops a retained terminal
	// goes through retainTerminalLocked / dropTerminalLocked to keep it honest.
	TerminalBytes int
}

func NewHub() *Hub {
	return &Hub{Chans: make(map[string]*Channel), closing: make(chan struct{})}
}

// nextSeq hands out a monotonic id used as the SSE id, so a reconnecting
// EventSource can ask for "everything after X".
//
// It floors on the wall clock for the same reason the live-sync hub does: after a
// restart the counter must resume ABOVE anything already handed out, or a client
// reconnecting with an old Last-Event-ID discards every new frame as stale. Unlike
// live-sync this needs no persisted high-water mark, because the wire retains
// nothing across a restart to be resumed into: the floor alone is sufficient, and
// borrowing live-sync's counter would mean taking its lock from inside this one.
// Caller holds wireHub.mu.
func (wh *Hub) nextSeqLocked() int64 {
	if now := time.Now().UnixMilli(); now > wh.seq {
		wh.seq = now
	} else {
		wh.seq++
	}
	return wh.seq
}

func (wh *Hub) ensureLocked(key string) *Channel {
	c, ok := wh.Chans[key]
	if !ok {
		c = &Channel{
			subs:     make(map[*Sub]struct{}),
			Terminal: make(map[string]Terminal),
		}
		wh.Chans[key] = c
	}
	return c
}

// Add installs a subscriber and returns the position it resumes from with the
// terminal frames retained past that position. The handler slot is decided under
// the SAME lock as membership, so two processes can never both believe they hold
// it. The position is chosen under that lock too, so it is never above a frame
// the subscriber is still owed: a page that reconnects from the cursor it was
// handed is replayed everything after it.
func (wh *Hub) Add(sub *Sub, lastEventID int64) (int64, [][]byte, error) {
	wh.Mu.Lock()
	defer wh.Mu.Unlock()
	if wh.closed {
		return 0, nil, errWireClosed
	}
	wh.sweepLocked()
	c := wh.ensureLocked(sub.Key)
	// The cap counts observers only, so the exclusive slot is reserved rather than
	// competed for: eight tails on a file must not be able to lock the user's own
	// agent out of a slot that is free.
	if !sub.Handler && len(c.subs) >= MaxWireSubs {
		return 0, nil, ErrWireBusy
	}
	if sub.Handler {
		if c.handler != nil {
			return 0, nil, ErrWireHandlerTaken
		}
		c.handler = sub
	}
	c.subs[sub] = struct{}{}

	// A fresh subscription replays nothing. Replaying to a subscriber with no
	// cursor would hand a reloaded page the terminal frame of a request it
	// cancelled before reloading, and the page, having no memory of the cancel,
	// would act on it. "Stop completely" has to survive a reload. Its position is
	// a sequence number taken now, which every frame published from here on
	// sorts after.
	if lastEventID <= 0 {
		return wh.nextSeqLocked(), nil, nil
	}
	var replay [][]byte
	for _, t := range c.Terminal {
		if t.seq > lastEventID {
			replay = append(replay, t.Frame)
		}
	}
	return lastEventID, replay, nil
}

func (wh *Hub) Remove(sub *Sub) {
	wh.Mu.Lock()
	if sub.Removed {
		wh.Mu.Unlock()
		return
	}
	sub.Removed = true
	if c, ok := wh.Chans[sub.Key]; ok {
		wh.removeFromChannelLocked(c, sub.Key, sub)
	}
	wh.sweepLocked()
	wh.Mu.Unlock()
	sub.stop()
}

func (wh *Hub) removeFromChannelLocked(c *Channel, key string, sub *Sub) {
	delete(c.subs, sub)
	if c.handler == sub {
		c.handler = nil
	}
	// A channel with no subscribers and no retained outcomes is not a channel.
	// Dropping it keeps the map bounded by live use rather than by history.
	if len(c.subs) == 0 && len(c.Terminal) == 0 {
		delete(wh.Chans, key)
	}
}

func (wh *Hub) transitionBinding(key string, bound bool) []*Sub {
	wh.Mu.Lock()
	defer wh.Mu.Unlock()
	c, ok := wh.Chans[key]
	if !ok {
		return nil
	}
	for id := range c.Terminal {
		wh.dropTerminalLocked(c, id)
	}
	var evicted []*Sub
	for sub := range c.subs {
		for {
			select {
			case <-sub.Ch:
				continue
			default:
			}
			break
		}
		if bound && sub.Handler {
			continue
		}
		sub.Removed = true
		delete(c.subs, sub)
		if c.handler == sub {
			c.handler = nil
		}
		evicted = append(evicted, sub)
	}
	if len(c.subs) == 0 {
		delete(wh.Chans, key)
	}
	return evicted
}

// closeChannel removes key's channel with its retained outcomes and answers its
// subscribers, marked removed, for the caller to stop outside the lock.
func (wh *Hub) closeChannel(key string) []*Sub {
	wh.Mu.Lock()
	defer wh.Mu.Unlock()
	c, ok := wh.Chans[key]
	if !ok {
		return nil
	}
	for id := range c.Terminal {
		wh.dropTerminalLocked(c, id)
	}
	subs := make([]*Sub, 0, len(c.subs))
	for sub := range c.subs {
		sub.Removed = true
		subs = append(subs, sub)
	}
	delete(wh.Chans, key)
	return subs
}

func stopWireSubs(subs []*Sub) {
	for _, sub := range subs {
		sub.stop()
	}
}

// retainTerminalLocked records one request's outcome and enforces both the
// channel's own count cap and the hub-wide byte cap.
func (wh *Hub) retainTerminalLocked(c *Channel, id string, t Terminal) {
	c.Terminal[id] = t
	wh.TerminalBytes += len(t.Frame)
	wh.expireLocked(c)
	wh.enforceTerminalBytesLocked()
}

func (wh *Hub) dropTerminalLocked(c *Channel, id string) {
	if t, ok := c.Terminal[id]; ok {
		wh.TerminalBytes -= len(t.Frame)
		delete(c.Terminal, id)
	}
}

func (wh *Hub) forgetTerminal(key, id string) {
	wh.Mu.Lock()
	defer wh.Mu.Unlock()
	if c := wh.Chans[key]; c != nil {
		wh.dropTerminalLocked(c, id)
	}
}

// enforceTerminalBytesLocked drops the oldest retained outcome anywhere in the hub
// until the total is back under the ceiling.
func (wh *Hub) enforceTerminalBytesLocked() {
	for wh.TerminalBytes > WireGlobalMaxTerminalBytes {
		var victim *Channel
		var victimKey, oldestID string
		var oldest time.Time
		for key, c := range wh.Chans {
			for id, t := range c.Terminal {
				if oldestID == "" || t.At.Before(oldest) {
					victim, victimKey, oldestID, oldest = c, key, id, t.At
				}
			}
		}
		if victim == nil {
			return
		}
		wh.dropTerminalLocked(victim, oldestID)
		if len(victim.subs) == 0 && len(victim.Terminal) == 0 {
			delete(wh.Chans, victimKey)
		}
	}
}

// sweepLocked expires every channel's retained terminals and drops the channels
// left with nothing.
//
// Per-channel expiry alone is not enough to bound the map: a channel keeps itself
// alive while it holds terminals, and nothing revisits a file whose handler has
// gone, so one channel per file that ever saw a request would live as long as the
// site. There is no janitor goroutine because there is nothing to do between
// operations: sweeping on subscribe and unsubscribe is O(channels), and the
// channel count is bounded by the files a user has actually pointed an agent at.
func (wh *Hub) sweepLocked() {
	for key, c := range wh.Chans {
		wh.expireLocked(c)
		if len(c.subs) == 0 && len(c.Terminal) == 0 {
			delete(wh.Chans, key)
		}
	}
	wh.enforceTerminalBytesLocked()
}

func (wh *Hub) expireLocked(c *Channel) {
	cutoff := time.Now().Add(-WireTerminalTTL)
	for id, t := range c.Terminal {
		if t.At.Before(cutoff) {
			wh.dropTerminalLocked(c, id)
		}
	}
	// TTL alone permits an unbounded burst inside one window, so the count cap is
	// enforced too, dropping the oldest first.
	for len(c.Terminal) > MaxWireTerminals {
		var oldestID string
		var oldest time.Time
		for id, t := range c.Terminal {
			if oldestID == "" || t.At.Before(oldest) {
				oldestID, oldest = id, t.At
			}
		}
		wh.dropTerminalLocked(c, oldestID)
	}
}

// Publish fans one envelope out and reports how many HANDLER queues took it.
//
// The handler count is what the page actually asked: "is an agent there". A count
// of successful subscriber writes would answer yes because the page's own
// observer stream took a copy of its own request, which is precisely the bug the
// Node router shipped.
func (wh *Hub) Publish(key string, env Envelope) (handlers int, observers int) {
	handlers, observers, _ = wh.publishFrom(key, nil, env)
	return handlers, observers
}

func (wh *Hub) PublishFromHandler(key string, owner *Sub, env Envelope) (handlers int, observers int, published bool) {
	return wh.publishFrom(key, owner, env)
}

func (wh *Hub) publishFrom(key string, owner *Sub, env Envelope) (handlers int, observers int, published bool) {
	wh.Mu.Lock()
	c, ok := wh.Chans[key]
	if !ok || wh.closed || owner != nil && c.handler != owner {
		wh.Mu.Unlock()
		return 0, 0, false
	}
	seq := wh.nextSeqLocked()
	f := sse.Frame(seq, env)
	if f == nil {
		wh.Mu.Unlock()
		return 0, 0, false
	}
	if env.IsTerminal() && env.ID != "" {
		// First terminal wins: a handler that reports done and then errors for one
		// request does not get to rewrite the outcome a subscriber already saw.
		if _, seen := c.Terminal[env.ID]; !seen {
			wh.retainTerminalLocked(c, env.ID, Terminal{seq: seq, Frame: f, At: time.Now()})
		}
	}
	var evicted []*Sub
	for sub := range c.subs {
		// A handler never receives its own output. Its queue is there for the
		// requests it exists to answer, and echoing its own frames into it means a
		// helper reporting progress faster than the dispatcher drains the echo
		// overflows that queue, is evicted from its own handler slot, and takes the
		// request's terminal frame down with it. Nothing but valid status is needed
		// to do it: 40 frames overflow the 32-entry queue.
		if sub == owner {
			continue
		}
		select {
		case sub.Ch <- f:
			if sub.Handler {
				handlers++
			} else {
				observers++
			}
		default:
			evicted = append(evicted, sub)
		}
	}
	for _, sub := range evicted {
		sub.Removed = true
		wh.removeFromChannelLocked(c, key, sub)
	}
	wh.Mu.Unlock()

	// Unblocking the writers happens outside the lock, so a stuck connection
	// cannot hold up the next publish.
	for _, sub := range evicted {
		sub.stop()
	}
	return handlers, observers, true
}

func (wh *Hub) handlerMode(key string) (string, bool) {
	wh.Mu.Lock()
	defer wh.Mu.Unlock()
	c, ok := wh.Chans[key]
	if !ok || c.handler == nil {
		return "", false
	}
	mode := c.handler.Mode
	if mode == "" {
		mode = "raw"
	}
	return mode, true
}

func (wh *Hub) rejectNamedForRawHandler(key string, request Envelope) (bool, int) {
	wh.Mu.Lock()
	c, ok := wh.Chans[key]
	if !ok || c.handler == nil || (c.handler.Mode != "" && c.handler.Mode != "raw") || wh.closed {
		wh.Mu.Unlock()
		return false, 0
	}
	env := Envelope{
		V:       1,
		Type:    "wire/error",
		ID:      request.ID,
		From:    "process",
		File:    key,
		Helper:  request.Helper,
		Text:    "this handler does not support named helpers",
		Payload: json.RawMessage(`{"source":"host","code":"helper_protocol_unsupported"}`),
	}
	seq := wh.nextSeqLocked()
	fr := sse.Frame(seq, env)
	if fr == nil {
		wh.Mu.Unlock()
		return true, 0
	}
	if _, seen := c.Terminal[env.ID]; !seen {
		wh.retainTerminalLocked(c, env.ID, Terminal{seq: seq, Frame: fr, At: time.Now()})
	}
	var evicted []*Sub
	observers := 0
	for sub := range c.subs {
		if sub.Handler {
			continue
		}
		select {
		case sub.Ch <- fr:
			observers++
		default:
			evicted = append(evicted, sub)
		}
	}
	for _, sub := range evicted {
		sub.Removed = true
		wh.removeFromChannelLocked(c, key, sub)
	}
	wh.Mu.Unlock()
	stopWireSubs(evicted)
	return true, observers
}

func (wh *Hub) Shutdown() {
	wh.Mu.Lock()
	if wh.closed {
		wh.Mu.Unlock()
		return
	}
	wh.closed = true
	close(wh.closing)
	subs := make([]*Sub, 0, len(wh.Chans))
	for _, c := range wh.Chans {
		for sub := range c.subs {
			sub.Removed = true
			subs = append(subs, sub)
		}
	}
	wh.Chans = make(map[string]*Channel)
	wh.TerminalBytes = 0
	wh.Mu.Unlock()
	for _, sub := range subs {
		sub.stop()
	}
}

func wireGuard(caller func(*http.Request) (isBrowser, ok bool), h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := caller(r); !ok {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// WireMux mounts both routes as one subtree behind one guard, so a route cannot
// be added to the wire without the guard in front of it. The alternative
// considered was giving every /_/ route a declared reach; that rewrites route
// registrations the wire does not touch, for a property only the wire needs.
func (s *Core) WireMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /_/wire/subscribe", s.handleWireSubscribe)
	mux.HandleFunc("POST /_/wire/send", s.handleWireSend)
	return wireGuard(s.docs.Caller, mux)
}

func (s *Core) handleWireSubscribe(w http.ResponseWriter, r *http.Request) {
	isBrowser, _ := s.docs.Caller(r)

	// Pages send; processes handle. A page allowed to hold the exclusive slot could
	// take it away from the user's agent, receive every request on this file
	// including other tabs', and answer them with fabricated terminal frames.
	//
	// On a plain wire, the classifier has one known residual: a browser that predates
	// Sec-Fetch-Site attests nothing at all on a same-origin GET (Safari before
	// 16.4, Firefox before 90), so wireCaller reads such a page as a process. What
	// it buys is squatting the slot and lying to its own origin's tabs; it is
	// already allowed to send frames, it cannot read or write any file, and it
	// cannot reach another project's origin. Helper-bound channels close this
	// ambiguity with the target's save token. No header fixes the plain-wire
	// classifier because a same-origin fetch may set any header without a preflight;
	// only a secret a page cannot read would, and that is a bigger change than the
	// residual is worth. A cross-origin caller is never affected: every cross-origin
	// request, EventSource included, carries Origin.
	//
	// Refused before the file is resolved: the answer does not depend on which
	// file, so resolving first would tell a caller that is being refused anyway
	// whether a given path exists. A page therefore never sees the 409 below, which
	// is just as well, since it could do nothing with a conflict over a slot it may
	// never hold.
	wantHandler := r.URL.Query().Get("role") == "handler"
	if wantHandler && isBrowser {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	mode := ""
	if wantHandler {
		var valid bool
		mode, valid = WireHandlerMode(r.URL.Query().Get("mode"))
		if !valid {
			s.writeError(w, http.StatusBadRequest, "invalid handler mode")
			return
		}
	}

	doc, ok := s.docs.Target(r, isBrowser, r.URL.Query().Get("file"))
	if !ok {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	sub := &Sub{
		Key:     doc.Path,
		Handler: wantHandler,
		Mode:    mode,
		Ch:      make(chan []byte, WireQueueSize),
		Done:    make(chan struct{}),
	}

	s.HelperBindings.mu.RLock()
	if s.HelperBindings.byFile[doc.Path].bound && !s.docs.MayUsePrograms(doc, isBrowser, r.URL.Query().Get(TokenQuery)) {
		s.HelperBindings.mu.RUnlock()
		s.writeError(w, http.StatusForbidden, "invalid token")
		return
	}
	cursor, replay, err := s.Hub.Add(sub, sse.ParseLastEventID(r))
	s.HelperBindings.mu.RUnlock()
	if err != nil {
		switch {
		case errors.Is(err, ErrWireHandlerTaken):
			http.Error(w, "Conflict", http.StatusConflict)
		case errors.Is(err, ErrWireBusy):
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		default:
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		}
		return
	}
	defer s.Hub.Remove(sub)

	rc := http.NewResponseController(w)
	// Clear the write deadline for this connection only; zeroing the server-wide
	// WriteTimeout would remove the bound from every other request.
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		s.Logger.Printf("wire: cannot clear write deadline: %v", err)
		http.Error(w, "Not Implemented", http.StatusNotImplemented)
		return
	}

	// The watch is the whole point of the handler role. An agent edits the FILE, and
	// the edit reaches the page through the ordinary external-change path, which only
	// runs for files the watcher is polling; without the lease an agent would be
	// useful only while a tab happened to already be open on the file it was editing.
	//
	// Raised after the handler slot is won, so a refused subscription leaves no lease
	// behind, and released on the same teardown as the subscription itself. The raise
	// and its defer sit together with nothing between them, so no failure inside the
	// seeding above can leak a reference.
	if wantHandler {
		// Refused rather than degraded. Attaching without a resolved key would start
		// an agent whose every write goes unversioned until someone happens to open
		// the file in a browser, and the one state worth keeping is the one that
		// existed before the agent touched it.
		if err := s.docs.OpenHistory(doc); err != nil {
			s.Logger.Printf("wire: cannot open history for %s: %v", doc.Name, err)
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		s.docs.Lease(doc)
		defer s.docs.Unlease(doc)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if err := rc.Flush(); err != nil {
		return
	}

	// A page's first frame is a cursor: a named event that never reaches
	// onmessage, carrying the position it resumes from. It is the floor of what
	// follows: the replay below and every live frame sort after it, so a stream
	// that drops between the cursor and a frame resumes with the frame still
	// owed. Pages only, because a process tailing the wire parses every data line
	// as a wire frame and has no use for a position it never presents.
	if isBrowser && !wantHandler {
		if !sse.Write(rc, w, sse.CursorFrame(cursor, false)) {
			return
		}
	}

	for _, fr := range replay {
		if !sse.Write(rc, w, fr) {
			return
		}
	}

	ticker := time.NewTicker(sse.KeepaliveInterval)
	defer ticker.Stop()

	// This goroutine is the single writer for this connection, so keepalives and
	// broadcasts never write concurrently to one ResponseWriter and the hub lock
	// is never held during a network write. A wire is idle between requests by
	// design, so the keepalive is what stops IdleTimeout from closing it.
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.Hub.closing:
			return
		case <-sub.Done:
			return
		case msg := <-sub.Ch:
			if !sse.Write(rc, w, msg) {
				return
			}
		case <-ticker.C:
			if !sse.Write(rc, w, []byte(": keepalive\n\n")) {
				return
			}
		}
	}
}

func (s *Core) handleWireSend(w http.ResponseWriter, r *http.Request) {
	isBrowser, _ := s.docs.Caller(r)

	// application/json is load-bearing rather than hygiene: it is not a
	// CORS-simple content type, so a cross-origin POST is forced into a preflight
	// that this mux answers with a bare 405 and no allow-origin header. The matcher
	// also accepts the `+json` suffix, which costs nothing here: the three simple
	// content types are form-urlencoded, multipart, and text/plain, so every type it
	// accepts is still preflighted.
	if !IsJSONContentType(r.Header.Get("Content-Type")) {
		s.writeError(w, http.StatusUnsupportedMediaType, "expected application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxWireBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			s.writeError(w, http.StatusRequestEntityTooLarge, "wire frame too large")
			return
		}
		s.writeError(w, http.StatusBadRequest, "could not read body")
		return
	}

	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// The router validates that a type is a wire type and that an id exists and is
	// bounded. It does not validate against a closed set of types: the parity
	// implementation already emits a batched delta type, and a router that never
	// reads payloads has no business rejecting a type it does not recognize.
	if !strings.HasPrefix(env.Type, "wire/") || len(env.Type) > 64 {
		s.writeError(w, http.StatusBadRequest, "invalid type")
		return
	}
	if env.ID == "" || len(env.ID) > maxWireIDLen {
		s.writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if len(env.Text) > maxWireText {
		env.Text = env.Text[:maxWireText]
	}

	doc, ok := s.docs.Target(r, isBrowser, env.File)
	if !ok {
		s.writeError(w, http.StatusNotFound, "unknown file")
		return
	}

	s.HelperBindings.mu.RLock()
	if s.HelperBindings.byFile[doc.Path].bound && !s.docs.MayUsePrograms(doc, isBrowser, r.Header.Get(TokenHeader)) {
		s.HelperBindings.mu.RUnlock()
		s.writeError(w, http.StatusForbidden, "invalid token")
		return
	}

	// From is a coarse origin tag, not a connection identity. Correlation is by
	// the sender's own opaque id, and a subscriber drops ids it did not issue, so
	// nothing needs to tell two pages apart. Stamping it server-side keeps it
	// honest: a value the sender chose is a value a hostile sender can choose to
	// be someone else's.
	env.V = 1
	env.File = doc.Path
	env.From = "process"
	if isBrowser {
		env.From = "page"
	}
	// The built-in helpers ai-edit and blockeditor-tool are answered here, before any
	// handler sees the request: they need no declaration and never take the
	// file's handler slot, and they share one per-document run slot. They always
	// require the document's token, bound helpers or not.
	if s.aiEditOffered() && aiedit.IsDocument(doc.Path) {
		start := builtinHelpers[env.Helper]
		builtin := start != nil && (env.Type == "wire/request" || env.Type == "wire/describe")
		if builtin || env.Type == "wire/cancel" {
			tokenOK := s.docs.MayUsePrograms(doc, isBrowser, r.Header.Get(TokenHeader))
			if builtin {
				s.HelperBindings.mu.RUnlock()
				if !tokenOK {
					s.writeError(w, http.StatusForbidden, "invalid token")
					return
				}
				start(s, doc, env)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"ok": true, "delivered": 1, "observers": 0})
				return
			}
			if tokenOK && s.cancelAIEdit(doc.Path, env.ID) {
				s.HelperBindings.mu.RUnlock()
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"ok": true, "delivered": 1, "observers": 0})
				return
			}
		}
	}

	if env.Helper != "" && (env.Type == "wire/request" || env.Type == "wire/describe") {
		if rejected, observers := s.Hub.rejectNamedForRawHandler(doc.Path, env); rejected {
			s.HelperBindings.mu.RUnlock()
			w.Header().Set("Content-Type", "application/json")
			// The refusal rides the POST reply as well as the stream. They are two
			// connections with no ordering between them, and "delivered":0 on its own
			// reads as "nobody is attached": a client that settles on it discards the
			// typed frame that arrives afterwards and reports the wrong reason for a
			// refusal that was specific and deliberate.
			json.NewEncoder(w).Encode(map[string]any{
				"ok":        true,
				"delivered": 0,
				"observers": observers,
				"refused": map[string]any{
					"source":  "host",
					"code":    "helper_protocol_unsupported",
					"message": "this handler does not support named helpers",
				},
			})
			return
		}
	}

	// A handler that reports a request finished has finished writing the file, so
	// the change is already on disk and the quiet interval has nothing left to
	// wait for. Without the poke the page's spinner ends on this frame and the
	// text arrives up to a poll plus a quiet interval later.
	//
	// Only from a process: a page has no way to know a write finished, and a
	// terminal frame is the one thing it could send to claim so. An error is
	// terminal too, and a handler that gave up has also stopped writing.
	if env.IsTerminal() && !isBrowser {
		s.docs.Poke(doc)
	}

	handlers, observers := s.Hub.Publish(doc.Path, env)
	s.HelperBindings.mu.RUnlock()
	if handlers == 0 && (env.Type == "wire/request" || env.Type == "wire/describe") {
		mode, _ := s.Hub.handlerMode(doc.Path)
		s.Logger.Printf("Wire request undelivered: file=%s helper=%q type=%q id=%q document=%q helpers_bound=%t handler_mode=%q", doc.Path, env.Helper, env.Type, env.ID, env.Document, s.HelpersBound(doc.Path), mode)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":        true,
		"delivered": handlers,
		"observers": observers,
	})
}

// WhileUnbound runs fn while path has no helper binding, holding the binding
// registry's read lock for the whole of fn, so no binding can begin in the
// middle of it. It answers false, without running fn, when path is bound.
func (s *Core) WhileUnbound(path string, fn func()) bool {
	s.HelperBindings.mu.RLock()
	defer s.HelperBindings.mu.RUnlock()
	if s.HelperBindings.byFile[path].bound {
		return false
	}
	fn()
	return true
}

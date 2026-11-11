//go:build managedtrial

package managedtrial

import (
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Local's live-sync stream routes. Spec §10 puts both halves on `/_/sync`; the
// legacy `/live-sync/stream` stays forever because hyperclayjs hardcodes it, and
// so does the inline script in every Collection dashboard ever minted, neither
// of which any library update can reach.
//
// A stream resumes before it subscribes, writes its cursor frame, then replays
// what the client missed and joins the channel. Node runs that whole sequence to
// completion, so a frame published while it runs cannot fall between the replay
// and the subscription; here it runs under the hub's own session lock.

const (
	// maxSharedSubs is MAX_SHARED_SUBS: how many `s` entries one connection may
	// carry. The cap is htmlclay's, because the client that opens this stream is
	// the same one.
	maxSharedSubs = 256

	// maxSafeInteger is Number.MAX_SAFE_INTEGER: the largest value either client
	// reads as a frame id.
	maxSafeInteger = 1<<53 - 1

	// preSpecSyncWorkerPath is the one address the SharedWorker script answers on,
	// and it is gated on the marker: a user folder that happens to be called `sync`
	// keeps serving its own worker.js through the static catch-all.
	preSpecSyncWorkerPath = "/sync/worker.js"
	// syncDataWorkerPath is Local's data worker, marker-gated the same way: one
	// stream per origin for every editing tab's room events and roster.
	syncDataWorkerPath = "/sync/data-worker.js"
)

// The two keepalives are Local's, and they differ per route: the list form pings
// faster than the single-document stream. They are variables so a unit test can
// watch a ping without waiting half a minute.
var (
	sharedStreamKeepalive = 25 * time.Second
	singleStreamKeepalive = 30 * time.Second
)

// liveWriteLimit bounds one frame's write to one stream. Node's res.write only
// queues, but a Go write blocks once the socket's buffer is full, and the hub
// writes under its lock from inside a save's commit. Without a limit, one tab
// that stops reading (a frozen background tab) would hold that save, and every
// later save, until its connection died. A stream that takes no bytes for this
// long is dropped, and its handler returns at its next keepalive.
var liveWriteLimit = 2 * time.Second

// sharedEntry is one `s` value: lane:since:document-url.
type sharedEntry struct {
	lane  string
	since int64
	href  string
}

// parseSharedEntry splits one `s` value on its first two colons only, so a
// document URL carrying a colon (a port, most often) survives.
func parseSharedEntry(raw string) (sharedEntry, bool) {
	first := strings.IndexByte(raw, ':')
	if first < 0 {
		return sharedEntry{}, false
	}
	second := strings.IndexByte(raw[first+1:], ':')
	if second < 0 {
		return sharedEntry{}, false
	}
	second += first + 1
	lane := raw[:first]
	if lane != "live" && lane != "saved" {
		return sharedEntry{}, false
	}
	sinceText := raw[first+1 : second]
	if sinceText == "" {
		return sharedEntry{}, false
	}
	for i := 0; i < len(sinceText); i++ {
		if sinceText[i] < '0' || sinceText[i] > '9' {
			return sharedEntry{}, false
		}
	}
	since, err := strconv.ParseInt(sinceText, 10, 64)
	if err != nil || since > maxSafeInteger {
		return sharedEntry{}, false
	}
	return sharedEntry{lane: lane, since: since, href: raw[second+1:]}, true
}

// parseLastEventID is the client's resume point. EventSource sends the header
// itself on reconnect; both hyperclay clients also accept the query form.
func parseLastEventID(r *http.Request) int64 {
	raw := r.Header.Get("Last-Event-Id")
	if raw == "" {
		raw = r.URL.Query().Get("lastEventId")
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || value != math.Trunc(value) {
		return 0
	}
	if value <= 0 || value > maxSafeInteger {
		return 0
	}
	return int64(value)
}

// cursorFrame is the resume baseline as a named event, carrying an id so a
// native EventSource records a position as early as possible. It never reaches
// onmessage, so it never looks like data.
func cursorFrame(seq int64, resync bool) string {
	data, _ := jsonStringify(struct {
		Seq    int64 `json:"seq"`
		Resync bool  `json:"resync,omitempty"`
	}{seq, resync})
	return "event: cursor\nid: " + strconv.FormatInt(seq, 10) + "\ndata: " + string(data) + "\n\n"
}

// sharedCursorFrame is the cursor of one subscription on a shared stream. No id:
// one id for many subscriptions would name a position for none of them.
func sharedCursorFrame(index int, seq int64, resync bool) string {
	data, _ := jsonStringify(struct {
		Sub    int   `json:"sub"`
		Seq    int64 `json:"seq"`
		Resync bool  `json:"resync,omitempty"`
	}{index, seq, resync})
	return "event: cursor\ndata: " + string(data) + "\n\n"
}

// notFoundCursorFrame answers one entry the host will not serve inside a stream
// that goes on serving the others: refusing the whole connection over one entry
// would cut every open page off because a single one closed its document.
func notFoundCursorFrame(index int) string {
	data, _ := jsonStringify(struct {
		Sub   int    `json:"sub"`
		Error string `json:"error"`
	}{index, "not-found"})
	return "event: cursor\ndata: " + string(data) + "\n\n"
}

// taggedSink is the subscriber record of one entry. The library writes
// `id: N\ndata: ...\n\n`, so prefixing the name yields
// `event: s<i>\nid: N\ndata: ...\n\n` — the exact shape htmlclay writes.
type taggedSink struct {
	sink liveSink
	tag  string
}

func (t taggedSink) write(message string) error { return t.sink.write(t.tag + message) }

// sameOriginStream is the list form's gate: a stream is opened by a page on this
// origin, and nothing else has a reason to hold one open.
func sameOriginStream(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") != "same-origin" {
		return false
	}
	origin := r.Header.Get("Origin")
	return origin == "" || origin == "http://"+r.Host
}

// hrefIsThisOrigin: a path is this host by construction; an absolute URL must
// name this host, or a page could subscribe a stream to somebody else's
// document. A value that is not an absolute URL is this origin, which is what
// Local's URL constructor refuses and its catch-all then accepts.
func hrefIsThisOrigin(r *http.Request, href string) bool {
	parsed, err := url.Parse(href)
	if err != nil || !parsed.IsAbs() {
		return href != ""
	}
	return parsed.Host == r.Host
}

// resolveSharedFile is the key the rest of the host uses: the relative path,
// resolved as a document this root may read.
func (s *rootServer) resolveSharedFile(r *http.Request, href string) (string, bool) {
	if !hrefIsThisOrigin(r, href) {
		return "", false
	}
	name := resolveResourceFromHref(href)
	if name == "" {
		return "", false
	}
	realPath, err := resolveReadPath(s.registry, name)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(realPath)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return name, true
}

// liveWriter is one SSE response as the hub writes to it.
type liveWriter struct {
	w      http.ResponseWriter
	rc     *http.ResponseController
	mu     sync.Mutex
	closed bool
}

func (w *liveWriter) write(message string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return os.ErrClosed
	}
	_ = w.rc.SetWriteDeadline(time.Now().Add(liveWriteLimit))
	defer w.rc.SetWriteDeadline(time.Time{})
	if _, err := w.w.Write([]byte(message)); err != nil {
		w.closed = true
		return err
	}
	if err := w.rc.Flush(); err != nil {
		w.closed = true
		return err
	}
	return nil
}

// destroy stops the writer handing any further frame to its socket. A Go
// response has no destroy: closing it is the handler's own business, and a
// writer that refuses every later frame is what the hub needs to hear.
func (w *liveWriter) destroy() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
}

// startEventStream writes the SSE headers and returns the writer the hub hands
// frames to. The two routes differ in one of them: the single-document stream is
// no-cache, and the list form, which carries a resume position per entry, is
// no-store.
func startEventStream(w http.ResponseWriter, cacheControl string) *liveWriter {
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", cacheControl)
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	header.Del("Content-Length")
	poweredBy(w)
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	_ = rc.Flush()
	return &liveWriter{w: w, rc: rc}
}

// serveLiveStream is Local's app.get(['/live-sync/stream', '/sync']).
func (s *rootServer) serveLiveStream(w http.ResponseWriter, r *http.Request) error {
	// The list form shares this address with the single-document form and is
	// told apart by `s`. The marker prefix is part of the address: a bare
	// `/sync?s=` is not the list form, so a user document named `sync` keeps
	// this route to itself.
	markedSync := strings.HasPrefix(r.RequestURI, "/_/sync")
	// A marked stream carrying a version or a member list is the v2 wire, which
	// needs a room registry to have members at all. This host has no rooms, so
	// such a request is answered exactly as an older Local answers it — the
	// rollback a v2 client already knows how to detect. handleSharedSyncStreamV2
	// and handleAuthorityLiveStream are the full-host stage's.
	query := r.URL.Query()
	if _, hasS := query["s"]; hasS && markedSync {
		return s.handleSharedSyncStream(w, r, query["s"])
	}

	// `document-url` is the spec spelling and wins; `page-url` is the pre-spec one.
	pageURL := query.Get("document-url")
	if pageURL == "" {
		pageURL = query.Get("page-url")
	}
	if pageURL == "" {
		sendText(w, r, http.StatusBadRequest, "document-url parameter required")
		return nil
	}
	file := resolveResourceFromHref(pageURL)
	if file == "" {
		sendText(w, r, http.StatusBadRequest, "could not resolve file from document-url")
		return nil
	}

	// Lane: edit-mode tabs ride 'live' and get pre-strip peer snapshots;
	// view-mode tabs pass ?lane=saved and only ever receive post-strip on-disk
	// HTML broadcast from the save paths. No auth — this host is single-user and
	// loopback.
	lane := "live"
	if query.Get("lane") == "saved" {
		lane = "saved"
	}

	writer := startEventStream(w, "no-cache")
	since := parseLastEventID(r)
	s.live.session(func() {
		answer := s.live.resume(file, lane, since)
		_ = writer.write(cursorFrame(answer.Baseline, answer.Resync))
		for _, message := range answer.Replay {
			_ = writer.write(message)
		}
		s.live.subscribeLocked(file, writer, lane)
		_ = writer.write(": connected\n\n")
	})
	return s.holdStream(r, writer, singleStreamKeepalive, ": ping\n\n", func() {
		s.live.unsubscribe(file, writer)
	})
}

// handleSharedSyncStream is Local's handleSharedSyncStream: one connection
// carrying several subscriptions, each frame named after the entry it belongs
// to, so a SharedWorker can demultiplex one stream into one subscription per
// page.
func (s *rootServer) handleSharedSyncStream(w http.ResponseWriter, r *http.Request, raw []string) error {
	if !sameOriginStream(r) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		sendText(w, r, http.StatusForbidden, "Forbidden")
		return nil
	}
	refuse := func() error {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		sendText(w, r, http.StatusBadRequest, "Bad Request")
		return nil
	}
	if len(raw) == 0 || len(raw) > maxSharedSubs {
		return refuse()
	}
	entries := make([]sharedEntry, len(raw))
	for i, value := range raw {
		entry, ok := parseSharedEntry(value)
		if !ok {
			return refuse()
		}
		entries[i] = entry
	}

	// Every entry is resolved before a byte is written.
	files := make([]string, len(entries))
	found := make([]bool, len(entries))
	for i, entry := range entries {
		files[i], found[i] = s.resolveSharedFile(r, entry.href)
	}

	writer := startEventStream(w, "no-store")
	type subscription struct {
		file string
		sink taggedSink
	}
	var subscribed []subscription
	s.live.session(func() {
		// Resume-then-subscribe with no gap: the cursor, the replay and the
		// subscription of one entry run with the hub to themselves.
		for i, entry := range entries {
			if !found[i] {
				_ = writer.write(notFoundCursorFrame(i))
				continue
			}
			answer := s.live.resume(files[i], entry.lane, entry.since)
			sink := taggedSink{sink: writer, tag: "event: s" + strconv.Itoa(i) + "\n"}
			_ = writer.write(sharedCursorFrame(i, answer.Baseline, answer.Resync))
			for _, message := range answer.Replay {
				_ = sink.write(message)
			}
			s.live.subscribeLocked(files[i], sink, entry.lane)
			subscribed = append(subscribed, subscription{file: files[i], sink: sink})
		}
	})
	return s.holdStream(r, writer, sharedStreamKeepalive, ": keepalive\n\n", func() {
		for _, entry := range subscribed {
			s.live.unsubscribe(entry.file, entry.sink)
		}
	})
}

// holdStream keeps one response open until its client leaves: the periodic
// comment that keeps a proxy from closing an idle stream, and the unsubscribe
// the departure owes.
func (s *rootServer) holdStream(r *http.Request, writer *liveWriter, every time.Duration, ping string, leave func()) error {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			leave()
			return nil
		case <-ticker.C:
			if err := writer.write(ping); err != nil {
				leave()
				return nil
			}
		}
	}
}

// serveWorkerScript answers one of spec §10's SharedWorker scripts. A worker
// script must be same-origin with the page that starts it, so the client library
// cannot bring its own: a host that announces `sync-worker` serves the stream
// worker, and one that announces `sync-worker-data` the data worker, each read
// from the checkout the binary already serves its runtime from.
func serveWorkerScript(w http.ResponseWriter, script []byte) error {
	header := w.Header()
	header.Set("Content-Type", "text/javascript; charset=utf-8")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Cache-Control", "no-cache")
	header.Set("Content-Length", strconv.Itoa(len(script)))
	poweredBy(w)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(script)
	return nil
}

// readWorkerScript is SYNC_WORKER_JS or SYNC_DATA_WORKER_JS: the checkout's copy
// of a worker script, read once, at boot.
func readWorkerScript(localSource, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(localSource, "src", "main", "assets", name))
}

// The write budget of one physical SSE response, shared by every sink that
// writes to it: the replay, the cursor, the named room events and the snapshot
// subscription. A connection that refuses this many frames in a row, or that
// has this much already buffered, is not reading, and the stream is torn down
// rather than buffered without bound.
const (
	sseFalseWriteLimit  = 32
	sseWriteBufferLimit = 8 * 1024 * 1024
)

// liveConn is a response a bounded writer can supervise: it says whether its
// last write was taken without buffering, how much it still holds, and when it
// drained. Go's http.ResponseWriter answers none of those, so Local wraps the
// response only on the v2 routes — which this host, having no rooms, does not
// serve yet. The port keeps the limit and its shape for the stage that does.
type liveConn interface {
	liveSink
	liveDrainable
	liveDestroyer
}

type boundedWriter struct {
	conn    liveConn
	refused int
	dropped bool
	// onDrop is the teardown a drop owes. It runs inside the hub's write path,
	// where the hub is already locked, so it may not call back into the hub: the
	// route takes the connection off the channel itself, on its own way out.
	onDrop func()
}

func newBoundedWriter(conn liveConn, onDrop func()) *boundedWriter {
	writer := &boundedWriter{conn: conn, onDrop: onDrop}
	conn.onDrain(func() {
		writer.refused = 0
	})
	return writer
}

func (b *boundedWriter) droppedOut() bool { return b.dropped }

func (b *boundedWriter) stop() {
	if b.dropped {
		return
	}
	b.dropped = true
	if b.onDrop != nil {
		b.onDrop()
	}
}

func (b *boundedWriter) write(message string) error {
	if b.dropped {
		return nil
	}
	if err := b.conn.write(message); err != nil {
		b.stop()
		return nil
	}
	if !b.conn.accepted() {
		b.refused++
	}
	if b.refused > sseFalseWriteLimit || b.conn.backlogBytes() > sseWriteBufferLimit {
		b.stop()
	}
	return nil
}

func (b *boundedWriter) destroy() {
	b.stop()
	b.conn.destroy()
}

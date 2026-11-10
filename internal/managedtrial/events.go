//go:build managedtrial

package managedtrial

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The managed trial host's event stream, ported from the cursor Local's own
// document host keeps: a ring of the last frames every client reads, the cursor
// that names a position in it, and the op record that answers what became of one
// request. A cursor is `<epoch>:<seq>`; the epoch is made once per process, so a
// cursor from a previous process is recognisably not this one's.

const (
	// eventRingFrames is how many frames the ring holds and how many op outcomes
	// it keeps.
	eventRingFrames = 10000

	// eventKeepAlive is how often an idle stream writes a line, so a proxy or a
	// socket that died is noticed.
	eventKeepAlive = 25 * time.Second

	// opRetention is how long an op outcome is kept. The ring keeps the last
	// 10,000 or 10 minutes of them, whichever is smaller.
	opRetention = 10 * time.Minute
)

// ringFrame is one encoded frame and the sequence number its cursor names.
type ringFrame struct {
	seq  uint64
	body []byte
}

// opRecord is what one opId became: its state, the checksum the write landed,
// and the cursor of the frame that reported it.
type opRecord struct {
	state    string
	checksum string
	cursor   string
	at       time.Time
}

// eventRing is the process's one stream of commit and watcher frames.
type eventRing struct {
	mu      sync.Mutex
	epoch   string
	seq     uint64
	frames  []ringFrame
	ops     map[string]opRecord
	opOrder []string
	// waiters is closed and replaced on every append, so a subscriber sleeping
	// on it wakes for the frame that was just added.
	waiters chan struct{}
}

func newEventRing() *eventRing {
	return &eventRing{
		epoch:   controlRandomHex(8),
		ops:     make(map[string]opRecord),
		waiters: make(chan struct{}),
	}
}

// controlRandomHex is n random bytes as lowercase hex: the epoch, an opId and
// anything else the control API has to make up.
func controlRandomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		seed := fmt.Sprintf("%0*x", n*2, time.Now().UnixNano())
		return seed[:n*2]
	}
	return hex.EncodeToString(buf)
}

// newOpID is the opId an HTTP write gets, because no client named one.
func newOpID() string {
	return controlRandomHex(8)
}

func (r *eventRing) cursor(seq uint64) string {
	return fmt.Sprintf("%s:%d", r.epoch, seq)
}

// append adds one frame, built with the cursor it has to carry, and wakes every
// subscriber.
func (r *eventRing) append(build func(cursor string) orderedObject) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	cursor := r.cursor(r.seq)
	body, err := jsonStringify(build(cursor))
	if err != nil {
		body = []byte("{}")
	}
	r.frames = append(r.frames, ringFrame{seq: r.seq, body: body})
	if len(r.frames) > eventRingFrames {
		r.frames = append([]ringFrame(nil), r.frames[len(r.frames)-eventRingFrames:]...)
	}
	close(r.waiters)
	r.waiters = make(chan struct{})
	return cursor
}

// changeFrame is one watcher observation, in chokidar's event names.
func (r *eventRing) changeFrame(root, event, rel string) string {
	return r.append(func(cursor string) orderedObject {
		return orderedObject{
			{Key: "type", Value: "change"},
			{Key: "root", Value: root},
			{Key: "event", Value: event},
			{Key: "rel", Value: rel},
			{Key: "cursor", Value: cursor},
		}
	})
}

// recordCommit keeps an op's outcome and appends the frame that reports it.
func (r *eventRing) recordCommit(opID, root, kind, rel, outcome, checksum string) string {
	cursor := r.append(func(cursor string) orderedObject {
		return orderedObject{
			{Key: "type", Value: "commit"},
			{Key: "root", Value: root},
			{Key: "kind", Value: kind},
			{Key: "rel", Value: rel},
			{Key: "outcome", Value: outcome},
			{Key: "checksum", Value: nullableString(checksum)},
			{Key: "opId", Value: opID},
			{Key: "cursor", Value: cursor},
		}
	})
	r.finishOp(opID, outcome, checksum, cursor)
	return cursor
}

// beginOp marks a request as running, so an opId read while it is in flight says
// so instead of looking unknown.
func (r *eventRing) beginOp(opID string) {
	if opID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	r.pruneOps(now)
	if _, ok := r.ops[opID]; !ok {
		r.opOrder = append(r.opOrder, opID)
	}
	r.ops[opID] = opRecord{state: "running", at: now}
}

// finishOp records what an op became.
func (r *eventRing) finishOp(opID, state, checksum, cursor string) {
	if opID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	r.pruneOps(now)
	if _, ok := r.ops[opID]; !ok {
		r.opOrder = append(r.opOrder, opID)
	}
	r.ops[opID] = opRecord{state: state, checksum: checksum, cursor: cursor, at: now}
}

// currentCursor is where the stream stands now, for a route that answers a
// cursor without producing a frame.
func (r *eventRing) currentCursor() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cursor(r.seq)
}

func (r *eventRing) lookupOp(opID string) (opRecord, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.ops[opID]
	return record, ok
}

// pruneOps drops the outcomes past the ring's count and past its age, oldest
// first. The caller holds the lock.
func (r *eventRing) pruneOps(now time.Time) {
	for len(r.opOrder) > eventRingFrames {
		delete(r.ops, r.opOrder[0])
		r.opOrder = r.opOrder[1:]
	}
	for len(r.opOrder) > 0 {
		record, ok := r.ops[r.opOrder[0]]
		if !ok {
			r.opOrder = r.opOrder[1:]
			continue
		}
		if now.Sub(record.at) <= opRetention {
			break
		}
		delete(r.ops, r.opOrder[0])
		r.opOrder = r.opOrder[1:]
	}
}

// backlogLocked is the frames a `since` cursor has not seen, the sequence the
// stream continues from, and whether the client has to be told its cursor cannot
// be resumed (another epoch, or frames the ring already dropped). The caller
// holds the lock.
func (r *eventRing) backlogLocked(since string) ([]ringFrame, uint64, bool) {
	if since == "" {
		return nil, r.seq + 1, false
	}
	epoch, raw, ok := strings.Cut(since, ":")
	if !ok {
		return append([]ringFrame(nil), r.frames...), r.seq + 1, true
	}
	seq, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return append([]ringFrame(nil), r.frames...), r.seq + 1, true
	}
	if epoch != r.epoch {
		return append([]ringFrame(nil), r.frames...), r.seq + 1, true
	}
	if seq >= r.seq {
		return nil, seq + 1, false
	}
	oldest := r.seq + 1
	if len(r.frames) > 0 {
		oldest = r.frames[0].seq
	}
	gap := seq+1 < oldest
	frames := make([]ringFrame, 0, len(r.frames))
	for _, frame := range r.frames {
		if frame.seq > seq {
			frames = append(frames, frame)
		}
	}
	return frames, r.seq + 1, gap
}

// heldFrames answers every frame the ring still holds, oldest first, for a
// caller that wants the stream's content without a stream.
func (r *eventRing) heldFrames() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	bodies := make([]string, 0, len(r.frames))
	for _, frame := range r.frames {
		bodies = append(bodies, string(frame.body))
	}
	return bodies
}

// serveEvents is GET /v1/events: newline-delimited JSON, a gap frame first when
// the cursor cannot be resumed, and a keepalive line while nothing happens.
func (r *eventRing) serveEvents(w http.ResponseWriter, req *http.Request) {
	flusher, _ := w.(http.Flusher)
	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	flush()

	r.mu.Lock()
	replay, next, gap := r.backlogLocked(req.URL.Query().Get("since"))
	gapBody := []byte(nil)
	if gap {
		body, err := jsonStringify(orderedObject{
			{Key: "type", Value: "gap"},
			{Key: "cursor", Value: r.cursor(r.seq)},
		})
		if err == nil {
			gapBody = body
		}
	}
	wait := r.waiters
	r.mu.Unlock()

	if gapBody != nil && !writeFrame(w, gapBody) {
		return
	}
	for _, frame := range replay {
		if !writeFrame(w, frame.body) {
			return
		}
	}
	flush()

	keepalive := time.NewTicker(eventKeepAlive)
	defer keepalive.Stop()
	for {
		r.mu.Lock()
		var fresh []ringFrame
		for _, frame := range r.frames {
			if frame.seq >= next {
				fresh = append(fresh, frame)
				next = frame.seq + 1
			}
		}
		wait = r.waiters
		r.mu.Unlock()

		for _, frame := range fresh {
			if !writeFrame(w, frame.body) {
				return
			}
		}
		if len(fresh) > 0 {
			flush()
		}

		select {
		case <-req.Context().Done():
			return
		case <-wait:
		case <-keepalive.C:
			if !writeFrame(w, []byte("{}")) {
				return
			}
			flush()
		}
	}
}

// writeFrame writes one line and reports whether the client is still there.
func writeFrame(w http.ResponseWriter, body []byte) bool {
	if _, err := w.Write(body); err != nil {
		return false
	}
	if _, err := w.Write([]byte("\n")); err != nil {
		return false
	}
	return true
}

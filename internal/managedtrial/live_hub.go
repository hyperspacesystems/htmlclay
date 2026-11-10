//go:build managedtrial

package managedtrial

import (
	"log"
	"strconv"
	"sync"
	"time"
)

// Local's livesync-hyperclay: the file-channel half, ported whole. One hub per
// process, as the module is a singleton there, so a tab on one root and a tab on
// another never share a channel unless they name the same key.
//
// Two lanes share each per-file channel: 'live' carries pre-strip peer
// snapshots, notifications and collection records to edit-mode tabs, the
// default everywhere; 'saved' carries whole post-strip documents to view-mode
// tabs and never pre-strip content. Every frame a channel publishes is also
// handed to the frame taps, which is how the replay store retains a frame that
// was published while nobody was subscribed.

// liveNow is Date.now for frame sequence numbers, which are milliseconds so they
// survive a restart. A golden records against a pinned clock.
var liveNow = func() int64 { return time.Now().UnixMilli() }

// browserSaveWindow is BROWSER_SAVE_WINDOW_MS: how long a file counts as saved by
// a browser, so the watcher skips its own write.
const browserSaveWindow int64 = 2000

// liveSink is one subscriber's write end, the object the hub hands a frame to.
// An error is Node's write that threw: the connection is gone, and the hub
// removes it.
type liveSink interface {
	write(message string) error
}

// liveDestroyer is a sink that can end its own connection now, the way
// res.destroy() frees a response whose socket has stopped reading.
type liveDestroyer interface {
	destroy()
}

// liveSubInfo is what onRemove and closeWhere are told about a connection.
type liveSubInfo struct {
	Lane string
	Meta any
}

// liveFrame is one published frame, as a tap sees it.
type liveFrame struct {
	File    string
	Lane    string
	Seq     int64
	Message string
}

// liveSubscriber is one connection on a channel.
type liveSubscriber struct {
	sink liveSink
	lane string
	meta any
}

type liveHub struct {
	mu      sync.Mutex
	clients map[string][]*liveSubscriber
	lastSeq int64
	// frameIDs prefixes every file-channel frame with `id: <seq>\n`, so an SSE
	// consumer's lastEventId carries the frame's seq.
	frameIDs bool
	taps     []func(liveFrame)
	removers []func(string, liveSubInfo)
	// recentBrowserSaves is keyed by the same full path the watcher asks about.
	recentBrowserSaves map[string]int64
}

func newLiveHub() *liveHub {
	return &liveHub{
		clients:            make(map[string][]*liveSubscriber),
		recentBrowserSaves: make(map[string]int64),
	}
}

// live is the process's one hub. The replay store is its one frame tap.
var live = newLiveHub()

// session runs fn with the hub to itself. Node runs a whole handler to
// completion, so a resume-then-subscribe cannot be split by a frame; a Go
// handler needs this lock to say the same thing.
func (h *liveHub) session(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	fn()
}

// laneOf is a connection's lane: absent in the library means 'live'.
func laneOfEntry(sub *liveSubscriber) string {
	if sub.lane == "" {
		return "live"
	}
	return sub.lane
}

// removeLocked is the one place a connection leaves a file channel, so every
// teardown path fires onRemove exactly once: membership in the channel is the
// record of whether a connection is still here.
func (h *liveHub) removeLocked(file string, sink liveSink) bool {
	subs, ok := h.clients[file]
	if !ok {
		return false
	}
	index := -1
	for i, sub := range subs {
		if sub.sink == sink {
			index = i
			break
		}
	}
	if index < 0 {
		return false
	}
	info := liveSubInfo{Lane: laneOfEntry(subs[index]), Meta: subs[index].meta}
	subs = append(subs[:index:index], subs[index+1:]...)
	if len(subs) == 0 {
		delete(h.clients, file)
	} else {
		h.clients[file] = subs
	}
	for _, handler := range h.removers {
		handleRemoval(handler, file, info)
	}
	return true
}

func handleRemoval(handler func(string, liveSubInfo), file string, info liveSubInfo) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[LiveSync] onRemove handler threw for %q", file)
		}
	}()
	handler(file, info)
}

// next is nextSeq: the wall clock, bumped by one when two frames land on the
// same millisecond.
func (h *liveHub) next() int64 {
	now := liveNow()
	if now > h.lastSeq {
		h.lastSeq = now
	} else {
		h.lastSeq++
	}
	return h.lastSeq
}

// withID prefixes a frame with its id line when frame ids are on.
func (h *liveHub) withID(seq int64, body string) string {
	if !h.frameIDs {
		return body
	}
	return "id: " + strconv.FormatInt(seq, 10) + "\n" + body
}

// tapFrame hands one published frame to every tap. A tap that fails is logged
// and skipped, so one bad consumer cannot stop delivery.
func (h *liveHub) tapFrame(file, lane string, seq int64, message string) {
	for _, tap := range h.taps {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Printf("[LiveSync] frame tap failed: %v", recovered)
				}
			}()
			tap(liveFrame{File: file, Lane: lane, Seq: seq, Message: message})
		}()
	}
}

// writeToAllLocked writes one pre-serialized frame to every subscriber on a
// lane and removes any whose write threw, so every broadcast-like method has
// identical failure semantics. A file channel's dead connection leaves through
// removeLocked, so it fires onRemove like any other departure.
func (h *liveHub) writeToAllLocked(file string, subscribers []*liveSubscriber, message, label, lane string) int {
	if len(subscribers) == 0 {
		return 0
	}
	var dead []liveSink
	sent := 0
	for _, sub := range subscribers {
		if lane != "all" && laneOfEntry(sub) != lane {
			continue
		}
		if err := sub.sink.write(message); err != nil {
			log.Printf("[LiveSync] Failed to write (%s): %v", label, err)
			dead = append(dead, sub.sink)
			continue
		}
		sent++
	}
	for _, sink := range dead {
		h.removeLocked(file, sink)
	}
	return sent
}

// subscribeLocked adds a connection to a channel on a lane.
func (h *liveHub) subscribeLocked(file string, sink liveSink, lane string) {
	if lane != "saved" {
		lane = "live"
	}
	for _, sub := range h.clients[file] {
		if sub.sink == sink {
			sub.lane = lane
			return
		}
	}
	h.clients[file] = append(h.clients[file], &liveSubscriber{sink: sink, lane: lane})
}

func (h *liveHub) subscribe(file string, sink liveSink, lane string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subscribeLocked(file, sink, lane)
}

func (h *liveHub) unsubscribe(file string, sink liveSink) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removeLocked(file, sink)
}

// onRemove registers a handler fired once for every connection that leaves a
// file channel, whichever path removed it.
func (h *liveHub) onRemove(handler func(string, liveSubInfo)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removers = append(h.removers, handler)
}

// configure turns frame ids on or off. They are off until a host asks for them,
// so an untouched caller's frames stay byte-identical.
func (h *liveHub) configureFrameIDs(enabled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.frameIDs = enabled
}

// onFrame taps every file-channel frame this hub publishes, including one
// written while nobody was subscribed.
func (h *liveHub) onFrame(handler func(liveFrame)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.taps = append(h.taps, handler)
}

// broadcastLocked gives a content frame its seq, taps it, and writes it to the
// subscribers of the lane it names.
func (h *liveHub) broadcastLocked(file string, payload livePayload, lane string) {
	subscribers := h.clients[file]
	if len(subscribers) == 0 && len(h.taps) == 0 {
		return
	}
	seq := h.next()
	message := h.withID(seq, "data: "+string(payload.frame(seq, lane))+"\n\n")
	h.tapFrame(file, lane, seq, message)
	if len(subscribers) == 0 {
		return
	}
	h.writeToAllLocked(file, subscribers, message, "broadcast "+file, lane)
}

func (h *liveHub) broadcast(file string, payload livePayload, lane string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.broadcastLocked(file, payload, lane)
}

// notify sends a toast to the clients subscribed to a file. A notification
// gains no seq field: the id line carries it, and an old client that reads the
// payload still sees exactly today's object.
func (h *liveHub) notify(file string, notice liveNotice, lane string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subscribers := h.clients[file]
	seq := h.next()
	message := h.withID(seq, "data: "+string(notice.frame())+"\n\n")
	h.tapFrame(file, lane, seq, message)
	if len(subscribers) == 0 {
		return
	}
	h.writeToAllLocked(file, subscribers, message, "notify "+file, lane)
}

// liveLimits is writeEvent's opt-in backpressure: a connection whose socket has
// refused more than MaxBacklogFrames frames since it last drained, or holds more
// than MaxBacklogBytes unsent, is dropped after this write.
type liveLimits struct {
	MaxBacklogFrames int
	MaxBacklogBytes  int
	HasFrames        bool
	HasBytes         bool
	Quiet            bool
}

func (l *liveLimits) bounded() bool {
	return l != nil && (l.HasFrames || l.HasBytes)
}

// writeEvent writes a named SSE event to a file channel, built per recipient.
// A frame built per recipient is neither given an id line nor handed to the
// taps: there is no single frame to tap and no single seq to name it.
func (h *liveHub) writeEvent(file, name string, build func(liveSubInfo) (any, bool), limits *liveLimits) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	subscribers := h.clients[file]
	if len(subscribers) == 0 {
		return 0
	}
	bounded := limits.bounded()
	maxFrames, maxBytes := -1, -1
	if limits != nil {
		if limits.HasFrames {
			maxFrames = limits.MaxBacklogFrames
		}
		if limits.HasBytes {
			maxBytes = limits.MaxBacklogBytes
		}
	}
	var dead, slow []liveSink
	sent := 0
	for _, sub := range append([]*liveSubscriber(nil), subscribers...) {
		if !h.stillSubscribed(file, sub.sink) {
			continue
		}
		payload, ok := build(liveSubInfo{Lane: laneOfEntry(sub), Meta: sub.meta})
		if !ok {
			continue
		}
		encoded, err := jsonStringify(payload)
		if err != nil {
			log.Printf("[LiveSync] Failed to build (%s %s): %v", name, file, err)
			continue
		}
		if err := sub.sink.write("event: " + name + "\ndata: " + string(encoded) + "\n\n"); err != nil {
			log.Printf("[LiveSync] Failed to write (%s %s): %v", name, file, err)
			dead = append(dead, sub.sink)
			continue
		}
		sent++
		if bounded {
			backlog := backlogOf(sub.sink)
			if !sinkAccepted(sub.sink) {
				backlog.frames++
			}
			unsent := sinkBacklogBytes(sub.sink)
			if (maxFrames >= 0 && backlog.frames > maxFrames) || (maxBytes >= 0 && unsent > maxBytes) {
				slow = append(slow, sub.sink)
			}
		}
	}
	for _, sink := range dead {
		h.removeLocked(file, sink)
	}
	for _, sink := range slow {
		h.dropSlowLocked(file, sink, name)
	}
	return sent
}

func (h *liveHub) stillSubscribed(file string, sink liveSink) bool {
	for _, sub := range h.clients[file] {
		if sub.sink == sink {
			return true
		}
	}
	return false
}

// dropSlowLocked takes a connection a bounded write found too far behind off
// the channel, then ends it so its buffered frames are freed at once instead of
// waiting on a socket that is not reading.
func (h *liveHub) dropSlowLocked(file string, sink liveSink, name string) {
	h.removeLocked(file, sink)
	log.Printf("[LiveSync] Dropped a connection that fell behind on %q for %q", name, file)
	if destroyer, ok := sink.(liveDestroyer); ok {
		destroyer.destroy()
	}
}

// backlogOf is the frames a connection has been handed while its socket
// reported it was behind, reset when the socket drains. Local's map is a
// WeakMap keyed on the response; this one holds an entry per connection that ever
// used a bounded write, which is the v2 routes' small, short-lived set.
func backlogOf(sink liveSink) *liveBacklog {
	liveBacklogsMu.Lock()
	defer liveBacklogsMu.Unlock()
	backlog, ok := liveBacklogs[sink]
	if !ok {
		backlog = &liveBacklog{}
		liveBacklogs[sink] = backlog
		if watcher, ok := sink.(liveDrainable); ok {
			watcher.onDrain(func() {
				liveBacklogsMu.Lock()
				backlog.frames = 0
				liveBacklogsMu.Unlock()
			})
		}
	}
	return backlog
}

type liveBacklog struct {
	frames int
}

// liveDrainable is a sink that can say when its socket emptied, and whether its
// last write was taken without buffering, and how much is still buffered.
type liveDrainable interface {
	onDrain(func())
	accepted() bool
	backlogBytes() int
}

var (
	liveBacklogsMu sync.Mutex
	liveBacklogs   = map[liveSink]*liveBacklog{}
)

func sinkAccepted(sink liveSink) bool {
	if drainable, ok := sink.(liveDrainable); ok {
		return drainable.accepted()
	}
	return true
}

func sinkBacklogBytes(sink liveSink) int {
	if drainable, ok := sink.(liveDrainable); ok {
		return drainable.backlogBytes()
	}
	return 0
}

// closeChannel force-closes every connection on a channel and drops it. Each
// connection leaves through the one teardown before any of them is ended, so a
// response's own close handler finds nothing and fires no second onRemove.
func (h *liveHub) closeChannel(file string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	subscribers := h.clients[file]
	if len(subscribers) == 0 {
		return 0
	}
	doomed := append([]*liveSubscriber(nil), subscribers...)
	closed := 0
	for _, sub := range doomed {
		if h.removeLocked(file, sub.sink) {
			closed++
		}
	}
	for _, sub := range doomed {
		if destroyer, ok := sub.sink.(liveDestroyer); ok {
			destroyer.destroy()
		}
	}
	return closed
}

// closeWhere force-closes exactly the connections on a channel whose metadata
// matches, leaving the rest receiving. A connection carrying no metadata never
// matches, and the predicate is not called for it.
func (h *liveHub) closeWhere(file string, predicate func(any) bool) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	subscribers := h.clients[file]
	if len(subscribers) == 0 {
		return 0
	}
	var doomed []*liveSubscriber
	for _, sub := range subscribers {
		if sub.meta == nil {
			continue
		}
		if predicate(sub.meta) {
			doomed = append(doomed, sub)
		}
	}
	closed := 0
	for _, sub := range doomed {
		if h.removeLocked(file, sub.sink) {
			closed++
		}
	}
	for _, sub := range doomed {
		if destroyer, ok := sub.sink.(liveDestroyer); ok {
			destroyer.destroy()
		}
	}
	return closed
}

// markBrowserSave marks a file as recently saved by a browser, so the watcher
// skips the "changed on disk" notice for its own write.
func (h *liveHub) markBrowserSave(file string) {
	h.mu.Lock()
	h.recentBrowserSaves[file] = liveNow()
	h.mu.Unlock()
	time.AfterFunc(time.Duration(browserSaveWindow+100)*time.Millisecond, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		savedAt, ok := h.recentBrowserSaves[file]
		if ok && liveNow()-savedAt >= browserSaveWindow {
			delete(h.recentBrowserSaves, file)
		}
	})
}

// wasBrowserSave reports whether a file was recently saved by a browser.
func (h *liveHub) wasBrowserSave(file string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	savedAt, ok := h.recentBrowserSaves[file]
	if !ok {
		return false
	}
	return liveNow()-savedAt < browserSaveWindow
}

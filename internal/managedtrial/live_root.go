//go:build managedtrial

package managedtrial

import (
	"log"

	"github.com/panphora/htmlclay/internal/clayruntime"
)

// Local's root-live: one root's projection of the process hub. A frame or an
// external-change notice goes to browsers, and a browser that receives a
// `https://clayjs.com/...` stylesheet href morphs it in and then fetches it,
// which fails offline. Both are projected the way a served document is, but
// without the descriptor: the receiving tab already holds one, and an inserted
// element would shift live sync's identity map.

// liveKeyFor is root-live's liveKeyFor. A personal root keys its channels by the
// relative path alone and any other root prefixes the key with its id; the
// trial's roots are personal, as Local's fixture declares them, so a key is the
// relative path and two roots share a channel only when they name the same
// document.
func liveKeyFor(rel string) string { return rel }

// livePayload is broadcast's payload: the snapshot or document a tab adopts,
// the optional identity map it was made with, and the two stamps that ride the
// editing lane only.
type livePayload struct {
	HTML        string
	Sender      string
	IdentityMap any
	Etag        string
	By          any
}

type liveBroadcastWire struct {
	HTML        string `json:"html"`
	Sender      string `json:"sender"`
	Seq         int64  `json:"seq"`
	IdentityMap any    `json:"identityMap,omitempty"`
	Etag        string `json:"etag,omitempty"`
	By          any    `json:"by,omitempty"`
}

// frame is the payload as it goes on the wire, in Local's key order. A stamp
// rides the editing lane only: a viewer holds a whole document, makes no saves
// and has no version to answer for, and an author on a saved-lane frame would
// name a person to whoever may view the page.
func (p livePayload) frame(seq int64, lane string) []byte {
	wire := liveBroadcastWire{HTML: p.HTML, Sender: p.Sender, Seq: seq}
	if p.IdentityMap != nil {
		wire.IdentityMap = p.IdentityMap
	}
	if p.Etag != "" && lane == "live" {
		wire.Etag = p.Etag
	}
	if p.By != nil && lane == "live" {
		wire.By = p.By
	}
	encoded, err := jsonStringify(wire)
	if err != nil {
		log.Printf("[LiveSync] Refusing to broadcast an unencodable payload: %v", err)
		return []byte("null")
	}
	return encoded
}

// liveNotice is notify's payload: a toast, with the structured data a chip or a
// reload hint reads.
type liveNotice struct {
	MsgType string `json:"msgType"`
	Msg     string `json:"msg"`
	Action  string `json:"action"`
	Data    any    `json:"data"`
}

type liveNoticeWire struct {
	Type    string `json:"type"`
	MsgType string `json:"msgType"`
	Msg     string `json:"msg"`
	Action  string `json:"action,omitempty"`
	Data    any    `json:"data,omitempty"`
}

func (n liveNotice) frame() []byte {
	encoded, err := jsonStringify(liveNoticeWire{
		Type:    "notification",
		MsgType: n.MsgType,
		Msg:     n.Msg,
		Action:  n.Action,
		Data:    n.Data,
	})
	if err != nil {
		log.Printf("[LiveSync] Refusing to notify with an unencodable payload: %v", err)
		return []byte("null")
	}
	return encoded
}

// rootLive is createRootLive: one root's projection of the hub.
type rootLive struct {
	releases map[string]clayruntime.Release
	selected string
	// notices is where a notification goes when a JS service is attached: the
	// service publishes to the tabs, so the hub here publishes nothing.
	notices func(name string, payload any)
}

func newRootLive(releases map[string]clayruntime.Release, selected string) *rootLive {
	return &rootLive{releases: releases, selected: selected}
}

// project points a payload's public runtime URLs at this host's copy of the
// bundled release. A payload with no runtime URL is passed through unchanged.
func (l *rootLive) project(html string) string {
	if html == "" {
		return html
	}
	result := clayruntime.ProjectRuntimeHTML([]byte(html), clayruntime.Options{
		Releases:        l.releases,
		SelectedVersion: l.selected,
		NoDescriptor:    true,
	})
	if len(result.Mappings) == 0 {
		return html
	}
	return result.HTML
}

func (l *rootLive) projectPayload(payload livePayload) livePayload {
	html := l.project(payload.HTML)
	if html == payload.HTML {
		return payload
	}
	payload.HTML = html
	return payload
}

func (l *rootLive) projectNotice(notice liveNotice) liveNotice {
	change, ok := notice.Data.(externalChange)
	if !ok || change.Kind != "external-change" || change.HTML == "" {
		return notice
	}
	html := l.project(change.HTML)
	if html == change.HTML {
		return notice
	}
	change.HTML = html
	notice.Data = change
	return notice
}

// session runs fn with the hub to itself, so a resume and the subscription that
// follows it cannot be split by a frame.
func (l *rootLive) session(fn func()) {
	live.session(fn)
}

func (l *rootLive) subscribe(rel string, sink liveSink, lane string) {
	live.subscribe(liveKeyFor(rel), sink, lane)
}

func (l *rootLive) unsubscribe(rel string, sink liveSink) {
	live.unsubscribe(liveKeyFor(rel), sink)
}

// subscribeLocked and unsubscribeLocked are the session's own pair: a manifest
// holds the hub for the resume, the replay and the subscription together, so the
// calls inside it must not ask for the lock again.
func (l *rootLive) subscribeLocked(rel string, sink liveSink, lane string) {
	live.subscribeLocked(liveKeyFor(rel), sink, lane)
}

func (l *rootLive) unsubscribeLocked(rel string, sink liveSink) {
	live.removeLocked(liveKeyFor(rel), sink)
}

func (l *rootLive) broadcast(rel string, payload livePayload, lane string) {
	if l.notices != nil {
		return
	}
	live.broadcast(liveKeyFor(rel), l.projectPayload(payload), lane)
}

func (l *rootLive) notify(rel string, notice liveNotice, lane string) {
	projected := l.projectNotice(notice)
	if l.notices != nil {
		l.notices(rel, projected)
		return
	}
	live.notify(liveKeyFor(rel), projected, lane)
}

// resume is the replay store's answer for one subscription of this root: the
// bucket a channel's frames are retained in is the channel's key.
func (l *rootLive) resume(rel, lane string, since int64) replayAnswer {
	return replay.resume(liveKeyFor(rel), lane, since)
}

func (l *rootLive) markBrowserSave(rel string) {
	live.markBrowserSave(liveKeyFor(rel))
}

func (l *rootLive) wasBrowserSave(rel string) bool {
	return live.wasBrowserSave(liveKeyFor(rel))
}

// liveGuard is the guard's transport: a data-loss event reaches the tabs on the
// live lane, where the chip that resolves it is.
type liveGuard struct {
	root *rootLive
}

func (g *liveGuard) Notify(name string, message guardNotification) {
	g.root.notify(name, liveNotice{
		MsgType: message.MsgType,
		Msg:     message.Msg,
		Action:  message.Action,
		Data:    message.Data,
	}, "live")
}

// liveHooks is the commit core's live half: a save's browser-save mark, the
// saved-lane broadcasts the save seams publish, and the data API's external
// change notice. G6a's goldens fix which hook calls each commit makes; this is
// where they reach subscribers.
type liveHooks struct {
	*guardHooks
	root  *rootLive
	guard *liveGuard
	// tailwind is this root's Tailwind work: a save's compile and a read's.
	tailwind *tailwindHost
}

func newLiveHooks(root *rootLive) *liveHooks {
	hooks := &liveHooks{root: root, guard: &liveGuard{root: root}}
	hooks.guardHooks = newGuardHooks(hooks.guard)
	return hooks
}

// guardLiveOf is the live transport a root's commit host hands the guard: a
// host that attached the live hooks has one, and a test host that did not
// notifies nobody.
func guardLiveOf(host *commitHost) guardLive {
	if host == nil {
		return nil
	}
	if hooks, ok := host.hooks.(*liveHooks); ok {
		return hooks.guard
	}
	return nil
}

func (h *liveHooks) MarkBrowserSave(name string) {
	h.root.markBrowserSave(name)
}

func (h *liveHooks) Broadcast(name string, message liveMessage, options broadcastOptions) {
	lane := options.Lane
	if lane == "" {
		lane = "live"
	}
	h.root.broadcast(name, livePayload{HTML: message.HTML, Sender: message.Sender}, lane)
}

func (h *liveHooks) Notify(name string, message liveNotification) {
	h.root.notify(name, liveNotice{
		MsgType: message.MsgType,
		Msg:     message.Msg,
		Action:  message.Action,
		Data:    message.Data,
	}, "live")
}

package server

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/dataguard"
	"github.com/panphora/htmlclay/internal/htmlutil"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/versions"
)

// Extractor is the guard's view of a document: the island its api rules tag
// carries, and whether one can be written back. Keeping it here is what lets
// dataguard judge an island without knowing about save tokens or the rules tag.
type Extractor struct{}

// Extract returns the island under the api rules tag. A page with no such tag is
// a nil island, not an error; a body that does not parse is an error.
func (Extractor) Extract(html []byte) (dataapi.Value, error) {
	return dataguard.ExtractIsland(htmlutil.StripToken(html))
}

// Restore writes recoverable onto html and reads it back out. A refused write
// (a shape mismatch, a clobber that emptied a list so there is no row to clone)
// or a round trip that does not match is a restore that would land wrong data, so
// it returns ErrNotRestorable instead of the merged document. A restore
// overwrites the user's file, so the answer is verified before it is written.
func (e Extractor) Restore(html []byte, recoverable dataapi.Value) ([]byte, error) {
	if recoverable == nil || dataguard.HasCodecs(recoverable) {
		return nil, dataguard.ErrNotRestorable
	}
	wrapped, ok := recoverable.(*dataapi.Object)
	if !ok {
		return nil, dataguard.ErrNotRestorable
	}
	data, ok := wrapped.Get("api")
	if !ok {
		return nil, dataguard.ErrNotRestorable
	}
	res, err := dataapi.WriteDocument(htmlutil.StripToken(html), data, "api")
	if err != nil || res == nil {
		return nil, fmt.Errorf("%w: %v", dataguard.ErrNotRestorable, err)
	}
	after, err := e.Extract(res.HTML)
	if err != nil || !dataguard.IslandsEqual(after, recoverable) {
		return nil, dataguard.ErrNotRestorable
	}
	return res.HTML, nil
}

// CanRestore reports whether recoverable can be written back onto html and read
// back out unchanged, so the panel offers Revert instead of a restore that would
// land wrong data.
func (e Extractor) CanRestore(html []byte, recoverable dataapi.Value) bool {
	_, err := e.Restore(html, recoverable)
	return err == nil
}

// GuardHistory is the guard's read-only view of one document's version history.
type GuardHistory struct {
	versions     *versions.Store
	key, absPath string
}

// NewGuardHistory returns the history reader for one document.
func NewGuardHistory(v *versions.Store, key, absPath string) *GuardHistory {
	return &GuardHistory{versions: v, key: key, absPath: absPath}
}

// newestDifferingScan is how many of the newest versions a recover copy may look
// through. An outside edit's recover copy only has to skip the version the write
// just added, so a short scan is enough and keeps a clobbered storm cheap.
const newestDifferingScan = 20

// NewestDifferingFrom returns the newest stored version whose bytes are not the
// ones named by hash. The watcher backs up the incoming bytes before the guard
// runs, so the newest version IS the clobber and the one before it is the data.
func (h *GuardHistory) NewestDifferingFrom(hash string) ([]byte, bool) {
	entries, err := h.versions.List(h.key, h.absPath)
	if err != nil {
		log.Printf("dataguard: could not list versions for %s: %v", h.absPath, err)
		return nil, false
	}
	if len(entries) > newestDifferingScan {
		entries = entries[:newestDifferingScan]
	}
	for _, entry := range entries {
		body, err := h.versions.Read(h.key, h.absPath, entry.Name)
		if err != nil {
			log.Printf("dataguard: could not read version %s of %s: %v", entry.Name, h.absPath, err)
			continue
		}
		if versions.Hash(body) != hash {
			return body, true
		}
	}
	return nil, false
}

// Keep stores content as a version of the document and returns the name of an
// entry that holds exactly those bytes. The guard uses it to make sure the bytes an
// outside write replaced exist somewhere before pinning them.
func (h *GuardHistory) Keep(content []byte) (string, error) {
	if name, ok := h.versionWithBytes(content); ok {
		return name, nil
	}
	if _, err := h.versions.Backup(h.key, h.absPath, content); err != nil {
		return "", err
	}
	if name, ok := h.versionWithBytes(content); ok {
		return name, nil
	}
	return "", fmt.Errorf("no version of %s holds the kept bytes", h.absPath)
}

// versionWithBytes finds a stored version whose bytes hash equal content.
func (h *GuardHistory) versionWithBytes(content []byte) (string, bool) {
	want := versions.Hash(content)
	entries, err := h.versions.List(h.key, h.absPath)
	if err != nil {
		return "", false
	}
	if len(entries) > newestDifferingScan {
		entries = entries[:newestDifferingScan]
	}
	for _, entry := range entries {
		body, err := h.versions.Read(h.key, h.absPath, entry.Name)
		if err != nil {
			continue
		}
		if versions.Hash(body) == want {
			return entry.Name, true
		}
	}
	return "", false
}

// Pin marks one version as kept until the user deletes it, so no pruning rule can
// remove the only copy of data the guard chose not to report.
func (h *GuardHistory) Pin(name string) error {
	return h.versions.Pin(h.key, h.absPath, name)
}

// provenanceFromRequest reads who drove a save off the request: a Save-Trigger of
// "user" is a person, anything else is the page acting on its own. A client that
// sends neither is a UI of unknown provenance, which the guard judges as one.
func provenanceFromRequest(r *http.Request) dataguard.Provenance {
	if trigger := strings.ToLower(strings.TrimSpace(r.Header.Get("Save-Trigger"))); trigger != "" {
		driven := trigger == "user"
		return dataguard.UIProvenanceOf(&driven)
	}
	switch r.Header.Get("X-Hyperclay-User-Driven") {
	case "1", "true":
		driven := true
		return dataguard.UIProvenanceOf(&driven)
	case "0", "false":
		driven := false
		return dataguard.UIProvenanceOf(&driven)
	}
	return dataguard.UIProvenanceOf(nil)
}

// writeProof is what a write showed about the version it was based on.
type writeProof struct {
	IfMatched    bool // the request named the exact stored version
	ProvenReader bool // ...and came from a program, not a page
	FromPage     bool // a browser page sent this write
}

// observeWrite hands one document write to the guard. It only enqueues: the guard
// parses the whole document, and a save must never wait for that. prev is the
// bytes the write replaced (nil when there were none) and next is what was stored.
func (s *Server) observeWrite(f *session.File, key string, prev, next []byte, prov dataguard.Provenance, proof ...writeProof) {
	if key == "" || !isExtractable(f.AbsPath) {
		return
	}
	s.noteGuardSeen(key, next)
	s.guardQueue.Do(key, func() {
		s.guard.Observe(dataguard.Observation{
			Key:          key,
			AbsPath:      f.AbsPath,
			NewHTML:      next,
			PrevHTML:     prev,
			Prov:         prov,
			IfMatched:    len(proof) > 0 && proof[0].IfMatched,
			ProvenReader: len(proof) > 0 && proof[0].ProvenReader,
			FromPage:     len(proof) > 0 && proof[0].FromPage,
			History:      NewGuardHistory(s.versions, key, f.AbsPath),
			Notify:       s.guardNotify(f, key),
		})
	})
}

// guardNotify announces a guard event on the document's live lane. The event is
// raised off the request path, so the evicted subscribers are dropped the same way
// every other live-lane notification drops them.
func (s *Server) guardNotify(f *session.File, key string) func(kind string, event *dataguard.ClientEvent, id string) {
	return func(kind string, event *dataguard.ClientEvent, id string) {
		switch kind {
		case "raised":
			if pErr := s.versions.SetProvisional(key, f.AbsPath, false); pErr != nil {
				s.logger.Printf("Could not clear provisional flag for %s: %v", f.RelPath, pErr)
			}
			s.coord.evict(f, s.hub.notifyGuard(f.AbsPath, "raised", "Saved data overwritten", event))
		case "resolved":
			s.coord.evict(f, s.hub.notifyGuard(f.AbsPath, "resolved", "Data guard resolved", map[string]string{"id": id}))
		}
	}
}

// catchUpGuard reports an outside edit that landed while no tab was open. Only the
// watcher sees a write to a file with no subscriber, and it drops one, so the page
// load is the only place such an edit can be caught. It compares and judges inside
// the document's queue job, against bytes it reads there, so it cannot mistake a
// save that already ran for an outside edit, and it waits for its own observation
// so the page it serves already carries the notice.
func (s *Server) catchUpGuard(f *session.File, key string) {
	if key == "" || !isExtractable(f.AbsPath) {
		return
	}
	done := make(chan struct{})
	s.guardQueue.Do(key, func() {
		defer close(done)
		data, err := s.readRegisteredFile(f.AbsPath)
		if err != nil {
			return
		}
		seen := s.guardSeenHash(key, f.AbsPath)
		if seen == "" || seen == versions.Hash(data) {
			return
		}
		s.noteGuardSeen(key, data)
		s.guard.Observe(dataguard.Observation{
			Key:     key,
			AbsPath: f.AbsPath,
			NewHTML: data,
			Prov:    dataguard.External,
			History: NewGuardHistory(s.versions, key, f.AbsPath),
			Notify:  s.guardNotify(f, key),
		})
	})
	<-done
}

// guardSeenHash returns the hash of the bytes the guard last saw for key, seeding
// the in-memory record from the stored state on first use, so a restart does not
// read every document as an outside edit. It reads the guard's own sidecar and
// never the document. An empty answer means the guard knows nothing yet about this
// document, which is not the same as knowing it holds nothing: a document the
// guard has never judged has no baseline to lose data against. An empty answer is
// never cached, so a baseline seeded later is found on the next look.
func (s *Server) guardSeenHash(key, absPath string) string {
	s.guardSeenMu.Lock()
	hash, ok := s.guardSeen[key]
	s.guardSeenMu.Unlock()
	if ok {
		return hash
	}
	st, err := s.guardStore.Read(key, absPath)
	if err != nil {
		return ""
	}
	if st.LastSeenHash != "" {
		s.guardSeenMu.Lock()
		s.guardSeen[key] = st.LastSeenHash
		s.guardSeenMu.Unlock()
	}
	return st.LastSeenHash
}

// noteGuardSeen records the bytes the guard is being asked to judge for key.
func (s *Server) noteGuardSeen(key string, data []byte) {
	if key == "" || data == nil {
		return
	}
	hash := versions.Hash(data)
	s.guardSeenMu.Lock()
	s.guardSeen[key] = hash
	s.guardSeenMu.Unlock()
}

// pendingGuardEvent is the pending data-loss event a document load should tell the
// person about, or nil. It re-validates the event against the bytes on disk read
// inside the guard's queue job, so a loss already undone elsewhere is cleared
// instead of shown again, and it seeds the guard's baseline on first sight the way
// the data-loss read does.
func (s *Server) pendingGuardEvent(f *session.File, key string) *dataguard.ClientEvent {
	if key == "" || !isExtractable(f.AbsPath) {
		return nil
	}
	event, err := s.guard.CurrentEvent(key, f.AbsPath, func() ([]byte, error) {
		return s.readRegisteredFile(f.AbsPath)
	})
	if err != nil {
		s.logger.Printf("Data guard notice for %s failed: %v", f.RelPath, err)
		return nil
	}
	if event == nil || event.Status != "pending" {
		return nil
	}
	return event
}

// guardSummaryText is the one short paragraph the bar shows about what was lost,
// worded the way the ClayJS notice words its detail. Paths are keys from the
// user's document, so the caller escapes the whole sentence.
func guardSummaryText(event *dataguard.ClientEvent) string {
	summary := event.LossSummary
	source := "A save from this page"
	if summary.Provenance == dataguard.External {
		source = "A change from outside this page"
	}
	loss := "changed saved data"
	switch {
	case summary.RemovedItems > 0:
		loss = "removed " + guardPlural(summary.RemovedItems, "item")
	case summary.DestroyedAtoms > 0:
		loss = "erased " + guardPlural(summary.DestroyedAtoms, "value")
	}
	text := source + " " + loss + "."
	if len(summary.Paths) > 0 {
		text += " Lost: " + strings.Join(summary.Paths, ", ") + "."
	}
	return text
}

// guardReasonTitle is the card's heading for an event the guard raised on an
// outside write, and "" for an event with no reason: those keep the older card.
func guardReasonTitle(reason string) string {
	switch reason {
	case "recent":
		return "A change from outside this page removed something you typed"
	case "all-typed":
		return "A change from outside this page removed everything you typed"
	case "emptied":
		return "A change from outside this page emptied this page's data"
	case "unreadable":
		return "A change from outside this page left its data unreadable"
	}
	return ""
}

// guardCardText is the card's heading and its one line of detail. A reason from
// the outside judge names what the person typed; every other event keeps the
// summary sentence.
func guardCardText(event *dataguard.ClientEvent) (string, string) {
	title := guardReasonTitle(event.Reason)
	if title == "" {
		return "Some saved data was removed", guardSummaryText(event)
	}
	switch event.Reason {
	case "recent", "all-typed":
		return title, guardTypedText(event.Typed)
	}
	return title, guardSummaryText(event)
}

// guardTypedText names up to three typed values the loss took, then counts the
// rest, in the person's own words.
func guardTypedText(rows []dataguard.TypedRow) string {
	if len(rows) == 0 {
		return ""
	}
	n := len(rows)
	if n > 3 {
		n = 3
	}
	parts := make([]string, n)
	for i, row := range rows[:n] {
		parts[i] = guardTypedLabel(row.Path) + `: "` + row.Yours + `"`
	}
	text := strings.Join(parts, "; ")
	if extra := len(rows) - n; extra > 0 {
		text += fmt.Sprintf(" and %d more", extra)
	}
	return text
}

// guardTypedLabel is the last key of an identity path, read as words:
// api.items[id:5].note -> note, api.tags[*] -> tags.
func guardTypedLabel(path string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		path = path[i+1:]
	}
	if i := strings.IndexByte(path, '['); i >= 0 {
		path = path[:i]
	}
	return strings.NewReplacer("_", " ", "-", " ").Replace(path)
}

func guardPlural(count int, word string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, word)
	}
	return fmt.Sprintf("%d %ss", count, word)
}

// guardBarHTML is the notice HTML Clay puts in the top-right corner of a document whose saved
// data was overwritten, for the pages whose own runtime cannot draw it: a
// document pinned to a ClayJS older than the notice module, or one using no
// ClayJS at all, never loads that module. It follows setupBarHTML: the same
// wrapper attributes and inline style values, plus data-htmlclay-guard so the
// module removes it when it takes over, and it sits inside the banner markers, so
// a save strips it byte-exactly. The buttons post the same choices the module
// posts, and the page reloads once the server says ok. It is a small card, like
// ClayJS's own notice, so it never covers the page's header. Restore is only drawn for
// an event the server says it can restore, a reason event offers Put it back
// instead, and a refusal is written into the bar's message span instead of being
// dropped.
func guardBarHTML(event *dataguard.ClientEvent) []byte {
	id, _ := json.Marshal(event.ID)
	var b strings.Builder
	b.WriteString(`<div clay="no-save no-snapshot no-watch" data-htmlclay-guard style="position:fixed;top:16px;right:16px;z-index:2147483647;width:min(330px,calc(100vw - 32px));font:13px/1.4 -apple-system,system-ui,sans-serif">`)
	b.WriteString(`<div style="display:flex;flex-wrap:wrap;align-items:center;justify-content:flex-end;gap:8px;padding:12px 14px;background:#f6ead0;color:#3b2f17;border:1px solid #e2cf9f;border-radius:8px;box-shadow:0 6px 20px rgba(59,47,23,.18)">`)
	title, text := guardCardText(event)
	b.WriteString(`<span style="flex:1 1 100%"><strong>` + html.EscapeString(title) + `</strong> ` + html.EscapeString(text) + `</span>`)
	b.WriteString(`<span data-guard-message style="color:#8a2a12"></span>`)
	primary := `padding:5px 12px;border:0;border-radius:6px;background:#2f6fde;color:#f7f9fc;font:inherit;cursor:pointer`
	if event.Reason != "" {
		if event.CanPutBack {
			b.WriteString(`<button type="button" data-choice="put-back" style="` + primary + `">Put it back</button>`)
		}
		if event.CanRevert {
			b.WriteString(`<button type="button" data-choice="revert" style="` + primary + `">Undo the change</button>`)
		}
		b.WriteString(`<button type="button" data-choice="dismiss" style="padding:5px 9px;border:0;border-radius:6px;background:transparent;color:inherit;font:inherit;cursor:pointer">Keep the change</button>`)
	} else {
		if event.Restorable {
			b.WriteString(`<button type="button" data-choice="restore" style="` + primary + `">Restore</button>`)
		}
		if event.CanRevert {
			b.WriteString(`<button type="button" data-choice="revert" style="` + primary + `">Undo the change</button>`)
		}
		b.WriteString(`<button type="button" data-choice="dismiss" style="padding:5px 9px;border:0;border-radius:6px;background:transparent;color:inherit;font:inherit;cursor:pointer">Keep as is</button>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<script>(function(){` +
		`var bar=document.currentScript.parentNode,busy=0,id=` + string(id) + `,msg=bar.querySelector('[data-guard-message]');` +
		`function send(choice){if(busy)return;busy=1;` +
		`fetch('/_/data-loss/'+encodeURIComponent(document.documentElement.getAttribute('savetoken')||''),{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:id,choice:choice})})` +
		`.then(function(r){return r.json().catch(function(){return null})}).then(function(d){busy=0;if(d&&d.ok){location.reload();return}msg.textContent=(d&&d.error)||'That did not work. Try again.'})` +
		`.catch(function(){busy=0;msg.textContent='That did not work. Try again.'})}` +
		`bar.addEventListener('click',function(e){var b=e.target.closest('button');if(!b)return;var c=b.getAttribute('data-choice');if(c)send(c)});` +
		`})();</script>`)
	b.WriteString(`</div>`)
	return []byte(b.String())
}

// observeExternalChange hands the watcher's confirmed external change to the
// guard. The watcher calls it with the file locked, so it only enqueues.
func (s *Server) observeExternalChange(f *session.File, key string, data []byte) {
	if owner, ok := s.sessions.LookupByPath(f.AbsPath); !ok || owner != f {
		return
	}
	s.observeWrite(f, key, nil, data, dataguard.External)
}

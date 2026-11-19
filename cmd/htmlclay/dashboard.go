package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/recents"
	"github.com/panphora/htmlclay/internal/server"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/specwire"
	"github.com/panphora/htmlclay/internal/trust"
	"github.com/panphora/htmlclay/internal/versions"
	servicefiles "github.com/panphora/htmlclay/service"
)

const (
	// dashboardAPIPrefix is the one part of the dashboard's API the app answers in
	// Go. Everything else under /api/ is the live-room service's, through the
	// served host's proxy.
	dashboardAPIPrefix = "/api/app/"

	// dashboardFolderPrefix is the read-only folder routes the app answers itself,
	// so a folder browses while the live-room service is stopped.
	dashboardFolderPrefix = dashboardAPIPrefix + "folders/"

	// dashboardDocumentPrefix is the version routes for one recent document: its
	// kept versions, one version's bytes, and the restore that puts one back.
	dashboardDocumentPrefix = dashboardAPIPrefix + "documents/"

	// whyNeedsApp is the reason a flag reads false on the dashboard: the action
	// needs the Mac app's consent, which a browser can never give.
	whyNeedsApp = "needs-app"

	// The refusal codes and sentences a version route answers with, in the shape
	// the dashboard reads.
	dashboardRefuseStale         = "stale"
	dashboardRefuseEtagRequired  = "etag-required"
	dashboardRefuseNotRestorable = "not-restorable"

	textVersionStale         = "This document changed since you looked, so HTML Clay didn't restore it."
	textVersionEtagRequired  = "This restore didn't include which version of the document you saw, so HTML Clay didn't restore it. Try again."
	textVersionNotRestorable = "This document isn't open or in a trusted folder, so its versions aren't available here. Open the document or trust its folder."

	// dashboardPingEvery is how often the event stream sends a comment line, so a
	// quiet stream is not closed for looking idle.
	dashboardPingEvery = 15 * time.Second

	// dashboardMaxBody and dashboardMaxActionID bound one action, as the shell's
	// listener bounds the Mac app's.
	dashboardMaxBody     = 64 << 10
	dashboardMaxActionID = 100
)

// dashboardAPI answers the dashboard's API: the app's own routes from Go,
// and every other route through proxy, which is the served host's window onto the
// live-room service.
func (a *app) dashboardAPI(proxy http.Handler) http.Handler {
	app := &dashboardApp{a: a}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, dashboardAPIPrefix) {
			app.ServeHTTP(w, r)
			return
		}
		if proxy == nil {
			w.Header().Set("Retry-After", "1")
			writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": http.StatusText(http.StatusServiceUnavailable), "reason": "unavailable"})
			return
		}
		proxy.ServeHTTP(w, r)
	})
}

// dashboardApp is the app's own part of the dashboard: the snapshot built for the
// dashboard channel, that snapshot whenever it changes, and the actions a browser
// on the dashboard's origin may send.
type dashboardApp struct {
	a *app
}

func (d *dashboardApp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasPrefix(r.URL.Path, dashboardFolderPrefix) {
		d.folder(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, dashboardDocumentPrefix) {
		d.document(w, r)
		return
	}
	switch r.URL.Path {
	case "/api/app/snapshot":
		if r.Method != http.MethodGet {
			writeDashboardJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": http.StatusText(http.StatusMethodNotAllowed), "reason": "unsupported"})
			return
		}
		writeDashboardJSON(w, http.StatusOK, d.view(d.a.snapshot()))
	case "/api/app/events":
		if r.Method != http.MethodGet {
			writeDashboardJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": http.StatusText(http.StatusMethodNotAllowed), "reason": "unsupported"})
			return
		}
		d.events(w, r)
	case "/api/app/templates":
		if r.Method != http.MethodGet {
			writeDashboardJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": http.StatusText(http.StatusMethodNotAllowed), "reason": "unsupported"})
			return
		}
		writeDashboardJSON(w, http.StatusOK, map[string]any{"templates": servicefiles.Cards()})
	case "/api/app/menu":
		if r.Method != http.MethodGet {
			writeDashboardJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": http.StatusText(http.StatusMethodNotAllowed), "reason": "unsupported"})
			return
		}
		writeDashboardJSON(w, http.StatusOK, map[string]any{"rows": d.menu()})
	case "/api/app/conflicts":
		if r.Method != http.MethodGet {
			writeDashboardJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": http.StatusText(http.StatusMethodNotAllowed), "reason": "unsupported"})
			return
		}
		writeDashboardJSON(w, http.StatusOK, map[string]any{"conflicts": d.a.dashboardConflicts()})
	case "/api/app/actions":
		if r.Method != http.MethodPost {
			writeDashboardJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": http.StatusText(http.StatusMethodNotAllowed), "reason": "unsupported"})
			return
		}
		d.act(w, r)
	default:
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": http.StatusText(http.StatusNotFound), "reason": "missing"})
	}
}

// folder answers the folder routes: one folder's entries, and the open that
// registers one of its documents. Both are read from disk, so a trusted folder
// browses without the live-room service.
func (d *dashboardApp) folder(w http.ResponseWriter, r *http.Request) {
	id, what, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, dashboardFolderPrefix), "/")
	if !ok || id == "" || what == "" || strings.Contains(what, "/") {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": http.StatusText(http.StatusNotFound), "reason": "missing"})
		return
	}
	if r.Method != http.MethodGet {
		writeDashboardJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": http.StatusText(http.StatusMethodNotAllowed), "reason": "unsupported"})
		return
	}
	switch what {
	case "list":
		d.list(w, r, id)
	case "open":
		d.open(w, r, id)
	case "history":
		page, ok := d.a.folderHistory(id, r.URL.Query().Get("before"))
		if !ok {
			writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": http.StatusText(http.StatusNotFound), "reason": "missing"})
			return
		}
		writeDashboardJSON(w, http.StatusOK, page)
	default:
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": http.StatusText(http.StatusNotFound), "reason": "missing"})
	}
}

// dashboardFolder is one folder the dashboard may read: the root that pins it, so
// no link inside it can lead outside, and whether it is served.
type dashboardFolder struct {
	root    *os.Root
	path    string
	id      string
	served  bool
	address string
}

func (f *dashboardFolder) close() { _ = f.root.Close() }

// dashboardFolderItem is one entry of a folder list.
type dashboardFolderItem struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
	Open     string `json:"open"`
}

// dashboardFolder finds the live folder the dashboard names by id: the served
// folder when it is being served, otherwise the trusted folder. Anything else,
// including a folder that is gone or no longer the one that was trusted, is not
// there.
func (a *app) dashboardFolder(id string) (*dashboardFolder, bool) {
	if id == "" {
		return nil, false
	}
	folder := &dashboardFolder{id: id}
	identity := ""
	if entry, ok := a.rt.cfg.ServedFolderByID(id); ok && entry.Serving {
		folder.path, folder.served, identity = entry.Path, true, entry.Identity
		folder.address = a.servedAddress(entry)
	} else {
		found := false
		for _, tf := range a.rt.cfg.TrustedFolderList() {
			if tf.ID == id {
				folder.path, identity, found = tf.Path, tf.Identity, true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	if info, err := os.Stat(folder.path); err != nil || !info.IsDir() {
		return nil, false
	}
	if !trust.IdentityOK(folder.path, identity, a.rt.home) {
		return nil, false
	}
	root, err := os.OpenRoot(folder.path)
	if err != nil {
		return nil, false
	}
	folder.root = root
	return folder, true
}

// list answers one folder's entries under parent, as a path inside the folder.
func (d *dashboardApp) list(w http.ResponseWriter, r *http.Request, id string) {
	folder, ok := d.a.dashboardFolder(id)
	if !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": http.StatusText(http.StatusNotFound), "reason": "missing"})
		return
	}
	defer folder.close()
	items, err := folder.list(r.URL.Query().Get("parent"))
	if err != nil {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": http.StatusText(http.StatusNotFound), "reason": "missing"})
		return
	}
	writeDashboardJSON(w, http.StatusOK, map[string]any{
		"items":        items,
		"capabilities": map[string]bool{"write": false},
	})
}

// open answers where one of the folder's documents opens: the document's own
// address for a served folder, and for a trusted, unserved folder the open that
// registers the document the way a direct open inside a trusted folder does.
func (d *dashboardApp) open(w http.ResponseWriter, r *http.Request, id string) {
	folder, ok := d.a.dashboardFolder(id)
	if !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": http.StatusText(http.StatusNotFound), "reason": "missing"})
		return
	}
	defer folder.close()
	rel, ok := folder.document(r.URL.Query().Get("path"))
	if !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": http.StatusText(http.StatusNotFound), "reason": "missing"})
		return
	}
	target := ""
	if folder.served {
		target = folder.address + "/" + dashboardFolderPath(rel)
	} else {
		target, ok = d.a.routeTrusted(filepath.Join(folder.path, filepath.FromSlash(rel)))
		if !ok {
			writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": http.StatusText(http.StatusNotFound), "reason": "missing"})
			return
		}
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// document resolves one path inside the folder to a document file through the
// root, so a link that leads outside the folder is refused here rather than
// followed.
func (f *dashboardFolder) document(name string) (string, bool) {
	rel, ok := dashboardFolderRel(name)
	if !ok {
		return "", false
	}
	info, err := f.root.Stat(rel)
	if err != nil || !info.Mode().IsRegular() || !dashboardFolderDocument(rel) {
		return "", false
	}
	return rel, true
}

// list answers the folder's entries under parent, in name order.
func (f *dashboardFolder) list(parent string) ([]dashboardFolderItem, error) {
	dir, ok := dashboardFolderRel(parent)
	if !ok {
		return nil, os.ErrNotExist
	}
	entries, err := fs.ReadDir(f.root.FS(), dir)
	if err != nil {
		return nil, err
	}
	items := make([]dashboardFolderItem, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		rel := path.Join(dir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			info, err = f.root.Stat(rel)
			if err != nil {
				continue
			}
		}
		kind := dashboardFolderKind(name, info)
		if kind == "" {
			continue
		}
		item := dashboardFolderItem{
			Name:     name,
			Path:     rel,
			Kind:     kind,
			Modified: info.ModTime().UTC().Format(time.RFC3339),
		}
		if !info.IsDir() {
			item.Size = info.Size()
		}
		if kind == "document" {
			if f.served {
				item.Open = f.address + "/" + dashboardFolderPath(rel)
			} else {
				item.Open = dashboardFolderPrefix + url.PathEscape(f.id) + "/open?path=" + url.QueryEscape(rel)
			}
		}
		items = append(items, item)
	}
	return items, nil
}

// dashboardFolderRel is a folder path as a name inside the root, refusing one
// that climbs out of it.
func dashboardFolderRel(name string) (string, bool) {
	rel := path.Clean(strings.TrimPrefix(name, "/"))
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

// dashboardFolderKind is which of the three kinds one entry is, or "" for an
// entry that is none of them.
func dashboardFolderKind(name string, info fs.FileInfo) string {
	switch {
	case info.IsDir():
		return "folder"
	case !info.Mode().IsRegular():
		return ""
	case dashboardFolderDocument(name):
		return "document"
	}
	return "file"
}

// dashboardFolderDocument reports whether a name is a document: .html or
// .htmlclay, any letter case.
func dashboardFolderDocument(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".html", ".htmlclay":
		return true
	}
	return false
}

// dashboardFolderPath escapes one folder path for a URL, segment by segment.
func dashboardFolderPath(rel string) string {
	segments := strings.Split(rel, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

// dashboardRecentDocument is one recent document the dashboard reads versions
// for: its path from the record and nowhere else, the key its history is filed
// under, and the bytes on disk its current etag stamps. A served document is not
// the loose host's to read or write, so it resolves to served and no bytes.
type dashboardRecentDocument struct {
	entry   recents.Entry
	path    string
	key     string
	data    []byte
	present bool
	served  bool
}

// document answers the version routes of one recent document: one page of its
// kept versions, one version's bytes, and the restore that puts one back.
func (d *dashboardApp) document(w http.ResponseWriter, r *http.Request) {
	id, tail, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, dashboardDocumentPrefix), "/")
	if !ok || id == "" {
		dashboardMissing(w)
		return
	}
	segments := strings.Split(tail, "/")
	switch {
	case len(segments) == 1 && segments[0] == "versions":
		if r.Method != http.MethodGet {
			dashboardUnsupported(w)
			return
		}
		d.versions(w, r, id)
	case len(segments) == 2 && segments[0] == "versions" && segments[1] != "":
		if r.Method != http.MethodGet {
			dashboardUnsupported(w)
			return
		}
		d.version(w, r, id, segments[1])
	case len(segments) == 3 && segments[0] == "versions" && segments[1] != "" && segments[2] == "restore":
		if r.Method != http.MethodPost {
			dashboardUnsupported(w)
			return
		}
		d.restore(w, r, id, segments[1])
	default:
		dashboardMissing(w)
	}
}

// dashboardRecent resolves one recent id to the loose document it names.
func (a *app) dashboardRecent(id string) (*dashboardRecentDocument, bool) {
	store := a.recentsStore()
	if store == nil || id == "" {
		return nil, false
	}
	entry, ok := store.Find(id)
	if !ok || entry.Path == "" {
		return nil, false
	}
	doc := &dashboardRecentDocument{entry: entry, path: entry.Path}
	if owner, err := a.ResolveDocument(entry.Path); err == nil && owner.Kind == OwnerServed {
		doc.served = true
		return doc, true
	}
	if data, err := os.ReadFile(entry.Path); err == nil {
		doc.data, doc.present = data, true
	}
	doc.key = a.looseHistoryKey(doc)
	return doc, true
}

// looseHistoryKey is the key a loose document's history is filed under: the one
// its live registration resolved, else the one its current bytes name, which is
// what the loose host itself falls back to.
func (a *app) looseHistoryKey(doc *dashboardRecentDocument) string {
	if _, f, ok := a.registeredFile(doc.path); ok {
		f.Lock()
		key := f.HistoryKey()
		f.Unlock()
		if key != "" {
			return key
		}
	}
	return versions.Key(doc.path, doc.data)
}

// registeredFile is the live registration of one path, with the site that holds
// it, so a version of that document routes to the host that owns it.
func (a *app) registeredFile(absPath string) (*site, *session.File, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, s := range a.sites {
		if s.closed.Load() {
			continue
		}
		if f, ok := s.sessions.LookupByPath(absPath); ok {
			return s, f, true
		}
	}
	return nil, nil, false
}

// looseWriter is the site and registration a restore writes through: one that is
// open, or one under a live trusted folder, whose registration the loose host's
// own open makes. Anything else is not restorable from the dashboard.
func (a *app) looseWriter(absPath string) (*site, *session.File, bool) {
	if s, f, ok := a.registeredFile(absPath); ok {
		return s, f, true
	}
	if !a.trustedLive(absPath) {
		return nil, nil, false
	}
	if _, ok := a.serveURL(absPath); !ok {
		return nil, nil, false
	}
	return a.registeredFile(absPath)
}

// versions answers one page of a recent document's kept versions, with the
// document's current etag so a restore can name the bytes the person looked at.
func (d *dashboardApp) versions(w http.ResponseWriter, r *http.Request, id string) {
	doc, ok := d.a.dashboardRecent(id)
	if !ok {
		dashboardMissing(w)
		return
	}
	if doc.served {
		refuseDashboard(w, http.StatusConflict, dashboardRefuseNotRestorable, textVersionNotRestorable)
		return
	}
	items, err := d.a.readHistoryPage(historyDocument{Path: doc.path}, r.URL.Query().Get("before"), historyPageSize+1)
	if err != nil {
		writeDashboardJSON(w, http.StatusInternalServerError, map[string]string{"error": http.StatusText(http.StatusInternalServerError), "reason": "list"})
		return
	}
	var next any
	if len(items) > historyPageSize {
		items = items[:historyPageSize]
		next = items[len(items)-1].ID
	}
	var etag any
	if doc.present {
		etag = specwire.Etag(doc.data)
	}
	writeDashboardJSON(w, http.StatusOK, map[string]any{"items": items, "next": next, "etag": etag})
}

// version answers one kept version's bytes as plain text, never as a page: a
// version is a document, and one carrying a script tag must not run here.
func (d *dashboardApp) version(w http.ResponseWriter, r *http.Request, id, name string) {
	doc, ok := d.a.dashboardRecent(id)
	if !ok {
		dashboardMissing(w)
		return
	}
	if doc.served {
		refuseDashboard(w, http.StatusConflict, dashboardRefuseNotRestorable, textVersionNotRestorable)
		return
	}
	if d.a.rt.versions == nil {
		dashboardMissing(w)
		return
	}
	data, err := d.a.rt.versions.Read(doc.key, doc.path, name)
	if err != nil {
		dashboardMissing(w)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Content-Security-Policy", "sandbox")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	w.Write(data)
}

// restore puts one kept version back through the loose host's own restore, so
// the bytes it replaces are kept first. Nothing is written unless the etag names
// the bytes on disk: a save that landed since the person looked is never lost.
func (d *dashboardApp) restore(w http.ResponseWriter, r *http.Request, id, name string) {
	doc, ok := d.a.dashboardRecent(id)
	if !ok {
		dashboardMissing(w)
		return
	}
	if doc.served {
		refuseDashboard(w, http.StatusConflict, dashboardRefuseNotRestorable, textVersionNotRestorable)
		return
	}
	ifMatch := r.Header.Get("If-Match")
	if strings.TrimSpace(ifMatch) == "" {
		refuseDashboard(w, http.StatusPreconditionRequired, dashboardRefuseEtagRequired, textVersionEtagRequired)
		return
	}
	if !doc.present || !specwire.IfMatchSatisfied(ifMatch, doc.data) {
		refuseDashboard(w, http.StatusPreconditionFailed, dashboardRefuseStale, textVersionStale)
		return
	}
	s, f, ok := d.a.looseWriter(doc.path)
	if !ok {
		refuseDashboard(w, http.StatusConflict, dashboardRefuseNotRestorable, textVersionNotRestorable)
		return
	}
	if err := s.srv.RestoreVersion(f, s.host, name); err != nil {
		var refusal *server.RestoreRefusal
		if errors.As(err, &refusal) && refusal.Status == http.StatusNotFound {
			dashboardMissing(w)
			return
		}
		refuseDashboard(w, http.StatusConflict, dashboardRefuseNotRestorable, textVersionNotRestorable)
		return
	}
	var etag any
	if data, err := os.ReadFile(doc.path); err == nil {
		etag = specwire.Etag(data)
	}
	writeDashboardJSON(w, http.StatusOK, map[string]any{"etag": etag})
}

// refuseDashboard answers one refusal in the shape the dashboard reads: the code
// that says which rule refused, and the sentence to show.
func refuseDashboard(w http.ResponseWriter, status int, code, text string) {
	writeDashboardJSON(w, status, map[string]any{"refusal": map[string]any{"code": code, "field": "", "text": text}})
}

// dashboardMissing is the answer to an address or an id that names nothing.
func dashboardMissing(w http.ResponseWriter) {
	writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": http.StatusText(http.StatusNotFound), "reason": "missing"})
}

// dashboardUnsupported is the answer to a method one route does not serve.
func dashboardUnsupported(w http.ResponseWriter) {
	writeDashboardJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": http.StatusText(http.StatusMethodNotAllowed), "reason": "unsupported"})
}

// act runs one action as the dashboard channel, so the class check refuses every
// grant before any effect and a browser can never answer a request.
func (d *dashboardApp) act(w http.ResponseWriter, r *http.Request) {
	var action appcore.Action
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, dashboardMaxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&action); err != nil || action.ID == "" || action.Type == "" || len(action.ID) > dashboardMaxActionID {
		writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": http.StatusText(http.StatusBadRequest), "reason": "bad-arguments"})
		return
	}
	snapshot, refusal := d.a.do(r.Context(), action, ChannelDashboard)
	if refusal != nil && refusal.Code == appcore.RefusePending {
		// A system dialog is asking the person. Nothing changed yet, and this is
		// not a failure: the change, if any, arrives on the event stream.
		writeDashboardJSON(w, http.StatusAccepted, map[string]any{"pending": true, "text": appcore.RefusalMessage(refusal), "snapshot": d.view(snapshot)})
		return
	}
	if refusal != nil {
		body := map[string]any{"code": refusal.Code, "text": appcore.RefusalMessage(refusal)}
		if refusal.Field != "" {
			body["field"] = refusal.Field
		}
		if refusal.Counts != nil {
			body["counts"] = refusal.Counts
		}
		if len(refusal.Documents) > 0 {
			body["documents"] = refusal.Documents
		}
		writeDashboardJSON(w, http.StatusConflict, map[string]any{"refusal": body})
		return
	}
	writeDashboardJSON(w, http.StatusOK, map[string]any{"snapshot": d.view(snapshot)})
}

// dashboardEvent is one data line of the event stream, the frame the dashboard
// reads (hyperclay-workspace src/local/app.ts, events).
type dashboardEvent struct {
	Type     string           `json:"type"`
	Rev      uint64           `json:"rev"`
	Snapshot appcore.Snapshot `json:"snapshot"`
}

// events streams one snapshot event per change, the current one first, and a
// comment line every dashboardPingEvery.
func (d *dashboardApp) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeDashboardJSON(w, http.StatusInternalServerError, map[string]string{"error": http.StatusText(http.StatusInternalServerError)})
		return
	}
	updates, unsubscribe := d.a.subscribe()
	defer unsubscribe()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	send := func(snapshot appcore.Snapshot) bool {
		view := d.view(snapshot)
		raw, err := json.Marshal(dashboardEvent{Type: "snapshot", Rev: view.Rev, Snapshot: view})
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", raw); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !send(d.a.snapshot()) {
		return
	}

	ticker := time.NewTicker(dashboardPingEvery)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case snapshot, open := <-updates:
			if !open {
				return
			}
			if !send(snapshot) {
				return
			}
		case <-ticker.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// dashboardSnapshot is the snapshot the dashboard channel reads: every flag that
// needs a grant reads false, with a why that says the Mac app has to do it. The
// class check refuses those actions anyway; this is what the dashboard shows.
//
// The snapshot is shared with every other reader (the Mac app, other tabs), so
// its map and folder list are copied before anything is changed.
func dashboardSnapshot(s appcore.Snapshot) appcore.Snapshot {
	s.Host.Can = maps.Clone(s.Host.Can)
	if s.Host.Can == nil {
		s.Host.Can = make(map[string]bool, len(appcore.ActionClasses))
	}
	s.Folders = slices.Clone(s.Folders)
	for typ, class := range appcore.ActionClasses {
		if class == appcore.ClassGrant {
			s.Host.Can[typ] = false
		}
	}
	text := appcore.RefusalMessage(&appcore.Refusal{Code: appcore.RefuseNeedsConfirmation})
	for i := range s.Folders {
		s.Folders[i].Can.Serve = false
		s.Folders[i].Why.Serve = &appcore.Reason{Code: whyNeedsApp, Text: text}
	}
	return s
}

// dashboardMenuRow is one row of the one menu as a dashboard page draws it:
// the Windows panel's gear menu. Action is what the row sends through
// /api/app/actions, nil for a row that only shows something; a row whose
// action this page may not send is disabled.
type dashboardMenuRow struct {
	ID        string              `json:"id"`
	Title     string              `json:"title"`
	Enabled   bool                `json:"enabled"`
	Separator bool                `json:"separator"`
	Action    *dashboardMenuEntry `json:"action"`
	Children  []dashboardMenuRow  `json:"children"`
}

type dashboardMenuEntry struct {
	Type string         `json:"type"`
	Args map[string]any `json:"args"`
}

// menu is the tray's one menu, with each row's action, for a dashboard page.
func (d *dashboardApp) menu() []dashboardMenuRow {
	s := d.a.snapshot()
	phase := appcore.PhaseOf(s)
	can := d.view(s).Host.Can
	var convert func([]appcore.MenuRow) []dashboardMenuRow
	convert = func(rows []appcore.MenuRow) []dashboardMenuRow {
		out := make([]dashboardMenuRow, 0, len(rows))
		for _, r := range rows {
			if r.ID == appcore.MenuSeparator {
				out = append(out, dashboardMenuRow{ID: r.ID, Separator: true, Children: []dashboardMenuRow{}})
				continue
			}
			row := dashboardMenuRow{ID: r.ID, Title: appcore.MenuTitle(r, s, phase), Enabled: r.Enabled, Children: convert(r.Children)}
			if typ, args := menuAction(r.ID, s); typ != "" {
				row.Action = &dashboardMenuEntry{Type: typ, Args: args}
				row.Enabled = row.Enabled && can[typ]
			}
			out = append(out, row)
		}
		return out
	}
	return convert(appcore.MenuRows(s, phase))
}

// view is the snapshot a dashboard page reads from this host.
func (d *dashboardApp) view(s appcore.Snapshot) appcore.Snapshot {
	if d.a.nativeGrants {
		return nativeDashboardSnapshot(s)
	}
	return dashboardSnapshot(s)
}

func writeDashboardJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

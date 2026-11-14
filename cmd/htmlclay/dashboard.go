package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/trust"
)

const (
	// dashboardAPIPrefix is the one part of the dashboard's API the app answers in
	// Go. Everything else under /api/ is the live-room service's, through the
	// served host's proxy.
	dashboardAPIPrefix = "/api/app/"

	// dashboardFolderPrefix is the read-only folder routes the app answers itself,
	// so a folder browses while the live-room service is stopped.
	dashboardFolderPrefix = dashboardAPIPrefix + "folders/"

	// whyNeedsApp is the reason a flag reads false on the dashboard: the action
	// needs the Mac app's consent, which a browser can never give.
	whyNeedsApp = "needs-app"

	// dashboardPingEvery is how often the event stream sends a comment line, so a
	// quiet stream is not closed for looking idle.
	dashboardPingEvery = 15 * time.Second

	// dashboardMaxBody and dashboardMaxActionID bound one action, as the shell's
	// listener bounds the Mac app's.
	dashboardMaxBody     = 64 << 10
	dashboardMaxActionID = 100
)

// dashboardAPI answers the dashboard's API: the app's own three routes from Go,
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
	switch r.URL.Path {
	case "/api/app/snapshot":
		if r.Method != http.MethodGet {
			writeDashboardJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": http.StatusText(http.StatusMethodNotAllowed), "reason": "unsupported"})
			return
		}
		writeDashboardJSON(w, http.StatusOK, dashboardSnapshot(d.a.snapshot()))
	case "/api/app/events":
		if r.Method != http.MethodGet {
			writeDashboardJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": http.StatusText(http.StatusMethodNotAllowed), "reason": "unsupported"})
			return
		}
		d.events(w, r)
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
	if refusal != nil {
		body := map[string]any{"code": refusal.Code, "text": appcore.RefusalMessage(refusal)}
		if refusal.Field != "" {
			body["field"] = refusal.Field
		}
		writeDashboardJSON(w, http.StatusConflict, map[string]any{"refusal": body})
		return
	}
	writeDashboardJSON(w, http.StatusOK, map[string]any{"snapshot": dashboardSnapshot(snapshot)})
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
		raw, err := json.Marshal(dashboardSnapshot(snapshot))
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
func dashboardSnapshot(s appcore.Snapshot) appcore.Snapshot {
	if s.Host.Can == nil {
		s.Host.Can = make(map[string]bool, len(appcore.ActionClasses))
	}
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

func writeDashboardJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

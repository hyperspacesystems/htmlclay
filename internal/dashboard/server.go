// Package dashboard serves the workspace in the browser at the dashboard host
// (htmlclay.localhost): its entry page, the bundle's static files, and who is
// let in. A port of Hyperclay Local's shell server rules
// (hyperclay-local/src/main/workspace/shell-server.js, admitted, browserAdmitted,
// /enter and the entry page) onto one fixed host.
package dashboard

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/dashboard/assets"
	servicefiles "github.com/panphora/htmlclay/service"
)

// TicketTTL is how long a ticket from EntryURL may wait to be spent.
const TicketTTL = 60 * time.Second

const sessionMaxAge = 86400

// cleanMaxAge is how long a browser keeps the record that its storage at this
// address was cleared: 400 days, the longest a browser keeps any cookie. After
// that it is cleared once more.
const cleanMaxAge = 400 * 86400

// TextHowToOpen is the page a request without a session gets when the person
// did not start it themselves (another page linked or framed the dashboard).
const TextHowToOpen = "To open the workspace, choose Open Workspace in the HTML Clay menu." // copy: whole page body, one sentence, max 90 chars

// pagePath is every address the bundle renders: Local's own list, plus the
// pages the dashboard's own Go routes read for a recent document, a waiting
// request, or a trusted folder's files, plus the settings pages (the app's
// settings on Windows and Linux, where no native settings window exists), plus
// the Windows panel's page.
var pagePath = regexp.MustCompile(`^/(roots/[^/]+/(files|document|new)|recent|needs-you|panel|recent/[^/]+/versions|trusted/[^/]+/files|synced/[^/]+/history|settings(/(general|files|folders|programs|updates|advanced|sync))?|settings/(folders|programs)/[^/]+)?$`)

// appAPIPrefix is the API the app answers in Go rather than through the service.
// It is let in by the session the entry page set together with the entry page's
// token, never by either alone.
const appAPIPrefix = "/api/app/"

// appOpenPath is the one app route a tab loads rather than a script: the page
// opens a trusted folder's document in a new tab, where it cannot add the token,
// and the answer is only a redirect to the document's own address.
var appOpenPath = regexp.MustCompile(`^/api/app/folders/[^/]+/open$`)

// templatePath is a starter template's preview image, answered from the
// pictures built into the app, so Home shows them while the live-room service
// is off. An <img> cannot add the token, and the images are the app's own, so
// they need neither the token nor a session.
var templatePath = regexp.MustCompile(`^/templates/([a-z0-9-]+)\.svg$`)

// Options configures one dashboard.
type Options struct {
	// Host is the exact Host header the dashboard answers, such as
	// "htmlclay.localhost:1212".
	Host string
	// ProductName names the app in the bundle (meta workspace-product).
	ProductName string
	// API receives every admitted /api/ request. The app's own prefix
	// (appAPIPrefix) is admitted by the session cookie with the entry page's
	// token; the rest by the token. Nil answers 503 with Retry-After: 1, which the
	// bundle shows as not answering.
	API http.Handler
	// Detour is another host on this port whose every address redirects to the
	// same address at Host: the address the dashboard had before this one. A
	// tab's load that a page on this origin started is sent out through it and
	// back. Empty sends nothing around.
	Detour string
	// CleanMark is set when a document site answered at this host before the
	// dashboard did. A browser's first entry then clears what that site stored
	// here, service workers included, and its first page leaves this value in a
	// cookie, so the dashboard's own storage is not cleared on every entry. Only
	// the config holds the value, so a page cannot write the cookie itself. Empty
	// clears nothing.
	CleanMark string
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Server is the dashboard's HTTP handler.
type Server struct {
	host, origin string
	product      string
	api          http.Handler
	now          func() time.Time
	manifest     assets.Manifest
	assetPaths   map[string]bool
	token        string
	session      string
	cookie       string
	cleanMark    string
	cleanCookie  string
	detour       string

	mu      sync.Mutex
	tickets map[string]ticket
}

type ticket struct {
	to      string
	expires time.Time
	detour  bool
}

// New builds a dashboard over the embedded bundle.
func New(o Options) (*Server, error) {
	if o.Host == "" || strings.ContainsAny(o.Host, "/ ") {
		return nil, fmt.Errorf("dashboard: bad host %q", o.Host)
	}
	m, err := assets.LoadManifest()
	if err != nil {
		return nil, err
	}
	s := &Server{
		host: o.Host, origin: "http://" + o.Host, product: o.ProductName, api: o.API, now: o.Now,
		manifest: m, assetPaths: map[string]bool{}, token: randomToken(), session: randomToken(),
		cleanMark: o.CleanMark, detour: o.Detour, tickets: map[string]ticket{},
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.product == "" {
		s.product = "HTML Clay"
	}
	// Cookies ignore the port, so the name carries it, as Local's does.
	s.cookie = "htmlclay-dashboard"
	s.cleanCookie = "htmlclay-origin-clean"
	if _, port, ok := strings.Cut(o.Host, ":"); ok {
		s.cookie += "-" + port
		s.cleanCookie += "-" + port
	}
	for _, a := range m.Assets {
		s.assetPaths["/"+a.Path] = true
	}
	return s, nil
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Origin is the dashboard's origin, http://<host>.
func (s *Server) Origin() string { return s.origin }

// EntryURL mints a ticket that opens to (a dashboard page path, "/" when
// empty) and answers the address that spends it.
func (s *Server) EntryURL(to string) (string, error) {
	if to == "" {
		to = "/"
	}
	u, err := url.Parse(to)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Fragment != "" || !pagePath.MatchString(u.Path) {
		return "", fmt.Errorf("dashboard: %q is not a dashboard page", to)
	}
	return s.origin + "/enter?ticket=" + s.mint(u.RequestURI(), false), nil
}

// mint makes a ticket for to: one that enters, or with detour one that only
// proves a load came back from the detour.
func (s *Server) mint(to string, detour bool) string {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, t := range s.tickets {
		if !now.Before(t.expires) {
			delete(s.tickets, id)
		}
	}
	id := randomToken()
	s.tickets[id] = ticket{to: to, expires: now.Add(TicketTTL), detour: detour}
	return id
}

func (s *Server) spend(id string, detour bool) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[id]
	delete(s.tickets, id)
	if !ok || t.detour != detour || !s.now().Before(t.expires) {
		return "", false
	}
	return t.to, true
}

func equal(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// fromHere reports a same-origin request or one with no origin at all.
func (s *Server) fromHere(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	site := r.Header.Get("Sec-Fetch-Site")
	return (origin == "" || origin == s.origin) && (site == "" || site == "same-origin")
}

func (s *Server) hasToken(r *http.Request) bool {
	return equal(r.Header.Get("X-Workspace-Token"), s.token)
}

// apiAdmitted is Local's rule: the entry page's token, the exact host, and a
// same-origin request or one with no origin at all.
func (s *Server) apiAdmitted(r *http.Request) bool {
	return s.hasToken(r) && s.fromHere(r)
}

// appAdmitted is who may reach the app's own routes: the entry page's script in
// the person's own browser, holding the session the entry page set and the token
// it was served. The token alone is not enough there, and neither is the shell's
// bearer. The session alone is not either: the browser sends it for every page on
// this origin, and a document that was served here before the dashboard was may
// still be open. appOpenPath is admitted without the token as a tab's own load.
func (s *Server) appAdmitted(r *http.Request) bool {
	if !s.hasSession(r) || !s.fromHere(r) {
		return false
	}
	return s.hasToken(r) || (r.Method == http.MethodGet && appOpenPath.MatchString(r.URL.Path) && tabLoad(r))
}

func (s *Server) hasSession(r *http.Request) bool {
	return carries(r, s.cookie, s.session)
}

// carries reports whether a cookie called name holds value. Every cookie of that
// name is read: a page that ran on this host can leave one on a longer path, and
// the browser sends that one first.
func carries(r *http.Request, name, value string) bool {
	for _, c := range r.CookiesNamed(name) {
		if equal(c.Value, value) {
			return true
		}
	}
	return false
}

// tabLoad reports a load the browser makes to show the answer in a tab of its
// own: an address, a link or window.open. A script's fetch, a frame and an embed
// are not one, and neither is the copy of a tab's load a service worker asked to
// be handed: those are the answers another script on this origin can read. A
// browser from before Fetch Metadata sends neither header and is served as it
// always was.
func tabLoad(r *http.Request) bool {
	dest := r.Header.Get("Sec-Fetch-Dest")
	mode := r.Header.Get("Sec-Fetch-Mode")
	return (dest == "" || dest == "document") && (mode == "" || mode == "navigate") &&
		r.Header.Get("Service-Worker-Navigation-Preload") == ""
}

// personStarted is Local's rule for a root's /: a top-level load the person
// started (a typed address, a bookmark, a link the app opened), not one another
// page caused.
func personStarted(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	dest := r.Header.Get("Sec-Fetch-Dest")
	return r.Method == http.MethodGet && (site == "" || site == "none") && (dest == "" || dest == "document")
}

// Owns reports whether p is an address the dashboard answers itself: its API,
// the entry, a page of the bundle, or one of the bundle's files. Its host was a
// document site's address once, and only a path that is none of these may be
// sent on to that site.
func (s *Server) Owns(p string) bool {
	return strings.HasPrefix(p, "/api/") || p == "/enter" || pagePath.MatchString(p) || s.assetPaths[p] || templatePath.MatchString(p)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Host != s.host {
		refuse(w, http.StatusForbidden, "forbidden")
		return
	}
	p := r.URL.Path
	if strings.HasPrefix(p, "/api/") {
		admitted := s.apiAdmitted(r)
		if strings.HasPrefix(p, appAPIPrefix) {
			admitted = s.appAdmitted(r)
		}
		if !admitted {
			refuse(w, http.StatusForbidden, "forbidden")
			return
		}
		if s.api == nil {
			w.Header().Set("Retry-After", "1")
			refuse(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		s.api.ServeHTTP(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		refuse(w, http.StatusMethodNotAllowed, "unsupported")
		return
	}
	switch {
	case p == "/enter":
		s.enter(w, r)
	case pagePath.MatchString(p):
		s.page(w, r)
	case s.assetPaths[p]:
		s.asset(w, r, strings.TrimPrefix(p, "/"))
	case templatePath.MatchString(p):
		s.picture(w, templatePath.FindStringSubmatch(p)[1])
	default:
		refuse(w, http.StatusNotFound, "missing")
	}
}

func (s *Server) enter(w http.ResponseWriter, r *http.Request) {
	to, ok := s.spend(r.URL.Query().Get("ticket"), false)
	if !ok {
		s.howToOpen(w, http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: s.cookie, Value: s.session, Path: "/", MaxAge: sessionMaxAge,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	// Cleared here because every browser passes this answer before its first page
	// with the token, and no service worker a document site registered can stand
	// in front of it: such a worker's scope is the folder its script was in, and
	// /enter is in no folder. The page that follows clears again and keeps the
	// record, so a browser that obeys the header on a page and not on a redirect
	// is cleared too.
	s.clearOnce(w, r, false)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", to)
	w.WriteHeader(http.StatusFound)
}

// clearOnce tells a browser that holds no record of it to clear what it stores
// for this address: the storage and the service workers of the site that was
// here. Cookies are left alone, since clearing them reaches every served folder's
// host. With record the answer leaves the record too.
func (s *Server) clearOnce(w http.ResponseWriter, r *http.Request, record bool) {
	if s.cleanMark == "" || carries(r, s.cleanCookie, s.cleanMark) {
		return
	}
	w.Header().Set("Clear-Site-Data", `"storage"`)
	if record {
		http.SetCookie(w, &http.Cookie{
			Name: s.cleanCookie, Value: s.cleanMark, Path: "/", MaxAge: cleanMaxAge,
			HttpOnly: true, SameSite: http.SameSiteLaxMode,
		})
	}
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if !s.hasSession(r) {
		if personStarted(r) {
			w.Header().Set("Cache-Control", "no-store")
			http.Redirect(w, r, "/enter?ticket="+url.QueryEscape(s.mint(r.URL.RequestURI(), false)), http.StatusFound)
			return
		}
		s.howToOpen(w, http.StatusForbidden)
		return
	}
	tab := tabLoad(r)
	if tab && s.detoured(w, r) {
		return
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8">`)
	// The token admits to the API, and the session reaches this page from any
	// page on this origin, which a document served here before the dashboard was
	// can still be. So the token rides only in a tab's own load: that document's
	// fetch of this address gets the page without it.
	if tab {
		fmt.Fprintf(&b, `<meta name="workspace-token" content="%s">`, html.EscapeString(s.token))
	}
	fmt.Fprintf(&b, `<meta name="workspace-product" content="%s">`, html.EscapeString(s.product))
	for _, css := range s.manifest.Entry.CSS {
		fmt.Fprintf(&b, `<link rel="stylesheet" href="/%s">`, html.EscapeString(css))
	}
	fmt.Fprintf(&b, `</head><body><div id="workspace-root"></div><script type="module" src="/%s"></script></body></html>`,
		html.EscapeString(s.manifest.Entry.JavaScript))
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "frame-ancestors 'none'")
	h.Set("X-Frame-Options", "DENY")
	// A tab another page opens here would be that page's to read. This policy
	// puts the tab in a browsing context group of its own, so the page that
	// opened it sees a closed window, whenever an answer earlier in the load
	// carried another policy or none. A document a site served carries none, and
	// detoured sees to the page whose policy a service worker wrote.
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	if tab {
		s.clearOnce(w, r, true)
	}
	w.Write([]byte(b.String()))
}

// detourParam names the ticket a load carries back from the detour.
const detourParam = "detour"

// detoured answers a tab's load that a page on this origin started with a
// redirect out through the detour host and back, and reports whether it
// answered. The opener policy cuts a tab off from the page that opened it only
// when the two differ, and a document that a service worker serves can carry
// any policy it likes. No policy matches an answer from another origin, so a
// tab that went around arrives cut off. The browser calls the load that comes
// back same-site, and its ticket is then dropped from the address. A browser
// that still calls it same-origin is served where it is, on the ticket's proof
// that it went around, rather than sent around again without end.
func (s *Server) detoured(w http.ResponseWriter, r *http.Request) bool {
	if s.detour == "" || r.Method != http.MethodGet {
		return false
	}
	started := r.Header.Get("Sec-Fetch-Site") == "same-origin"
	clean, id := splitDetour(r.URL.RequestURI())
	var to string
	switch {
	case id != "" && !started:
		to = clean
	case id != "" && s.wentAround(id, clean):
		return false
	case started:
		mark := "?"
		if strings.Contains(clean, "?") {
			mark = "&"
		}
		to = "http://" + s.detour + clean + mark + detourParam + "=" + s.mint(clean, true)
	default:
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, to, http.StatusFound)
	return true
}

func (s *Server) wentAround(id, clean string) bool {
	to, ok := s.spend(id, true)
	return ok && to == clean
}

// splitDetour takes the detour's ticket off the end of an address.
func splitDetour(uri string) (clean, id string) {
	i := strings.LastIndexAny(uri, "?&")
	if i < 0 || !strings.HasPrefix(uri[i+1:], detourParam+"=") {
		return uri, ""
	}
	return uri[:i], uri[i+len(detourParam)+2:]
}

func (s *Server) howToOpen(w http.ResponseWriter, status int) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	h.Set("X-Frame-Options", "DENY")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><title>%s</title></head><body><p>%s</p></body></html>`,
		html.EscapeString(s.product), html.EscapeString(TextHowToOpen))
}

func (s *Server) asset(w http.ResponseWriter, r *http.Request, name string) {
	b, err := fs.ReadFile(assets.Bundle(), name)
	if err != nil {
		refuse(w, http.StatusNotFound, "missing")
		return
	}
	ctype := mime.TypeByExtension(path.Ext(name))
	if path.Ext(name) == ".woff2" {
		ctype = "font/woff2"
	}
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(b)
}

func (s *Server) picture(w http.ResponseWriter, id string) {
	b, ok := servicefiles.Picture(id)
	if !ok {
		refuse(w, http.StatusNotFound, "missing")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/svg+xml")
	h.Set("Cache-Control", "no-cache")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.Write(b)
}

func refuse(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": http.StatusText(status), "reason": reason})
}

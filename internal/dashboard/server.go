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
)

// TicketTTL is how long a ticket from EntryURL may wait to be spent.
const TicketTTL = 60 * time.Second

const sessionMaxAge = 86400

// TextHowToOpen is the page a request without a session gets when the person
// did not start it themselves (another page linked or framed the dashboard).
const TextHowToOpen = "To open the workspace, choose Open Workspace in the HTML Clay menu." // copy: whole page body, one sentence, max 90 chars

// pagePath is every address the bundle renders: Local's own list.
var pagePath = regexp.MustCompile(`^/(roots/[^/]+/(files|document|new))?$`)

// appAPIPrefix is the API the app answers in Go rather than through the service.
// It is let in by the session the entry page set, never by the token alone.
const appAPIPrefix = "/api/app/"

// Options configures one dashboard.
type Options struct {
	// Host is the exact Host header the dashboard answers, such as
	// "htmlclay.localhost:1212".
	Host string
	// ProductName names the app in the bundle (meta workspace-product).
	ProductName string
	// API receives every admitted /api/ request. The app's own prefix
	// (appAPIPrefix) is admitted by the session cookie; the rest by the entry
	// page's token. Nil answers 503 with Retry-After: 1, which the bundle shows as
	// not answering.
	API http.Handler
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

	mu      sync.Mutex
	tickets map[string]ticket
}

type ticket struct {
	to      string
	expires time.Time
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
		tickets: map[string]ticket{},
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.product == "" {
		s.product = "HTML Clay"
	}
	// Cookies ignore the port, so the name carries it, as Local's does.
	s.cookie = "htmlclay-dashboard"
	if _, port, ok := strings.Cut(o.Host, ":"); ok {
		s.cookie += "-" + port
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
	return s.origin + "/enter?ticket=" + s.mint(u.RequestURI()), nil
}

func (s *Server) mint(to string) string {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, t := range s.tickets {
		if !now.Before(t.expires) {
			delete(s.tickets, id)
		}
	}
	id := randomToken()
	s.tickets[id] = ticket{to: to, expires: now.Add(TicketTTL)}
	return id
}

func (s *Server) spend(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[id]
	delete(s.tickets, id)
	if !ok || !s.now().Before(t.expires) {
		return "", false
	}
	return t.to, true
}

func equal(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// apiAdmitted is Local's rule: the entry page's token, the exact host, and a
// same-origin request or one with no origin at all.
func (s *Server) apiAdmitted(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	site := r.Header.Get("Sec-Fetch-Site")
	return equal(r.Header.Get("X-Workspace-Token"), s.token) &&
		(origin == "" || origin == s.origin) &&
		(site == "" || site == "same-origin")
}

// appAdmitted is who may reach the app's own routes: the person's own browser on
// the dashboard's origin, holding the session the entry page set. The entry page's
// token is not enough there, and neither is the shell's bearer.
func (s *Server) appAdmitted(r *http.Request) bool {
	if !s.hasSession(r) {
		return false
	}
	origin := r.Header.Get("Origin")
	site := r.Header.Get("Sec-Fetch-Site")
	return (origin == "" || origin == s.origin) && (site == "" || site == "same-origin")
}

func (s *Server) hasSession(r *http.Request) bool {
	c, err := r.Cookie(s.cookie)
	return err == nil && equal(c.Value, s.session)
}

// personStarted is Local's rule for a root's /: a top-level load the person
// started (a typed address, a bookmark, a link the app opened), not one another
// page caused.
func personStarted(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	dest := r.Header.Get("Sec-Fetch-Dest")
	return r.Method == http.MethodGet && (site == "" || site == "none") && (dest == "" || dest == "document")
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
	default:
		refuse(w, http.StatusNotFound, "missing")
	}
}

func (s *Server) enter(w http.ResponseWriter, r *http.Request) {
	to, ok := s.spend(r.URL.Query().Get("ticket"))
	if !ok {
		s.howToOpen(w, http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: s.cookie, Value: s.session, Path: "/", MaxAge: sessionMaxAge,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", to)
	w.WriteHeader(http.StatusFound)
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if !s.hasSession(r) {
		if personStarted(r) {
			w.Header().Set("Cache-Control", "no-store")
			http.Redirect(w, r, "/enter?ticket="+url.QueryEscape(s.mint(r.URL.RequestURI())), http.StatusFound)
			return
		}
		s.howToOpen(w, http.StatusForbidden)
		return
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8">`)
	fmt.Fprintf(&b, `<meta name="workspace-token" content="%s">`, html.EscapeString(s.token))
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
	w.Write([]byte(b.String()))
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

func refuse(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": http.StatusText(status), "reason": reason})
}

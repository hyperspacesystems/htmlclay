package mirror

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Prefix is where every offline route lives on a document's origin.
const Prefix = "/_/offline/"

const (
	maxNominations  = 500
	maxListBody     = 256 << 10
	hardBodyCap     = 1 << 30
	hostWorkTimeout = 2 * time.Minute
)

//go:embed sw.js
var workerSource []byte

const offWorker = `// /_/offline/sw.js, served by the HTML Clay host with offline copies switched off.
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (event) => event.waitUntil(self.registration.unregister()));
`

var workerBody = versioned(workerSource)

func versioned(src []byte) []byte {
	sum := sha256.Sum256(src)
	return bytes.Replace(src, []byte("{{VERSION}}"), []byte(hex.EncodeToString(sum[:6])), 1)
}

// Owns reports whether path is an offline route.
func Owns(path string) bool { return strings.HasPrefix(path, Prefix) }

// Handler answers the offline routes for one origin at a time.
type Handler struct {
	Store *Store
	Fetch *Fetcher
	// Off reports whether the person switched offline copies off. Nil reads as on.
	Off func() bool
}

func (h *Handler) off() bool { return h.Off != nil && h.Off() }

// Serve answers r for origin, the hostname the document is served at.
func (h *Handler) Serve(w http.ResponseWriter, r *http.Request, origin string) {
	origin = strings.ToLower(origin)
	path := strings.TrimPrefix(r.URL.Path, Prefix)
	if path == "sw.js" {
		h.serveWorker(w, r)
		return
	}
	if r.Header.Get("Sec-Fetch-Site") != "same-origin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	switch path {
	case "store":
		h.serveStore(w, r, origin)
	case "nominate":
		h.serveNominate(w, r, origin)
	case "copy":
		h.serveCopy(w, r, origin)
	case "has":
		h.serveHas(w, r, origin)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) serveWorker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if h.off() {
		io.WriteString(w, offWorker)
		return
	}
	maxBytes := "Infinity"
	if limit := h.Store.Limits().PerFile; limit > 0 {
		maxBytes = strconv.FormatInt(limit, 10)
	}
	w.Write(bytes.Replace(workerBody, []byte("{{MAX_BYTES}}"), []byte(maxBytes), 1))
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	return false
}

func hintsFor(r *http.Request, dest string) Hints {
	return Hints{UserAgent: r.Header.Get("User-Agent"), Accept: acceptFor(dest)}
}

func acceptFor(dest string) string {
	switch dest {
	case "style":
		return "text/css,*/*;q=0.1"
	case "image":
		return "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8"
	case "font":
		return "font/woff2,font/woff,*/*;q=0.5"
	default:
		return "*/*"
	}
}

// hostWork is the context for fetches a request starts: they finish even when the page
// that asked goes away, within a bound.
func hostWork(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), hostWorkTimeout)
}

// serveStore takes a copy from the worker. A body with a Clay-Content-Type header is
// stored as given; an empty body asks the host to fetch the URL itself, for an opaque
// response the worker could not read.
func (h *Handler) serveStore(w http.ResponseWriter, r *http.Request, origin string) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	target, err := NormalizeURL(r.URL.Query().Get("url"))
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	dest := r.URL.Query().Get("dest")
	limit := h.Store.Limits().PerFile
	if limit <= 0 {
		limit = hardBodyCap
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if int64(len(body)) > limit {
		http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
		return
	}
	if h.off() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ctx, cancel := hostWork(r)
	defer cancel()
	hints := hintsFor(r, dest)
	_, given := r.Header[http.CanonicalHeaderKey("Clay-Content-Type")]
	if !given && len(body) == 0 {
		if h.Fetch != nil {
			h.Fetch.Fetch(ctx, origin, target, hints, "host")
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	contentType := r.Header.Get("Clay-Content-Type")
	if err := h.Store.Put(origin, target, contentType, body, "page"); err != nil {
		if errors.Is(err, ErrTooBig) {
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if isCSS(contentType) && h.Fetch != nil {
		h.Fetch.Crawl(ctx, origin, target, body, hints, "host", true)
	}
	w.WriteHeader(http.StatusNoContent)
}

type nomination struct {
	URL  string `json:"url"`
	Kind string `json:"kind"`
}

// serveNominate fetches the files a page loaded before its worker took control, each one
// its origin does not hold yet.
func (h *Handler) serveNominate(w http.ResponseWriter, r *http.Request, origin string) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var list []nomination
	if err := json.NewDecoder(io.LimitReader(r.Body, maxListBody)).Decode(&list); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if len(list) > maxNominations {
		list = list[:maxNominations]
	}
	if h.off() || h.Fetch == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ctx, cancel := hostWork(r)
	defer cancel()
	for _, n := range list {
		if h.Store.Held(origin, []string{n.URL}) > 0 {
			continue
		}
		h.Fetch.Fetch(ctx, origin, n.URL, hintsFor(r, kindDest(n.Kind)), "host")
	}
	w.WriteHeader(http.StatusNoContent)
}

// kindDest maps a Resource Timing initiator type to the request destination it implies.
func kindDest(kind string) string {
	switch kind {
	case "css", "link":
		return "style"
	case "img", "image":
		return "image"
	default:
		return ""
	}
}

func (h *Handler) serveCopy(w http.ResponseWriter, r *http.Request, origin string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.off() {
		http.NotFound(w, r)
		return
	}
	entry, body, ok := h.Store.Get(origin, r.URL.Query().Get("url"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", entry.Type)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		w.Write(body)
	}
}

func (h *Handler) serveHas(w http.ResponseWriter, r *http.Request, origin string) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var urls []string
	if err := json.NewDecoder(io.LimitReader(r.Body, maxListBody)).Decode(&urls); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	held := 0
	if !h.off() {
		held = h.Store.Held(origin, urls)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"held": held})
}

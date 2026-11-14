// Package agentlane is the read-only lane a local program reads documents
// through. It knows nothing about the app that serves them: the Host it is
// handed lists the documents a program may read and reaches one of them.
//
// Admission is an exact Host and the absence of every header a browser sends.
// That says which address the request named and that a page did not make it; it
// does not prove which process is calling, and nothing here claims it does.
package agentlane

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// Prefix is where the lane answers on the bare loopback host.
const Prefix = "/_/agent/v1"

const (
	// ListLimit is how many documents one list holds at most.
	ListLimit = 2000

	// pageSize is how many versions one page of a document's history holds.
	pageSize = 50

	// versionScanPages is how many history pages a version read scans for the
	// version's own creation time.
	versionScanPages = 20
)

// The refusal codes this lane answers with.
const (
	codeBadRequest       = "bad-request"
	codeForbidden        = "forbidden"
	codeNotFound         = "not-found"
	codeNotReachable     = "not-reachable"
	codeMethodNotAllowed = "method-not-allowed"
	codeInternal         = "internal"
)

// The sentences a refusal carries, so the lane never invents wording at a call
// site.
const (
	textForbidden        = "This address answers only a request for 127.0.0.1 on this port with no browser headers." // TODO-COPY: agent lane, max 120 chars
	textNotFound         = "No route answers at this address."                                                       // TODO-COPY: agent lane, max 120 chars
	textNotReachable     = "That document is not in a served folder, a live trusted folder, or open."                // TODO-COPY: agent lane, max 120 chars
	textMethodNotAllowed = "This lane reads; only GET is answered."                                                  // TODO-COPY: agent lane, max 120 chars
	textBadRequest       = "That document id is not one this address issues."                                        // TODO-COPY: agent lane, max 120 chars
	textInternal         = "That document could not be read."                                                        // TODO-COPY: agent lane, max 120 chars
)

// Version is one kept version of a document.
type Version struct {
	ID         string `json:"id"`
	CreatedAt  string `json:"createdAt"`
	Size       int64  `json:"size"`
	Kind       string `json:"kind"`
	Pinned     bool   `json:"pinned"`
	Restorable bool   `json:"restorable"`
}

// Document is one document a program may read, as a list names it.
type Document struct {
	Path   string `json:"path"`
	URL    string `json:"url"`
	Folder string `json:"folder"`
	Kind   string `json:"kind"` // "served", "trusted" or "open"
	Open   bool   `json:"open"`
}

// Access is one document's reads.
type Access interface {
	Read(ctx context.Context) (html []byte, etag string, err error)
	Versions(ctx context.Context, before string, limit int) (page []Version, next string, err error)
	Version(ctx context.Context, id string) ([]byte, error)
}

// Host is the app as this lane sees it.
type Host interface {
	Version() string
	List(ctx context.Context, limit int) (docs []Document, truncated bool, err error)
	Access(ctx context.Context, path string) (Access, Document, error)
}

var (
	// ErrNotReachable is a path no document owner may reach.
	ErrNotReachable = errors.New("not reachable")

	// ErrNotFound is a document or version that is not there.
	ErrNotFound = errors.New("not found")
)

// browserHeaders are the headers every browser sends and a program does not.
// One present, even empty, means a page is calling.
var browserHeaders = []string{"Origin", "Sec-Fetch-Site", "Sec-Fetch-Mode", "Sec-Fetch-Dest", "Cookie"}

// documentRow is one row of a list.
type documentRow struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	URL    string `json:"url"`
	Folder string `json:"folder"`
	Kind   string `json:"kind"`
	Open   bool   `json:"open"`
}

// listBody is a list's answer.
type listBody struct {
	Documents []documentRow `json:"documents"`
	Truncated bool          `json:"truncated"`
}

// versionsBody is one page of a document's history.
type versionsBody struct {
	Versions   []Version `json:"versions"`
	NextBefore *string   `json:"nextBefore"`
}

// errorBody is one refusal, in the envelope the agent routes share.
type errorBody struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	ETag        string `json:"etag,omitempty"`
	SaveID      string `json:"saveId,omitempty"`
	WouldRemove any    `json:"wouldRemove,omitempty"`
}

type envelope struct {
	Error errorBody `json:"error"`
}

// handler answers every route below Prefix on one loopback address.
type handler struct {
	host Host
	addr string
}

// Handler serves everything below /_/agent/v1/. port is the bare loopback port;
// admission requires r.Host to be exactly 127.0.0.1:<port>.
func Handler(host Host, port int) http.Handler {
	return &handler{host: host, addr: "127.0.0.1:" + strconv.Itoa(port)}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !admitted(r, h.addr) {
		refuse(w, http.StatusForbidden, codeForbidden, textForbidden)
		return
	}
	kind, id, versionID := route(r.URL.Path)
	if kind == "" {
		refuse(w, http.StatusNotFound, codeNotFound, textNotFound)
		return
	}
	if r.Method != http.MethodGet {
		refuse(w, http.StatusMethodNotAllowed, codeMethodNotAllowed, textMethodNotAllowed)
		return
	}
	switch kind {
	case "whoami":
		h.whoami(w)
	case "documents":
		h.list(w, r)
	case "document":
		h.read(w, r, id)
	case "versions":
		h.versions(w, r, id)
	case "version":
		h.version(w, r, id, versionID)
	}
}

// admitted reports whether the request named this lane's own address and carried
// no header a browser sends.
func admitted(r *http.Request, addr string) bool {
	if r.Host != addr {
		return false
	}
	for _, name := range browserHeaders {
		if _, ok := r.Header[http.CanonicalHeaderKey(name)]; ok {
			return false
		}
	}
	return true
}

// route names what the path asks for: the route, the document id and, for a
// version, the version id. An empty kind is a route this lane does not have.
func route(path string) (kind, id, versionID string) {
	rest, ok := strings.CutPrefix(path, Prefix)
	if !ok {
		return "", "", ""
	}
	switch rest {
	case "/whoami":
		return "whoami", "", ""
	case "/documents":
		return "documents", "", ""
	}
	tail, ok := strings.CutPrefix(rest, "/documents/")
	if !ok {
		return "", "", ""
	}
	segments := strings.Split(tail, "/")
	switch {
	case len(segments) == 1 && segments[0] != "":
		return "document", segments[0], ""
	case len(segments) == 2 && segments[0] != "" && segments[1] == "versions":
		return "versions", segments[0], ""
	case len(segments) == 3 && segments[0] != "" && segments[1] == "versions" && segments[2] != "":
		return "version", segments[0], segments[2]
	}
	return "", "", ""
}

// whoami names the product, its version and what this lane offers today.
func (h *handler) whoami(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{
		"host":         "htmlclay",
		"version":      h.host.Version(),
		"capabilities": []string{"read", "versions"},
	})
}

// list answers every document a program may read.
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	docs, truncated, err := h.host.List(r.Context(), ListLimit)
	if err != nil {
		refuse(w, http.StatusInternalServerError, codeInternal, textInternal)
		return
	}
	rows := make([]documentRow, 0, len(docs))
	for _, doc := range docs {
		rows = append(rows, documentRow{
			ID:     DocumentID(doc.Path),
			Path:   doc.Path,
			URL:    doc.URL,
			Folder: doc.Folder,
			Kind:   doc.Kind,
			Open:   doc.Open,
		})
	}
	writeJSON(w, http.StatusOK, listBody{Documents: rows, Truncated: truncated})
}

// read answers one document's bytes and the stamp its own save lane compares.
func (h *handler) read(w http.ResponseWriter, r *http.Request, id string) {
	path, ok := PathFromID(id)
	if !ok {
		refuse(w, http.StatusBadRequest, codeBadRequest, textBadRequest)
		return
	}
	access, doc, err := h.host.Access(r.Context(), path)
	if err != nil {
		h.fail(w, err)
		return
	}
	html, etag, err := access.Read(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("ETag", etag)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":   id,
		"path": path,
		"url":  doc.URL,
		"etag": etag,
		"html": string(html),
	})
}

// versions answers one page of a document's kept versions.
func (h *handler) versions(w http.ResponseWriter, r *http.Request, id string) {
	path, ok := PathFromID(id)
	if !ok {
		refuse(w, http.StatusBadRequest, codeBadRequest, textBadRequest)
		return
	}
	access, _, err := h.host.Access(r.Context(), path)
	if err != nil {
		h.fail(w, err)
		return
	}
	page, next, err := access.Versions(r.Context(), r.URL.Query().Get("before"), pageSize)
	if err != nil {
		h.fail(w, err)
		return
	}
	rows := make([]Version, 0, len(page))
	rows = append(rows, page...)
	body := versionsBody{Versions: rows}
	if next != "" {
		body.NextBefore = &next
	}
	writeJSON(w, http.StatusOK, body)
}

// version answers one kept version's bytes and the time it was kept.
func (h *handler) version(w http.ResponseWriter, r *http.Request, id, versionID string) {
	path, ok := PathFromID(id)
	if !ok {
		refuse(w, http.StatusBadRequest, codeBadRequest, textBadRequest)
		return
	}
	access, _, err := h.host.Access(r.Context(), path)
	if err != nil {
		h.fail(w, err)
		return
	}
	html, err := access.Version(r.Context(), versionID)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":        versionID,
		"createdAt": createdAt(r.Context(), access, versionID),
		"html":      string(html),
	})
}

// createdAt is when the version was kept, from the first page that names it. A
// version no page holds answers "".
func createdAt(ctx context.Context, access Access, id string) string {
	before := ""
	for range versionScanPages {
		page, next, err := access.Versions(ctx, before, pageSize)
		if err != nil {
			return ""
		}
		for _, version := range page {
			if version.ID == id {
				return version.CreatedAt
			}
		}
		if next == "" || next == before {
			return ""
		}
		before = next
	}
	return ""
}

// fail answers an access error in the lane's envelope.
func (h *handler) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotReachable):
		refuse(w, http.StatusNotFound, codeNotReachable, textNotReachable)
	case errors.Is(err, ErrNotFound):
		refuse(w, http.StatusNotFound, codeNotFound, textNotFound)
	default:
		refuse(w, http.StatusInternalServerError, codeInternal, textInternal)
	}
}

// refuse answers one refusal: the status and the envelope carrying the code and
// the sentence that explains it.
func refuse(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, envelope{Error: errorBody{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// DocumentID is a document's id in this lane: p_ plus the base64url encoding of
// its canonical path.
func DocumentID(path string) string {
	return "p_" + base64.RawURLEncoding.EncodeToString([]byte(path))
}

// PathFromID is the path an id names, and false when the prefix or the encoding
// is wrong.
func PathFromID(id string) (string, bool) {
	encoded, ok := strings.CutPrefix(id, "p_")
	if !ok || encoded == "" {
		return "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(decoded) == 0 {
		return "", false
	}
	return string(decoded), true
}

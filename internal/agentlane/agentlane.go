// Package agentlane is the lane a local program reads and writes documents
// through. It knows nothing about the app that serves them: the Host it is
// handed lists the documents a program may read and reaches one of them.
//
// Admission is an exact Host and the absence of every header a browser sends.
// That says which address the request named and that a page did not make it; it
// does not prove which process is calling, and nothing here claims it does.
package agentlane

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	// saveBodyLimit bounds a write's JSON body. The owner refuses a document over
	// its own limit with ErrTooLarge.
	saveBodyLimit = 64 << 20

	// maxOperationID is the longest operation id a write may name.
	maxOperationID = 128
)

// The refusal codes this lane answers with.
const (
	codeBadRequest       = "bad-request"
	codeForbidden        = "forbidden"
	codeNotFound         = "not-found"
	codeNotReachable     = "not-reachable"
	codeMethodNotAllowed = "method-not-allowed"
	codeInternal         = "internal"

	codePreconditionRequired = "precondition-required"
	codeInvalidMatch         = "invalid-match"
	codeOperationReused      = "operation-reused"
	codeConflict             = "conflict"
	codeTooLarge             = "too-large"
	codeInvalidDocument      = "invalid-document"
	codeRecoveryUnavailable  = "recovery-unavailable"

	codeReadRequired     = "read-required"
	codeNoAPIRules       = "no-api-rules"
	codeBadAPIRules      = "bad-api-rules"
	codeBadData          = "bad-data"
	codePatchUnsupported = "patch-unsupported"

	codeUnauthorized = "unauthorized"
)

// The sentences a refusal carries, so the lane never invents wording at a call
// site.
const (
	textForbidden            = "This address requires 127.0.0.1 on this port with no browser headers. Send the request to 127.0.0.1 without them."
	textNotFound             = "No route matches this address. Check the address and try again."
	textNotReachable         = "That document is not open or in a served or live trusted folder. Open it, or serve or trust its folder."
	textMethodNotAllowed     = "This route does not accept that method. Use a method this route supports."
	textBadRequest           = "This address did not issue that document id. Use a document id from this address."
	textInternal             = "HTML Clay could not read that document. Check the document and try again."
	textPreconditionRequired = "A write needs If-Match. Read the document first, then send the etag from that read."
	textInvalidMatch         = "If-Match must be exactly one etag from a read: not *, not a weak tag, and not a list. Read the document and send that etag."
	textOperationReused      = "You already used this operationId for a different write. Read the document again and send a new operationId."
	textConflict             = "The document changed since you read it. Read it again and make your change on that version."
	textTooLarge             = "That document is larger than HTML Clay can save. Reduce its size and try again."
	textInvalidDocument      = "That is not a whole HTML document. Send a complete HTML document and try again."
	textRecoveryUnavailable  = "The current version could not be stored in history, so nothing was written. Try again." // contract text, verbatim
	textBadBody              = "Send a JSON body with html. Include operationId in the body or send a Save-ID header."
	textWriteFailed          = "HTML Clay could not write that document. Check the file and try again."
	textBadDataBody          = "Send a JSON body with data or patch, never both. Include operationId in the body or send a Save-ID header."
	textNoAPIRules           = "This document has no api rules tag, so it has no data. Read its HTML and save the whole document."
	textPatchUnsupported     = "This document's folder only accepts whole data. Read the data, change it, and send it as data."
	textDataTooLarge         = "That data is too large for HTML Clay to write at once. Send fewer fields, or save the whole document."
	textUnauthorized         = "This request did not include the current agent token. Send the token from %s as Authorization: Bearer <token>."
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

// Access is one document's reads and writes.
type Access interface {
	Read(ctx context.Context) (html []byte, etag string, err error)
	Versions(ctx context.Context, before string, limit int) (page []Version, next string, err error)
	Version(ctx context.Context, id string) ([]byte, error)
	Save(ctx context.Context, html []byte, ifMatch, saveID string) (WriteResult, error)
	Restore(ctx context.Context, versionID, ifMatch, saveID string) (WriteResult, error)
	ReadData(ctx context.Context) (data []byte, etag string, err error)
	WriteData(ctx context.Context, write DataWrite) (WriteResult, error)
}

// DataWrite is a write to the data a document publishes through its api rules
// tag: the JSON to write through the rules or the JSON of a sheet patch's
// operations, never both, and the etag the program read.
type DataWrite struct {
	Data    []byte
	Patch   []byte
	IfMatch string
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

// WriteResult is what a write answers: the stamp the document now has, whether its
// bytes moved, and the version holding the bytes it replaced.
type WriteResult struct {
	ETag        string
	Changed     bool
	UndoVersion string
	// Stale is a sheet patch's own answer: the owner cannot recalculate, so the
	// patched sheets hold formula results from before the patch.
	Stale bool
}

// StaleError is a write whose If-Match no longer names the document. It carries the
// stamp the document has now and the Save-ID that wrote it, when that is known.
type StaleError struct {
	ETag   string
	SaveID string
}

func (e *StaleError) Error() string { return "document changed since it was read" }

var (
	// ErrBackup is a write refused because the version it would replace could not
	// be kept. Nothing was written.
	ErrBackup = errors.New("the current version could not be kept")

	// ErrTooLarge is a document larger than its owner saves.
	ErrTooLarge = errors.New("document too large")

	// ErrNotDocument is bytes that are not a whole HTML document.
	ErrNotDocument = errors.New("not a complete HTML document")

	// ErrNoAPIRules is a document with no api rules tag, so it has no data.
	ErrNoAPIRules = errors.New("the document has no api rules tag")

	// ErrPatchUnsupported is a patch sent to a document whose owner takes whole
	// data only.
	ErrPatchUnsupported = errors.New("the document's owner takes no patches")
)

// WouldRemove is what a refused write would have taken from the document, as its
// owner's data guard counts it.
type WouldRemove struct {
	Items  int      `json:"items"`
	Values int      `json:"values"`
	Paths  []string `json:"paths"`
}

// ReadRequiredError is a write its owner's data guard refused: it would remove
// data, and the program has not shown it read the version it would replace. It
// carries the guard's own sentence, the stamp to read at and what would go.
type ReadRequiredError struct {
	ETag        string
	Message     string
	WouldRemove WouldRemove
}

func (e *ReadRequiredError) Error() string { return "the write would remove data that was not read" }

// DataError is a data read or write the document's owner refused, with the status
// and the sentence its own data route answers and its code when it names one.
// Rules says the document's api rules tag is at fault, not what the program
// sent. Nothing was written.
type DataError struct {
	Status  int
	Code    string
	Rules   bool
	Message string
}

func (e *DataError) Error() string { return e.Message }

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
	host   Host
	addr   string
	replay *Replay
	// token is what a request's Authorization header must carry; requireToken
	// says the check runs at all. An empty required token admits nothing.
	token        string
	tokenFile    string
	requireToken bool
}

// Option changes how Handler admits a request.
type Option func(*handler)

// RequireToken makes every request carry Authorization: Bearer <token>. file is
// where the token lives, named in the refusal so a program knows where to look.
func RequireToken(token, file string) Option {
	return func(h *handler) {
		h.token = token
		h.tokenFile = file
		h.requireToken = true
	}
}

// Handler serves everything below /_/agent/v1/. port is the bare loopback port;
// admission requires r.Host to be exactly 127.0.0.1:<port>.
func Handler(host Host, port int, opts ...Option) http.Handler {
	h := &handler{host: host, addr: "127.0.0.1:" + strconv.Itoa(port), replay: NewReplay(nil)}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// tokenMatches reports whether the request carries this lane's token. An empty
// token never matches, so a lane whose token could not be read admits nothing.
func (h *handler) tokenMatches(r *http.Request) bool {
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || h.token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.token)) == 1
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !admitted(r, h.addr) {
		refuse(w, http.StatusForbidden, codeForbidden, textForbidden)
		return
	}
	if h.requireToken && !h.tokenMatches(r) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		refuse(w, http.StatusUnauthorized, codeUnauthorized, fmt.Sprintf(textUnauthorized, h.tokenFile))
		return
	}
	kind, id, versionID := route(r.URL.Path)
	if kind == "" {
		refuse(w, http.StatusNotFound, codeNotFound, textNotFound)
		return
	}
	switch {
	case kind == "document" && r.Method == http.MethodPut:
		h.save(w, r, id)
		return
	case kind == "restore" && r.Method == http.MethodPost:
		h.restore(w, r, id, versionID)
		return
	case kind == "data" && r.Method == http.MethodPost:
		h.writeData(w, r, id)
		return
	case kind == "restore" || r.Method != http.MethodGet:
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
	case "data":
		h.data(w, r, id)
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
	case len(segments) == 2 && segments[0] != "" && segments[1] == "data":
		return "data", segments[0], ""
	case len(segments) == 2 && segments[0] != "" && segments[1] == "versions":
		return "versions", segments[0], ""
	case len(segments) == 3 && segments[0] != "" && segments[1] == "versions" && segments[2] != "":
		return "version", segments[0], segments[2]
	case len(segments) == 4 && segments[0] != "" && segments[1] == "versions" && segments[2] != "" && segments[3] == "restore":
		return "restore", segments[0], segments[2]
	}
	return "", "", ""
}

// whoami names the product, its version and what this lane offers today.
func (h *handler) whoami(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{
		"host":         "htmlclay",
		"version":      h.host.Version(),
		"capabilities": []string{"read", "save", "data", "versions", "restore"},
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

// data answers the data a document publishes through its api rules tag, and the
// stamp its owner's data write compares.
func (h *handler) data(w http.ResponseWriter, r *http.Request, id string) {
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
	data, etag, err := access.ReadData(r.Context())
	if err != nil {
		if answer, ok := dataRefusal(err); ok {
			send(w, answer)
			return
		}
		h.fail(w, err)
		return
	}
	w.Header().Set("ETag", etag)
	writeJSON(w, http.StatusOK, map[string]any{
		"data": json.RawMessage(data),
		"etag": etag,
	})
}

// writeBody is a write's JSON body. A save names html and a data write names
// data or patch; each may name the operation id, which a Save-ID header may name
// instead.
type writeBody struct {
	HTML        *string         `json:"html"`
	Data        json.RawMessage `json:"data"`
	Patch       json.RawMessage `json:"patch"`
	OperationID string          `json:"operationId"`
}

// bodyKind is which write a body belongs to, and so what it must name.
type bodyKind int

const (
	restoreBody bodyKind = iota // may be empty
	saveBody                    // names html
	dataBody                    // names data or patch, never both
)

// save replaces one document's bytes through its owner's own save lane.
func (h *handler) save(w http.ResponseWriter, r *http.Request, id string) {
	path, ok := PathFromID(id)
	if !ok {
		refuse(w, http.StatusBadRequest, codeBadRequest, textBadRequest)
		return
	}
	tag, refused := strongIfMatch(w, r)
	if refused {
		return
	}
	body, key, ok := readWriteBody(w, r, saveBody)
	if !ok {
		return
	}
	h.write(w, id+"\x00"+key, fingerprintOf(r.Method, r.URL.Path, tag, *body.HTML), func() Answer {
		access, _, err := h.host.Access(r.Context(), path)
		if err != nil {
			return writeRefusal(err)
		}
		result, err := access.Save(r.Context(), []byte(*body.HTML), tag, key)
		if err != nil {
			return writeRefusal(err)
		}
		return writtenAnswer(result, key, false)
	})
}

// restore makes a kept version the document's bytes again, keeping the current
// bytes first.
func (h *handler) restore(w http.ResponseWriter, r *http.Request, id, versionID string) {
	path, ok := PathFromID(id)
	if !ok {
		refuse(w, http.StatusBadRequest, codeBadRequest, textBadRequest)
		return
	}
	tag, refused := strongIfMatch(w, r)
	if refused {
		return
	}
	_, key, ok := readWriteBody(w, r, restoreBody)
	if !ok {
		return
	}
	h.write(w, id+"\x00"+key, fingerprintOf(r.Method, r.URL.Path, tag), func() Answer {
		access, _, err := h.host.Access(r.Context(), path)
		if err != nil {
			return writeRefusal(err)
		}
		result, err := access.Restore(r.Context(), versionID, tag, key)
		if err != nil {
			return writeRefusal(err)
		}
		return writtenAnswer(result, key, false)
	})
}

// writeData changes the data a document publishes through its api rules tag,
// through its owner's own data write: the whole data the body names, or a sheet
// patch.
func (h *handler) writeData(w http.ResponseWriter, r *http.Request, id string) {
	path, ok := PathFromID(id)
	if !ok {
		refuse(w, http.StatusBadRequest, codeBadRequest, textBadRequest)
		return
	}
	tag, refused := strongIfMatch(w, r)
	if refused {
		return
	}
	body, key, ok := readWriteBody(w, r, dataBody)
	if !ok {
		return
	}
	patch := named(body.Patch)
	write, shape, raw := DataWrite{Data: body.Data, IfMatch: tag}, "data", body.Data
	if patch {
		write, shape, raw = DataWrite{Patch: body.Patch, IfMatch: tag}, "patch", body.Patch
	}
	h.write(w, id+"\x00"+key, fingerprintOf(r.Method, r.URL.Path, tag, shape, string(raw)), func() Answer {
		access, _, err := h.host.Access(r.Context(), path)
		if err != nil {
			return writeRefusal(err)
		}
		result, err := access.WriteData(r.Context(), write)
		if err != nil {
			if answer, ok := dataRefusal(err); ok {
				return answer
			}
			return writeRefusal(err)
		}
		return writtenAnswer(result, key, patch)
	})
}

// write runs one write once per document and operation id, and answers what the
// first run answered.
func (h *handler) write(w http.ResponseWriter, key, fingerprint string, run func() Answer) {
	answer, err := h.replay.Do(key, fingerprint, run)
	if errors.Is(err, ErrOperationReused) {
		answer = refusalAnswer(http.StatusBadRequest, errorBody{Code: codeOperationReused, Message: textOperationReused})
	}
	send(w, answer)
}

func send(w http.ResponseWriter, answer Answer) {
	for name, values := range answer.Header {
		w.Header()[name] = values
	}
	w.WriteHeader(answer.Status)
	_, _ = w.Write(answer.Body)
}

// strongIfMatch is the one strong etag a write names. A missing header answers 428;
// *, a weak tag, a list or two headers answer 400. refused reports that the answer
// was sent.
func strongIfMatch(w http.ResponseWriter, r *http.Request) (tag string, refused bool) {
	values := r.Header.Values("If-Match")
	if len(values) == 0 {
		refuse(w, http.StatusPreconditionRequired, codePreconditionRequired, textPreconditionRequired)
		return "", true
	}
	tag = strings.TrimSpace(values[0])
	if len(values) > 1 || tag == "" || tag == "*" || strings.HasPrefix(tag, "W/") || strings.Contains(tag, ",") {
		refuse(w, http.StatusBadRequest, codeInvalidMatch, textInvalidMatch)
		return "", true
	}
	return tag, false
}

// readWriteBody decodes a write's body and names its operation id: the body's
// operationId, or the Save-ID header, which must agree when both are sent. A save
// must name html, a data write must name data or patch and never both, and a
// restore may send no body at all. ok false means the answer was sent.
func readWriteBody(w http.ResponseWriter, r *http.Request, kind bodyKind) (body writeBody, key string, ok bool) {
	text := textBadBody
	if kind == dataBody {
		text = textBadDataBody
	}
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, saveBodyLimit)).Decode(&body)
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		refuse(w, http.StatusRequestEntityTooLarge, codeTooLarge, textTooLarge)
		return body, "", false
	case errors.Is(err, io.EOF) && kind == restoreBody:
	case err != nil:
		refuse(w, http.StatusBadRequest, codeBadRequest, text)
		return body, "", false
	}
	header := r.Header.Get("Save-ID")
	key = body.OperationID
	if key == "" {
		key = header
	}
	whole := true
	switch kind {
	case saveBody:
		whole = body.HTML != nil
	case dataBody:
		whole = named(body.Data) != named(body.Patch)
	}
	if key == "" || len(key) > maxOperationID || (header != "" && header != key) || !whole {
		refuse(w, http.StatusBadRequest, codeBadRequest, text)
		return body, "", false
	}
	return body, key, true
}

// named reports whether a body named this field with a value: a field left out
// and a JSON null both name nothing.
func named(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

// fingerprintOf is what makes two writes with one operation id the same write.
func fingerprintOf(parts ...string) string {
	sum := sha256.New()
	for _, part := range parts {
		fmt.Fprintf(sum, "%d:%s", len(part), part)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// writtenAnswer is a write's success: the new stamp, whether the bytes moved, the
// operation id, and the version holding the replaced bytes (null when nothing
// was replaced). A sheet patch also answers stale.
func writtenAnswer(result WriteResult, key string, patch bool) Answer {
	var undo any
	if result.UndoVersion != "" {
		undo = result.UndoVersion
	}
	body := map[string]any{
		"etag":          result.ETag,
		"changed":       result.Changed,
		"operationId":   key,
		"undoVersionId": undo,
	}
	if patch {
		body["stale"] = result.Stale
	}
	answer := jsonAnswer(http.StatusOK, body)
	answer.Header.Set("ETag", result.ETag)
	return answer
}

// dataRefusal is a data route's own error as the lane answers it, and false for
// an error every write shares. The data guard's refusal keeps the guard's
// sentence, the stamp to read at and what the write would have removed.
func dataRefusal(err error) (Answer, bool) {
	var required *ReadRequiredError
	var refused *DataError
	switch {
	case errors.As(err, &required):
		answer := refusalAnswer(http.StatusPreconditionRequired, errorBody{
			Code:        codeReadRequired,
			Message:     required.Message,
			ETag:        required.ETag,
			WouldRemove: required.WouldRemove,
		})
		answer.Header.Set("ETag", required.ETag)
		return answer, true
	case errors.Is(err, ErrNoAPIRules):
		return refusalAnswer(http.StatusBadRequest, errorBody{Code: codeNoAPIRules, Message: textNoAPIRules}), true
	case errors.Is(err, ErrPatchUnsupported):
		return refusalAnswer(http.StatusBadRequest, errorBody{Code: codePatchUnsupported, Message: textPatchUnsupported}), true
	case errors.Is(err, ErrTooLarge):
		return refusalAnswer(http.StatusRequestEntityTooLarge, errorBody{Code: codeTooLarge, Message: textDataTooLarge}), true
	case errors.As(err, &refused):
		status, code := refused.Status, refused.Code
		if status < http.StatusBadRequest || status >= http.StatusInternalServerError {
			status = http.StatusBadRequest
		}
		if code == "" {
			code = codeBadData
			if refused.Rules {
				code = codeBadAPIRules
			}
		}
		return refusalAnswer(status, errorBody{Code: code, Message: refused.Message}), true
	}
	return Answer{}, false
}

// writeRefusal is a write's error as the lane answers it.
func writeRefusal(err error) Answer {
	var stale *StaleError
	switch {
	case errors.As(err, &stale):
		answer := refusalAnswer(http.StatusPreconditionFailed, errorBody{Code: codeConflict, Message: textConflict, ETag: stale.ETag, SaveID: stale.SaveID})
		answer.Header.Set("ETag", stale.ETag)
		return answer
	case errors.Is(err, ErrBackup):
		return refusalAnswer(http.StatusServiceUnavailable, errorBody{Code: codeRecoveryUnavailable, Message: textRecoveryUnavailable})
	case errors.Is(err, ErrTooLarge):
		return refusalAnswer(http.StatusRequestEntityTooLarge, errorBody{Code: codeTooLarge, Message: textTooLarge})
	case errors.Is(err, ErrNotDocument):
		return refusalAnswer(http.StatusUnprocessableEntity, errorBody{Code: codeInvalidDocument, Message: textInvalidDocument})
	case errors.Is(err, ErrNotReachable):
		return refusalAnswer(http.StatusNotFound, errorBody{Code: codeNotReachable, Message: textNotReachable})
	case errors.Is(err, ErrNotFound):
		return refusalAnswer(http.StatusNotFound, errorBody{Code: codeNotFound, Message: textNotFound})
	}
	return refusalAnswer(http.StatusInternalServerError, errorBody{Code: codeInternal, Message: textWriteFailed})
}

func refusalAnswer(status int, body errorBody) Answer {
	return jsonAnswer(status, envelope{Error: body})
}

func jsonAnswer(status int, v any) Answer {
	body, _ := json.Marshal(v)
	header := http.Header{}
	header.Set("Content-Type", "application/json; charset=utf-8")
	return Answer{Status: status, Header: header, Body: append(body, '\n')}
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

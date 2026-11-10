//go:build managedtrial

package managedtrial

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"unicode/utf8"
)

// The private control API: the one client Hyperclay Local's own Node code
// speaks to instead of writing a served root itself. Every route answers on the
// control listener G1 already authenticates, takes JSON, and names files the way
// Local does — a root id plus a logical name — so no request ever carries a
// canonical path.

// maxControlBody is the largest request body the API reads: bigger than the
// largest document Local notifies whole, so no single write is refused here.
const maxControlBody = 64 << 20

// controlRoot is one served root as the control API sees it: the id the READY
// line named it, the folder, the one writer, and the event stream it feeds.
type controlRoot struct {
	id       string
	baseDir  string
	registry *consentRegistry
	commit   *commitHost
	events   *eventRing
	watcher  *rootWatcher
}

func newControlRoot(id, baseDir string, registry *consentRegistry, host *commitHost, events *eventRing) *controlRoot {
	if events == nil {
		events = newEventRing()
	}
	if host != nil {
		host.events = events
	}
	return &controlRoot{
		id:       id,
		baseDir:  baseDir,
		registry: registry,
		commit:   host,
		events:   events,
		watcher:  newRootWatcher(id, baseDir, events, guardLiveOf(host)),
	}
}

// controlAPI routes the control listener's /v1 paths to the root they name.
type controlAPI struct {
	roots  []*controlRoot
	byID   map[string]*controlRoot
	events *eventRing
}

func newControlAPI(roots []*controlRoot) *controlAPI {
	api := &controlAPI{roots: roots, byID: make(map[string]*controlRoot, len(roots))}
	for _, root := range roots {
		api.byID[root.id] = root
		if api.events == nil {
			api.events = root.events
		}
	}
	if api.events == nil {
		api.events = newEventRing()
	}
	return api
}

// route is the API's route table: the path and method the request names, or nil
// for a path no route owns.
func (a *controlAPI) route(r *http.Request) http.HandlerFunc {
	switch {
	case r.URL.Path == "/v1/commit" && r.Method == http.MethodPost:
		return a.serveCommit
	case r.URL.Path == "/v1/restructure" && r.Method == http.MethodPost:
		return a.serveRestructure
	case r.URL.Path == "/v1/page-state" && r.Method == http.MethodPost:
		return a.servePageState
	case r.URL.Path == "/v1/read" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		return a.serveRead
	case strings.HasPrefix(r.URL.Path, "/v1/op/") && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		return a.serveOp
	case r.URL.Path == "/v1/events" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		return a.serveEvents
	}
	return nil
}

// root finds the root a request names, or answers why it cannot.
func (a *controlAPI) root(w http.ResponseWriter, id string) (*controlRoot, bool) {
	root, ok := a.byID[id]
	if !ok {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "unknown root: " + id})
		return nil, false
	}
	return root, true
}

// controlBytesBody is one payload: `utf8` when the caller sent a string, and
// `base64` for bytes that are not text.
type controlBytesBody struct {
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
}

// decode is the bytes and whether they came as text.
func (b *controlBytesBody) decode() ([]byte, bool, error) {
	switch b.Encoding {
	case "utf8":
		return []byte(b.Data), true, nil
	case "base64":
		raw, err := base64.StdEncoding.DecodeString(b.Data)
		if err != nil {
			return nil, false, fmt.Errorf("bytes are not valid base64: %w", err)
		}
		return raw, false, nil
	}
	return nil, false, fmt.Errorf("unknown byte encoding: %s", b.Encoding)
}

// controlBytesValue is the other direction: a stored string as utf8, bytes that
// are text as utf8 too, and bytes that are not as base64. Nothing at all is
// null, so a lane that named no bytes answers null rather than an empty payload.
func controlBytesValue(value any) any {
	switch typed := value.(type) {
	case string:
		return orderedObject{{Key: "encoding", Value: "utf8"}, {Key: "data", Value: typed}}
	case []byte:
		if utf8.Valid(typed) && !bytes.ContainsRune(typed, 0) {
			return orderedObject{{Key: "encoding", Value: "utf8"}, {Key: "data", Value: string(typed)}}
		}
		return orderedObject{{Key: "encoding", Value: "base64"}, {Key: "data", Value: base64.StdEncoding.EncodeToString(typed)}}
	}
	return nil
}

// nullableString is Local's `field: null` for the fields a lane leaves empty.
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// frameChecksum is the checksum a commit frame reports: the lane's own when it
// computed one, else the etag it stamped.
func frameChecksum(result commitResult) string {
	if result.HasChecksum && result.Checksum != "" {
		return result.Checksum
	}
	return result.Etag
}

// decodeControlJSON reads one JSON body, answering 400 itself when it cannot.
func decodeControlJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	body := http.MaxBytesReader(w, r.Body, maxControlBody)
	if err := json.NewDecoder(body).Decode(target); err != nil {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "invalid JSON body: " + err.Error()})
		return false
	}
	return true
}

// controlCommitBody is one POST /v1/commit.
type controlCommitBody struct {
	OpID       string            `json:"opId"`
	Root       string            `json:"root"`
	Name       string            `json:"name"`
	Kind       string            `json:"kind"`
	Expected   *controlExpected  `json:"expected"`
	Bytes      *controlBytesBody `json:"bytes"`
	Edit       bool              `json:"edit"`
	Safety     bool              `json:"safety"`
	ModifiedAt *int64            `json:"modifiedAt"`
	PageRecord *PageRecord       `json:"pageRecord"`
	SaveID     string            `json:"saveId"`
	UserDriven *bool             `json:"userDriven"`
	Hooks      *json.RawMessage  `json:"hooks"`
}

// controlExpected is Local's `expected`: the precondition a write is judged
// against under the document's lock.
type controlExpected struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// serveCommit is POST /v1/commit: G6's commit core with the kind the caller
// named, answered with Local's CommitResult and the frame it produced.
func (a *controlAPI) serveCommit(w http.ResponseWriter, r *http.Request) {
	var body controlCommitBody
	if !decodeControlJSON(w, r, &body) {
		return
	}
	root, ok := a.root(w, body.Root)
	if !ok {
		return
	}
	if body.Kind == "checkpoint" {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "checkpoint needs rooms"})
		return
	}
	if _, known := commitKinds[body.Kind]; !known {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "Unknown document write kind: " + body.Kind})
		return
	}
	if body.Hooks != nil {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "hooks ended in M16"})
		return
	}

	request := &commitRequest{
		Kind:       body.Kind,
		Name:       body.Name,
		Edit:       body.Edit,
		Safety:     body.Safety,
		SaveID:     body.SaveID,
		ModifiedAt: body.ModifiedAt,
		PageRecord: body.PageRecord,
	}
	if body.Expected != nil {
		request.Expected = &commitExpectation{Type: body.Expected.Type, Value: body.Expected.Value}
	}
	if body.Bytes != nil {
		bytes, text, err := body.Bytes.decode()
		if err != nil {
			writeControlJSON(w, http.StatusBadRequest, controlError{Error: err.Error()})
			return
		}
		request.Bytes = bytes
		request.Text = text
	}
	if body.UserDriven != nil {
		request.UserDriven = *body.UserDriven
		request.UserDrivenSet = true
	}

	opID := body.OpID
	if opID == "" {
		opID = newOpID()
	}
	request.OpID = opID
	root.events.beginOp(opID)
	result, err := root.commit.commit(request)
	if err != nil {
		root.events.finishOp(opID, "refused", "", "")
		writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
		return
	}

	cursor := result.Cursor
	writeControlJSON(w, http.StatusOK, orderedObject{
		{Key: "outcome", Value: result.Outcome},
		{Key: "reason", Value: nullableString(result.Reason)},
		{Key: "etag", Value: nullableString(result.Etag)},
		{Key: "checksum", Value: nullableString(result.Checksum)},
		{Key: "stored", Value: controlBytesValue(result.Stored)},
		{Key: "content", Value: controlBytesValue(result.Content)},
		{Key: "opId", Value: opID},
		{Key: "cursor", Value: cursor},
	})
}

// controlErrorStatus is the status an error a route could not answer with a
// refusal is reported as.
func controlErrorStatus(err error) int {
	var pathErr *PathError
	if errors.As(err, &pathErr) && pathErr.Status != 0 {
		return pathErr.Status
	}
	return http.StatusBadRequest
}

// controlStructureBody is one POST /v1/restructure.
type controlStructureBody struct {
	OpID  string                 `json:"opId"`
	Root  string                 `json:"root"`
	Kind  string                 `json:"kind"`
	Steps []controlStructureStep `json:"steps"`
}

// controlStructureStep is one logical move: where a name is and where it goes.
type controlStructureStep struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// serveRestructure is POST /v1/restructure: G6b's restructure over the paths the
// steps' names resolve to.
func (a *controlAPI) serveRestructure(w http.ResponseWriter, r *http.Request) {
	var body controlStructureBody
	if !decodeControlJSON(w, r, &body) {
		return
	}
	root, ok := a.root(w, body.Root)
	if !ok {
		return
	}
	entries := make([]structureEntry, 0, len(body.Steps))
	for _, step := range body.Steps {
		fromPath, err := resolveWritePath(root.registry, step.From)
		if err != nil {
			writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
			return
		}
		toPath, err := resolveWritePath(root.registry, step.To)
		if err != nil {
			writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
			return
		}
		isDir := false
		if info, statErr := os.Stat(fromPath); statErr == nil {
			isDir = info.IsDir()
		}
		entries = append(entries, structureEntry{FromPath: fromPath, ToPath: toPath, IsDir: isDir})
	}

	opID := body.OpID
	if opID == "" {
		opID = newOpID()
	}
	cursor := root.events.currentCursor()
	root.events.beginOp(opID)
	result, err := root.commit.restructure(body.Kind, entries)
	if err != nil {
		root.events.finishOp(opID, "refused", "", cursor)
		writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
		return
	}
	state := "committed"
	if result.Outcome != "committed" {
		state = "refused"
	}
	root.events.finishOp(opID, state, "", cursor)
	writeControlJSON(w, http.StatusOK, orderedObject{
		{Key: "outcome", Value: result.Outcome},
		{Key: "completed", Value: result.Completed},
		{Key: "reason", Value: nullableString(result.Reason)},
		{Key: "opId", Value: opID},
		{Key: "cursor", Value: cursor},
	})
}

// controlPageStateBody is one POST /v1/page-state.
type controlPageStateBody struct {
	OpID   string      `json:"opId"`
	Root   string      `json:"root"`
	Op     string      `json:"op"`
	Name   string      `json:"name"`
	To     string      `json:"to"`
	Record *PageRecord `json:"record"`
}

// servePageState is POST /v1/page-state: Local's host pageState over G4b's
// store, answering what each operation returns (nothing, for all five).
func (a *controlAPI) servePageState(w http.ResponseWriter, r *http.Request) {
	var body controlPageStateBody
	if !decodeControlJSON(w, r, &body) {
		return
	}
	root, ok := a.root(w, body.Root)
	if !ok {
		return
	}
	opID := body.OpID
	if opID == "" {
		opID = newOpID()
	}
	root.events.beginOp(opID)
	var err error
	switch body.Op {
	case "write":
		if body.Record == nil {
			err = errors.New("page-state write needs a record")
		} else {
			err = writePageState(root.baseDir, body.Name, body.Record, false)
		}
	case "move":
		err = movePageState(root.baseDir, body.Name, body.To)
	case "moveDir":
		err = movePageStateDir(root.baseDir, body.Name, body.To)
	case "drop":
		err = dropPageState(root.baseDir, body.Name)
	case "dropDir":
		err = dropPageStateDir(root.baseDir, body.Name)
	default:
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "Unknown page-state operation: " + body.Op})
		return
	}
	if err != nil {
		root.events.finishOp(opID, "refused", "", "")
		writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
		return
	}
	root.events.finishOp(opID, "committed", "", "")
	writeControlJSON(w, http.StatusOK, nil)
}

// serveRead is GET /v1/read: Local's `read`, which takes no lock.
func (a *controlAPI) serveRead(w http.ResponseWriter, r *http.Request) {
	root, ok := a.root(w, r.URL.Query().Get("root"))
	if !ok {
		return
	}
	filePath, err := resolveWritePath(root.registry, r.URL.Query().Get("name"))
	if err != nil {
		writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
		return
	}
	state, err := readFile(filePath)
	if err != nil {
		writeControlJSON(w, http.StatusInternalServerError, controlError{Error: err.Error()})
		return
	}
	answer := orderedObject{{Key: "exists", Value: state != nil}}
	if state == nil {
		answer = append(answer, kv{Key: "checksum", Value: nil}, kv{Key: "bytes", Value: nil})
	} else {
		checksum := state.Checksum
		answer = append(answer,
			kv{Key: "checksum", Value: checksum},
			kv{Key: "bytes", Value: controlBytesValue(state.Bytes)},
		)
	}
	writeControlJSON(w, http.StatusOK, answer)
}

// serveOp is GET /v1/op/<opId>: what became of one request.
func (a *controlAPI) serveOp(w http.ResponseWriter, r *http.Request) {
	opID := strings.TrimPrefix(r.URL.Path, "/v1/op/")
	record, ok := a.events.lookupOp(opID)
	if !ok {
		writeControlJSON(w, http.StatusOK, orderedObject{{Key: "state", Value: "unknown"}})
		return
	}
	answer := orderedObject{{Key: "state", Value: record.state}}
	if record.checksum != "" {
		answer = append(answer, kv{Key: "checksum", Value: record.checksum})
	}
	if record.cursor != "" {
		answer = append(answer, kv{Key: "cursor", Value: record.cursor})
	}
	writeControlJSON(w, http.StatusOK, answer)
}

// serveEvents is GET /v1/events: the process's commit and watcher frames.
func (a *controlAPI) serveEvents(w http.ResponseWriter, r *http.Request) {
	a.events.serveEvents(w, r)
}

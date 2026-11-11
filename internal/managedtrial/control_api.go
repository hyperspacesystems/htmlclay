//go:build managedtrial

package managedtrial

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
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
	watcher := newRootWatcher(id, baseDir, events, guardLiveOf(host))
	if host != nil {
		watcher.observe = host.observeOutside
	}
	return &controlRoot{
		id:       id,
		baseDir:  baseDir,
		registry: registry,
		commit:   host,
		events:   events,
		watcher:  watcher,
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
	case r.URL.Path == "/v1/checkpoint" && r.Method == http.MethodPost:
		return a.serveCheckpoint
	case r.URL.Path == "/v1/artifact" && r.Method == http.MethodPost:
		return a.serveArtifact
	case r.URL.Path == "/v1/restructure" && r.Method == http.MethodPost:
		return a.serveRestructure
	case r.URL.Path == "/v1/blockeditor-state" && r.Method == http.MethodPost:
		return a.servePageState
	case r.URL.Path == "/v1/snapshot/take" && r.Method == http.MethodPost:
		return a.serveSnapshotTake
	case r.URL.Path == "/v1/read" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		return a.serveRead
	case r.URL.Path == "/v1/durable" && r.Method == http.MethodPost:
		return a.serveDurable
	case r.URL.Path == "/v1/fault" && r.Method == http.MethodPost:
		return a.serveFault
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

// controlOpID is the shape a caller's opId has: 16 to 64 lowercase hex.
var controlOpID = regexp.MustCompile("^[0-9a-f]{16,64}$")

// claimControlOp is the opId rule every mutating control route shares: none
// given means Go makes one; one given must be 16 to 64 lowercase hex and new to
// this process. It answers the refusal itself and reports false.
func claimControlOp(w http.ResponseWriter, events *eventRing, given string) (string, bool) {
	if given == "" {
		opID := newOpID()
		events.beginOp(opID)
		return opID, true
	}
	if !controlOpID.MatchString(given) {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "invalid opId"})
		return "", false
	}
	if !events.claimOp(given) {
		writeControlJSON(w, http.StatusConflict, controlError{Error: "op-reused"})
		return "", false
	}
	return given, true
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
	PageRecord *PageRecord       `json:"blockeditorRecord"`
	SaveID     string            `json:"saveId"`
	UserDriven *bool             `json:"userDriven"`
	Hooks      *json.RawMessage  `json:"hooks"`
	OpHandle   *string           `json:"opHandle"`
	Headers    map[string]string `json:"headers"`
	Origin     string            `json:"origin"`
	CaptureID  *string           `json:"captureId"`
}

// refusalValue is a decided refusal as the control API reports it, or null.
func refusalValue(refusal *serviceRefusal) any {
	if refusal == nil {
		return nil
	}
	var body any = json.RawMessage(refusal.Body)
	if len(refusal.Body) == 0 {
		body = nil
	}
	return orderedObject{
		{Key: "status", Value: refusal.Status},
		{Key: "code", Value: refusal.Code},
		{Key: "body", Value: body},
		{Key: "headers", Value: refusal.Headers},
	}
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
		Kind:          body.Kind,
		Name:          body.Name,
		Edit:          body.Edit,
		Safety:        body.Safety,
		SaveID:        body.SaveID,
		ModifiedAt:    body.ModifiedAt,
		PageRecord:    body.PageRecord,
		OpHandle:      body.OpHandle,
		Headers:       body.Headers,
		ConsultOrigin: body.Origin,
		CaptureID:     body.CaptureID,
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

	opID, ok := claimControlOp(w, root.events, body.OpID)
	if !ok {
		return
	}
	request.OpID = opID
	result, err := root.commit.commit(request)
	if err != nil {
		root.events.finishOp(opID, "refused", "", "")
		writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
		return
	}

	cursor := result.Cursor
	answer := orderedObject{
		{Key: "outcome", Value: result.Outcome},
		{Key: "reason", Value: nullableString(result.Reason)},
		{Key: "refusal", Value: refusalValue(result.Refusal)},
		{Key: "etag", Value: nullableString(result.Etag)},
		{Key: "checksum", Value: nullableString(result.Checksum)},
		{Key: "stored", Value: controlBytesValue(result.Stored)},
		{Key: "content", Value: controlBytesValue(result.Content)},
		{Key: "opId", Value: opID},
		{Key: "cursor", Value: cursor},
	}
	encoded, err := json.Marshal(answer)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	root.events.recordOpAnswer(opID, encoded)

	if takeFault(faultDropReply, body.Name) {
		dropControlReply(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
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
	OpID     string                 `json:"opId"`
	Root     string                 `json:"root"`
	Kind     string                 `json:"kind"`
	Steps    []controlStructureStep `json:"steps"`
	OpHandle *string                `json:"opHandle"`
}

// controlStructureStep is one logical move: where a name is and where it goes,
// plus the blockeditor-state work the step carries.
type controlStructureStep struct {
	From      string `json:"from"`
	To        string `json:"to"`
	PageState string `json:"blockeditorState"`
}

// validateStructureSteps is the shape check every restructure makes before any
// op is begun: the workspace kinds carry their own step shapes, and a blockeditor-state
// move or drop is only what its kind can carry.
func validateStructureSteps(kind string, steps []controlStructureStep) string {
	switch kind {
	case "empty-trash":
		if len(steps) != 0 {
			return "empty-trash takes no steps"
		}
		return ""
	case "folder":
		for _, step := range steps {
			if step.From != "" || step.To == "" {
				return "folder steps carry only a destination"
			}
			if step.PageState != "" {
				return "folder steps carry no page state"
			}
		}
		return ""
	}
	for _, step := range steps {
		if step.From == "" || step.To == "" {
			return "structure steps need from and to"
		}
		switch step.PageState {
		case "":
		case "move":
			if kind != "move" && kind != "sync-move" {
				return "blockeditorState move needs kind move or sync-move"
			}
		case "drop":
			if kind != "trash" && kind != "sync-trash" {
				return "blockeditorState drop needs kind trash or sync-trash"
			}
		default:
			return "Unknown blockeditorState: " + step.PageState
		}
	}
	return ""
}

// structurePageStateValue is the answer's blockeditorState: nothing when the page state
// was carried, else the failure's code as `reason` reports it.
func structurePageStateValue(err error) any {
	if err == nil {
		return nil
	}
	if code := structureErrorCode(err); code != "" && code != "failed" {
		return code
	}
	return err.Error()
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
	if message := validateStructureSteps(body.Kind, body.Steps); message != "" {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: message})
		return
	}
	entries := make([]structureEntry, 0, len(body.Steps))
	for _, step := range body.Steps {
		fromPath := ""
		if step.From != "" {
			resolved, err := resolveWritePath(root.registry, step.From)
			if err != nil {
				writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
				return
			}
			fromPath = resolved
		}
		toPath, err := resolveWritePath(root.registry, step.To)
		if err != nil {
			writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
			return
		}
		isDir := false
		if fromPath != "" {
			if info, statErr := os.Stat(fromPath); statErr == nil {
				isDir = info.IsDir()
			}
		}
		entries = append(entries, structureEntry{
			FromPath:  fromPath,
			ToPath:    toPath,
			IsDir:     isDir,
			FromName:  step.From,
			ToName:    step.To,
			PageState: step.PageState,
		})
	}

	cursor := root.events.currentCursor()
	opID, ok := claimControlOp(w, root.events, body.OpID)
	if !ok {
		return
	}
	result, err := root.commit.restructureOp(body.Kind, entries, opID, body.OpHandle)
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
		{Key: "refusal", Value: refusalValue(result.Refusal)},
		{Key: "blockeditorState", Value: structurePageStateValue(result.PageStateErr)},
		{Key: "opId", Value: opID},
		{Key: "cursor", Value: cursor},
	})
}

// controlPageStateBody is one POST /v1/blockeditor-state.
type controlPageStateBody struct {
	OpID   string      `json:"opId"`
	Root   string      `json:"root"`
	Op     string      `json:"op"`
	Name   string      `json:"name"`
	To     string      `json:"to"`
	Record *PageRecord `json:"record"`
}

// servePageState is POST /v1/blockeditor-state: Local's host blockeditorState over G4b's
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
	var unlock func()
	switch body.Op {
	case "write":
		filePath, resolveErr := resolveWritePath(root.registry, body.Name)
		if resolveErr != nil {
			writeControlJSON(w, controlErrorStatus(resolveErr), controlError{Error: resolveErr.Error()})
			return
		}
		unlock = fileLocks.lock(filePath)
	case "move", "moveDir", "drop", "dropDir":
		unlock = fileLocks.lockStructure()
	default:
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "Unknown blockeditor-state operation: " + body.Op})
		return
	}
	defer unlock()

	opID, ok := claimControlOp(w, root.events, body.OpID)
	if !ok {
		return
	}
	var err error
	switch body.Op {
	case "write":
		if body.Record == nil {
			err = errors.New("blockeditor-state write needs a record")
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
	}
	if err != nil {
		root.events.finishOp(opID, "refused", "", "")
		writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
		return
	}
	root.events.finishOp(opID, "committed", "", "")
	writeControlJSON(w, http.StatusOK, nil)
}

// controlSnapshotTakeBody is one POST /v1/snapshot/take.
type controlSnapshotTakeBody struct {
	Root string `json:"root"`
	Name string `json:"name"`
}

// serveSnapshotTake is POST /v1/snapshot/take: Local's getAndClearSnapshot,
// the one read the sync engine makes before it uploads a page. The answer
// carries the working snapshot the relay cached and the provenance bit the last
// save left, and the entry is gone afterwards whether or not the caller uses it.
func (a *controlAPI) serveSnapshotTake(w http.ResponseWriter, r *http.Request) {
	var body controlSnapshotTakeBody
	if !decodeControlJSON(w, r, &body) {
		return
	}
	root, ok := a.root(w, body.Root)
	if !ok {
		return
	}
	entry, found := snapshotStoreFor(root.id).getAndClearSnapshot(body.Name)
	if !found {
		writeControlJSON(w, http.StatusOK, orderedObject{{Key: "snapshot", Value: nil}})
		return
	}
	snapshot := orderedObject{{Key: "html", Value: nil}}
	if entry.HTML != nil {
		snapshot[0].Value = *entry.HTML
	}
	if entry.UserDrivenSet {
		snapshot = append(snapshot, kv{Key: "userDriven", Value: entry.UserDriven})
	}
	writeControlJSON(w, http.StatusOK, orderedObject{{Key: "snapshot", Value: snapshot}})
}

// serveRead is GET /v1/read: one coherent snapshot of a document, taken under
// the document's lock, so the bytes and the page state they were read with are
// never a pair from two different saves.
func (a *controlAPI) serveRead(w http.ResponseWriter, r *http.Request) {
	root, ok := a.root(w, r.URL.Query().Get("root"))
	if !ok {
		return
	}
	name := r.URL.Query().Get("name")
	filePath, err := resolveWritePath(root.registry, name)
	if err != nil {
		writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
		return
	}
	unlock := fileLocks.lock(filePath)
	defer unlock()
	answer, err := readSnapshot(root.baseDir, name, filePath)
	if err != nil {
		writeControlJSON(w, http.StatusInternalServerError, controlError{Error: err.Error()})
		return
	}
	writeControlJSON(w, http.StatusOK, answer)
}

// readSnapshot is one document as GET /v1/read and every consultation send it,
// read by a caller that holds the document's lock: the native exists,
// checksum and bytes, then fileSha256, identity, and for a UTF-8 page document
// its logical form and its stored blockeditor-state record exactly as the sidecar
// holds it (rev, tag, generation, prev), because Local's blockeditor-state readers
// resolve revisions and lineage from the whole record.
func readSnapshot(baseDir, name, filePath string) (orderedObject, error) {
	state, err := readFile(filePath)
	if err != nil {
		return nil, err
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
	identity, err := identityOfPath(filePath)
	if err != nil {
		return nil, err
	}
	var fileSha256 any
	var identityValue any
	var logical any
	var sidecar any
	if state != nil {
		sum := sha256.Sum256(state.Bytes)
		fileSha256 = hex.EncodeToString(sum[:])
	}
	if identity != nil {
		identityValue = orderedObject{
			{Key: "dev", Value: identity.Dev},
			{Key: "ino", Value: identity.Ino},
			{Key: "size", Value: identity.Size},
			{Key: "mtimeNs", Value: identity.MtimeNs},
		}
	}
	if state != nil && pageExtension.MatchString(name) && utf8.Valid(state.Bytes) {
		logicalText, err := logicalDocument(baseDir, name, string(state.Bytes))
		if err != nil {
			return nil, err
		}
		logical = controlBytesValue(logicalText)
		stored, err := readStored(baseDir, name)
		if err != nil {
			return nil, err
		}
		if stored != nil {
			sidecar = stored
		}
	}
	answer = append(answer,
		kv{Key: "fileSha256", Value: fileSha256},
		kv{Key: "identity", Value: identityValue},
		kv{Key: "logical", Value: logical},
		kv{Key: "sidecar", Value: sidecar},
	)
	return answer, nil
}

// controlDurableBody is one POST /v1/durable.
type controlDurableBody struct {
	Root     string `json:"root"`
	Name     string `json:"name"`
	Checksum string `json:"checksum"`
}

// syncFile makes one file's bytes durable.
func syncFile(filePath string) error {
	flag := os.O_RDONLY
	if runtime.GOOS == "windows" {
		flag = os.O_RDWR
	}
	handle, err := os.OpenFile(filePath, flag, 0)
	if err != nil {
		return err
	}
	err = handle.Sync()
	if closeErr := handle.Close(); err == nil {
		err = closeErr
	}
	return err
}

// syncDurableDir syncs one directory, ignoring the failure of a platform that
// cannot sync a directory at all, as atomicWriteFile does.
func syncDurableDir(dir string) error {
	if err := atomicDirSync(dir); err != nil && !dirSyncUnsupported(err, runtime.GOOS) {
		return err
	}
	return nil
}

// controlFaultBody is one POST /v1/fault: the fault point a harness arms, the
// logical path it fires for, and how many writes take it.
type controlFaultBody struct {
	Point string `json:"point"`
	Rel   string `json:"rel"`
	Count *int   `json:"count"`
}

// controlFaultMaxCount bounds how many writes one arming covers.
const controlFaultMaxCount = 100

// serveFault is POST /v1/fault: a harness arms one of the trial's fault points,
// or clears them all.
func (a *controlAPI) serveFault(w http.ResponseWriter, r *http.Request) {
	var body controlFaultBody
	if !decodeControlJSON(w, r, &body) {
		return
	}
	if body.Point == "clear" {
		clearFaults()
		writeControlJSON(w, http.StatusOK, orderedObject{{Key: "armed", Value: 0}})
		return
	}
	if body.Point == "release" {
		writeControlJSON(w, http.StatusOK, orderedObject{{Key: "released", Value: releaseTailwind(body.Rel)}})
		return
	}
	if body.Point == "held" {
		writeControlJSON(w, http.StatusOK, orderedObject{{Key: "held", Value: heldTailwind(body.Rel)}})
		return
	}
	if body.Point == "remaining" {
		writeControlJSON(w, http.StatusOK, orderedObject{{Key: "remaining", Value: remainingFaults(body.Rel)}})
		return
	}
	if body.Point == "finished" {
		writeControlJSON(w, http.StatusOK, orderedObject{{Key: "finished", Value: heldTailwindFinished(body.Rel)}})
		return
	}
	if !faultPoints[body.Point] {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "unknown fault point: " + body.Point})
		return
	}
	if body.Rel == "" {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "fault needs a rel"})
		return
	}
	count := 1
	if body.Count != nil {
		count = *body.Count
	}
	if count < 1 || count > controlFaultMaxCount {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "fault count out of range"})
		return
	}
	writeControlJSON(w, http.StatusOK, orderedObject{{Key: "armed", Value: armFault(body.Point, body.Rel, count)}})
}

// serveDurable is POST /v1/durable: the proof a checkpoint's bytes are durable
// after an indeterminate write. Under the document's lock, a file that still
// holds the bytes the caller names has it, its sidecar and their directories
// synced.
func (a *controlAPI) serveDurable(w http.ResponseWriter, r *http.Request) {
	var body controlDurableBody
	if !decodeControlJSON(w, r, &body) {
		return
	}
	root, ok := a.root(w, body.Root)
	if !ok {
		return
	}
	filePath, err := resolveWritePath(root.registry, body.Name)
	if err != nil {
		writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
		return
	}
	unlock := fileLocks.lock(filePath)
	defer unlock()
	state, err := readFile(filePath)
	if err != nil {
		writeControlJSON(w, http.StatusInternalServerError, controlError{Error: err.Error()})
		return
	}
	if state == nil || state.Checksum != body.Checksum {
		var checksum any
		if state != nil {
			checksum = state.Checksum
		}
		writeControlJSON(w, http.StatusOK, orderedObject{
			{Key: "changed", Value: true},
			{Key: "checksum", Value: checksum},
		})
		return
	}
	if err := syncFile(filePath); err != nil {
		writeControlJSON(w, http.StatusInternalServerError, controlError{Error: err.Error()})
		return
	}
	sidecarDir := ""
	if pageExtension.MatchString(body.Name) {
		target, err := pageStatePath(root.baseDir, body.Name)
		if err != nil {
			writeControlJSON(w, http.StatusInternalServerError, controlError{Error: err.Error()})
			return
		}
		if pageStateExists(target) {
			if err := syncFile(target); err != nil {
				writeControlJSON(w, http.StatusInternalServerError, controlError{Error: err.Error()})
				return
			}
			sidecarDir = filepath.Dir(target)
		}
	}
	// The durable-fail fault is a disk that cannot sync this document's
	// directory: the proof fails as long as it is armed, as a real one would.
	if takeFault(faultDurableFail, filePath) {
		writeControlJSON(w, http.StatusInternalServerError, controlError{Error: "fsync " + filepath.Dir(filePath) + ": input/output error"})
		return
	}
	if err := syncDurableDir(filepath.Dir(filePath)); err != nil {
		writeControlJSON(w, http.StatusInternalServerError, controlError{Error: err.Error()})
		return
	}
	if sidecarDir != "" {
		if err := syncDurableDir(sidecarDir); err != nil {
			writeControlJSON(w, http.StatusInternalServerError, controlError{Error: err.Error()})
			return
		}
	}
	writeControlJSON(w, http.StatusOK, orderedObject{
		{Key: "durable", Value: true},
		{Key: "checksum", Value: state.Checksum},
	})
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
	if len(record.answer) > 0 {
		answer = append(answer, kv{Key: "answer", Value: json.RawMessage(record.answer)})
	}
	writeControlJSON(w, http.StatusOK, answer)
}

// serveEvents is GET /v1/events: the process's commit and watcher frames.
func (a *controlAPI) serveEvents(w http.ResponseWriter, r *http.Request) {
	a.events.serveEvents(w, r)
}

//go:build managedtrial

package managedtrial

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/clayruntime"
	"github.com/panphora/htmlclay/internal/specwire"
)

// The managed trial host's commit core, ported from Local's
// src/main/document-host.js. One Go function per Local write lane, so the
// bytes, versions, blockeditor-state sidecars and API sidecars left on disk are what
// Hyperclay Local leaves, and no save is ever half-applied or falsely reported.
//
// The native stage runs with no local authority, so every branch that needs a
// room, a confirmed write target or a token is left out below; the full-host
// stage adds it.

// maxExternalHTML is Local's MAX_EXTERNAL_HTML: the largest document a data API
// notify carries whole.
const maxExternalHTML = 12 * 1024 * 1024

// loopbackRuntime is Local's LOOPBACK_RUNTIME: the loopback origin a runtime URL
// in the document is named against. Go's regexp has no lookahead, so the
// `/_/clayjs/` the origin is matched against is part of the match and trimmed.
var loopbackRuntime = regexp.MustCompile(`(?i)\bhttps?://(?:localhost|127\.0\.0\.1)(?::\d{1,5})?/_/clayjs/`)

// commitLane is one lane of Local's KINDS table.
type commitLane struct {
	lane   string
	origin string
}

// commitKinds is Local's KINDS: which lane a request kind runs on, and what the
// bytes of that lane are called when they reach the receipts and the sidecar.
var commitKinds = map[string]commitLane{
	"browser-save":    {lane: "save", origin: "save"},
	"data-api":        {lane: "save", origin: "api"},
	"restore":         {lane: "replace"},
	"guard-restore":   {lane: "replace"},
	"sync-download":   {lane: "exact"},
	"external-rebase": {lane: "external-rebase"},
	"workspace":       {lane: ""},
	"create":          {lane: "create"},
}

// commitRequest mirrors Local's request object. There is no filePath: the host
// resolves the canonical path itself.
type commitRequest struct {
	Kind       string
	Name       string
	Bytes      []byte
	Text       bool
	Edit       bool
	Safety     bool
	UserDriven bool
	// UserDrivenSet says the request expressed a provenance at all: Local's
	// `userDriven` is a tri-state, and a save that says nothing is ui-unknown.
	UserDrivenSet bool
	SaveID        string
	DataLossPrev  *string
	Expected      *commitExpectation
	ModifiedAt    *int64
	PageRecord    *PageRecord
	Hooks         bool
	// OpID names the write in the event stream and in GET /v1/op. The control
	// API passes the caller's; a write that arrives over HTTP gets a new one.
	OpID string
	// OpHandle is the opId the JS service sent with a /v1/commit, echoed in
	// this write's consultation so the service finds its own state by it.
	OpHandle *string
	// Headers are the request headers Local's lanes read (lower-case names),
	// sent with admit.
	Headers map[string]string
	// ConsultOrigin names the write's origin to the service when it is not the
	// lane's own (a checkpoint).
	ConsultOrigin string
	// CaptureID is the checkpoint capture a write stores, marked saved on settle.
	CaptureID *string
	// BeforeRename runs immediately before the document's rename; an error
	// leaves the document as it was. A checkpoint checks there that the file is
	// still the one it read.
	BeforeRename func() error
}

// commitExpectation is Local's `expected`: the precondition a request is judged
// against under the document's lock.
type commitExpectation struct {
	Type  string
	Value string
}

// commitResult mirrors Local's answer. HasChecksum and HasFile say which lane
// answered, because the lanes do not all name the same fields.
type commitResult struct {
	Outcome     string
	Reason      string
	Etag        string
	Checksum    string
	Stored      any
	Content     any
	File        string
	HasChecksum bool
	HasFile     bool
	// Cursor is the event stream position of the frame that reported this write.
	Cursor  string
	Refusal *serviceRefusal // a refusal answered to the client as it was decided
}

// errCheckpointTargetChanged is Local's `moved-or-deleted` from a checkpoint's
// identity check: the file is not the one the checkpoint read, so nothing
// replaced it.
var errCheckpointTargetChanged = errors.New("Checkpoint target changed since it was read")

// refused is Local's refusal shape.
func refused(reason string) commitResult {
	return commitResult{Outcome: "refused", Reason: reason}
}

// indeterminateError marks a write whose bytes landed but whose disk record may
// not survive: reported as indeterminate, never as committed.
type indeterminateError struct {
	err  error
	code string
}

func (e *indeterminateError) Error() string { return e.err.Error() }

func (e *indeterminateError) Unwrap() error { return e.err }

// errorCode is the JS-style code Local reports as the indeterminate reason.
func errorCode(err error) string {
	var afterRename *AfterRenameError
	if errors.As(err, &afterRename) {
		return errnoCode(afterRename.Err)
	}
	var indeterminate *indeterminateError
	if errors.As(err, &indeterminate) {
		return indeterminate.code
	}
	return ""
}

// errnoCode names the failures a post-rename directory sync reports, the way
// Node's error.code does; anything unrecognised is Local's `after-rename`.
func errnoCode(err error) string {
	var errno interface{ Error() string }
	if !errors.As(err, &errno) {
		return "after-rename"
	}
	switch err.Error() {
	case "input/output error":
		return "EIO"
	case "no space left on device":
		return "ENOSPC"
	case "permission denied":
		return "EACCES"
	case "operation not permitted":
		return "EPERM"
	case "read-only file system":
		return "EROFS"
	}
	return "after-rename"
}

// commitHost is the one writer of a root's documents.
type commitHost struct {
	baseDir string
	rootID  string
	// events is the stream every write this core makes is reported on, from any
	// route or the control API. newControlRoot attaches it.
	events   *eventRing
	registry *consentRegistry
	locks    *filePathLocks
	hooks    commitHooks
	store    *snapshotStore
	releases map[string]clayruntime.Release
	// service is the JS service this root's writes consult, or nil.
	service *serviceLink
}

// newCommitHost builds the commit core for one served folder.
func newCommitHost(baseDir, rootID string, registry *consentRegistry, hooks commitHooks, releases map[string]clayruntime.Release) *commitHost {
	if hooks == nil {
		hooks = noopCommitHooks{}
	}
	return &commitHost{
		baseDir:  baseDir,
		rootID:   rootID,
		registry: registry,
		locks:    fileLocks,
		hooks:    hooks,
		store:    snapshotStoreFor(rootID),
		releases: releases,
	}
}

// commit is Local's dispatcher: an unknown kind or a `hooks` request is a
// contract error, the precondition is judged under the document's lock, and a
// write whose directory entry may not have survived is indeterminate.
func (h *commitHost) commit(request *commitRequest) (commitResult, error) {
	if request == nil {
		return commitResult{}, errors.New("Unknown document write kind: <nil>")
	}
	policy, ok := commitKinds[request.Kind]
	if !ok {
		return commitResult{}, fmt.Errorf("Unknown document write kind: %s", request.Kind)
	}
	if request.Hooks {
		return commitResult{}, errors.New("hooks ended in M16")
	}
	filePath, err := h.resolveWrite(request.Name)
	if err != nil {
		return commitResult{}, err
	}

	unlock := h.locks.lock(filePath)
	defer unlock()

	result, err := h.commitLocked(request, policy, filePath)
	if err != nil {
		if code := errorCode(err); code != "" {
			result := commitResult{Outcome: "indeterminate", Reason: code}
			h.report(request, &result)
			return result, nil
		}
		return commitResult{}, err
	}
	return result, nil
}

// commitLocked is the dispatcher's work for a caller that already holds the
// document's lock: the precondition is judged against the bytes on disk, then
// the lane runs. The /_/save route calls it directly, because its own If-Match
// judgement has to happen inside the same lock, before anything is written.
func (h *commitHost) commitLocked(request *commitRequest, policy commitLane, filePath string) (commitResult, error) {
	changed, err := h.preconditionFailed(request, filePath)
	if err != nil {
		return commitResult{}, err
	}
	if changed {
		result := refused("changed")
		h.report(request, &result)
		return result, nil
	}

	result, err := h.runLane(request, policy, filePath)
	if err != nil {
		return commitResult{}, err
	}
	// The bytes the guard must treat as the last ones seen.
	if result.Outcome == "committed" {
		h.hooks.NoteSeen(h.baseDir, request.Name, result.Stored)
	}
	h.report(request, &result)
	return result, nil
}

// report appends the commit frame for one write and keeps its opId's outcome,
// so the event stream and GET /v1/op see every write this host made: a browser
// save, a version or guard restore, and a control API commit alike.
func (h *commitHost) report(request *commitRequest, result *commitResult) {
	if h.events == nil {
		return
	}
	opID := request.OpID
	if opID == "" {
		opID = newOpID()
	}
	result.Cursor = h.events.recordCommit(opID, h.rootID, request.Kind, request.Name, result.Outcome, frameChecksum(*result))
}

// runLane is Local's LANES table.
func (h *commitHost) runLane(request *commitRequest, policy commitLane, filePath string) (commitResult, error) {
	switch policy.lane {
	case "save":
		return h.commitSave(request, policy, filePath)
	case "exact":
		return h.commitExact(request, filePath)
	case "create":
		return h.commitCreate(request, filePath)
	case "replace":
		return h.commitReplace(request, filePath)
	case "external-rebase":
		return h.commitRebase(request, filePath)
	}
	return commitResult{}, fmt.Errorf("Document write kind %s is not migrated yet", request.Kind)
}

// resolveWrite is G3's canonical write resolution for a request's name.
func (h *commitHost) resolveWrite(name string) (string, error) {
	return resolveWritePath(h.registry, name)
}

// readFileState is one document as the preconditions see it: its bytes and
// Local's checksum.
type readFileState struct {
	Bytes    []byte
	Checksum string
}

// readFile is Local's `read`: nil when the file is not there.
func readFile(filePath string) (*readFileState, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return &readFileState{Bytes: bytes, Checksum: specwire.Etag(bytes)}, nil
}

// preconditionFailed is Local's `expected` judgement, made under the document's
// lock so two saves of one file cannot both pass it.
func (h *commitHost) preconditionFailed(request *commitRequest, filePath string) (bool, error) {
	expected := request.Expected
	if expected == nil || expected.Type == "any" {
		return false, nil
	}
	current, err := readFile(filePath)
	if err != nil {
		return false, err
	}
	switch expected.Type {
	case "absent":
		return current != nil, nil
	case "checksum":
		return current == nil || current.Checksum != expected.Value, nil
	}
	return false, fmt.Errorf("Unknown expected type: %s", expected.Type)
}

// splitWrite is Local's splitWrite: the file part that reaches disk and the
// logical document everything else describes.
func (h *commitHost) splitWrite(name, html string, keepState bool) (string, string, error) {
	file, state := splitPage(html)
	if state != nil {
		return file, composePage(file, state), nil
	}
	if !keepState {
		return file, file, nil
	}
	logical, err := logicalDocument(h.baseDir, name, file)
	if err != nil {
		return "", "", err
	}
	return file, logical, nil
}

// canonicalForWrite is Local's canonicalForWrite: a loopback runtime URL becomes
// the public one and the descriptor it was projected with is dropped, so bytes
// written here stay portable. AD1: no formatHtml, and no scopeTailwindLink.
func (h *commitHost) canonicalForWrite(html string) string {
	documentURL := "http://localhost/"
	if origin := loopbackRuntime.FindString(html); origin != "" {
		documentURL = strings.ToLower(strings.TrimSuffix(origin, "/_/clayjs/")) + "/"
	}
	result := clayruntime.CanonicalizeRuntimeHTML([]byte(html), h.releases, documentURL)
	if result.Changed == 0 && !result.DescriptorRemoved {
		return html
	}
	return result.HTML
}

// placeBytes is Local's placeBytes: the sidecar lands before the file, and only
// the file part reaches disk.
func (h *commitHost) placeBytes(name, filePath, logical string, beforeRename func() error) (string, error) {
	stored, err := storePageState(h.baseDir, name, logical, true)
	if err != nil {
		return "", err
	}
	h.hooks.MarkPending(h.baseDir, name, stored)
	if err := atomicWriteFile(filePath, []byte(stored), beforeRename, true); err != nil {
		return "", err
	}
	h.hooks.MarkBrowserSave(name)
	return stored, nil
}

// observer is the version store's window onto the hooks, for the lanes that
// hand the backup writer an emit (Local passes one only where it does).
func (h *commitHost) observer() *VersionObserver {
	return &VersionObserver{Emit: func(event string, fields []VersionField) {
		payload := make(map[string]any, len(fields))
		for _, field := range fields {
			payload[field.Key] = field.Value
		}
		h.hooks.Emit(event, payload)
	}}
}

// backupSiteName is Local's `backupName`: the document's name without its
// extension, which is the versions directory's layout.
func backupSiteName(name string) string {
	return pageExtension.ReplaceAllString(name, "")
}

// isDocumentName is Local's DOCUMENT_NAME test.
func isDocumentName(name string) bool {
	return pageExtension.MatchString(name)
}

// mtimeNs is a file's mtime in nanoseconds, or nil when it cannot be read.
func mtimeNs(filePath string) *int64 {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil
	}
	nanos := info.ModTime().UnixNano()
	return &nanos
}

// modifiedTime turns the request's epoch milliseconds into a time.
func modifiedTime(ms int64) time.Time {
	return time.UnixMilli(ms)
}

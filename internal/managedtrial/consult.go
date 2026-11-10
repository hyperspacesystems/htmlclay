//go:build managedtrial

package managedtrial

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// maxPendingBatches bounds one call's artifact loop, so a service that keeps
// answering pending cannot hold a document's lock past this many batches.
const maxPendingBatches = 16

var artifactIDPattern = regexp.MustCompile("^[0-9a-f]{16}$")

// serviceAnswer is every answer a consultation call can get: a pending batch,
// an admit decision, or a settle result.
type serviceAnswer struct {
	Pending     bool              `json:"pending"`
	Artifacts   []serviceArtifact `json:"artifacts"`
	Decision    string            `json:"decision"`
	Logical     *controlBytesBody `json:"logical"`
	File        *controlBytesBody `json:"file"`
	Reservation *string           `json:"reservation"`
	Status      int               `json:"status"`
	Code        string            `json:"code"`
	Body        json.RawMessage   `json:"body"`
	Headers     map[string]string `json:"headers"`
	OK          *bool             `json:"ok"`
	Error       string            `json:"error"`
	Kind        string            `json:"kind"`
	Revision    json.RawMessage   `json:"revision"`
	CaptureID   *string           `json:"captureId"`
	Reason      string            `json:"reason"`
}

// serviceArtifact is one file the service needs written before its handler
// can go on: a safety version of the consulted document, or a recovered copy
// in any served root.
type serviceArtifact struct {
	ArtifactID string           `json:"artifactId"`
	Type       string           `json:"type"`
	Root       string           `json:"root"`
	Name       string           `json:"name"`
	Logical    controlBytesBody `json:"logical"`
}

// artifactReceipt is what Go wrote for one artifact, or why it could not.
type artifactReceipt struct {
	ArtifactID string  `json:"artifactId"`
	Type       string  `json:"type"`
	Root       *string `json:"root"`
	Name       *string `json:"name"`
	Version    *string `json:"version"`
	Sha256     *string `json:"sha256"`
	Error      *string `json:"error"`
}

// consultation is one exchange with the JS service about one write, made while
// this host holds the document's lock: admit, any artifact batches, settle.
type consultation struct {
	link          *serviceLink
	ID            string
	OpID          string
	OpHandle      *string
	Root          string
	Name          string
	CanonicalPath string
	Lane          string
	Origin        string
	Reservation   *string
	// baseDir is the consulted root's real folder, where a safety version goes.
	baseDir string
}

func (l *serviceLink) begin(opID string, opHandle *string, root, baseDir, name, canonicalPath, lane, origin string) *consultation {
	return &consultation{
		link: l, ID: newConsultID(), OpID: opID, OpHandle: opHandle,
		Root: root, Name: name, CanonicalPath: canonicalPath, Lane: lane, Origin: origin,
		baseDir: baseDir,
	}
}

func (c *consultation) head() orderedObject {
	var opHandle any
	if c.OpHandle != nil {
		opHandle = *c.OpHandle
	}
	return orderedObject{
		{Key: "consultId", Value: c.ID},
		{Key: "opId", Value: c.OpID},
		{Key: "opHandle", Value: opHandle},
		{Key: "root", Value: c.Root},
		{Key: "name", Value: c.Name},
		{Key: "canonicalPath", Value: c.CanonicalPath},
		{Key: "lane", Value: c.Lane},
		{Key: "origin", Value: c.Origin},
	}
}

// call makes one consultation call and runs its artifact batches: while the
// answer is pending, the batch is written in order, its receipts are posted,
// and that call's answer is the next one.
func (c *consultation) call(path string, body any) (serviceAnswer, error) {
	var answer serviceAnswer
	if err := c.link.consult(path, body, &answer); err != nil {
		return serviceAnswer{}, err
	}
	for batches := 0; answer.Pending; batches++ {
		if batches == maxPendingBatches {
			return serviceAnswer{}, fmt.Errorf("%s: more than %d artifact batches", path, maxPendingBatches)
		}
		receipts := make([]artifactReceipt, 0, len(answer.Artifacts))
		for _, artifact := range answer.Artifacts {
			receipts = append(receipts, c.writeArtifact(artifact))
		}
		var next serviceAnswer
		written := orderedObject{{Key: "consultId", Value: c.ID}, {Key: "receipts", Value: receipts}}
		if err := c.link.consult("/v1/artifacts-written", written, &next); err != nil {
			return serviceAnswer{}, err
		}
		answer = next
	}
	return answer, nil
}

// writeArtifact writes one artifact with Local's naming and answers its
// receipt. The service never names a path: a safety version goes beside the
// consulted document's versions, and a recovered copy under the named root's
// own recovered folder, by that root's resolver.
func (c *consultation) writeArtifact(artifact serviceArtifact) artifactReceipt {
	receipt := artifactReceipt{ArtifactID: artifact.ArtifactID, Type: artifact.Type}
	fail := func(message string) artifactReceipt {
		receipt.Error = &message
		return receipt
	}
	if !artifactIDPattern.MatchString(artifact.ArtifactID) {
		return fail("invalid artifactId")
	}
	content, _, err := artifact.Logical.decode()
	if err != nil {
		return fail(err.Error())
	}
	switch artifact.Type {
	case "safety-version":
		published := CreateBackup(c.baseDir, backupSiteName(c.Name), content, nil)
		if published == nil {
			return fail("backup-failed")
		}
		receipt.Version = &published.Filename
	case "recovered-copy":
		root, name := artifact.Root, artifact.Name
		receipt.Root, receipt.Name = &root, &name
		baseDir, ok := c.link.rootDir(root)
		if !ok {
			return fail("unknown root")
		}
		published, err := publishRecoveredCopy(baseDir, name, content)
		if err != nil {
			return fail(err.Error())
		}
		receipt.Version = &published.Filename
	default:
		return fail("unknown artifact type")
	}
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	receipt.Sha256 = &digest
	return receipt
}

// admitArgs is what one lane tells the service before its first backup.
type admitArgs struct {
	Incoming       []byte
	DataLossPrev   *string
	Headers        map[string]string
	Edit           bool
	Safety         bool
	ExplicitCreate bool
}

// admitHeaders are the request headers Local's lanes read, every one present.
var admitHeaders = []string{"if-match", "page-conversion", "page-write", "page-format", "persistent"}

// admit asks the service to prepare one write: Local's normalization, stale
// rebase, room admission and any artifacts. A proceed answer must carry the
// logical document and its file part; a refusal a 4xx or 5xx status and a code.
func (c *consultation) admit(args admitArgs) (serviceAnswer, error) {
	current, err := readSnapshot(c.baseDir, c.Name, c.CanonicalPath)
	if err != nil {
		return serviceAnswer{}, err
	}
	headers := orderedObject{}
	for _, name := range admitHeaders {
		var value any
		if given, ok := args.Headers[name]; ok {
			value = given
		}
		headers = append(headers, kv{Key: name, Value: value})
	}
	var dataLossPrev any
	if args.DataLossPrev != nil {
		dataLossPrev = controlBytesValue(*args.DataLossPrev)
	}
	body := append(c.head(),
		kv{Key: "current", Value: current},
		kv{Key: "incoming", Value: controlBytesValue(args.Incoming)},
		kv{Key: "versions", Value: rebaseVersions(c.baseDir, c.Name, string(args.Incoming))},
		kv{Key: "dataLossPrev", Value: dataLossPrev},
		kv{Key: "headers", Value: headers},
		kv{Key: "edit", Value: args.Edit},
		kv{Key: "safety", Value: args.Safety},
		kv{Key: "explicitCreate", Value: args.ExplicitCreate},
	)
	answer, err := c.call("/v1/admit", body)
	if err != nil {
		return serviceAnswer{}, err
	}
	switch answer.Decision {
	case "proceed":
		if answer.Logical == nil || answer.File == nil {
			return serviceAnswer{}, errors.New("admit: proceed without logical and file")
		}
		c.Reservation = answer.Reservation
	case "refuse":
		if answer.Status < 400 || answer.Status > 599 || answer.Code == "" {
			return serviceAnswer{}, fmt.Errorf("admit: refusal with status %d and code %q", answer.Status, answer.Code)
		}
	default:
		return serviceAnswer{}, fmt.Errorf("admit: unknown decision %q", answer.Decision)
	}
	return answer, nil
}

// settleArgs is one write's outcome as the service confirms it.
type settleArgs struct {
	Outcome        string
	Logical        *string
	File           *string
	Etag           *string
	Sender         string
	UserDriven     *bool
	ExplicitCreate bool
	CaptureID      *string
	SaveID         *string
}

// settle confirms one write to the service at Local's confirmation point. A
// write whose bytes landed (committed or indeterminate) and whose settle gets no
// good answer leaves the document unconfirmed and its abort retried until the
// service answers, because the service still holds what it reserved.
func (c *consultation) settle(args settleArgs) error {
	var identity any
	if args.Outcome == "committed" || args.Outcome == "indeterminate" {
		found, err := identityOfPath(c.CanonicalPath)
		if err == nil && found != nil {
			identity = orderedObject{
				{Key: "dev", Value: found.Dev},
				{Key: "ino", Value: found.Ino},
				{Key: "size", Value: found.Size},
				{Key: "mtimeNs", Value: found.MtimeNs},
			}
		}
	}
	textValue := func(text *string) any {
		if text == nil {
			return nil
		}
		return controlBytesValue(*text)
	}
	nullable := func(text *string) any {
		if text == nil {
			return nil
		}
		return *text
	}
	var reservation any
	if c.Reservation != nil {
		reservation = *c.Reservation
	}
	var userDriven any
	if args.UserDriven != nil {
		userDriven = *args.UserDriven
	}
	body := append(c.head(),
		kv{Key: "reservation", Value: reservation},
		kv{Key: "outcome", Value: args.Outcome},
		kv{Key: "logical", Value: textValue(args.Logical)},
		kv{Key: "file", Value: textValue(args.File)},
		kv{Key: "etag", Value: nullable(args.Etag)},
		kv{Key: "sender", Value: args.Sender},
		kv{Key: "userDriven", Value: userDriven},
		kv{Key: "explicitCreate", Value: args.ExplicitCreate},
		kv{Key: "captureId", Value: nullable(args.CaptureID)},
		kv{Key: "saveId", Value: nullable(args.SaveID)},
		kv{Key: "identity", Value: identity},
	)
	answer, err := c.call("/v1/settle", body)
	if err == nil && (answer.OK == nil || !*answer.OK) {
		err = fmt.Errorf("settle: %s", answer.Error)
	}
	if err != nil {
		if args.Outcome == "committed" || args.Outcome == "indeterminate" {
			c.link.markUnconfirmed(c.CanonicalPath, c.Root, c.Name)
			c.link.abort(c.ID, "settle-failed")
		} else {
			c.link.abort(c.ID, "failed")
		}
		return err
	}
	return nil
}

// renderCheckpoint asks the service to render a dirty room while this host
// holds the document's lock (protocol v1, render-checkpoint). current is the
// snapshot the render is judged against; the room, its target and its
// admission claim are the service's, found by opHandle.
func (c *consultation) renderCheckpoint(roomIdentity string, generation json.RawMessage, current orderedObject) (serviceAnswer, error) {
	var gen any
	if len(generation) > 0 {
		gen = generation
	}
	body := append(c.head(),
		kv{Key: "roomIdentity", Value: roomIdentity},
		kv{Key: "generation", Value: gen},
		kv{Key: "current", Value: current},
	)
	answer, err := c.call("/v1/render-checkpoint", body)
	if err != nil {
		return serviceAnswer{}, err
	}
	switch answer.Kind {
	case "render":
		if answer.Logical == nil {
			return serviceAnswer{}, errors.New("render-checkpoint: render without logical")
		}
	case "clean", "current", "refused":
	default:
		return serviceAnswer{}, fmt.Errorf("render-checkpoint: unknown kind %q", answer.Kind)
	}
	return answer, nil
}

// end aborts a consultation that never reached settle, naming why.
func (c *consultation) end(err error) {
	c.link.abort(c.ID, abortReason(err))
}

// beginLane starts the consultation of one write to a page document when a
// service is attached. It answers nil and no refusal when there is no service
// or the name is not a page document, and a refusal when the document's last
// settle failed and it has not been reconciled.
func (h *commitHost) beginLane(request *commitRequest, filePath, lane, origin string) (*consultation, *commitResult) {
	if h.service == nil || !isDocumentName(request.Name) {
		return nil, nil
	}
	if h.service.barred(filePath) {
		result := refusedBy(retryableRefusal("unconfirmed"))
		return nil, &result
	}
	if request.OpID == "" {
		request.OpID = newOpID()
	}
	if request.ConsultOrigin != "" {
		origin = request.ConsultOrigin
	}
	return h.service.begin(request.OpID, request.OpHandle, h.rootID, h.baseDir, request.Name, filePath, lane, origin), nil
}

// admitted runs admit for one lane and decodes a proceed answer into the
// logical document and its file part. A refusal, a service that cannot be
// reached, or an answer that cannot be decoded comes back as the lane's
// answer, with the consultation ended.
func (h *commitHost) admitted(consult *consultation, args admitArgs) (string, string, *commitResult) {
	answer, err := consult.admit(args)
	if err != nil {
		consult.end(err)
		result := refusedBy(retryableRefusal("service-unavailable"))
		return "", "", &result
	}
	if answer.Decision == "refuse" {
		h.service.abort(consult.ID, "refused")
		result := refusedBy(serviceRefusal{Status: answer.Status, Code: answer.Code, Body: answer.Body, Headers: answer.Headers})
		return "", "", &result
	}
	logical, _, err := answer.Logical.decode()
	var file []byte
	if err == nil {
		file, _, err = answer.File.decode()
	}
	if err != nil {
		consult.end(err)
		result := refusedBy(retryableRefusal("service-unavailable"))
		return "", "", &result
	}
	return string(logical), string(file), nil
}

// settleFailed settles a write whose document write failed: indeterminate when
// the bytes may have landed, failed when they cannot have.
func (c *consultation) settleFailed(err error, sender string) {
	outcome := "failed"
	if errorCode(err) != "" {
		outcome = "indeterminate"
	}
	_ = c.settle(settleArgs{Outcome: outcome, Sender: sender})
}

// abortReason is the abort reason for a call that failed before any byte of
// the document was written.
func abortReason(err error) string {
	var epoch *serviceEpochError
	switch {
	case errors.As(err, &epoch):
		return "epoch"
	case errors.Is(err, errServiceUnavailable):
		return "deadline"
	}
	return "failed"
}

// serviceRefusal is a refusal the service decided, or the host's own when the
// service could not be consulted, answered to the client as it is.
type serviceRefusal struct {
	Status  int
	Code    string
	Body    json.RawMessage
	Headers map[string]string
}

func refusedBy(refusal serviceRefusal) commitResult {
	return commitResult{Outcome: "refused", Reason: refusal.Code, Refusal: &refusal}
}

// retryableRefusal is the host's own 503 for a write it could not admit: the
// service did not answer, or the document waits for its reconciliation.
func retryableRefusal(code string) serviceRefusal {
	body, _ := json.Marshal(map[string]any{"reason": code, "retryable": true})
	return serviceRefusal{Status: http.StatusServiceUnavailable, Code: code, Body: body, Headers: map[string]string{"Retry-After": "1"}}
}

// writeRefusal answers a client with a refusal as it was decided.
func writeRefusal(w http.ResponseWriter, r *http.Request, refusal *serviceRefusal) error {
	for name, value := range refusal.Headers {
		w.Header().Set(name, value)
	}
	body := []byte(refusal.Body)
	if len(body) == 0 {
		body = []byte("{}")
	}
	sendJSON(w, r, refusal.Status, body)
	return nil
}

// refusalError carries a decided refusal through a caller that answers errors.
type refusalError struct{ refusal *serviceRefusal }

func (e *refusalError) Error() string { return "refused: " + e.refusal.Code }

// lowerHeaders are the request headers Local's save lanes read.
var lowerHeaders = []string{"If-Match", "Page-Conversion", "Page-Write", "Page-Format"}

// laneHeaders is the subset of a request's headers admit carries, by
// lower-case name, present ones only.
func laneHeaders(r *http.Request) map[string]string {
	headers := map[string]string{}
	for _, name := range lowerHeaders {
		if values, ok := r.Header[http.CanonicalHeaderKey(name)]; ok && len(values) > 0 {
			headers[strings.ToLower(name)] = values[0]
		}
	}
	return headers
}

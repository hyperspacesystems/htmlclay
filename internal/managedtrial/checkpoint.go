//go:build managedtrial

package managedtrial

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
)

// controlCheckpointBody is one POST /v1/checkpoint.
type controlCheckpointBody struct {
	OpID         string          `json:"opId"`
	Root         string          `json:"root"`
	Name         string          `json:"name"`
	RoomIdentity string          `json:"roomIdentity"`
	Generation   json.RawMessage `json:"generation"`
}

// serveCheckpoint is POST /v1/checkpoint: Local's checkpoint, run by this host.
// Under the document's lock the service renders the dirty room, and a render
// is written through the save lane with origin checkpoint, which admits and
// settles it like any save. The file's identity is checked again immediately
// before the rename, as Local's is.
func (a *controlAPI) serveCheckpoint(w http.ResponseWriter, r *http.Request) {
	var body controlCheckpointBody
	if !decodeControlJSON(w, r, &body) {
		return
	}
	root, ok := a.root(w, body.Root)
	if !ok {
		return
	}
	link := root.commit.service
	if link == nil {
		writeControlJSON(w, http.StatusNotFound, controlError{Error: "checkpoint needs the service"})
		return
	}
	if !isDocumentName(body.Name) || body.RoomIdentity == "" {
		writeControlJSON(w, http.StatusBadRequest, controlError{Error: "checkpoint needs a page document and a room identity"})
		return
	}
	filePath, err := root.commit.resolveWrite(body.Name)
	if err != nil {
		writeControlJSON(w, controlErrorStatus(err), controlError{Error: err.Error()})
		return
	}
	opID, ok := claimControlOp(w, root.events, body.OpID)
	if !ok {
		return
	}

	unlock := root.commit.locks.lock(filePath)
	defer unlock()

	refuse := func(refusal serviceRefusal) {
		root.events.finishOp(opID, "refused", "", "")
		_ = writeRefusal(w, r, &refusal)
	}
	answer := func(kind string, fields ...kv) {
		writeControlJSON(w, http.StatusOK, append(orderedObject{{Key: "kind", Value: kind}}, fields...))
	}
	if link.barred(filePath) {
		refuse(retryableRefusal("unconfirmed"))
		return
	}

	// Read in this order so the check before the rename catches any change after
	// the identity: the identity, the bytes the guard compares against, then the
	// snapshot the service renders against.
	before, err := identityOfPath(filePath)
	if err != nil {
		root.events.finishOp(opID, "refused", "", "")
		writeControlJSON(w, http.StatusInternalServerError, controlError{Error: err.Error()})
		return
	}
	var dataLossPrev *string
	if data, err := os.ReadFile(filePath); err == nil {
		text := string(data)
		dataLossPrev = &text
	}
	current, err := readSnapshot(root.baseDir, body.Name, filePath)
	if err != nil {
		root.events.finishOp(opID, "refused", "", "")
		writeControlJSON(w, http.StatusInternalServerError, controlError{Error: err.Error()})
		return
	}

	opHandle := opID
	consult := link.begin(opID, &opHandle, root.id, root.baseDir, body.Name, filePath, "save", "checkpoint")
	rendered, err := consult.renderCheckpoint(body.RoomIdentity, body.Generation, current)
	if err != nil {
		consult.end(err)
		refuse(retryableRefusal("service-unavailable"))
		return
	}
	if rendered.Kind != "render" {
		root.events.finishOp(opID, "refused", "", "")
		answer(rendered.Kind, kv{Key: "reason", Value: nullableString(rendered.Reason)}, kv{Key: "opId", Value: opID}, kv{Key: "cursor", Value: nil})
		return
	}
	logical, _, err := rendered.Logical.decode()
	if err != nil {
		consult.end(err)
		refuse(retryableRefusal("service-unavailable"))
		return
	}

	request := &commitRequest{
		Kind: "browser-save", Name: body.Name, Bytes: logical, Text: true,
		UserDriven: false, UserDrivenSet: true, SaveID: "",
		DataLossPrev: dataLossPrev, OpID: opID, OpHandle: &opHandle,
		ConsultOrigin: "checkpoint", CaptureID: rendered.CaptureID,
	}
	if before != nil {
		request.BeforeRename = func() error {
			now, err := identityOfPath(filePath)
			if err != nil {
				return err
			}
			if now == nil || *now != *before {
				return errCheckpointTargetChanged
			}
			return nil
		}
	}
	result, err := root.commit.commitLocked(request, commitKinds["browser-save"], filePath)
	if err != nil {
		if errors.Is(err, errCheckpointTargetChanged) {
			root.events.finishOp(opID, "refused", "", "")
			answer("refused", kv{Key: "reason", Value: "invalid-identity"}, kv{Key: "opId", Value: opID}, kv{Key: "cursor", Value: nil})
			return
		}
		if code := errorCode(err); code != "" {
			indeterminate := commitResult{Outcome: "indeterminate", Reason: code}
			root.commit.report(request, &indeterminate)
			answer("indeterminate", kv{Key: "reason", Value: code}, kv{Key: "opId", Value: opID}, kv{Key: "cursor", Value: indeterminate.Cursor})
			return
		}
		root.events.finishOp(opID, "refused", "", "")
		writeControlJSON(w, http.StatusInternalServerError, controlError{Error: err.Error()})
		return
	}
	if result.Refusal != nil && result.Refusal.Status == http.StatusServiceUnavailable {
		// The save's own consultation could not go on: answered as the
		// recoverable 503, so the service's checkpoint throws and its scheduler
		// retries, as it does for any transient failure.
		_ = writeRefusal(w, r, result.Refusal)
		return
	}
	var written any
	if result.Outcome == "committed" {
		written = controlBytesValue(result.Content)
	}
	var revision any
	if len(rendered.Revision) > 0 {
		revision = rendered.Revision
	}
	answer("ok",
		kv{Key: "revision", Value: revision},
		kv{Key: "written", Value: written},
		kv{Key: "opId", Value: opID},
		kv{Key: "cursor", Value: result.Cursor},
	)
}

//go:build managedtrial

package managedtrial

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Local's data-clobber guard endpoint. A page's "Saved data was overwritten"
// notice reads /_/data-loss and restores through /_/data-loss/<id>. The site is
// identified by ?file= (GET), body.file (POST), falling back to the Document-URL
// header, because this host serves many documents by name.
//
// AD2: with a local authority Local runs this read-modify-write through
// runDocumentWrite and confirms the write-back through the room's target and
// token. There are no rooms here, so the region holds the document's own
// canonical-path queue slot and the write-back is the commit core's
// guard-restore lane.

// dataLossBodyLimit is Local's express.json({ limit: '1mb' }) on this route.
const dataLossBodyLimit = 1024 * 1024

// guardFile is resolveGuardFile's answer: the document a data-loss request names
// and the path this host would write it at.
type guardFile struct {
	Name string
	Path string
}

// resolveGuardFile is Local's resolveGuardFile: the request's file, resolved as
// a document name. A name this host cannot write is not a document it can hold a
// guard record for, so it resolves to nothing.
func (s *rootServer) resolveGuardFile(r *http.Request, body any) *guardFile {
	raw := r.URL.Query().Get("file")
	if raw == "" {
		if object, ok := body.(map[string]any); ok {
			if value, ok := object["file"].(string); ok {
				raw = value
			}
		}
	}
	if raw == "" {
		raw = documentURLHeader(r)
	}
	if raw == "" {
		return nil
	}
	name := resolveResourceFromHref(raw)
	filePath, err := resolveWriteTarget(s.registry, name)
	if err != nil {
		return nil
	}
	return &guardFile{Name: name, Path: filePath}
}

// serveDataLossGet is Local's app.get('/data-loss'): the pending event, after the
// pass an edit made while this host was not looking never got.
func (s *rootServer) serveDataLossGet(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	resolved := s.resolveGuardFile(r, nil)
	if resolved == nil {
		return dataLossAnswer(w, r, http.StatusOK, orderedObject{{Key: "event", Value: (*guardClientEvent)(nil)}})
	}
	var currentBytes []byte
	if data, err := os.ReadFile(resolved.Path); err == nil {
		currentBytes = data
	}
	currentHTML := string(currentBytes)
	// Local skips the catch-up for an empty file, because a zero-length Buffer is
	// falsy there.
	if len(currentBytes) > 0 {
		catchUpOutsideEdit(s.baseDir, resolved.Name, currentBytes, currentHTML, s.hooks.guard)
	}
	return dataLossAnswer(w, r, http.StatusOK, orderedObject{
		{Key: "event", Value: getGuardEvent(s.baseDir, resolved.Name, &currentHTML, s.hooks.guard)},
	})
}

// serveDataLossPost is Local's app.post(/^\/data-loss(?:\/(.+))?$/): dismiss,
// revert or restore the pending event, with the write-back published through the
// guard-restore lane.
func (s *rootServer) serveDataLossPost(w http.ResponseWriter, r *http.Request, id string) error {
	body, tooLarge, err := readDataLossBody(r)
	if err != nil {
		return err
	}
	if tooLarge {
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		sendText(w, r, http.StatusRequestEntityTooLarge, payloadTooLargePage)
		return nil
	}

	resolved := s.resolveGuardFile(r, body)
	if resolved == nil {
		return dataLossAnswer(w, r, http.StatusBadRequest, orderedObject{{Key: "error", Value: "file required"}})
	}
	object, _ := body.(map[string]any)
	if id == "" {
		if value, ok := object["id"].(string); ok {
			id = value
		}
	}
	choice, _ := object["choice"].(string)
	if choice != "dismiss" && choice != "revert" && choice != "restore" {
		return dataLossAnswer(w, r, http.StatusBadRequest, orderedObject{
			{Key: "error", Value: "choice must be dismiss | revert | restore"},
		})
	}

	// A1: the restore region is read-modify-write too: the current body is read,
	// the guard decides against it, and the write-back publishes. All of it holds
	// the same canonical-path queue slot a concurrent /save would need.
	unlock := s.commit.locks.lock(resolved.Path)
	defer unlock()

	currentHTML := ""
	if data, err := os.ReadFile(resolved.Path); err == nil {
		currentHTML = string(data)
	}
	result := resolveGuard(resolveGuardArgs{
		BaseDir:     s.baseDir,
		Name:        resolved.Name,
		ID:          id,
		Choice:      choice,
		CurrentHTML: currentHTML,
		WriteBack: func(html string) error {
			result, err := s.publishHostWrite(resolved.Name, resolved.Path, html, false, false, "guard-restore")
			if err == nil && result.Outcome == "refused" {
				err = fmt.Errorf("guard write-back refused: %s", result.Reason)
			}
			return err
		},
		Live: s.hooks.guard,
	})
	if !result.OK {
		status := result.StatusCode
		if status == 0 {
			status = http.StatusBadRequest
		}
		return dataLossAnswer(w, r, status, orderedObject{{Key: "error", Value: result.Error}})
	}
	// Rider 1: after a local dismiss Local nudges the platform so the owner's
	// other devices clear the same incident. A trial has no platform, and the
	// local UI has already cleared, so the answer is the whole of it.
	return dataLossAnswer(w, r, http.StatusOK, orderedObject{
		{Key: "ok", Value: true},
		{Key: "choice", Value: result.Choice},
		{Key: "status", Value: result.Status},
	})
}

// readDataLossBody is express.json on this route: a JSON body when the request
// says it carries one, nothing when it does not (Express leaves NO body), a 400
// for bytes that are not JSON, and the 413 page above the parser's limit.
func readDataLossBody(r *http.Request) (any, bool, error) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return nil, false, nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, dataLossBodyLimit+1))
	if err != nil {
		return nil, false, err
	}
	if len(raw) > dataLossBodyLimit {
		return nil, true, nil
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, false, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false, badJSONBodyError()
	}
	// The parser runs in strict mode: a primitive is not a body it accepts.
	switch value.(type) {
	case map[string]any, []any:
		return value, false, nil
	}
	return nil, false, badJSONBodyError()
}

// badJSONBodyError is Express's default answer for a body that does not parse.
func badJSONBodyError() error {
	return &PathError{Status: http.StatusBadRequest, Message: "Bad request"}
}

// dataLossAnswer writes one JSON answer of the guard route.
func dataLossAnswer(w http.ResponseWriter, r *http.Request, status int, body orderedObject) error {
	encoded, err := jsonStringify(body)
	if err != nil {
		return err
	}
	sendJSON(w, r, status, encoded)
	return nil
}

// publishHostWrite is Local's publishHostWrite: the one publish sequence for a
// write this app makes to a live document, so the guard's write-back and a
// restore cannot drift. Returns the commit result, whose Content is the composed
// document the stamp describes.
//
// AD2: the caller already holds the document's canonical-path queue slot, and a
// write whose directory entry may not have survived is indeterminate.
func (s *rootServer) publishHostWrite(name, filePath, html string, edit, safety bool, kind string) (commitResult, error) {
	policy, ok := commitKinds[kind]
	if !ok {
		return commitResult{}, fmt.Errorf("Unknown document write kind: %s", kind)
	}
	result, err := s.commit.commitLocked(&commitRequest{
		Kind:   kind,
		Name:   name,
		Bytes:  []byte(html),
		Text:   true,
		Edit:   edit,
		Safety: safety,
	}, policy, filePath)
	if err != nil {
		if code := errorCode(err); code != "" {
			return commitResult{Outcome: "indeterminate", Reason: code}, nil
		}
		return commitResult{}, err
	}
	if result.Outcome == "indeterminate" {
		return commitResult{}, errors.New(result.Reason)
	}
	return result, nil
}

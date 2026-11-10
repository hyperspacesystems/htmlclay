//go:build managedtrial

package managedtrial

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/panphora/htmlclay/internal/specwire"
)

// Local's POST /_/save, and the body parser mounted in front of it. The route
// takes the document as text and puts everything else about the save in a
// header, so every content type is read as text: a JSON body then arrives as a
// string and can be refused with a real message instead of reaching the handler
// as an unparsed blank.

// maxSaveIDLength is Local's MAX_SAVE_ID_LEN. An overlong id is DROPPED rather
// than truncated: a truncated id could collide with another client's id.
const maxSaveIDLength = 128

// isJSONContentType is Local's isJsonContentType: the media type only, ignoring
// parameters, and covering the `+json` structured suffix.
func isJSONContentType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

// ephemeralAttrs are the root attributes a save must never persist: the save
// token in both its spellings, and this host's own response stamp.
var ephemeralAttrs = []string{"savetoken", "htmlclaytoken", ephemeralAttribute}

// hasHTMLRoot is Local's hasHtmlRoot: spec §4 obliges a host to refuse a save
// body that is not a complete HTML document with a top-level <html> element.
func hasHTMLRoot(content string) bool {
	_, _, ok := scanRootHtmlTag(content)
	return ok
}

// stripSaveToken is Local's stripSaveToken: the ephemeral root metadata comes
// back from clients and from a stored version, and none of it belongs in the
// bytes this host writes. It edits the ROOT start tag and nothing else, so a
// body that merely quotes the attribute is left alone.
//
// AD1: Local first hands a document carrying the workspace bridge attributes to
// stripWorkspaceBridge. The trial serves no workspace bridge, so nothing can
// reach it carrying one.
func stripSaveToken(html string) string {
	start, end, ok := scanRootHtmlTag(html)
	if !ok {
		return html
	}
	attrs := html[start+5 : end]
	stripped := stripRootAttributes(attrs)
	if stripped == attrs {
		return html
	}
	return html[:start+5] + stripped + html[end:]
}

// stripRootAttributes drops the ephemeral attributes from one start tag's
// attribute text, keeping every other byte.
func stripRootAttributes(attrs string) string {
	return attributePattern.ReplaceAllStringFunc(attrs, func(whole string) string {
		match := attributePattern.FindStringSubmatch(whole)
		if len(match) > 1 && containsString(ephemeralAttrs, strings.ToLower(match[1])) {
			return ""
		}
		return whole
	})
}

// saveConflict is one conflict the route judges inside the document's lock,
// carried out of it so the answer is written after the lock is released.
type saveConflict struct {
	Status    int
	Msg       string
	Code      string
	Etag      string
	ChangedBy string
	SaveID    string
}

// codedWriteError is a write failure Local answers with a 409 and its code.
type codedWriteError struct {
	code    string
	message string
}

func (e *codedWriteError) Error() string { return e.message }

// userDrivenFromHeader is data-loss-guard.js's userDrivenFromHeader. The second
// answer says whether the request expressed a provenance at all: a save that
// says nothing is ui-unknown, never ui-background.
func userDrivenFromHeader(r *http.Request) (bool, bool) {
	trigger := strings.ToLower(strings.TrimSpace(r.Header.Get("Save-Trigger")))
	if trigger != "" {
		return trigger == "user", true
	}
	switch r.Header.Get("X-Hyperclay-User-Driven") {
	case "1", "true":
		return true, true
	case "0", "false":
		return false, true
	}
	return false, false
}

// saveIDHeader is §6's receipt id: opaque, never a credential, and never minted
// here, so a client that sees its own id back knows its own save wrote these
// bytes.
func saveIDHeader(r *http.Request) string {
	saveID := r.Header.Get("Save-ID")
	if jsStringLength(saveID) > maxSaveIDLength {
		return ""
	}
	return saveID
}

// readSaveBody is express.text's read of the whole body as one UTF-8 string,
// with its limit. Above the limit the route answers what Express's default
// error handler answers: 413 and that handler's HTML page. The stack Express
// prints in development cannot be reproduced, so this is its production body.
func readSaveBody(r *http.Request) (string, bool, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, saveMaxBytes+1))
	if err != nil {
		return "", false, err
	}
	if int64(len(body)) > saveMaxBytes {
		return "", true, nil
	}
	return strings.ToValidUTF8(string(body), "\ufffd"), false, nil
}

// saveAnswer writes one JSON answer of the save route.
func saveAnswer(w http.ResponseWriter, r *http.Request, status int, body orderedObject) error {
	encoded, err := jsonStringify(body)
	if err != nil {
		return err
	}
	sendJSON(w, r, status, encoded)
	return nil
}

// serveSave is Local's app.post('/save'): both spellings of the route reach it,
// because the `/_/` marker is stripped before dispatch and the bare name is not
// one the gate refuses.
func (s *rootServer) serveSave(w http.ResponseWriter, r *http.Request) error {
	content, tooLarge, err := readSaveBody(r)
	if err != nil {
		return err
	}
	if tooLarge {
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		sendText(w, r, http.StatusRequestEntityTooLarge, payloadTooLargePage)
		return nil
	}

	href := documentURLHeader(r)
	if href == "" {
		return saveAnswer(w, r, http.StatusBadRequest, orderedObject{
			{Key: "msg", Value: "Document-URL header required."},
			{Key: "msgType", Value: "error"},
		})
	}
	name := resolveResourceFromHref(href)

	if isJSONContentType(r.Header.Get("Content-Type")) {
		return saveAnswer(w, r, http.StatusUnsupportedMediaType, orderedObject{
			{Key: "msg", Value: "/_/save takes the document as text, not JSON."},
			{Key: "msgType", Value: "error"},
			{Key: "code", Value: "unsupported-type"},
		})
	}

	if !hasHTMLRoot(content) {
		return saveAnswer(w, r, http.StatusUnprocessableEntity, orderedObject{
			{Key: "msg", Value: "Not a complete HTML document with a top-level <html> element."},
			{Key: "msgType", Value: "error"},
			{Key: "code", Value: "invalid-document"},
		})
	}

	// §9, and it has to happen HERE, before the etag is computed: the stamp
	// must describe the bytes that reach disk.
	content = s.commit.canonicalForWrite(stripSaveToken(content))
	userDriven, userDrivenSet := userDrivenFromHeader(r)
	saveID := saveIDHeader(r)

	filePath, err := resolveWriteTarget(s.registry, name)
	if err != nil {
		status := http.StatusBadRequest
		var pathErr *PathError
		if errors.As(err, &pathErr) && pathErr.Status != 0 {
			status = pathErr.Status
		}
		return saveAnswer(w, r, status, orderedObject{
			{Key: "msg", Value: err.Error()},
			{Key: "msgType", Value: "error"},
		})
	}

	request := &commitRequest{
		Kind:          "browser-save",
		Name:          name,
		Bytes:         []byte(content),
		Text:          true,
		UserDriven:    userDriven,
		UserDrivenSet: userDrivenSet,
		SaveID:        saveID,
	}

	// A1: the queue wraps the ENTIRE read-modify-write region, so the If-Match
	// judgement below is made against bytes no other save can replace while it
	// is being judged.
	unlock := s.commit.locks.lock(filePath)
	defer unlock()

	if ifMatch := r.Header.Get("If-Match"); ifMatch != "" || headerPresent(r, "If-Match") {
		if conflict := s.judgeIfMatch(filePath, name, ifMatch); conflict != nil {
			return s.writeConflict(w, r, name, conflict)
		}
	}

	result, err := s.commit.commitLocked(request, commitLane{lane: "save", origin: "save"}, filePath)
	if err != nil {
		var coded *codedWriteError
		if errors.As(err, &coded) && (coded.code == "moved-or-deleted" || coded.code == "unsaved-edits") {
			return saveAnswer(w, r, http.StatusConflict, orderedObject{
				{Key: "msg", Value: coded.message},
				{Key: "msgType", Value: "error"},
				{Key: "code", Value: coded.code},
			})
		}
		return saveAnswer(w, r, http.StatusInternalServerError, orderedObject{
			{Key: "msg", Value: "Server error saving file: " + err.Error()},
			{Key: "msgType", Value: "error"},
		})
	}

	if result.Outcome == "refused" {
		// AD2: Local rebases this save onto the stored page; the trial refuses
		// it before anything is written, and says which refusal it is.
		if result.Reason == "stale-page-lineage" {
			return saveAnswer(w, r, http.StatusConflict, orderedObject{
				{Key: "msg", Value: "This page has newer edits on this computer. Reload it, then save again."},
				{Key: "msgType", Value: "error"},
				{Key: "code", Value: "stale-page-lineage"},
			})
		}
		return saveAnswer(w, r, http.StatusConflict, orderedObject{
			{Key: "msg", Value: name + " could not be saved."},
			{Key: "msgType", Value: "error"},
			{Key: "code", Value: result.Reason},
		})
	}

	// §6: a host advertising `conditional` returns the stamp with EVERY save, not
	// only its refusals, or a client has nothing to send as its next If-Match.
	body := orderedObject{
		{Key: "msg", Value: "Saved"},
		{Key: "msgType", Value: "success"},
		{Key: "etag", Value: result.Etag},
	}
	if saveID != "" {
		body = append(body, kv{Key: "saveId", Value: saveID})
	}
	if r.Header.Get("Prefer") == "return=representation" && result.HasFile {
		body = append(body, kv{Key: "html", Value: result.File})
	}
	return saveAnswer(w, r, http.StatusOK, body)
}

// payloadTooLargePage is Express's default answer above the body parser's limit.
const payloadTooLargePage = "<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<title>Error</title>\n</head>\n<body>\n<pre>Payload Too Large</pre>\n</body>\n</html>\n"

// headerPresent reports whether the header was sent at all: an empty If-Match is
// a client that computed its stamp wrong, and is refused rather than quietly
// dropped back to overwriting.
func headerPresent(r *http.Request, name string) bool {
	_, ok := r.Header[http.CanonicalHeaderKey(name)]
	return ok
}

// judgeIfMatch is §6's conditional check, made inside the document's lock. It
// answers nil when the save may proceed.
func (s *rootServer) judgeIfMatch(filePath, name, ifMatch string) *saveConflict {
	storedBytes := []byte{}
	unreadable := false
	if data, err := os.ReadFile(filePath); err == nil {
		logical, logicalErr := logicalBytes(s.commit.baseDir, name, data)
		if logicalErr != nil {
			unreadable = true
		} else {
			storedBytes = logical
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		unreadable = true
	}

	if unreadable {
		// A conditional save is a promise to compare, so a host that cannot read
		// says so instead of judging against the empty bytes the failure left.
		return &saveConflict{
			Status: http.StatusInternalServerError,
			Msg:    "Could not read " + name + " to check it against your copy.",
		}
	}
	if specwire.IfMatchSatisfied(ifMatch, storedBytes) {
		return nil
	}

	currentEtag := specwire.Etag(storedBytes)
	conflict := &saveConflict{Status: http.StatusPreconditionFailed, Etag: currentEtag}
	if record, ok := s.commit.store.etag(filePath); ok && record.Writer != "api" &&
		record.Etag == currentEtag && record.MtimeNs != nil {
		// Both halves have to agree: the bytes are the ones this process last
		// wrote, AND nothing has rewritten the file since. Otherwise the field is
		// omitted, because the wrong answer available here is the reassuring one.
		if now := mtimeNs(filePath); now != nil && *now == *record.MtimeNs {
			conflict.ChangedBy = "another-tab"
		}
	}
	// §6's late-duplicate rule: a client that recognises its OWN id here learns
	// its own earlier save moved the document, which is no conflict at all.
	conflict.SaveID = s.commit.store.saveReceipt(filePath, currentEtag)
	return conflict
}

// writeConflict answers a judged conflict: 500 when the bytes could not be read,
// and otherwise the 412 that carries the current stamp so a client can recover
// in one round trip.
func (s *rootServer) writeConflict(w http.ResponseWriter, r *http.Request, name string, conflict *saveConflict) error {
	if conflict.Status == http.StatusInternalServerError {
		return saveAnswer(w, r, conflict.Status, orderedObject{
			{Key: "msg", Value: conflict.Msg},
			{Key: "msgType", Value: "error"},
		})
	}
	body := orderedObject{
		{Key: "msg", Value: name + " changed since you last loaded it. Your version was not saved."},
		{Key: "msgType", Value: "error"},
		{Key: "code", Value: "conflict"},
		{Key: "etag", Value: conflict.Etag},
	}
	if conflict.ChangedBy != "" {
		body = append(body, kv{Key: "changedBy", Value: conflict.ChangedBy})
	}
	if conflict.SaveID != "" {
		body = append(body, kv{Key: "saveId", Value: conflict.SaveID})
	}
	return saveAnswer(w, r, conflict.Status, body)
}

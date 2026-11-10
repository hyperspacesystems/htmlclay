//go:build managedtrial

package managedtrial

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/panphora/htmlclay/internal/pageadmission"
)

// Local's live-sync relay (spec §10): the editing tab's working state handed to
// the other tabs of its page, and the snapshot its platform sync uploads. It
// never writes: §10 is flat about that, and the only state it keeps is the
// snapshot map in receipts.go, which holds what the next upload owes the
// platform rather than the document.

const (
	// relayBodyLimit is express.json({ limit: '10mb' }) on both addresses.
	relayBodyLimit = 10 << 20

	// relaySpecPath is the spec §10 address; `/live-sync/save` is the pre-spec
	// one and is the only address that reads `html`.
	relaySpecPath = "/sync"

	// relayLegacyPath is Local's `/live-sync/save`.
	relayLegacyPath = "/live-sync/save"
)

// relayBody is one parsed JSON body, kept as the raw bytes of each field: the
// presence test is Local's (`undefined` and `null` both mean "not this one"),
// and identityMap is forwarded as the bytes the client sent, never re-read.
type relayBody map[string]json.RawMessage

// present is Local's `body[key] !== undefined && body[key] !== null`.
func (b relayBody) present(key string) bool {
	raw, ok := b[key]
	if !ok {
		return false
	}
	return string(bytes.TrimSpace(raw)) != "null"
}

// string is Local's `typeof body[key] === 'string'`.
func (b relayBody) string(key string) (string, bool) {
	raw, ok := b[key]
	if !ok {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

// relayBodyProblem is what express.json made of a request body.
type relayBodyProblem int

const (
	relayBodyOK relayBodyProblem = iota
	relayBodyTooLarge
	relayBodyUnparsable
)

// readRelayBody is express.json on the two live-sync addresses: a body that is
// not JSON is not read at all, one over the limit is refused before it is
// parsed, and anything else becomes the object the route reads.
func readRelayBody(r *http.Request) (relayBody, relayBodyProblem, error) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		_, err := io.Copy(io.Discard, io.LimitReader(r.Body, relayBodyLimit))
		return relayBody{}, relayBodyOK, err
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, relayBodyLimit+1))
	if err != nil {
		return nil, relayBodyOK, err
	}
	if len(body) > relayBodyLimit {
		return nil, relayBodyTooLarge, nil
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return relayBody{}, relayBodyOK, nil
	}
	// body-parser's strict mode takes an object or an array and nothing else. An
	// array carries none of the fields this route reads, so it reads as empty.
	if trimmed[0] == '[' {
		var list []json.RawMessage
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return nil, relayBodyUnparsable, nil
		}
		return relayBody{}, relayBodyOK, nil
	}
	fields := relayBody{}
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return nil, relayBodyUnparsable, nil
	}
	return fields, relayBodyOK, nil
}

// relayAnswer writes one JSON answer of the relay route.
func relayAnswer(w http.ResponseWriter, r *http.Request, status int, body orderedObject) {
	encoded, err := jsonStringify(body)
	if err != nil {
		sendText(w, r, http.StatusInternalServerError, "Internal server error")
		return
	}
	sendJSON(w, r, status, encoded)
}

// relayError is Local's `res.status(status).json({ error })`.
func relayError(w http.ResponseWriter, r *http.Request, status int, message string) {
	relayAnswer(w, r, status, orderedObject{{Key: "error", Value: message}})
}

// serveRelay is Local's `app.post(['/live-sync/save', '/sync'])`: every
// validation answer in Local's order, then the document broadcast on the saved
// lane or the snapshot cached and broadcast on the live lane. urlPath is the
// path with the `/_/` marker already stripped, which is the address Express
// matches its routes against.
func (s *rootServer) serveRelay(w http.ResponseWriter, r *http.Request, urlPath string) error {
	body, problem, err := readRelayBody(r)
	if err != nil {
		return err
	}
	switch problem {
	case relayBodyTooLarge:
		// Local's error handler answers every status it does not name with this
		// body, and body-parser's `entity.too.large` carries no other.
		sendText(w, r, http.StatusRequestEntityTooLarge, "Internal server error")
		return nil
	case relayBodyUnparsable:
		sendText(w, r, http.StatusBadRequest, "Bad request")
		return nil
	}

	// §10 names two artifacts and the field says which audience each is for. A
	// snapshot is the sending tab's working state and goes to the other EDITORS;
	// a document is the durable, stripped one and goes to the VIEWERS. Presence
	// is the test, not string-ness: an explicit null reads as "not this one",
	// because a client building { snapshot, document } in JS naturally nulls the
	// half it is not using.
	//
	// `html` is the pre-spec spelling of `snapshot` and is read ON THE PRE-SPEC
	// ADDRESS ONLY: both frozen clients pair the key with the address in one wire
	// profile chosen once for the life of the page.
	legacyAddress := urlPath != relaySpecPath
	names := []string{"snapshot", "document"}
	if legacyAddress {
		names = []string{"snapshot", "html", "document"}
	}
	laneOf := func(name string) string {
		if name == "html" {
			return "snapshot"
		}
		return name
	}
	named := make([]string, 0, len(names))
	lanes := map[string]bool{}
	for _, name := range names {
		if body.present(name) {
			named = append(named, name)
			lanes[laneOf(name)] = true
		}
	}

	pageURL := documentURLHeader(r)
	if pageURL == "" {
		relayError(w, r, http.StatusBadRequest, "Document-URL header is required")
		return nil
	}
	name := resolveResourceFromHref(pageURL)
	if name == "" {
		relayError(w, r, http.StatusBadRequest, "could not resolve file from Document-URL")
		return nil
	}
	if len(lanes) != 1 {
		relayError(w, r, http.StatusBadRequest, "Send exactly one of snapshot or document.")
		return nil
	}
	lane := ""
	for name := range lanes {
		lane = name
	}
	field := ""
	for _, candidate := range named {
		if laneOf(candidate) == lane {
			field = candidate
			break
		}
	}
	html, isString := body.string(field)
	if !isString {
		relayError(w, r, http.StatusBadRequest, "a snapshot or a document must be a string")
		return nil
	}

	// The same bar the save lane holds bytes to, on the spec route only: the
	// pre-spec address has always taken any string, including the body
	// innerHTML hyperclayjs's captureBodyForSync() returns, and adding a rule
	// there would break documents nobody can update.
	if !legacyAddress && !hasHTMLRoot(html) {
		relayError(w, r, http.StatusUnprocessableEntity, "Not a complete HTML document.")
		return nil
	}

	// Shape-checked, then forwarded as sent: this host never parses keys or
	// interprets ids. A null is refused like an array or a string, because a
	// receiver would have no way to tell a confused client from an empty map.
	var identityMap json.RawMessage
	if raw, ok := body["identityMap"]; ok {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			relayError(w, r, http.StatusBadRequest, "identityMap must be a plain object.")
			return nil
		}
		identityMap = raw
	}

	// §6: the stamp of what this host stored for the bytes a tab just saved,
	// carried unread and only ever on a snapshot.
	etag := ""
	if _, ok := body["etag"]; ok {
		value, isString := body.string("etag")
		if !isString {
			relayError(w, r, http.StatusBadRequest, "etag must be a string.")
			return nil
		}
		etag = value
	}

	// §9 runs in both directions: a save token belongs to one response and one
	// tab, so relaying one hands another tab a credential that is not theirs.
	relayed := s.commit.canonicalForWrite(stripSaveToken(html))

	filePath, err := resolveWriteTarget(s.registry, name)
	if err != nil {
		status := http.StatusBadRequest
		var pathErr *PathError
		if errors.As(err, &pathErr) && pathErr.Status != 0 {
			status = pathErr.Status
		}
		relayError(w, r, status, err.Error())
		return nil
	}

	// Doc 11 §6b: the relay hands whole documents between tabs and never writes,
	// but a tab that cannot read a floor-raised document must not receive one.
	// With no rooms there is no authority's confirmed bytes, so the floor is the
	// document's own bytes on disk; an unreadable file is floor 0.
	floor := 0
	if disk, readErr := os.ReadFile(filePath); readErr == nil {
		floor = pageadmission.PageFormatFloor(string(disk))
	}
	if floor > pageadmission.FormatCapability(r.Header.Get(pageadmission.PageFormatHeader)) {
		relayAnswer(w, r, http.StatusConflict, orderedObject{
			{Key: "code", Value: pageadmission.PageFormatUpgrade},
			{Key: "msg", Value: "This document uses a newer page format; update the editor to keep writing."},
		})
		return nil
	}

	sender, _ := body.string("sender")
	if lane == "document" {
		// Viewers only, and nothing else happens: this artifact is not a
		// snapshot, so it must not reach the platform-sync cache, which exists to
		// upload the unstripped working state.
		s.live.broadcast(name, livePayload{HTML: relayed, Sender: sender}, "saved")
		relayAnswer(w, r, http.StatusOK, orderedObject{{Key: "success", Value: true}})
		return nil
	}

	// Cache the snapshot for platform sync, preserving any userDriven bit a prior
	// /save cached: the peer body does not carry it, and overwriting blindly
	// would drop the human-gesture provenance and make a clean save read as
	// ui-unknown.
	previous, _ := s.commit.store.snapshot(name)
	s.commit.store.setSnapshot(name, snapshotEntry{
		HTML:          &relayed,
		UserDriven:    previous.UserDriven,
		UserDrivenSet: previous.UserDrivenSet,
		Timestamp:     snapshotNow(),
	})

	// identityMap and etag ride this lane and only this one: an editor pairs
	// elements across a morph by stable id, and a stamp tells a receiver the
	// version its next save is answering. A viewer has neither.
	payload := livePayload{HTML: relayed, Sender: sender, Etag: etag}
	if len(identityMap) > 0 {
		payload.IdentityMap = identityMap
	}
	s.live.broadcast(name, payload, "live")

	relayAnswer(w, r, http.StatusOK, orderedObject{{Key: "success", Value: true}})
	return nil
}

// snapshotNow is the clock the relay stamps a snapshot with and the sweep reads.
var snapshotNow = func() int64 { return time.Now().UnixMilli() }

// snapshotSweepEvery is the interval Local's root-server pool runs the sweep on.
var snapshotSweepEvery = 5 * time.Minute

// sweepExpiredSnapshots is Local's sweepExpiredSnapshots: an entry older than
// five minutes is dropped from every served root's store, because nobody is
// going to upload it and the next save's provenance must not sit beside it.
func sweepExpiredSnapshots(now int64) {
	cutoff := now - 5*60*1000
	rootStoresMu.Lock()
	defer rootStoresMu.Unlock()
	for _, store := range rootStores {
		store.mu.Lock()
		for key, entry := range store.snapshots {
			if entry.Timestamp < cutoff {
				delete(store.snapshots, key)
			}
		}
		store.mu.Unlock()
	}
}

// startSnapshotSweep runs the sweep on Local's interval until the returned
// function is called.
func startSnapshotSweep() func() {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(snapshotSweepEvery)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				sweepExpiredSnapshots(snapshotNow())
			}
		}
	}()
	return func() { close(done) }
}

// getAndClearSnapshot is Local's getAndClearSnapshot: what a document owes the
// platform on its next sync upload, read once and cleared, because an upload
// that read it twice would send the same working state as two saves.
func (s *snapshotStore) getAndClearSnapshot(name string) (snapshotEntry, bool) {
	s.mu.Lock()
	entry, ok := s.snapshots[name]
	delete(s.snapshots, name)
	s.mu.Unlock()
	if !ok {
		return snapshotEntry{}, false
	}
	if entry.HTML != nil && *entry.HTML == "" {
		entry.HTML = nil
	}
	// Local returns null for an entry that names neither half: !entry.html &&
	// entry.userDriven === undefined.
	if entry.HTML == nil && !entry.UserDrivenSet {
		return snapshotEntry{}, false
	}
	return entry, true
}

// relayAddress answers POST /live-sync/save and POST /sync, on the bare address
// or under the `/_/` marker.
func relayAddress(urlPath string) bool {
	return urlPath == relaySpecPath || urlPath == relayLegacyPath
}

package server

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/dataguard"
	"github.com/panphora/htmlclay/internal/gridcodec"
	"github.com/panphora/htmlclay/internal/htmlutil"
	"github.com/panphora/htmlclay/internal/pageadmission"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/specwire"
	"github.com/panphora/htmlclay/internal/versions"
)

const maxDataWriteSize = 1 << 20

// writeErrorBody is the write face's error shape, shared with hyperclay-local's
// applySiteDataLocal: details carries the engine's structured refusals.
type writeErrorBody struct {
	Error       string           `json:"error"`
	Message     string           `json:"message"`
	Code        string           `json:"code,omitempty"`
	Details     any              `json:"details,omitempty"`
	Etag        string           `json:"etag,omitempty"`
	WouldRemove *wouldRemoveBody `json:"wouldRemove,omitempty"`
}

// wouldRemoveBody says what a refused program write would have taken, so the
// program can see why it has to read the document before writing.
type wouldRemoveBody struct {
	Items  int      `json:"items"`
	Values int      `json:"values"`
	Paths  []string `json:"paths"`
}

func writeWriteError(w http.ResponseWriter, status int, body writeErrorBody) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	dataHeaders(w)
	if body.Etag != "" {
		w.Header().Set("ETag", body.Etag)
	}
	w.WriteHeader(status)
	w.Write(bytes.TrimRight(buf.Bytes(), "\n"))
}

// handleDataFileWrite serves POST <page>?data={...}: the same write as the /_/api face, addressed
// the way a GET of the page is. Anything else on the catch-all keeps the 405 ServeMux used to give
// it, so this route adds an address rather than a second way into a document.
func (s *Server) handleDataFileWrite(w http.ResponseWriter, r *http.Request) {
	rawPath := r.PathValue("path")
	if rawPath == "_" || strings.HasPrefix(rawPath, "_/") ||
		!isExtractable(extractFilePath(rawPath)) || !rawQueryHasDataKey(r.URL.RawQuery) {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	// The path value is passed through unchanged: this is an alias for the API face, and both must
	// resolve "test.htmlclay" to the same file. Reserved paths never get here.
	s.handleDataAPIWrite(w, r)
}

// handleDataAPIWrite serves POST /_/api/<path>: apply a JSON body to the document through its own
// api rules tag, content only, and commit the result exactly as a save does.
func (s *Server) handleDataAPIWrite(w http.ResponseWriter, r *http.Request) {
	isBrowser, ok := wireCaller(r)
	if !ok {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		writeWriteError(w, http.StatusUnsupportedMediaType, writeErrorBody{
			Error:   "Unsupported Media Type",
			Message: "A JSON write takes Content-Type: application/json.",
		})
		return
	}

	rawPath := r.PathValue("path")
	if strings.Trim(rawPath, "/") == "" {
		writeWriteError(w, http.StatusBadRequest, writeErrorBody{
			Error:   "Missing path",
			Message: "Write to a file, for example /_/api/notes.htmlclay.",
		})
		return
	}
	relPath := extractFilePath(rawPath)
	if !isExtractable(relPath) {
		writeWriteError(w, http.StatusNotFound, writeErrorBody{
			Error:   "Unsupported file type",
			Message: "Only .html and .htmlclay files publish a data API.",
		})
		return
	}

	// Before any file is resolved or revealed: a ?data= caller writes through its own rules, and a
	// malformed one is refused on the query string alone.
	mode, answered := s.dataModeForQuery(w, r, relPath)
	if answered {
		return
	}
	if !mode.active() {
		mode = dataMode{face: faceAPI}
	}

	var f *session.File
	if isBrowser {
		// A page proves the target's token before anything else is resolved or revealed. It can
		// only hold a token for a registered file, so only registered files are looked up, and
		// every failure gives one answer, whether or not the file exists.
		if absPath, ok := s.resolveWriteTarget(relPath); ok {
			f, _ = s.sessions.LookupByPath(absPath)
		}
		if f == nil || subtle.ConstantTimeCompare([]byte(r.Header.Get(helperTokenHeader)), []byte(f.Token)) != 1 {
			writeWriteError(w, http.StatusForbidden, writeErrorBody{
				Error:   "Save-Token required",
				Message: "A page writes JSON only with the target file's Save-Token header.",
			})
			return
		}
		if s.trustedWriteRevoked(f) {
			writeWriteError(w, http.StatusForbidden, writeErrorBody{
				Error:   "Not writable",
				Message: "This document's folder is no longer trusted.",
			})
			return
		}
	} else {
		var status int
		var body writeErrorBody
		if f, status, body = s.writableFile(r, relPath); f == nil {
			writeWriteError(w, status, body)
			return
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxDataWriteSize)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeWriteError(w, http.StatusRequestEntityTooLarge, writeErrorBody{
				Error:   "Payload Too Large",
				Message: "The JSON body is limited to 1 MB.",
			})
			return
		}
		writeWriteError(w, http.StatusBadRequest, writeErrorBody{Error: "Invalid JSON body", Message: "The request body could not be read."})
		return
	}
	data, err := dataapi.ParseStrict(string(raw))
	if err != nil {
		writeWriteError(w, http.StatusBadRequest, writeErrorBody{Error: "Invalid JSON body", Message: "The request body is not valid JSON."})
		return
	}

	s.writeApplied(w, r, f, data, mode, isBrowser)
}

// resolveWriteTarget turns the request path into the absolute path a write would touch, or reports
// that it names nothing writable. It is text work plus symlink resolution, never registration.
func (s *Server) resolveWriteTarget(relPath string) (string, bool) {
	absPath, err := ValidatePath(relPath, s.sessions.HomeDir())
	if err != nil || s.isInternal(absPath) {
		return "", false
	}
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = filepath.Clean(resolved)
	}
	return absPath, true
}

// writableFile resolves the request path to a registered file this route may write, or answers why
// not. Every refusal for an unregistered path is decided on the path's text and the in-memory
// trusted-folder list, so it cannot reveal whether a file exists.
func (s *Server) writableFile(r *http.Request, relPath string) (*session.File, int, writeErrorBody) {
	notFound := writeErrorBody{Error: "Not Found", Message: "No such document."}
	absPath, ok := s.resolveWriteTarget(relPath)
	if !ok {
		return nil, http.StatusNotFound, notFound
	}

	f, ok := s.sessions.LookupByPath(absPath)
	if !ok && s.mayRegisterForWrite(absPath) {
		if url, routed := s.hooks.Route(absPath); routed {
			if f, ok = s.sessions.LookupByPath(absPath); !ok {
				return nil, http.StatusConflict, writeErrorBody{
					Error:   "Wrong origin",
					Message: "This document is served from " + url + ". Send the write there.",
				}
			}
		}
	}
	if !ok {
		return nil, http.StatusForbidden, writeErrorBody{
			Error:   "Not writable",
			Message: "Open this document in HTML Clay, or keep it in a trusted folder, before writing to it.",
		}
	}
	if s.trustedWriteRevoked(f) {
		return nil, http.StatusForbidden, writeErrorBody{
			Error:   "Not writable",
			Message: "This document's folder is no longer trusted.",
		}
	}
	return f, 0, writeErrorBody{}
}

// mayRegisterForWrite is shouldAutoRegister's path test without its navigation gate and its
// per-server cap: a write names one file explicitly, so it is not the fan-out the cap limits.
func (s *Server) mayRegisterForWrite(absPath string) bool {
	return s.hooks.Route != nil &&
		strings.EqualFold(filepath.Ext(absPath), ".htmlclay") &&
		s.trustedCovers(absPath) &&
		!session.HasHiddenComponent(s.sessions.HomeDir(), absPath) &&
		!s.isInternal(absPath)
}

// writeApplied is handleSave's commit sequence with the body computed from the stored bytes.
func (s *Server) writeApplied(w http.ResponseWriter, r *http.Request, f *session.File, data dataapi.Value, mode dataMode, isBrowser bool) {
	f.Lock()
	current, readErr := s.readRegisteredFile(f.AbsPath)
	if readErr != nil {
		f.Unlock()
		if errors.Is(readErr, errRefusedInternal) || errors.Is(readErr, os.ErrNotExist) {
			writeWriteError(w, http.StatusNotFound, writeErrorBody{Error: "Site content not found", Message: "The document has no content on disk."})
			return
		}
		s.logger.Printf("Data write could not read %s: %v", f.RelPath, readErr)
		writeWriteError(w, http.StatusInternalServerError, writeErrorBody{Error: "Write failed", Message: "Could not read the stored document."})
		return
	}

	if ifMatch, sent := listHeader(r, "If-Match"); sent && !specwire.IfMatchSatisfied(ifMatch, current) {
		etag := specwire.Etag(current)
		f.Unlock()
		w.Header().Set("ETag", etag)
		writeWriteError(w, http.StatusPreconditionFailed, writeErrorBody{
			Error:   "Precondition Failed",
			Message: f.Name + " changed since you read it. Nothing was written.",
		})
		return
	}

	if mode.face == faceAPI {
		stripped := htmlutil.StripToken(current)
		if rules, _, found, rErr := dataapi.DocumentRules(stripped, "api"); rErr == nil && found && dataapi.IsPatchBody(data, rules) {
			s.patchApplied(w, r, f, current, data, isBrowser)
			return
		}
	}

	var res *dataapi.WriteResult
	var err error
	if mode.face == faceQuery {
		mode.value, err = dataapi.ParseRelaxed(mode.rules)
		mode.resolved = err == nil
		if err == nil {
			res, err = dataapi.WriteDocumentWithRules(htmlutil.StripToken(current), data, mode.value)
		}
	} else {
		res, err = dataapi.WriteDocument(htmlutil.StripToken(current), data, "api")
	}
	if err != nil {
		f.Unlock()
		if mode.face == faceQuery {
			var parse *dataapi.RulesParseError
			var selector dataapi.SelectorFailure
			if errors.As(err, &parse) || errors.As(err, &selector) {
				status, body := mapDataError(err, faceQuery)
				writeDataError(w, status, body)
				return
			}
		}
		status, body := mapWriteError(err)
		if status == http.StatusInternalServerError {
			s.logger.Printf("Data write of %s failed: %v", f.RelPath, err)
		}
		writeWriteError(w, status, body)
		return
	}
	// A query face's own rules are judged against the bytes the engine produced, before
	// anything is written, so a write the caller could not read back never lands.
	if mode.face == faceQuery {
		doc, extractErr := dataapi.ParseBytes(res.HTML)
		if extractErr == nil {
			_, extractErr = doc.Extract(mode.value)
		}
		if extractErr != nil {
			f.Unlock()
			status, body := mapDataError(extractErr, faceQuery)
			writeDataError(w, status, body)
			return
		}
	}

	written := current
	key := ""
	if res.Changed {
		ifMatch, _ := listHeader(r, "If-Match")
		written, key, err = s.commitDataEdit(f, current, res.HTML, r.Host,
			dataguard.NamesExactVersion(ifMatch) && !isBrowser, !isBrowser && !dataguard.NamesExactVersion(ifMatch))
		if err != nil {
			f.Unlock()
			var refusal *dataEditError
			if errors.As(err, &refusal) {
				writeWriteError(w, refusal.status, refusal.body)
				return
			}
			writeWriteError(w, http.StatusInternalServerError, writeErrorBody{Error: "Write failed", Message: "write error"})
			return
		}
		f.NoteWriteFromOutsideTabs()
		if pErr := s.versions.SetProvisional(key, f.AbsPath, false); pErr != nil {
			s.logger.Printf("Could not clear provisional flag for %s: %v", f.RelPath, pErr)
		}
		s.coord.acceptServerReplacement(f)
		// Announced as an external change, not a save: no tab relays this write to the others,
		// so edit-mode tabs must hear it on the live lane, through the same dirty-region
		// protection a text editor's write gets.
		s.coord.publishExternalChange(f, f.Name+" was updated through the data API", forBrowser(written, key, s.documentURLFor(f)), specwire.Etag(written))
	}
	f.Unlock()

	if res.Changed {
		ifMatch, _ := listHeader(r, "If-Match")
		exact := dataguard.NamesExactVersion(ifMatch)
		s.observeWrite(f, key, current, written, dataguard.External, writeProof{IfMatched: exact, ProvenReader: exact && !isBrowser, FromPage: isBrowser})
		s.versions.MaybePrune(key, f.AbsPath)
		s.logger.Printf("Wrote data into %s (%d bytes, spliced=%v)", f.RelPath, len(written), res.Spliced)
	}
	s.writeExtracted(w, written, mode)
}

// patchApplied applies a sheet patch body. Caller holds f.Lock; patchApplied releases it.
func (s *Server) patchApplied(w http.ResponseWriter, r *http.Request, f *session.File, current []byte, body dataapi.Value, isBrowser bool) {
	patch, _ := body.(*dataapi.Object).Get("patch")
	res, err := dataapi.PatchDocument(htmlutil.StripToken(current), patch, dataapi.PatchOptions{Token: "api"})
	if err != nil {
		f.Unlock()
		status, errBody := mapWriteError(err)
		if status == http.StatusInternalServerError {
			s.logger.Printf("Sheet patch of %s failed: %v", f.RelPath, err)
		}
		writeWriteError(w, status, errBody)
		return
	}
	ifMatch, _ := listHeader(r, "If-Match")
	written, key, err := s.commitDataEdit(f, current, res.HTML, r.Host,
		dataguard.NamesExactVersion(ifMatch) && !isBrowser, !isBrowser && !dataguard.NamesExactVersion(ifMatch))
	if err != nil {
		f.Unlock()
		var refusal *dataEditError
		if errors.As(err, &refusal) {
			writeWriteError(w, refusal.status, refusal.body)
			return
		}
		writeWriteError(w, http.StatusInternalServerError, writeErrorBody{Error: "Write failed", Message: "write error"})
		return
	}
	f.NoteWriteFromOutsideTabs()
	if pErr := s.versions.SetProvisional(key, f.AbsPath, false); pErr != nil {
		s.logger.Printf("Could not clear provisional flag for %s: %v", f.RelPath, pErr)
	}
	s.coord.acceptServerReplacement(f)
	s.coord.publishExternalChange(f, f.Name+" was updated through the data API", forBrowser(written, key, s.documentURLFor(f)), specwire.Etag(written))
	f.Unlock()

	exact := dataguard.NamesExactVersion(ifMatch)
	s.observeWrite(f, key, current, written, dataguard.External, writeProof{IfMatched: exact, ProvenReader: exact && !isBrowser, FromPage: isBrowser})
	s.versions.MaybePrune(key, f.AbsPath)
	s.logger.Printf("Patched sheet cells in %s (%d cells, %d rows)", f.RelPath, res.Changed.Cells, res.Changed.Rows)

	etag := specwire.Etag(written)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(patchAnswer{Etag: etag, Changed: res.Changed, Recalculated: false, Stale: res.Stale}); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("ETag", etag)
	dataHeaders(w)
	w.WriteHeader(http.StatusOK)
	w.Write(bytes.TrimRight(buf.Bytes(), "\n"))
}

type patchAnswer struct {
	Etag         string            `json:"etag"`
	Changed      gridcodec.Changed `json:"changed"`
	Recalculated bool              `json:"recalculated"`
	Stale        bool              `json:"stale"`
}

// dataEditError is the refusal commitDataEdit answers with, so the write face sends
// the exact body it always has.
type dataEditError struct {
	status int
	body   writeErrorBody
}

func (e *dataEditError) Error() string { return e.body.Message }

// commitDataEdit commits an edit computed from the stored bytes: canonicalize, hold
// the page format floor, check the result against the page on disk, then back up and
// write it. It returns the stored bytes and the document's history key. Caller holds
// f.Lock.
//
// The stored document is the only source of an edit, so the bytes it produced can
// carry a runtime URL only if disk already held one. Canonicalizing before the
// admission check means what is judged is what would be written.
func (s *Server) commitDataEdit(f *session.File, current, html []byte, host string, provenReader, readFirst bool) ([]byte, string, error) {
	html = canonicalForWrite(host, html)
	html = []byte(pageadmission.WithPageFormatFloor(string(html),
		pageadmission.PageFormatFloor(string(current))))

	decision, admissionErr := pageadmission.CheckWrite(string(current), string(html), "")
	if admissionErr == nil && decision != pageadmission.Ordinary {
		admissionErr = &pageadmission.Refusal{Reason: "file-changed"}
	}
	if admissionErr != nil {
		_, code, reason := pageAdmissionFields(admissionErr)
		if code == "" {
			code = "blockeditor-format"
		}
		if reason == "" {
			reason = code
		}
		return nil, "", &dataEditError{status: http.StatusConflict, body: writeErrorBody{
			Error: "Page format", Message: admissionErr.Error(),
			Details: map[string]string{"code": code, "reason": reason},
		}}
	}

	written := []byte(pageadmission.StripPageTag(string(html)))
	if readFirst {
		if D := readRequiredWrite(current, written); D != nil {
			return nil, "", &dataEditError{status: http.StatusPreconditionRequired, body: writeErrorBody{
				Code:        "read-required",
				Error:       "Read first",
				Message:     "This write would remove data. Read the document first and send its etag as If-Match.",
				Etag:        specwire.Etag(current),
				WouldRemove: &wouldRemoveBody{Items: D.RemovedItems, Values: D.DestroyedLeaves, Paths: D.Paths},
			}}
		}
	}
	key, _ := s.ensureHistoryKeyLocked(f, current)
	// A proven reader is a program that read this exact version and computes its
	// edit from it, so its write replaces bytes nothing else holds. Those bytes have
	// to be kept first: if they cannot be, the write does not happen at all.
	if provenReader && !s.recentVersionsHold(key, f.AbsPath, current) {
		if bErr := s.backupVersion(key, f.AbsPath, current); bErr != nil {
			s.logger.Printf("Could not keep the current version of %s: %v", f.RelPath, bErr)
			return nil, "", &dataEditError{status: http.StatusServiceUnavailable, body: writeErrorBody{
				Code:    "recovery-unavailable",
				Error:   "Could not keep the current version",
				Message: "The current version could not be stored in history, so nothing was written. Try again.",
			}}
		}
	}
	if !s.versions.HasHistory(key, f.AbsPath) {
		if bErr := s.backupVersion(key, f.AbsPath, current); bErr != nil {
			s.logger.Printf("Pre-write backup failed for %s: %v", f.RelPath, bErr)
		}
	}
	if bErr := s.backupVersion(key, f.AbsPath, written); bErr != nil {
		s.logger.Printf("Backup failed for %s: %v", f.RelPath, bErr)
	}
	if err := atomicWriteFile(f.AbsPath, written); err != nil {
		s.logger.Printf("Error writing %s: %v", f.AbsPath, err)
		return nil, "", err
	}
	if sErr := s.versions.SettlePageState(key, f.AbsPath, written); sErr != nil {
		s.logger.Printf("Could not settle page state for %s: %v", f.RelPath, sErr)
	}
	f.RecordServerWrite(versions.Hash(written))
	f.NoteWriteByThisHost()
	return written, key, nil
}

// readRequiredWrite judges what a program's write would destroy between the
// document on disk and the bytes about to be written, with the same extraction
// and classification the guard uses. A nil answer means nothing would be removed.
func readRequiredWrite(current, written []byte) *dataguard.Destruction {
	ext := Extractor{}
	base, err := ext.Extract(current)
	if err != nil || dataguard.IsEmptyIsland(base) {
		return nil
	}
	inc, incErr := ext.Extract(written)
	D := dataguard.ClassifyDestruction(base, inc)
	if incErr != nil || D.AnyDestruction {
		return D
	}
	return nil
}

// backupVersion stores content as a version of the document, or fails the way an
// injected test hook says it should.
func (s *Server) backupVersion(key, absPath string, content []byte) error {
	if s.backupHook != nil {
		if err := s.backupHook(); err != nil {
			return err
		}
	}
	_, err := s.versions.Backup(key, absPath, content)
	return err
}

// recentVersionsChecked is how many of the newest versions recentVersionsHold looks
// through. A few, not one: the watcher and the page save lane both store versions of
// the same document, so a write caught mid-save lands after the copy it is checking.
const recentVersionsChecked = 3

// recentVersionsHold reports whether one of the newest few versions holds exactly
// content, so bytes already kept need no second copy.
func (s *Server) recentVersionsHold(key, absPath string, content []byte) bool {
	entries, err := s.versions.List(key, absPath)
	if err != nil {
		return false
	}
	if len(entries) > recentVersionsChecked {
		entries = entries[:recentVersionsChecked]
	}
	want := versions.Hash(content)
	for _, entry := range entries {
		body, err := s.versions.Read(key, absPath, entry.Name)
		if err != nil {
			continue
		}
		if versions.Hash(body) == want {
			return true
		}
	}
	return false
}

// mapWriteError gives the write engine's refusals the bodies hyperclay-local sends, and defers to
// mapDataError for the rules-tag failures both faces share.
func mapWriteError(err error) (int, writeErrorBody) {
	var noTag *dataapi.NoRulesTag
	if errors.As(err, &noTag) {
		return http.StatusBadRequest, writeErrorBody{Error: "No api rules tag", Message: `This page has no rules tag with data-rules-name~="api".`}
	}
	var rejected *dataapi.WriteRejected
	if errors.As(err, &rejected) {
		return http.StatusBadRequest, writeErrorBody{Error: "Write rejected", Message: err.Error(), Details: map[string]any{
			"unknownKeys": rejected.UnknownKeys, "unmatched": rejected.Unmatched,
		}}
	}
	var refused *dataapi.WriteRefused
	if errors.As(err, &refused) {
		return http.StatusBadRequest, writeErrorBody{Error: "Write refused", Message: err.Error(), Details: refused.Refusals}
	}
	var shape *dataapi.ShapeMismatch
	if errors.As(err, &shape) {
		return http.StatusBadRequest, writeErrorBody{Error: "Shape mismatch", Message: err.Error(), Details: shape.Mismatches}
	}
	var empty *dataapi.EmptyListInsert
	if errors.As(err, &empty) {
		return http.StatusBadRequest, writeErrorBody{Error: "Cannot grow list", Message: err.Error(), Details: empty.Path}
	}
	var readOnly *dataapi.RuleTargetReadOnly
	if errors.As(err, &readOnly) {
		return http.StatusBadRequest, writeErrorBody{Error: "Read-only rule", Message: err.Error()}
	}
	status, body := mapDataError(err, faceAPI)
	return status, writeErrorBody{Error: body.Error, Message: body.Message, Code: body.Code}
}

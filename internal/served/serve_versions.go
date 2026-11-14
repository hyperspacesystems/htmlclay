package served

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/pageadmission"
	"github.com/panphora/htmlclay/internal/specwire"
)

// Local's three version-history routes, addressed by the document's path under
// the served folder. This host mints no tokens, so the path IS the identity, the
// same way the static route and Document-URL name a document. The store is the
// one the save path writes, `.hyperclay/versions/<rootRel without extension>/`.
//
// Each route answers ONLY through the `/_/` marker, so a user folder actually
// named `versions` keeps serving normally.

var (
	// docTailPattern and docVersionPattern are Local's DOC_TAIL and
	// DOC_AND_VERSION: everything up to the page extension is the document, and a
	// version name is one path segment after it.
	docTailPattern    = regexp.MustCompile(`(?i)^(.+\.(?:html|htmlclay))$`)
	docVersionPattern = regexp.MustCompile(`(?i)^(.+\.(?:html|htmlclay))/([^/]+)$`)
)

// specErrorCodes is Local's SPEC_ERROR_CODES: a code only where the spec's
// registry names one for the status.
var specErrorCodes = map[int]string{
	http.StatusForbidden:             "forbidden",
	http.StatusNotFound:              "not-found",
	http.StatusRequestEntityTooLarge: "too-large",
	http.StatusUnsupportedMediaType:  "unsupported-type",
	http.StatusUnprocessableEntity:   "invalid-document",
}

// routeTail is the part of a route's path after its prefix. Express matches
// these routes with `(.+)`, so an empty tail is not a match and the request goes
// on to the catch-all.
func routeTail(urlPath, prefix string) (string, bool) {
	if !strings.HasPrefix(urlPath, prefix) {
		return "", false
	}
	rest := urlPath[len(prefix):]
	return rest, rest != ""
}

// versionsDirFor is Local's versionsDirFor: the document's path under the served
// folder without its extension, which is what the save path backs up under.
func versionsDirFor(baseDir, rel string) string {
	return filepath.Join(baseDir, versionsDirName, documentBase(rel))
}

func recoveredDirFor(baseDir, rel string) string {
	return filepath.Join(baseDir, recoveredDirName, documentBase(rel))
}

func recoveredVersionName(name string) bool {
	return strings.HasPrefix(name, "recovered-")
}

func storedVersionName(name string) string {
	if recoveredVersionName(name) {
		return strings.TrimPrefix(name, "recovered-")
	}
	return name
}

// isVersionFileName is Local's isVersionName: exactly one generated filename,
// and a document. Anything else is refused rather than resolved, so no name can
// address something outside the store.
func isVersionFileName(name string) bool {
	stored := storedVersionName(name)
	return IsVersionName(stored) && strings.HasSuffix(strings.ToLower(stored), ".html")
}

// versionFilePath is Local's versionsDirReal: the store's directory as a real
// directory chain, so a directory symlink planted under `.hyperclay` cannot
// redirect a restore read out of tree.
func versionFilePath(baseDir, rel, versionName string) (string, error) {
	canonicalBase, err := canonicalizeBase(baseDir)
	if err != nil {
		return "", err
	}
	dir := versionsDirFor(baseDir, rel)
	if recoveredVersionName(versionName) {
		dir = recoveredDirFor(baseDir, rel)
	}
	chain, err := rebaseOntoCanonical(canonicalBase, baseDir, dir)
	if err != nil {
		return "", err
	}
	if err := assertRealDirChain(canonicalBase, chain); err != nil {
		return "", err
	}
	return filepath.Join(chain, storedVersionName(versionName)), nil
}

// versionHistoryEntry is one entry of the listing, before it is written out in
// htmlclay's shape.
type versionHistoryEntry struct {
	Name      string
	Time      string
	Order     int64
	Seq       int64
	Size      int64
	Recovered bool
	// Pinned is a version the guard kept because the next change removed data.
	Pinned bool
}

// The two labels of a pinned version, as the hosted history words them.
const (
	pinnedVersionLabel  = "Before an outside change"
	pinnedVersionDetail = "Kept because the next change removed data."
)

// answer is the entry as the panel reads it.
func (e versionHistoryEntry) answer() orderedObject {
	row := orderedObject{
		{Key: "name", Value: e.Name},
		{Key: "time", Value: e.Time},
		{Key: "seq", Value: e.Seq},
		{Key: "size", Value: e.Size},
	}
	if e.Recovered {
		row = append(row, kv{Key: "kind", Value: "recovered"})
	}
	if e.Pinned {
		row = append(row,
			kv{Key: "pinned", Value: true},
			kv{Key: "label", Value: pinnedVersionLabel},
			kv{Key: "detail", Value: pinnedVersionDetail},
		)
	}
	return row
}

// isoTimestamp is Date.prototype.toISOString: UTC, milliseconds always written.
func isoTimestamp(ms int64) string {
	return time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z")
}

// listVersionEntries is the history of one document store, newest first, in
// htmlclay's entry shape. The instant is parsed rather than read off the name (a
// name carries a zone), so two versions written either side of a DST fall-back
// still order correctly.
func listVersionEntries(dir string, recovered bool) []versionHistoryEntry {
	raw, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	found := make([]VersionEntry, 0, len(raw))
	for _, item := range raw {
		name := item.Name()
		if !isVersionFileName(name) {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		found = append(found, VersionEntry{Name: name, Full: filepath.Join(dir, name), MtimeMs: info.ModTime().UnixMilli()})
	}
	sortNewestFirst(found)
	entries := make([]versionHistoryEntry, 0, len(found))
	for _, entry := range found {
		info, err := os.Stat(entry.Full)
		if err != nil {
			continue
		}
		name := entry.Name
		listed := versionHistoryEntry{
			Name:      name,
			Order:     VersionSortKey(entry),
			Seq:       CollisionSuffix(name),
			Size:      info.Size(),
			Recovered: recovered,
			Pinned:    !recovered && versionPinned(dir, name),
		}
		listed.Time = isoTimestamp(listed.Order)
		if recovered {
			listed.Name = "recovered-" + name
		}
		entries = append(entries, listed)
	}
	return entries
}

// VersionRow is one kept version of a served document, in the same item shape
// the loose store answers: id, createdAt, size, kind, pinned, restorable.
type VersionRow struct {
	ID         string
	CreatedAt  time.Time
	Size       int64
	Kind       string
	Pinned     bool
	Restorable bool
}

// servedVersionRow is one version as the read page orders it.
type servedVersionRow struct {
	value VersionRow
	order servedVersionOrder
}

// servedVersionOrder is the whole ordering key of one served version: the
// instant, then the collision suffix, then the id. Two versions written in one
// millisecond need the suffix, and a kept version and a recovered one can share
// both, so the id is the tiebreak that makes the key total and lets a page
// boundary never repeat or skip a row.
type servedVersionOrder struct {
	instant int64
	seq     int64
	id      string
}

// newerThan is the one comparison: newest first by instant, then suffix, then
// id. The sort and the before cursor both use it.
func (o servedVersionOrder) newerThan(other servedVersionOrder) bool {
	if o.instant != other.instant {
		return o.instant > other.instant
	}
	if o.seq != other.seq {
		return o.seq > other.seq
	}
	return o.id > other.id
}

// versions answers one page of a document's kept versions, newest first, in the
// shape the loose store answers too. It reads and never writes: no version, no
// pin and no document is created, and a document that is gone still answers its
// history. before names one version, an empty before starts at the newest row.
func (s *rootServer) versions(rel, before string, limit int) ([]VersionRow, error) {
	if _, err := validateSegments(rel); err != nil {
		return nil, err
	}
	if !isDocumentName(rel) {
		return nil, invalidFilePathError()
	}
	filePath, err := resolveWritePath(s.registry, rel)
	if err != nil {
		return nil, err
	}
	// A document that is gone is not restorable: a restore would create it, and
	// reading its history gives no right to do that.
	restorable := false
	if _, statErr := os.Lstat(filePath); statErr == nil {
		restorable = true
	}

	rows := make([]servedVersionRow, 0, 4)
	for _, store := range []struct {
		dir       string
		recovered bool
	}{
		{versionsDirFor(s.baseDir, rel), false},
		{recoveredDirFor(s.baseDir, rel), true},
	} {
		kind := "saved"
		if store.recovered {
			kind = "recovered"
		}
		for _, entry := range listVersionEntries(store.dir, store.recovered) {
			rows = append(rows, servedVersionRow{
				value: VersionRow{
					ID:         entry.Name,
					CreatedAt:  time.UnixMilli(entry.Order).UTC(),
					Size:       entry.Size,
					Kind:       kind,
					Pinned:     entry.Pinned,
					Restorable: restorable,
				},
				order: servedVersionOrder{instant: entry.Order, seq: entry.Seq, id: entry.Name},
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].order.newerThan(rows[j].order) })
	if before != "" {
		cursor, err := servedVersionCursor(rows, before)
		if err != nil {
			return nil, err
		}
		older := make([]servedVersionRow, 0, len(rows))
		for _, r := range rows {
			if cursor.newerThan(r.order) {
				older = append(older, r)
			}
		}
		rows = older
	}
	if limit >= 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]VersionRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.value)
	}
	return out, nil
}

// servedVersionCursor is the ordering key of the version named before. A name
// carries its own instant and suffix, so a cursor still lands correctly after
// its version has been pruned; a legacy name with no zone has no instant of its
// own and is read from the listing while that version is still there.
func servedVersionCursor(rows []servedVersionRow, before string) (servedVersionOrder, error) {
	for _, r := range rows {
		if r.value.ID == before {
			return r.order, nil
		}
	}
	stored := storedVersionName(before)
	instant, ok := ParseVersionTimestamp(stored)
	if !ok {
		return servedVersionOrder{}, fmt.Errorf("served: not a version name: %q", before)
	}
	return servedVersionOrder{instant: instant, seq: CollisionSuffix(stored), id: before}, nil
}

// serveVersions is Local's GET /_/versions/<rootRel>:
// `{ ok, name, versions: [{ name, time, seq, size }] }`.
func (s *rootServer) serveVersions(w http.ResponseWriter, r *http.Request, tail string) error {
	match := docTailPattern.FindStringSubmatch(tail)
	if match == nil {
		return versionErrorAnswer(w, r, http.StatusBadRequest, "not an HTML document")
	}
	rel := match[1]
	filePath, err := resolveWriteTarget(s.registry, rel)
	if err != nil {
		return versionErrorAnswer(w, r, versionErrorStatus(err), "document not found")
	}
	versions := append(
		listVersionEntries(versionsDirFor(s.baseDir, rel), false),
		listVersionEntries(recoveredDirFor(s.baseDir, rel), true)...,
	)
	sort.SliceStable(versions, func(i, j int) bool {
		if versions[i].Order != versions[j].Order {
			return versions[i].Order > versions[j].Order
		}
		return versions[i].Seq > versions[j].Seq
	})
	rows := make([]orderedObject, 0, len(versions))
	for _, entry := range versions {
		rows = append(rows, entry.answer())
	}
	w.Header().Set("Cache-Control", "no-store")
	return versionJSON(w, r, http.StatusOK, orderedObject{
		{Key: "ok", Value: true},
		{Key: "name", Value: filepath.Base(filePath)},
		{Key: "versions", Value: rows},
	})
}

// serveVersion is Local's GET /_/version/<rootRel>/<versionName>: the version's
// own bytes, as a document.
func (s *rootServer) serveVersion(w http.ResponseWriter, r *http.Request, tail string) error {
	match := docVersionPattern.FindStringSubmatch(tail)
	if match == nil {
		return versionErrorAnswer(w, r, http.StatusBadRequest, "not an HTML document")
	}
	rel, versionName := match[1], match[2]
	if !isVersionFileName(versionName) {
		return versionErrorAnswer(w, r, http.StatusBadRequest, "invalid version name")
	}
	if _, err := resolveWriteTarget(s.registry, rel); err != nil {
		return versionErrorAnswer(w, r, versionErrorStatus(err), "document not found")
	}
	filePath, err := versionFilePath(s.baseDir, rel, versionName)
	if err == nil {
		var data []byte
		if data, err = os.ReadFile(filePath); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			sendBytes(w, r, http.StatusOK, data)
			return nil
		}
	}
	return versionErrorAnswer(w, r, http.StatusNotFound, "version not found")
}

// versionRefusal is one refusal of the restore route, carried out of the locked
// region so the answer is written after the lock is released.
type versionRefusal struct {
	Status  int
	Message string
	Etag    string
}

// serveRestore is Local's POST /_/restore/<rootRel>/<versionName>: put one
// version back.
func (s *rootServer) serveRestore(w http.ResponseWriter, r *http.Request, tail string) error {
	match := docVersionPattern.FindStringSubmatch(tail)
	if match == nil {
		return versionErrorAnswer(w, r, http.StatusBadRequest, "not an HTML document")
	}
	rel, versionName := match[1], match[2]
	if !isVersionFileName(versionName) {
		return versionErrorAnswer(w, r, http.StatusBadRequest, "invalid version name")
	}
	filePath, err := resolveWriteTarget(s.registry, rel)
	if err != nil {
		return versionErrorAnswer(w, r, versionErrorStatus(err), "document not found")
	}

	// A1: the whole read-decide-write region holds the same canonical-path queue
	// slot a concurrent /save would need.
	unlock := s.commit.locks.lock(filePath)
	defer unlock()

	refusal, restoredEtag, writeErr := s.restoreVersion(r, rel, versionName, filePath)
	if writeErr != nil {
		var coded *codedWriteError
		if errors.As(writeErr, &coded) && coded.code == "moved-or-deleted" {
			return versionErrorAnswer(w, r, http.StatusConflict, coded.message)
		}
		return versionErrorAnswer(w, r, http.StatusInternalServerError, "write error")
	}
	if refusal != nil {
		// A refusal that knows the document's current stamp carries it, so a
		// client can recover in one round trip.
		if refusal.Etag != "" {
			return versionJSON(w, r, refusal.Status, orderedObject{
				{Key: "ok", Value: false},
				{Key: "error", Value: refusal.Message},
				{Key: "msg", Value: refusal.Message},
				{Key: "msgType", Value: "error"},
				{Key: "code", Value: "conflict"},
				{Key: "etag", Value: refusal.Etag},
			})
		}
		return versionErrorAnswer(w, r, refusal.Status, refusal.Message)
	}
	w.Header().Set("Cache-Control", "no-store")
	body := orderedObject{
		{Key: "ok", Value: true},
		{Key: "msg", Value: "Restored " + versionName},
		{Key: "msgType", Value: "success"},
	}
	// §6: a client that sent If-Match gets the stamp its write produced, so its
	// next save can be conditional too.
	if headerPresent(r, "If-Match") {
		body = append(body, kv{Key: "etag", Value: restoredEtag})
	}
	return versionJSON(w, r, http.StatusOK, body)
}

// restoreVersion is the locked half of the restore route: judge the request
// against the live bytes, read the version, and publish it.
func (s *rootServer) restoreVersion(r *http.Request, rel, versionName, filePath string) (*versionRefusal, string, error) {
	// The safety backup is mandatory, so a live file that exists but cannot be
	// read is a hard refusal rather than a skipped backup: proceeding would
	// destroy the bytes with no recovery copy. A file that is simply absent has
	// nothing to lose.
	current, err := currentLogical(s.baseDir, rel, filePath)
	if err != nil {
		return &versionRefusal{
			Status:  http.StatusInternalServerError,
			Message: "current file cannot be read, so no safety backup is possible",
		}, "", nil
	}
	currentBytes := []byte{}
	if current != nil {
		currentBytes = []byte(*current)
	}
	if headerPresent(r, "If-Match") && !specwire.IfMatchSatisfied(r.Header.Get("If-Match"), currentBytes) {
		return &versionRefusal{
			Status:  http.StatusPreconditionFailed,
			Message: "document changed since it was loaded",
			Etag:    specwire.Etag(currentBytes),
		}, "", nil
	}

	versionPath, err := versionFilePath(s.baseDir, rel, versionName)
	if err != nil {
		return &versionRefusal{Status: http.StatusNotFound, Message: "version not found"}, "", nil
	}
	data, err := os.ReadFile(versionPath)
	if err != nil {
		return &versionRefusal{Status: http.StatusNotFound, Message: "version not found"}, "", nil
	}
	if int64(len(data)) > saveMaxBytes {
		return &versionRefusal{Status: http.StatusRequestEntityTooLarge, Message: "version is too large to restore"}, "", nil
	}

	// Stripped rather than trusted: a restore must never write a save token to
	// disk, wherever the version's bytes came from.
	html := stripSaveToken(string(data))
	// Without rooms: Local stamps the stored page record's revision onto these
	// bytes so open tabs merge the restore onto the page they hold, and keeps the
	// blockeditor-format floor the live document had established.
	state, err := readPageState(s.baseDir, rel, PageStateReadOptions{})
	if err != nil {
		return nil, "", err
	}
	// A restore is a person's explicit choice, so it is never refused; it only
	// keeps the floor the document already established.
	storedFloor := pageadmission.PageFormatFloor(string(currentBytes))
	stamped := ""
	if state != nil {
		if _, versionState := splitPage(html); versionState == nil {
			floor := storedFloor
			if carried := pageadmission.PageFormatFloor(html); carried > floor {
				floor = carried
			}
			stamped, _ = stampPageRev(html, state.Rev, floor)
		}
	}
	edit := stamped != ""
	if edit {
		html = stamped
	} else {
		html = pageadmission.WithPageFormatFloor(html, storedFloor)
	}
	if !hasHTMLRoot(html) {
		return &versionRefusal{
			Status:  http.StatusUnprocessableEntity,
			Message: "version is not a complete HTML document",
		}, "", nil
	}

	published, err := s.publishHostWrite(rel, filePath, html, edit, true, "restore")
	if err != nil {
		return nil, "", err
	}
	if published.Outcome == "refused" {
		switch published.Reason {
		case "room-unavailable":
			return &versionRefusal{
				Status:  http.StatusInternalServerError,
				Message: "live room cannot be backed up",
			}, "", nil
		case "backup-failed":
			return &versionRefusal{
				Status:  http.StatusInternalServerError,
				Message: "could not create a safety backup",
			}, "", nil
		}
		if published.Refusal != nil {
			return &versionRefusal{
				Status:  published.Refusal.Status,
				Message: "restore was refused: " + published.Reason,
			}, "", nil
		}
	}
	content, _ := published.Content.(string)
	// Without live sync there is no edit-mode tab to announce the restore to;
	// the saved-lane broadcast the replace lane makes is the whole of it.
	return nil, specwire.Etag([]byte(content)), nil
}

// versionErrorStatus is Local's `error.status || 404` for a refused document.
func versionErrorStatus(err error) int {
	var pathErr *PathError
	if errors.As(err, &pathErr) && pathErr.Status != 0 {
		return pathErr.Status
	}
	return http.StatusNotFound
}

// versionErrorAnswer is Local's versionError: both spellings of the message
// clients read, and a code only where the spec's registry names one.
func versionErrorAnswer(w http.ResponseWriter, r *http.Request, status int, message string) error {
	w.Header().Set("Cache-Control", "no-store")
	body := orderedObject{
		{Key: "ok", Value: false},
		{Key: "error", Value: message},
		{Key: "msg", Value: message},
		{Key: "msgType", Value: "error"},
	}
	if code, ok := specErrorCodes[status]; ok {
		body = append(body, kv{Key: "code", Value: code})
	}
	return versionJSON(w, r, status, body)
}

// versionJSON writes one JSON answer of the history routes.
func versionJSON(w http.ResponseWriter, r *http.Request, status int, body orderedObject) error {
	encoded, err := jsonStringify(body)
	if err != nil {
		return err
	}
	sendJSON(w, r, status, encoded)
	return nil
}

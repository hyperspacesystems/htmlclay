//go:build managedtrial

package managedtrial

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The page editor's merge state for one document, kept out of the person's
// file. Ported from Local's src/main/page-state-store.js: a record is
// { v: 1, rev, tag }, the shape splitPage and composePage use, and the sidecar
// beside a document holds the newest record plus the one before it, so a write
// that never landed still leaves the file on disk with its own state.

// pageStateDirName is Local's artifact-paths.js PAGE_STATE_DIR.
const pageStateDirName = ".hyperclay/page-state"

// pageExtension is Local's PAGE_EXTENSION: the document extensions whose
// sidecar names may also carry the older extension-less form.
var pageExtension = regexp.MustCompile(`(?i)\.(html|htmlclay)$`)

// pageGenerationPattern is Local's GENERATION: 32 lowercase hex characters.
var pageGenerationPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// pageStateNewGeneration is Local's newGeneration, crypto.randomBytes(16) in
// hex. It is a variable so the golden test can replay the generator's stub.
var pageStateNewGeneration = func() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}

// PageState is one stored sidecar: the newest record, the record before it, and
// the generation naming the page lineage both belong to.
type PageState struct {
	V          int            `json:"v"`
	Rev        string         `json:"rev"`
	Tag        string         `json:"tag"`
	Generation string         `json:"generation,omitempty"`
	Prev       *PageStatePrev `json:"prev,omitempty"`
}

// PageStatePrev is the record the sidecar keeps beside the newest one.
type PageStatePrev struct {
	Rev        string `json:"rev"`
	Tag        string `json:"tag"`
	Generation string `json:"generation,omitempty"`
}

// PageStateRecord is the stored record for one article revision, generation
// included: the answer crash recovery reads.
type PageStateRecord struct {
	Rev        string `json:"rev"`
	Tag        string `json:"tag"`
	Generation string `json:"generation"`
}

// PageStateReadOptions is readPageState's options: rev is the article revision
// of the file the caller holds, and exact asks for that revision's record only,
// never the newest one in its place.
type PageStateReadOptions struct {
	Rev   string
	Exact bool
}

// pageStateSlot is one record a write can descend from, generation included, so
// a slot can be the newest record or the record before it.
type pageStateSlot struct {
	Rev        string
	Tag        string
	Generation string
}

func validGeneration(generation string) bool {
	return pageGenerationPattern.MatchString(generation)
}

// pageStateJSON encodes a sidecar the way JSON.stringify does: no HTML escaping
// and no trailing newline, so the bytes on disk match Local's exactly.
func pageStateJSON(state *PageState) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(state); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// pageStateContainedPath resolves a joined page-state path and refuses one that
// escapes the page-state directory.
func pageStateContainedPath(baseDir, target string) (string, error) {
	root := upathResolve(upathJoin(baseDir, pageStateDirName))
	abs := upathResolve(target)
	if !strings.HasPrefix(abs, root+"/") {
		return "", errors.New("Page state path escapes base directory")
	}
	return abs, nil
}

// pageStatePath is the sidecar for one document: its complete name with .json
// appended, so two extensions of one basename keep separate records.
func pageStatePath(baseDir, name string) (string, error) {
	return pageStateContainedPath(baseDir, upathJoin(baseDir, pageStateDirName, name+".json"))
}

// legacyPageStatePath is the name a sidecar carried before names kept their
// extension: notes.html shared notes.json.
func legacyPageStatePath(baseDir, name string) (string, error) {
	stripped := pageExtension.ReplaceAllString(name, "")
	return pageStateContainedPath(baseDir, upathJoin(baseDir, pageStateDirName, stripped+".json"))
}

// pageStateDirPath is the mirror directory a folder's page state lives in.
func pageStateDirPath(baseDir, name string) (string, error) {
	root := upathResolve(upathJoin(baseDir, pageStateDirName))
	abs := upathResolve(upathJoin(baseDir, pageStateDirName, name))
	if abs != root && !strings.HasPrefix(abs, root+"/") {
		return "", errors.New("Page state path escapes base directory")
	}
	return abs, nil
}

// pageStateExists is Local's fs.access probe: a path that stats is there.
func pageStateExists(target string) bool {
	_, err := os.Stat(target)
	return err == nil
}

// pageStateTruthy is JavaScript truthiness over a decoded JSON value.
func pageStateTruthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case float64:
		return typed != 0
	case string:
		return typed != ""
	}
	return true
}

// pageStateVersionOne is Local's `state.v === 1` over a decoded JSON value.
func pageStateVersionOne(value any) bool {
	number, ok := value.(float64)
	return ok && number == 1
}

// pageStatePrevFrom reads the record kept beside the newest one. Local checks it
// as validRecord({ v: 1, ...state.prev }), so a v it carries must still be 1.
func pageStatePrevFrom(raw any) *PageStatePrev {
	object, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	if version, present := object["v"]; present && !pageStateVersionOne(version) {
		return nil
	}
	rev, ok := object["rev"].(string)
	if !ok {
		return nil
	}
	tag, ok := object["tag"].(string)
	if !ok {
		return nil
	}
	prev := &PageStatePrev{Rev: rev, Tag: tag}
	if generation, ok := object["generation"].(string); ok && validGeneration(generation) {
		prev.Generation = generation
	}
	return prev
}

// parseState reads a sidecar's text, or nothing when it is not a valid record.
// Every field is checked against the decoded JSON rather than against a Go
// struct, so a wrong-typed extra field is dropped exactly the way Local drops
// it instead of failing the whole record.
func parseState(text string) *PageState {
	var raw any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	if !pageStateVersionOne(object["v"]) {
		return nil
	}
	rev, ok := object["rev"].(string)
	if !ok {
		return nil
	}
	tag, ok := object["tag"].(string)
	if !ok {
		return nil
	}
	state := &PageState{V: 1, Rev: rev, Tag: tag}
	if generation, ok := object["generation"].(string); ok && validGeneration(generation) {
		state.Generation = generation
	}
	if prev, present := object["prev"]; present && pageStateTruthy(prev) {
		state.Prev = pageStatePrevFrom(prev)
	}
	return state
}

// pick is the record for one article revision, or the newest one when that
// revision has none of its own.
func pick(state *PageState, rev string) *PageRecord {
	if state == nil {
		return nil
	}
	if rev != "" && state.Rev != rev && state.Prev != nil && state.Prev.Rev == rev {
		return &PageRecord{V: 1, Rev: state.Prev.Rev, Tag: state.Prev.Tag}
	}
	return &PageRecord{V: 1, Rev: state.Rev, Tag: state.Tag}
}

// recordFor is the record that belongs to one article revision, or nothing when
// the sidecar keeps none for it.
func recordFor(state *PageState, rev string) *PageRecord {
	if state == nil || rev == "" {
		return nil
	}
	if state.Rev == rev {
		return &PageRecord{V: 1, Rev: rev, Tag: state.Tag}
	}
	if state.Prev != nil && state.Prev.Rev == rev {
		return &PageRecord{V: 1, Rev: rev, Tag: state.Prev.Tag}
	}
	return nil
}

// adoptLegacyPageState adopts an old-format sidecar once, and only when the
// other extension's document is not also beside it: two documents sharing one
// record would merge into each other.
func adoptLegacyPageState(baseDir, name string) (*PageState, error) {
	extension := pageExtension.FindString(name)
	if extension == "" {
		return nil, nil
	}
	other := ".html"
	if strings.ToLower(extension) == ".html" {
		other = ".htmlclay"
	}
	if pageStateExists(upathJoin(baseDir, name[:len(name)-len(extension)]+other)) {
		return nil, nil
	}
	legacy, err := legacyPageStatePath(baseDir, name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(legacy)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	state := parseState(string(data))
	if state == nil {
		return nil, nil
	}
	target, err := pageStatePath(baseDir, name)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o777); err != nil {
		return nil, err
	}
	if err := os.Rename(legacy, target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return state, nil
}

// readStored is the sidecar's state, adopting an old-format one on first touch.
func readStored(baseDir, name string) (*PageState, error) {
	target, err := pageStatePath(baseDir, name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return adoptLegacyPageState(baseDir, name)
		}
		return nil, err
	}
	return parseState(string(data)), nil
}

// readPageRecord is the stored record for one article revision, generation
// included: only ever a record that names its page lineage.
func readPageRecord(baseDir, name, rev string) (*PageStateRecord, error) {
	stored, err := readStored(baseDir, name)
	if err != nil {
		return nil, err
	}
	if stored == nil || rev == "" {
		return nil, nil
	}
	var slot *pageStateSlot
	switch {
	case stored.Rev == rev:
		slot = &pageStateSlot{Rev: stored.Rev, Tag: stored.Tag, Generation: stored.Generation}
	case stored.Prev != nil && stored.Prev.Rev == rev:
		slot = &pageStateSlot{Rev: stored.Prev.Rev, Tag: stored.Prev.Tag, Generation: stored.Prev.Generation}
	}
	if slot == nil || !validGeneration(slot.Generation) {
		return nil, nil
	}
	return &PageStateRecord{Rev: slot.Rev, Tag: slot.Tag, Generation: slot.Generation}, nil
}

// readPageState is the record a reader sees: the one for its revision, or the
// newest record when exact is false.
func readPageState(baseDir, name string, options PageStateReadOptions) (*PageRecord, error) {
	stored, err := readStored(baseDir, name)
	if err != nil {
		return nil, err
	}
	if options.Exact {
		return recordFor(stored, options.Rev), nil
	}
	return pick(stored, options.Rev), nil
}

// diskRev is the article revision of the file on disk, or nothing.
func diskRev(baseDir, name string) string {
	data, err := os.ReadFile(upathJoin(baseDir, name))
	if err != nil {
		return ""
	}
	return pageRev(string(data))
}

// writePageState publishes the sidecar before the caller writes the file. The
// record kept beside the new one is the one for the file on disk, because the
// newest record may be for a write that never landed.
func writePageState(baseDir, name string, state *PageRecord, durable bool) error {
	target, err := pageStatePath(baseDir, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o777); err != nil {
		return err
	}
	existing, err := readStored(baseDir, name)
	if err != nil {
		existing = nil
	}
	onDisk := diskRev(baseDir, name)
	slotFor := func(rev string) *pageStateSlot {
		if existing == nil || rev == "" {
			return nil
		}
		if existing.Rev == rev {
			return &pageStateSlot{Rev: existing.Rev, Tag: existing.Tag, Generation: existing.Generation}
		}
		if existing.Prev != nil && existing.Prev.Rev == rev {
			return &pageStateSlot{Rev: existing.Prev.Rev, Tag: existing.Prev.Tag, Generation: existing.Prev.Generation}
		}
		return nil
	}
	var kept *PageRecord
	if onDisk != "" && onDisk != state.Rev {
		kept = recordFor(existing, onDisk)
	}
	var prevSlot *pageStateSlot
	switch {
	case kept != nil:
		prevSlot = slotFor(kept.Rev)
	case existing != nil && existing.Rev != state.Rev:
		prevSlot = slotFor(existing.Rev)
	case existing != nil && existing.Prev != nil:
		prevSlot = &pageStateSlot{Rev: existing.Prev.Rev, Tag: existing.Prev.Tag, Generation: existing.Prev.Generation}
	}
	next := &PageState{V: 1, Rev: state.Rev, Tag: state.Tag}
	if prevSlot != nil {
		next.Prev = &PageStatePrev{Rev: prevSlot.Rev, Tag: prevSlot.Tag}
		if validGeneration(prevSlot.Generation) {
			next.Prev.Generation = prevSlot.Generation
		}
	}
	// The new record descends from the one for the file on disk, so it keeps that
	// page's lineage; a file replaced from outside descends from nothing.
	base := slotFor(onDisk)
	if base == nil {
		base = slotFor(state.Rev)
	}
	generation := ""
	if base != nil {
		generation = base.Generation
	}
	if generation == "" {
		generation = pageStateNewGeneration()
	}
	next.Generation = generation
	data, err := pageStateJSON(next)
	if err != nil {
		return err
	}
	return atomicWriteFile(target, data, nil, durable)
}

// dropPageState removes one document's sidecar, missing or not.
func dropPageState(baseDir, name string) error {
	target, err := pageStatePath(baseDir, name)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// movePageState carries a document's sidecar to its new name.
func movePageState(baseDir, fromName, toName string) error {
	from, err := pageStatePath(baseDir, fromName)
	if err != nil {
		return err
	}
	to, err := pageStatePath(baseDir, toName)
	if err != nil {
		return err
	}
	if !pageStateExists(from) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o777); err != nil {
		return err
	}
	if err := os.Rename(from, to); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// movePageStateDir moves a folder's page state, which is a mirror of the folder,
// so the whole directory moves with it.
func movePageStateDir(baseDir, fromName, toName string) error {
	from, err := pageStateDirPath(baseDir, fromName)
	if err != nil {
		return err
	}
	to, err := pageStateDirPath(baseDir, toName)
	if err != nil {
		return err
	}
	if !pageStateExists(from) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o777); err != nil {
		return err
	}
	if err := os.Rename(from, to); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// dropPageStateDir removes a folder's whole page-state mirror.
func dropPageStateDir(baseDir, name string) error {
	target, err := pageStateDirPath(baseDir, name)
	if err != nil {
		return err
	}
	return os.RemoveAll(target)
}

// logicalDocument is the document an editor sees: the stored file with its merge
// state put back.
func logicalDocument(baseDir, name, stored string) (string, error) {
	if baseDir == "" || name == "" {
		return stored, nil
	}
	record, err := readPageState(baseDir, name, PageStateReadOptions{Rev: pageRev(stored)})
	if err != nil {
		return "", err
	}
	return composePage(stored, record), nil
}

// logicalBytes is the same composition for a caller holding the file's BYTES: a
// document with no state is handed back untouched, because decoding and
// re-encoding would rewrite every file that is not valid UTF-8.
func logicalBytes(baseDir, name string, stored []byte) ([]byte, error) {
	if baseDir == "" || name == "" {
		return stored, nil
	}
	state, err := readStored(baseDir, name)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return stored, nil
	}
	html := string(stored)
	return []byte(composePage(html, pick(state, pageRev(html)))), nil
}

// storePageState writes the sidecar, before the caller writes the file, and
// returns the file to store.
func storePageState(baseDir, name, html string, durable bool) (string, error) {
	file, state := splitPage(html)
	if state != nil {
		if err := writePageState(baseDir, name, state, durable); err != nil {
			return "", err
		}
	}
	return file, nil
}

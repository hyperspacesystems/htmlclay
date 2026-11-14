package served

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/dataguard"
	"github.com/panphora/htmlclay/internal/htmlutil"
	"github.com/panphora/htmlclay/internal/specwire"
)

// rootFor answers one served root, or the refusal every route gives an id that is
// not served.
func (h *Host) rootFor(rootID string) (*rootServer, error) {
	h.mu.Lock()
	server, ok := h.byID[rootID]
	h.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("served: root %q is not served", rootID)
	}
	return server, nil
}

// Read answers one served document's bytes as its own commit lane reads them, and
// the stamp that lane judges an If-Match against. It is the document on disk,
// never the page a browser is served: no runtime and no save credential is
// injected, no tab is opened, no version is kept and nothing is registered.
//
// The stamp is the one the save lane compares, which is not always the stamp of
// the bytes answered: a page's state lives in a sidecar beside the file, and the
// lane judges the file and that state as one document.
func (h *Host) Read(rootID, rel string) ([]byte, string, error) {
	server, err := h.rootFor(rootID)
	if err != nil {
		return nil, "", err
	}
	filePath, err := resolveWriteTarget(server.registry, rel)
	if err != nil {
		return nil, "", err
	}
	state, err := readFile(filePath)
	if err != nil {
		return nil, "", err
	}
	if state == nil {
		return nil, "", fs.ErrNotExist
	}
	logical, err := logicalBytes(server.baseDir, rel, state.Bytes)
	if err != nil {
		return nil, "", err
	}
	return state.Bytes, specwire.Etag(logical), nil
}

// Version answers one kept version's own bytes, through the lane
// GET /_/version/<rootRel>/<versionName> reads them.
func (h *Host) Version(rootID, rel, id string) ([]byte, error) {
	server, err := h.rootFor(rootID)
	if err != nil {
		return nil, err
	}
	if !isVersionFileName(id) {
		return nil, fmt.Errorf("served: %q is not a version name", id)
	}
	if _, err := resolveWriteTarget(server.registry, rel); err != nil {
		return nil, err
	}
	filePath, err := versionFilePath(server.baseDir, rel, id)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filePath)
}

// DocumentSave is a program's whole-document write to a served document: the
// bytes, the stamp the program read and the id its save is remembered by.
type DocumentSave struct {
	HTML    []byte
	IfMatch string
	SaveID  string
	// Owned is asked once the document's lock is held. False means the document
	// stopped being this folder's while the save waited, and nothing is written.
	Owned func() bool
}

// DocumentSaved is what a program's save or restore answers.
type DocumentSaved struct {
	Etag    string
	Changed bool
	// Replaced names the kept version that holds the bytes the write replaced.
	// It is empty when nothing was written.
	Replaced string
}

// What a program's save or restore is refused with. Nothing was written.
var (
	ErrTooLarge       = errors.New("served: the document is larger than a save may be")
	ErrNotDocument    = errors.New("served: not a complete HTML document")
	ErrVersionNotKept = errors.New("served: the current version could not be kept, so nothing was written")
	ErrNotOwned       = errors.New("served: this folder no longer serves the document")
)

// StaleError refuses a save whose IfMatch does not name the document as it is
// now. Etag is the stamp it has now, and SaveID is the id of the save that stored
// those bytes when this host can prove the pairing.
type StaleError struct {
	Etag   string
	SaveID string
}

func (e *StaleError) Error() string { return "served: the document changed since it was read" }

// SaveDocument writes one whole document for a program that read it first. It
// runs the lane a page's save runs (commitSave, through commitLocked), under the
// lock /_/save holds, as that lane's write with no tab behind it: tabs that have
// the document open hear of it the way they hear of a data API write, and the
// guard judges it as a write by a reader that named the version it replaced.
// Nothing is written unless IfMatch names exactly the document as it is, and the
// document must already exist.
func (h *Host) SaveDocument(rootID, rel string, save DocumentSave) (DocumentSaved, error) {
	server, err := h.rootFor(rootID)
	if err != nil {
		return DocumentSaved{}, ErrNotOwned
	}
	if int64(len(save.HTML)) > saveMaxBytes {
		return DocumentSaved{}, ErrTooLarge
	}
	content := strings.ToValidUTF8(string(save.HTML), "�")
	if !hasHTMLRoot(content) {
		return DocumentSaved{}, ErrNotDocument
	}
	content = string(htmlutil.StripBanner([]byte(stripSaveToken(content))))
	content = server.commit.canonicalForWrite(content)
	return h.programWrite(server, rootID, rel, save, func([]byte) (string, error) { return content, nil })
}

// RestoreDocument puts one kept version back for a program: the version's bytes
// are prepared as the restore route prepares them, then saved as SaveDocument
// saves, so the bytes it replaces are kept first.
func (h *Host) RestoreDocument(rootID, rel, versionID string, save DocumentSave) (DocumentSaved, error) {
	server, err := h.rootFor(rootID)
	if err != nil {
		return DocumentSaved{}, ErrNotOwned
	}
	if !isVersionFileName(versionID) {
		return DocumentSaved{}, fmt.Errorf("served: %q is not a version name: %w", versionID, fs.ErrNotExist)
	}
	return h.programWrite(server, rootID, rel, save, func(current []byte) (string, error) {
		html, _, refusal, err := server.restorableVersion(rel, versionID, current)
		if err != nil {
			return "", err
		}
		if refusal != nil {
			switch refusal.Status {
			case http.StatusRequestEntityTooLarge:
				return "", ErrTooLarge
			case http.StatusUnprocessableEntity:
				return "", ErrNotDocument
			}
			return "", fmt.Errorf("served: version %q of %s: %w", versionID, rel, fs.ErrNotExist)
		}
		return server.commit.canonicalForWrite(html), nil
	})
}

// held resolves a document's owner again once its lock is held: the folder may
// have stopped being served, or the document may have moved, while the write
// waited for the lock. A nil answer means the document is still this folder's
// to write.
func (h *Host) held(server *rootServer, rootID, rel, filePath string, owned func() bool) error {
	if now, err := h.rootFor(rootID); err != nil || now != server {
		return ErrNotOwned
	}
	if again, err := resolveWriteTarget(server.registry, rel); err != nil || again != filePath {
		return ErrNotOwned
	}
	if owned != nil && !owned() {
		return ErrNotOwned
	}
	return server.commit.folderGone()
}

// programWrite is the locked half of a program's save and restore. incoming
// answers the bytes to write over the document as the save lane compares it.
func (h *Host) programWrite(server *rootServer, rootID, rel string, save DocumentSave, incoming func(current []byte) (string, error)) (DocumentSaved, error) {
	filePath, err := resolveWriteTarget(server.registry, rel)
	if err != nil {
		return DocumentSaved{}, err
	}

	unlock := server.commit.locks.lock(filePath)
	defer unlock()

	if err := h.held(server, rootID, rel, filePath, save.Owned); err != nil {
		return DocumentSaved{}, err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return DocumentSaved{}, err
	}
	logical, err := logicalBytes(server.commit.baseDir, rel, data)
	if err != nil {
		return DocumentSaved{}, err
	}

	// A program's save is always conditional: a stamp that names no one version
	// is judged as one that matches nothing.
	ifMatch := save.IfMatch
	if !dataguard.NamesExactVersion(ifMatch) {
		ifMatch = ""
	}
	if conflict := server.judgeIfMatch(filePath, rel, ifMatch); conflict != nil {
		if conflict.Status != http.StatusPreconditionFailed {
			return DocumentSaved{}, errors.New(conflict.Msg)
		}
		return DocumentSaved{}, &StaleError{Etag: conflict.Etag, SaveID: conflict.SaveID}
	}

	content, err := incoming(logical)
	if err != nil {
		return DocumentSaved{}, err
	}
	previous := string(logical)
	if content == previous || content == string(data) {
		return DocumentSaved{Etag: specwire.Etag(logical)}, nil
	}

	saveID := save.SaveID
	if jsStringLength(saveID) > maxSaveIDLength {
		saveID = ""
	}
	request := &commitRequest{
		Kind:         "data-api",
		Name:         rel,
		Bytes:        []byte(content),
		Text:         true,
		IfMatch:      ifMatch,
		IfMatched:    true,
		Program:      true,
		SaveID:       saveID,
		DataLossPrev: &previous,
		Headers:      map[string]string{"if-match": ifMatch},
	}
	result, err := server.commit.commitLocked(request, commitLane{lane: "save", origin: "api"}, filePath)
	if err != nil {
		return DocumentSaved{}, err
	}
	if result.Outcome != "committed" {
		if result.Reason == "backup-failed" {
			return DocumentSaved{}, ErrVersionNotKept
		}
		reason := result.Reason
		if result.Refusal != nil && result.Refusal.Code != "" {
			reason = result.Refusal.Code
		}
		return DocumentSaved{}, fmt.Errorf("served: %s was not saved: %s", rel, reason)
	}
	return DocumentSaved{
		Etag:     result.Etag,
		Changed:  result.Etag != specwire.Etag(logical),
		Replaced: result.Replaced,
	}, nil
}

// ErrNoDataRules is a document with no api rules tag: it publishes no data to
// read or write.
var ErrNoDataRules = errors.New("served: the document has no api rules tag")

// ReadRequiredError refuses a program's data write that would remove data when
// its IfMatch does not name the exact version it read. It is the data guard's
// own answer: its sentence, the stamp to read at and what the write would have
// removed. Nothing was written.
type ReadRequiredError struct {
	Etag    string
	Message string
	Items   int
	Values  int
	Paths   []string
}

func (e *ReadRequiredError) Error() string {
	return "served: the write would remove data that was not read"
}

// DataRefusedError is a data read or write the document's rules or the data sent
// do not allow, with the sentence the data route answers. Err is the engine's
// own error when the engine refused. Nothing was written.
type DataRefusedError struct {
	Message string
	Err     error
}

func (e *DataRefusedError) Error() string { return "served: " + e.Message }

func (e *DataRefusedError) Unwrap() error { return e.Err }

// ReadData answers the data one served document publishes through its api rules
// tag, as JSON, and the stamp its data write judges an If-Match against: the
// logical document's. It reads the document on disk and never the data sidecar
// GET /_/api may answer from, so the data and the stamp are of the same bytes.
func (h *Host) ReadData(rootID, rel string) ([]byte, string, error) {
	server, err := h.rootFor(rootID)
	if err != nil {
		return nil, "", err
	}
	filePath, err := resolveWriteTarget(server.registry, rel)
	if err != nil {
		return nil, "", err
	}
	state, err := readFile(filePath)
	if err != nil {
		return nil, "", err
	}
	if state == nil {
		return nil, "", fs.ErrNotExist
	}
	logical, err := logicalBytes(server.baseDir, rel, state.Bytes)
	if err != nil {
		return nil, "", err
	}
	data, err := apiExtractViaTag(logical)
	if err != nil {
		if mapped, ok := mapAPITagError(err); ok {
			return nil, "", refusedData(err, mapped)
		}
		return nil, "", err
	}
	if data == nil {
		return nil, "", ErrNoDataRules
	}
	encoded, err := dataapi.Marshal(data)
	if err != nil {
		return nil, "", err
	}
	return encoded, specwire.Etag(logical), nil
}

// DataWrite is a program's write to the data a served document publishes
// through its api rules tag: the JSON to write through the rules and the stamp
// the program read. A served folder takes whole data only.
type DataWrite struct {
	Data    []byte
	IfMatch string
	// Owned is asked once the document's lock is held. False means the document
	// stopped being this folder's while the write waited, and nothing is written.
	Owned func() bool
}

// WriteData writes a served document's data for a program. It runs the function
// behind POST /_/api under the lock that route holds, as that route's write by a
// program: the lane judges it, so a write that would remove data is refused with
// ReadRequiredError unless IfMatch names exactly the document as it is, and the
// bytes it replaces are kept first or the write is refused. An IfMatch that
// names another version is StaleError.
func (h *Host) WriteData(rootID, rel string, write DataWrite) (DocumentSaved, error) {
	server, err := h.rootFor(rootID)
	if err != nil {
		return DocumentSaved{}, ErrNotOwned
	}
	if len(write.Data) > apiWriteMaxBytes {
		return DocumentSaved{}, ErrTooLarge
	}
	data, err := dataapi.ParseStrict(string(write.Data))
	if err != nil {
		return DocumentSaved{}, refusedData(nil, apiInvalidJSONBody)
	}
	filePath, err := resolveWriteTarget(server.registry, rel)
	if err != nil {
		return DocumentSaved{}, err
	}

	unlock := server.commit.locks.lock(filePath)
	defer unlock()

	if err := h.held(server, rootID, rel, filePath, write.Owned); err != nil {
		return DocumentSaved{}, err
	}
	written, refusal := server.writeSiteData(rel, filePath, siteDataWrite{
		data:        data,
		ifMatch:     write.IfMatch,
		ifMatchSent: write.IfMatch != "",
		program:     true,
	})
	if refusal != nil {
		return DocumentSaved{}, refusal.err
	}
	return DocumentSaved{Etag: written.etag, Changed: written.changed, Replaced: written.replaced}, nil
}

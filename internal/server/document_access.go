package server

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"os"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/dataguard"
	"github.com/panphora/htmlclay/internal/htmlutil"
	"github.com/panphora/htmlclay/internal/specwire"
	"github.com/panphora/htmlclay/internal/versions"
)

// ReadDocument answers one loose document's bytes as its own save lane reads them,
// through the same descriptor check /_/save uses, and the stamp that lane judges
// an If-Match against. Nothing is registered, opened or kept: it is a read of the
// file on disk exactly as it is.
func (s *Server) ReadDocument(absPath string) ([]byte, string, error) {
	data, err := s.readRegisteredFile(absPath)
	if err != nil {
		return nil, "", err
	}
	return data, specwire.Etag(data), nil
}

// ReadDocumentVersion answers one kept version's own bytes, through the lane
// GET /_/versions/<name> reads them. The document's history identity is the open
// file's when this server holds one, since that is resolved once at first serve,
// and is derived from the document's bytes otherwise. A read creates no history.
func (s *Server) ReadDocumentVersion(absPath, name string) ([]byte, error) {
	if _, _, err := versions.ParseEntryName(name); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(absPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	key := ""
	if f, ok := s.sessions.LookupByPath(absPath); ok {
		key = f.HistoryKey()
	}
	if key == "" {
		key = versions.Key(absPath, data)
	}
	return s.versions.Read(key, absPath, name)
}

// DocumentSave is a program's whole-document write to a document this server
// has registered: the bytes, the stamp the program read and the id its save is
// remembered by.
type DocumentSave struct {
	HTML    []byte
	IfMatch string
	SaveID  string
	// Owned is asked once the file's lock is held. False means the document
	// stopped being this host's to write while the save waited, and nothing is
	// written.
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
	ErrTooLarge       = errors.New("server: the document is larger than a save may be")
	ErrNotDocument    = errors.New("server: not a complete HTML document")
	ErrVersionNotKept = errors.New("server: the current version could not be kept, so nothing was written")
	ErrNotOwned       = errors.New("server: this host no longer holds the document open for writing")
)

// StaleError refuses a save whose IfMatch does not name the document as it is
// now. Etag is the stamp it has now, and SaveID is the id of the save that stored
// those bytes when this host can prove the pairing.
type StaleError struct {
	Etag   string
	SaveID string
}

func (e *StaleError) Error() string { return "server: the document changed since it was read" }

// SaveDocument writes one whole document for a program that read it first. It
// runs the function behind /_/save under the file's own lock, as that lane's save
// with no tab behind it: the bytes it replaces are kept first and the save is
// refused when they cannot be. Nothing is written unless IfMatch names exactly
// the document as it is, and the document must already exist and be registered.
func (s *Server) SaveDocument(absPath string, save DocumentSave) (DocumentSaved, error) {
	if len(save.HTML) > maxSaveSize {
		return DocumentSaved{}, ErrTooLarge
	}
	f, ok := s.sessions.LookupByPath(absPath)
	if !ok {
		return DocumentSaved{}, ErrNotOwned
	}

	// A program's save is always conditional: a stamp that names no one version
	// is judged as one that matches nothing.
	ifMatch := save.IfMatch
	if !dataguard.NamesExactVersion(ifMatch) {
		ifMatch = ""
	}
	saveID := save.SaveID
	if len(saveID) > maxSaveIDLen {
		saveID = ""
	}
	saved, refusal := s.saveDocument(f, documentSave{
		host:        s.Host(),
		body:        save.HTML,
		ifMatch:     ifMatch,
		ifMatchSent: true,
		saveID:      saveID,
		// A program is not an editor with a page format of its own: its save keeps
		// the floor the document has and is never refused for it, as a data API
		// write and a restore are not.
		formatCapability: math.MaxInt,
		provenance:       dataguard.External,
		program:          true,
		owned: func() bool {
			held, ok := s.sessions.LookupByPath(absPath)
			return ok && held == f && !s.trustedWriteRevoked(f) && (save.Owned == nil || save.Owned())
		},
	})
	if refusal != nil {
		return DocumentSaved{}, refusal.err
	}
	return DocumentSaved{Etag: saved.etag, Changed: saved.changed, Replaced: saved.replaced}, nil
}

// RestoreDocument puts one kept version back for a program: the version's bytes
// are read and checked as the restore route checks them, then saved as
// SaveDocument saves, so the bytes it replaces are kept first.
func (s *Server) RestoreDocument(absPath, versionID string, save DocumentSave) (DocumentSaved, error) {
	if _, _, err := versions.ParseEntryName(versionID); err != nil {
		return DocumentSaved{}, fmt.Errorf("server: %q is not a version name: %w", versionID, fs.ErrNotExist)
	}
	f, ok := s.sessions.LookupByPath(absPath)
	if !ok {
		return DocumentSaved{}, ErrNotOwned
	}
	f.Lock()
	key := historyKey(f)
	f.Unlock()
	data, err := s.versions.Read(key, absPath, versionID)
	if err != nil {
		return DocumentSaved{}, fmt.Errorf("server: version %q of %s: %w", versionID, f.RelPath, fs.ErrNotExist)
	}
	data = htmlutil.StripBanner(data)
	if len(data) > maxRestoreSize {
		return DocumentSaved{}, ErrTooLarge
	}
	if !htmlutil.IsCompleteHTMLDocument(data) {
		return DocumentSaved{}, ErrNotDocument
	}
	// The version's own stored id is stripped, never adopted; it may have come
	// from a clone or a rename.
	save.HTML = htmlutil.StripHTMLClayID(htmlutil.StripToken(data))
	return s.SaveDocument(absPath, save)
}

// ErrNoDataRules is a document with no api rules tag: it publishes no data to
// read or write.
var ErrNoDataRules = errors.New("server: the document has no api rules tag")

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
	return "server: the write would remove data that was not read"
}

// DataRefusedError is a data read or write the document's rules, the data sent
// or the page's format does not allow, with the status, the code when there is
// one and the sentence the data route answers. Err is the engine's own error
// when the engine refused. Nothing was written.
type DataRefusedError struct {
	Status  int
	Code    string
	Message string
	Err     error
}

func (e *DataRefusedError) Error() string { return "server: " + e.Message }

func (e *DataRefusedError) Unwrap() error { return e.Err }

// ReadData answers the data one loose document publishes through its api rules
// tag, as JSON, and the stamp its data write judges an If-Match against. It is a
// read of the file on disk: nothing is registered, opened or kept.
func (s *Server) ReadData(absPath string) ([]byte, string, error) {
	raw, err := s.readRegisteredFile(absPath)
	if err != nil {
		return nil, "", err
	}
	res, err := dataapi.ReadDocument(htmlutil.StripToken(raw), dataapi.ReadOptions{Token: "api"})
	if err == nil && !res.Found {
		err = errNoAPIRulesTag
	}
	if err != nil {
		status, body := mapDataError(err, faceAPI)
		return nil, "", refusedData(err, status, writeErrorBody{Error: body.Error, Message: body.Message, Code: body.Code})
	}
	data, err := dataapi.Marshal(res.Data)
	if err != nil {
		return nil, "", err
	}
	return data, specwire.Etag(raw), nil
}

// DataWrite is a program's write to the data a registered document publishes
// through its api rules tag: the JSON to write through the rules or the JSON of
// a sheet patch's operations, never both, and the stamp the program read.
type DataWrite struct {
	Data    []byte
	Patch   []byte
	IfMatch string
	// Owned is asked once the file's lock is held. False means the document
	// stopped being this host's to write while the write waited, and nothing is
	// written.
	Owned func() bool
}

// DataWritten is what a program's data write answers.
type DataWritten struct {
	Etag    string
	Changed bool
	// Replaced names the kept version that holds the bytes the write replaced.
	// It is empty when nothing was written.
	Replaced string
	// Stale is a sheet patch's own answer: this host cannot recalculate, so the
	// patched sheets hold formula results from before the patch.
	Stale bool
}

// WriteData writes a registered document's data for a program. It runs the
// function behind POST /_/api under the file's own lock, as that route's write
// by a program: the data guard judges it, so a write that would remove data is
// refused with ReadRequiredError unless IfMatch names exactly the document as it
// is. Unlike that route, the bytes it replaces are kept first whatever IfMatch
// proves, and the write is refused when they cannot be. An IfMatch that names
// another version is StaleError.
func (s *Server) WriteData(absPath string, write DataWrite) (DataWritten, error) {
	raw, shape := write.Data, dataWhole
	if len(write.Patch) > 0 {
		raw, shape = write.Patch, dataPatch
	}
	if len(raw) > maxDataWriteSize {
		return DataWritten{}, ErrTooLarge
	}
	data, err := dataapi.ParseStrict(string(raw))
	if err != nil {
		return DataWritten{}, &DataRefusedError{Status: http.StatusBadRequest, Message: "The request body is not valid JSON."}
	}
	f, ok := s.sessions.LookupByPath(absPath)
	if !ok {
		return DataWritten{}, ErrNotOwned
	}
	written, refusal := s.writeData(f, dataWrite{
		host:        s.Host(),
		data:        data,
		shape:       shape,
		mode:        dataMode{face: faceAPI},
		ifMatch:     write.IfMatch,
		ifMatchSent: write.IfMatch != "",
		program:     true,
		keep:        true,
		owned: func() bool {
			held, ok := s.sessions.LookupByPath(absPath)
			return ok && held == f && !s.trustedWriteRevoked(f) && (write.Owned == nil || write.Owned())
		},
	})
	if refusal != nil {
		return DataWritten{}, refusal.err
	}
	out := DataWritten{Etag: specwire.Etag(written.stored), Changed: written.changed, Replaced: written.replaced}
	if written.patch != nil {
		out.Stale = written.patch.Stale
	}
	return out, nil
}

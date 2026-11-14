package main

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/pathrel"
	"github.com/panphora/htmlclay/internal/served"
	"github.com/panphora/htmlclay/internal/server"
)

// Version is one kept version of a document, in the one shape both stores answer
// (see historyItem).
type Version = historyItem

// Save is a whole-document write: the bytes, the stamp the writer last saw and
// the id that makes a retry of the same save recognizable.
type Save struct {
	HTML    []byte
	IfMatch string
	SaveID  string
}

// DataWrite is a write to the data a document publishes through its api rules
// tag: the JSON to write through the rules or the JSON of a sheet patch's
// operations, never both, and the stamp the writer last saw.
type DataWrite struct {
	Data    []byte
	Patch   []byte
	IfMatch string
}

// Result is what a write answers: the stamp the document now has, whether the
// bytes moved, and the version the write replaced.
type Result struct {
	ETag        string
	Changed     bool
	UndoVersion string
	// Stale is a sheet patch's own answer: its owner cannot recalculate, so the
	// patched sheets hold formula results from before the patch.
	Stale bool
}

// ErrNotReachable is a path no owner may reach: not inside a served folder, not
// under a live trusted folder, and not a loose file that is open now.
var ErrNotReachable = errors.New("document not reachable")

// ErrStale refuses a save or a restore whose stamp does not name the document as
// it is now. ETag is the stamp it has now, and SaveID is the id of the save that
// stored those bytes when its owner can prove the pairing. Nothing was written.
type ErrStale struct {
	ETag   string
	SaveID string
}

func (e ErrStale) Error() string { return "document changed since it was read" }

// What else a save or a restore is refused with. Nothing was written.
var (
	// ErrBackup is a write whose replaced bytes could not be kept as a version.
	ErrBackup      = errors.New("the current version could not be kept")
	ErrTooLarge    = errors.New("document too large")
	ErrNotDocument = errors.New("not a complete HTML document")
)

// What else a data read or write is refused with. Nothing was written.
var (
	// ErrNoAPIRules is a document with no api rules tag, so it has no data.
	ErrNoAPIRules = errors.New("document has no api rules tag")
	// ErrPatchUnsupported is a patch sent to a served document: a served folder
	// takes whole data only.
	ErrPatchUnsupported = errors.New("a served folder takes no data patches")
)

// ErrReadRequired refuses a program's data write that would remove data when its
// stamp does not name the exact version it read. It carries the owner's data
// guard's own answer: its sentence, the stamp to read at and what would go.
type ErrReadRequired struct {
	ETag    string
	Message string
	Items   int
	Values  int
	Paths   []string
}

func (e ErrReadRequired) Error() string { return "the write would remove data that was not read" }

// ErrDataRefused is a data read or write the document's rules or the data sent
// do not allow, with the status and the sentence its owner's data route answers
// and that route's code when it names one. Rules says the document's own api
// rules tag is at fault, not what was sent.
type ErrDataRefused struct {
	Status  int
	Code    string
	Rules   bool
	Message string
}

func (e ErrDataRefused) Error() string { return e.Message }

// DocumentAccess is the one way to reach a document, whoever owns it. Served
// calls the served folder's own lanes and loose calls the lanes behind /_/save
// and /_/api, so there is never a second write engine and never a copy of the
// document's bytes made for reading.
type DocumentAccess interface {
	Read(ctx context.Context) (html []byte, etag string, err error)
	Versions(ctx context.Context, before string, limit int) (page []Version, next string, err error)
	Version(ctx context.Context, id string) ([]byte, error)
	Save(ctx context.Context, req Save) (Result, error)
	Restore(ctx context.Context, versionID, ifMatch, saveID string) (Result, error)
	ReadData(ctx context.Context) (data []byte, etag string, err error)
	WriteData(ctx context.Context, req DataWrite) (Result, error)
}

// documentAccess picks the one owner of absPath with ResolveDocument and nothing
// else: a served folder first, then a live trusted folder, then a loose file that
// is registered now. Anything else is ErrNotReachable. Picking an owner reads:
// no tab is opened, nothing is trusted, no loose file is registered.
func (a *app) documentAccess(absPath string) (DocumentAccess, error) {
	resolved, err := documentPath(absPath)
	if err != nil {
		return nil, err
	}
	owner, err := a.ResolveDocument(resolved)
	if err != nil {
		return nil, err
	}
	if owner.Kind == OwnerServed {
		return servedDocumentAccess{app: a, path: resolved, rootID: owner.FolderID, rel: owner.RelativePath}, nil
	}

	a.mu.Lock()
	site, _, open := a.lookupLocked(resolved)
	if !open && owner.Kind == OwnerTrusted {
		site = a.siteAtLocked(owner.FolderPath)
	}
	a.mu.Unlock()
	if site == nil || site.closed.Load() {
		return nil, ErrNotReachable
	}
	return looseDocumentAccess{app: a, site: site, server: site.srv, path: resolved}, nil
}

// documentPath is absPath as the owners see it: every symlink resolved, with what
// is already on disk resolved when the file itself does not exist yet.
func documentPath(absPath string) (string, error) {
	abs, err := filepath.Abs(absPath)
	if err != nil {
		return "", err
	}
	resolved, err := resolveSymlinks(abs)
	if err != nil {
		return pathrel.RealPath(abs)
	}
	return resolved, nil
}

// historyPage is one page of doc's kept versions, with the id to continue from
// when there is another page. It reads the one history 7.17 reads.
func (a *app) historyPage(doc historyDocument, before string, limit int) ([]Version, string, error) {
	if limit <= 0 {
		items, err := a.readHistoryPage(doc, before, limit)
		return items, "", err
	}
	items, err := a.readHistoryPage(doc, before, limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = items[limit-1].ID
	}
	return items, next, nil
}

// writeResult is an owner's answer to a write, in the one shape and with the one
// set of errors both owners answer.
func writeResult(etag string, changed bool, replaced string, err error) (Result, error) {
	if err != nil {
		return Result{}, ownerError(err)
	}
	return Result{ETag: etag, Changed: changed, UndoVersion: replaced}, nil
}

// ownerError is an owner's refusal as one of the errors both owners answer. An
// error neither owner names is answered as it is.
func ownerError(err error) error {
	var servedStale *served.StaleError
	var looseStale *server.StaleError
	var servedRequired *served.ReadRequiredError
	var looseRequired *server.ReadRequiredError
	var servedRefused *served.DataRefusedError
	var looseRefused *server.DataRefusedError
	switch {
	case errors.As(err, &servedStale):
		return ErrStale{ETag: servedStale.Etag, SaveID: servedStale.SaveID}
	case errors.As(err, &looseStale):
		return ErrStale{ETag: looseStale.Etag, SaveID: looseStale.SaveID}
	case errors.Is(err, served.ErrVersionNotKept), errors.Is(err, server.ErrVersionNotKept):
		return ErrBackup
	case errors.Is(err, served.ErrTooLarge), errors.Is(err, server.ErrTooLarge):
		return ErrTooLarge
	case errors.Is(err, served.ErrNotDocument), errors.Is(err, server.ErrNotDocument):
		return ErrNotDocument
	case errors.Is(err, served.ErrNotOwned), errors.Is(err, server.ErrNotOwned):
		return ErrNotReachable
	case errors.As(err, &servedRequired):
		return ErrReadRequired{ETag: servedRequired.Etag, Message: servedRequired.Message,
			Items: servedRequired.Items, Values: servedRequired.Values, Paths: servedRequired.Paths}
	case errors.As(err, &looseRequired):
		return ErrReadRequired{ETag: looseRequired.Etag, Message: looseRequired.Message,
			Items: looseRequired.Items, Values: looseRequired.Values, Paths: looseRequired.Paths}
	case errors.Is(err, served.ErrNoDataRules), errors.Is(err, server.ErrNoDataRules):
		return ErrNoAPIRules
	case errors.As(err, &servedRefused):
		return ErrDataRefused{Status: http.StatusBadRequest, Rules: rulesAtFault(err), Message: servedRefused.Message}
	case errors.As(err, &looseRefused):
		return ErrDataRefused{Status: looseRefused.Status, Code: looseRefused.Code, Rules: rulesAtFault(err), Message: looseRefused.Message}
	}
	return err
}

// rulesAtFault reports that the data engine refused a read or a write for the
// document's own api rules tag rather than for what was sent.
func rulesAtFault(err error) bool {
	var parse *dataapi.RulesParseError
	var version *dataapi.UnknownRulesVersion
	var codec *dataapi.UnknownCodec
	var selector dataapi.SelectorFailure
	return errors.As(err, &parse) || errors.As(err, &version) || errors.As(err, &codec) || errors.As(err, &selector)
}

// servedDocumentAccess is a document in a folder this app serves.
type servedDocumentAccess struct {
	app    *app
	path   string
	rootID string
	rel    string
}

// owned resolves the document's owner again. Its lane asks once it holds the
// document's lock, so a folder that stopped being served while the write waited
// is not written into.
func (d servedDocumentAccess) owned() bool {
	owner, err := d.app.ResolveDocument(d.path)
	return err == nil && owner.Kind == OwnerServed && owner.FolderID == d.rootID && owner.RelativePath == d.rel
}

func (d servedDocumentAccess) Read(ctx context.Context) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if d.app.served == nil {
		return nil, "", errors.New("no served folders")
	}
	return d.app.served.Read(d.rootID, d.rel)
}

func (d servedDocumentAccess) Versions(ctx context.Context, before string, limit int) ([]Version, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	return d.app.historyPage(historyDocument{RootID: d.rootID, Path: d.rel}, before, limit)
}

func (d servedDocumentAccess) Version(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d.app.served == nil {
		return nil, errors.New("no served folders")
	}
	return d.app.served.Version(d.rootID, d.rel, id)
}

func (d servedDocumentAccess) Save(ctx context.Context, req Save) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if d.app.served == nil {
		return Result{}, errors.New("no served folders")
	}
	saved, err := d.app.served.SaveDocument(d.rootID, d.rel, served.DocumentSave{
		HTML: req.HTML, IfMatch: req.IfMatch, SaveID: req.SaveID, Owned: d.owned,
	})
	return writeResult(saved.Etag, saved.Changed, saved.Replaced, err)
}

func (d servedDocumentAccess) Restore(ctx context.Context, versionID, ifMatch, saveID string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if d.app.served == nil {
		return Result{}, errors.New("no served folders")
	}
	saved, err := d.app.served.RestoreDocument(d.rootID, d.rel, versionID, served.DocumentSave{
		IfMatch: ifMatch, SaveID: saveID, Owned: d.owned,
	})
	return writeResult(saved.Etag, saved.Changed, saved.Replaced, err)
}

func (d servedDocumentAccess) ReadData(ctx context.Context) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if d.app.served == nil {
		return nil, "", errors.New("no served folders")
	}
	data, etag, err := d.app.served.ReadData(d.rootID, d.rel)
	if err != nil {
		return nil, "", ownerError(err)
	}
	return data, etag, nil
}

func (d servedDocumentAccess) WriteData(ctx context.Context, req DataWrite) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if d.app.served == nil {
		return Result{}, errors.New("no served folders")
	}
	// A served folder's data engine applies whole data and has no patch engine.
	if len(req.Patch) > 0 {
		return Result{}, ErrPatchUnsupported
	}
	saved, err := d.app.served.WriteData(d.rootID, d.rel, served.DataWrite{
		Data: req.Data, IfMatch: req.IfMatch, Owned: d.owned,
	})
	return writeResult(saved.Etag, saved.Changed, saved.Replaced, err)
}

// looseDocumentAccess is a document under a live trusted folder, or a loose file
// this app has open now.
type looseDocumentAccess struct {
	app    *app
	site   *site
	server *server.Server
	path   string
}

func (d looseDocumentAccess) Read(ctx context.Context) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	return d.server.ReadDocument(d.path)
}

func (d looseDocumentAccess) Versions(ctx context.Context, before string, limit int) ([]Version, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	return d.app.historyPage(historyDocument{Path: d.path}, before, limit)
}

func (d looseDocumentAccess) Version(ctx context.Context, id string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return d.server.ReadDocumentVersion(d.path, id)
}

// register gives a trusted folder's document what its site gives it when a page
// first reaches it: the site's trusted root (the router arms a site on its first
// request, and a write may be the first thing to reach this one) and the
// registration that holds the lock a write takes. A loose file is reachable only
// while it is open, so it is registered already.
func (d looseDocumentAccess) register() error {
	if d.site.closed.Load() {
		return ErrNotReachable
	}
	d.site.armOnce.Do(d.site.arm)
	d.app.mu.Lock()
	_, _, open := d.app.lookupLocked(d.path)
	d.app.mu.Unlock()
	if open {
		return nil
	}
	if _, routed := d.app.routeTrusted(d.path); !routed {
		return ErrNotReachable
	}
	return nil
}

// owned resolves the document's owner again. Its lane asks once it holds the
// file's lock, so a file whose folder started being served while the write
// waited is left to the served folder's own lane.
func (d looseDocumentAccess) owned() bool {
	owner, err := d.app.ResolveDocument(d.path)
	return err == nil && owner.Kind != OwnerServed
}

func (d looseDocumentAccess) Save(ctx context.Context, req Save) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := d.register(); err != nil {
		return Result{}, err
	}
	saved, err := d.server.SaveDocument(d.path, server.DocumentSave{
		HTML: req.HTML, IfMatch: req.IfMatch, SaveID: req.SaveID, Owned: d.owned,
	})
	return writeResult(saved.Etag, saved.Changed, saved.Replaced, err)
}

func (d looseDocumentAccess) Restore(ctx context.Context, versionID, ifMatch, saveID string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := d.register(); err != nil {
		return Result{}, err
	}
	saved, err := d.server.RestoreDocument(d.path, versionID, server.DocumentSave{
		IfMatch: ifMatch, SaveID: saveID, Owned: d.owned,
	})
	return writeResult(saved.Etag, saved.Changed, saved.Replaced, err)
}

func (d looseDocumentAccess) ReadData(ctx context.Context) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	data, etag, err := d.server.ReadData(d.path)
	if err != nil {
		return nil, "", ownerError(err)
	}
	return data, etag, nil
}

func (d looseDocumentAccess) WriteData(ctx context.Context, req DataWrite) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := d.register(); err != nil {
		return Result{}, err
	}
	written, err := d.server.WriteData(d.path, server.DataWrite{
		Data: req.Data, Patch: req.Patch, IfMatch: req.IfMatch, Owned: d.owned,
	})
	result, err := writeResult(written.Etag, written.Changed, written.Replaced, err)
	result.Stale = written.Stale
	return result, err
}

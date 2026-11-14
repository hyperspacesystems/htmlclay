package main

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/pathrel"
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

// Result is what a save or a restore answers: the stamp the document now has,
// whether the bytes moved, and the version the write replaced.
type Result struct {
	ETag        string
	Changed     bool
	UndoVersion string
}

// ErrNotReachable is a path no owner may reach: not inside a served folder, not
// under a live trusted folder, and not a loose file that is open now.
var ErrNotReachable = errors.New("document not reachable")

// ErrNotBuilt is a lane this step declares and a later one fills in.
var ErrNotBuilt = errors.New("not built yet")

// DocumentAccess is the one way to reach a document, whoever owns it. Served
// calls the served folder's own lane and loose calls the lane behind /_/save, so
// there is never a second write engine and never a copy of the document's bytes
// made for reading.
type DocumentAccess interface {
	Read(ctx context.Context) (html []byte, etag string, err error)
	Versions(ctx context.Context, before string, limit int) (page []Version, next string, err error)
	Version(ctx context.Context, id string) ([]byte, error)
	Save(ctx context.Context, req Save) (Result, error)
	Restore(ctx context.Context, versionID, ifMatch, saveID string) (Result, error)
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
		return servedDocumentAccess{app: a, rootID: owner.FolderID, rel: owner.RelativePath}, nil
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
	return looseDocumentAccess{app: a, server: site.srv, path: resolved}, nil
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

// servedDocumentAccess is a document in a folder this app serves.
type servedDocumentAccess struct {
	app    *app
	rootID string
	rel    string
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
	return Result{}, ErrNotBuilt
}

func (d servedDocumentAccess) Restore(ctx context.Context, versionID, ifMatch, saveID string) (Result, error) {
	return Result{}, ErrNotBuilt
}

// looseDocumentAccess is a document under a live trusted folder, or a loose file
// this app has open now.
type looseDocumentAccess struct {
	app    *app
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

func (d looseDocumentAccess) Save(ctx context.Context, req Save) (Result, error) {
	return Result{}, ErrNotBuilt
}

func (d looseDocumentAccess) Restore(ctx context.Context, versionID, ifMatch, saveID string) (Result, error) {
	return Result{}, ErrNotBuilt
}

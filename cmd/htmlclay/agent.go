package main

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/panphora/htmlclay/internal/agentlane"
	"github.com/panphora/htmlclay/internal/platform"
)

// agentLane is the lane a local program reads and writes documents through, on
// the bare loopback host. It lists what this app may reach and reaches one
// document through the one document access lane, which reads and registers
// nothing.
func (a *app) agentLane() http.Handler {
	a.lane.once.Do(func() {
		a.lane.handler = agentlane.Handler(agentHost{a: a}, a.rt.port, agentlane.RequireToken(a.rt.agentToken, a.rt.agentTokenFile))
	})
	return a.lane.handler
}

// agentLaneHandler is the lane's one handler, built on the first request so the
// replay store that answers a retried write lives as long as the app.
type agentLaneHandler struct {
	once    sync.Once
	handler http.Handler
}

// agentHost is the app as the lane's Host.
type agentHost struct{ a *app }

func (h agentHost) Version() string { return version }

// List answers every document a program may read: the documents in each served
// folder, in each live trusted folder, and each loose file open now. A folder is
// walked through an os.Root, so a link inside it cannot lead outside.
func (h agentHost) List(ctx context.Context, limit int) ([]agentlane.Document, bool, error) {
	a := h.a
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	open := a.agentOpenFiles()
	docs := make([]agentlane.Document, 0, len(open))
	seen := make(map[string]bool, len(open))
	add := func(doc agentlane.Document) {
		if seen[doc.Path] {
			return
		}
		seen[doc.Path] = true
		if address, ok := open[doc.Path]; ok {
			doc.Open = true
			doc.URL = address
		}
		docs = append(docs, doc)
	}
	// The walks stop once there is one row past the limit, so a large trusted
	// folder answers as fast as a small one; the answer says it was cut.
	full := func() bool { return limit > 0 && len(docs) > limit }

	for _, folder := range a.rt.cfg.ServedFolderList() {
		if !folder.Serving {
			continue
		}
		address := a.servedAddress(folder)
		a.walkAgentFolder(ctx, folder.Path, full, func(rel, abs string) {
			add(agentlane.Document{
				Path:   abs,
				URL:    address + "/" + dashboardFolderPath(rel),
				Folder: folder.Name,
				Kind:   "served",
			})
		})
	}

	for _, folder := range a.rt.cfg.TrustedFolderList() {
		if served, ok := a.rt.cfg.ServedFolderForPath(folder.Path); ok && served.Serving {
			continue
		}
		address := a.agentTrustedAddress(folder.Path)
		a.walkAgentFolder(ctx, folder.Path, full, func(rel, abs string) {
			doc := agentlane.Document{
				Path:   abs,
				Folder: filepath.Base(folder.Path),
				Kind:   "trusted",
			}
			// A download nobody vouched for is served on an origin of its
			// own, sandboxed; its folder's address would not open it.
			if address != "" && !a.listedAsStranger(abs) {
				doc.URL = address + "/" + dashboardFolderPath(rel)
			}
			add(doc)
		})
	}

	loose := make([]string, 0, len(open))
	for path := range open {
		loose = append(loose, path)
	}
	sort.Strings(loose)
	for _, path := range loose {
		add(agentlane.Document{Path: path, URL: open[path], Kind: "open", Open: true})
	}

	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	truncated := false
	if limit > 0 && len(docs) > limit {
		docs, truncated = docs[:limit], true
	}
	return docs, truncated, nil
}

// Access reaches one document through the one document access lane, and answers
// how a list would name it.
func (h agentHost) Access(ctx context.Context, absPath string) (agentlane.Access, agentlane.Document, error) {
	access, err := h.a.documentAccess(absPath)
	if err != nil {
		return nil, agentlane.Document{}, agentError(err)
	}
	return agentAccess{access: access}, h.a.agentDocument(absPath), nil
}

// walkAgentFolder calls add for every document under folder, at any depth. Names
// starting with a dot are skipped, the way the dashboard's folder list skips
// them, and so is a link, which the root would only follow within the folder.
// The walk stops when full reports true or ctx is done.
func (a *app) walkAgentFolder(ctx context.Context, folder string, full func() bool, add func(rel, abs string)) {
	if info, err := os.Stat(folder); err != nil || !info.IsDir() {
		return
	}
	root, err := os.OpenRoot(folder)
	if err != nil {
		return
	}
	defer root.Close()
	_ = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if full() || ctx.Err() != nil {
			return fs.SkipAll
		}
		if name == "." {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || !dashboardFolderDocument(entry.Name()) {
			return nil
		}
		add(name, filepath.Join(folder, filepath.FromSlash(name)))
		return nil
	})
}

// agentOpenFiles is every loose file a live site holds open now, and the address
// it answers at. It is the registrations enumeration the recents list joins,
// filtered to the files no folder owns.
func (a *app) agentOpenFiles() map[string]string {
	var regs []registration
	a.mu.Lock()
	for _, s := range a.sites {
		if s.closed.Load() {
			continue
		}
		for _, info := range s.sessions.Registrations() {
			f, ok := s.sessions.LookupByPath(info.Path)
			if !ok {
				continue
			}
			regs = append(regs, registration{path: info.Path, url: fileURL(s.host, f.RelPath)})
		}
	}
	a.mu.Unlock()

	open := make(map[string]string, len(regs))
	for _, reg := range regs {
		if _, ok := open[reg.path]; ok {
			continue
		}
		owner, err := a.ResolveDocument(reg.path)
		if err != nil {
			continue
		}
		// A download inside a trusted folder has an origin of its own, so the
		// site holding it is the one that opens it.
		if owner.Kind != OwnerLoose && !a.listedAsStranger(reg.path) {
			continue
		}
		open[reg.path] = reg.url
	}
	return open
}

// agentOpenURL is the address one document answers at while a live site holds it
// open, and false when none does.
func (a *app) agentOpenURL(absPath string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, _, open := a.lookupLocked(absPath)
	if !open || s == nil {
		return "", false
	}
	f, ok := s.sessions.LookupByPath(absPath)
	if !ok {
		return "", false
	}
	return fileURL(s.host, f.RelPath), true
}

// agentTrustedAddress is the address a live trusted folder answers at, or "" when
// the folder has no live site.
func (a *app) agentTrustedAddress(folder string) string {
	a.mu.Lock()
	s := a.siteAtLocked(folder)
	a.mu.Unlock()
	if s == nil {
		return ""
	}
	return "http://" + s.host
}

// listedAsStranger reports a document a list must not send to its folder's
// address: a download nobody vouched for, which serves sandboxed there and only
// runs on an origin of its own. The download mark is checked first, because a
// list walks large folders and hashing every file would make it crawl.
func (a *app) listedAsStranger(absPath string) bool {
	if platform.Downloaded(absPath) {
		return true
	}
	store := a.tierStore()
	if store == nil {
		return false
	}
	if _, trusted := store.PathTrust(absPath); trusted {
		return false
	}
	seen, ok := store.Seen(absPath)
	return ok && seen.Downloaded
}

// agentDocument names one document the way a list does.
func (a *app) agentDocument(absPath string) agentlane.Document {
	doc := agentlane.Document{Path: absPath}
	if address, open := a.agentOpenURL(absPath); open {
		doc.Open = true
		doc.URL = address
	}
	owner, err := a.ResolveDocument(absPath)
	if err != nil {
		return doc
	}
	switch owner.Kind {
	case OwnerServed:
		doc.Kind, doc.Open = "served", false
		if folder, ok := a.rt.cfg.ServedFolderByID(owner.FolderID); ok {
			doc.Folder = folder.Name
			doc.URL = a.servedAddress(folder) + "/" + dashboardFolderPath(owner.RelativePath)
		}
	case OwnerTrusted:
		doc.Kind = "trusted"
		doc.Folder = filepath.Base(owner.FolderPath)
		if doc.URL == "" && !a.listedAsStranger(absPath) {
			if address := a.agentTrustedAddress(owner.FolderPath); address != "" {
				doc.URL = address + "/" + dashboardFolderPath(owner.RelativePath)
			}
		}
	default:
		doc.Kind = "open"
	}
	return doc
}

// agentAccess adapts one document's own access lane to what this lane offers.
type agentAccess struct{ access DocumentAccess }

func (d agentAccess) Read(ctx context.Context) ([]byte, string, error) {
	html, etag, err := d.access.Read(ctx)
	return html, etag, agentError(err)
}

func (d agentAccess) Versions(ctx context.Context, before string, limit int) ([]agentlane.Version, string, error) {
	page, next, err := d.access.Versions(ctx, before, limit)
	if err != nil {
		return nil, "", agentError(err)
	}
	versions := make([]agentlane.Version, 0, len(page))
	for _, version := range page {
		versions = append(versions, agentlane.Version{
			ID:         version.ID,
			CreatedAt:  version.CreatedAt,
			Size:       version.Size,
			Kind:       version.Kind,
			Pinned:     version.Pinned,
			Restorable: version.Restorable,
		})
	}
	return versions, next, nil
}

func (d agentAccess) Version(ctx context.Context, id string) ([]byte, error) {
	html, err := d.access.Version(ctx, id)
	return html, agentError(err)
}

func (d agentAccess) Save(ctx context.Context, html []byte, ifMatch, saveID string) (agentlane.WriteResult, error) {
	result, err := d.access.Save(ctx, Save{HTML: html, IfMatch: ifMatch, SaveID: saveID})
	return agentResult(result), agentError(err)
}

func (d agentAccess) Restore(ctx context.Context, versionID, ifMatch, saveID string) (agentlane.WriteResult, error) {
	result, err := d.access.Restore(ctx, versionID, ifMatch, saveID)
	return agentResult(result), agentError(err)
}

func (d agentAccess) ReadData(ctx context.Context) ([]byte, string, error) {
	data, etag, err := d.access.ReadData(ctx)
	return data, etag, agentError(err)
}

func (d agentAccess) WriteData(ctx context.Context, write agentlane.DataWrite) (agentlane.WriteResult, error) {
	result, err := d.access.WriteData(ctx, DataWrite{Data: write.Data, Patch: write.Patch, IfMatch: write.IfMatch})
	return agentResult(result), agentError(err)
}

func agentResult(result Result) agentlane.WriteResult {
	return agentlane.WriteResult{ETag: result.ETag, Changed: result.Changed, UndoVersion: result.UndoVersion, Stale: result.Stale}
}

// agentError is a document lane's error as this lane names it.
func agentError(err error) error {
	var stale ErrStale
	var required ErrReadRequired
	var refused ErrDataRefused
	switch {
	case err == nil:
		return nil
	case errors.As(err, &stale):
		return &agentlane.StaleError{ETag: stale.ETag, SaveID: stale.SaveID}
	case errors.As(err, &required):
		return &agentlane.ReadRequiredError{ETag: required.ETag, Message: required.Message, WouldRemove: agentlane.WouldRemove{
			Items: required.Items, Values: required.Values, Paths: append([]string{}, required.Paths...),
		}}
	case errors.Is(err, ErrNoAPIRules):
		return agentlane.ErrNoAPIRules
	case errors.Is(err, ErrPatchUnsupported):
		return agentlane.ErrPatchUnsupported
	case errors.As(err, &refused):
		return &agentlane.DataError{Status: refused.Status, Code: refused.Code, Rules: refused.Rules, Message: refused.Message}
	case errors.Is(err, ErrNotReachable):
		return agentlane.ErrNotReachable
	case errors.Is(err, ErrBackup):
		return agentlane.ErrBackup
	case errors.Is(err, ErrTooLarge):
		return agentlane.ErrTooLarge
	case errors.Is(err, ErrNotDocument):
		return agentlane.ErrNotDocument
	case errors.Is(err, fs.ErrNotExist):
		return agentlane.ErrNotFound
	}
	return err
}

var _ agentlane.Host = agentHost{}

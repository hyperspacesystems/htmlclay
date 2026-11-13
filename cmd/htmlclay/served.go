package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/pathrel"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/served"
	"github.com/panphora/htmlclay/internal/served/assets"
	"github.com/panphora/htmlclay/internal/tailwind"
	"github.com/panphora/htmlclay/internal/trust"
)

// servedMount is one served folder mounted on the app's port.
type servedMount struct {
	folder  config.ServedFolder
	host    string // <name>.htmlclay.localhost:<port>
	handler http.Handler
	ctx     context.Context
	cancel  context.CancelFunc
}

func servedHostName(name string, port int) string {
	return name + ".htmlclay.localhost:" + strconv.Itoa(port)
}

// newServedHost builds the host that owns every served folder's handler. The
// profile name is the one new versions are authored under, as --person-name is
// for a site.
func (a *app) newServedHost() *served.Host {
	person := ""
	if p := a.rt.cfg.ProfileState(); p.Enabled {
		person = p.Name
	}
	return served.NewHost(served.Options{
		Assets:          assets.ProductFS(),
		CompileTailwind: a.compileServedTailwind,
		PersonName:      person,
	})
}

// compileServedTailwind is the host's Tailwind seam: the same compiler and
// internal directory a site uses, with a budget of its own.
func (a *app) compileServedTailwind(html string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	css, err := tailwind.CompileDocument(ctx, filepath.Join(a.rt.configDir, "tailwind", "bin"), []byte(html))
	if err != nil {
		return "", err
	}
	return string(css), nil
}

// OwnerKind is which host writes a document.
type OwnerKind int

const (
	OwnerLoose OwnerKind = iota
	OwnerTrusted
	OwnerServed
)

// Owner is the one host that writes a document, and where a person reaches it.
type Owner struct {
	Kind         OwnerKind
	FolderID     string // served folder id, for OwnerServed
	FolderPath   string // the served or trusted folder, empty for a loose file
	RelativePath string // slash-separated path inside FolderPath
	PublicOrigin string // http://<name>.htmlclay.localhost:<port>, for OwnerServed
}

// URL is where a person reaches the document this Owner describes. Only a served
// document has one; a site's address is its own business.
func (o Owner) URL() string {
	if o.Kind != OwnerServed {
		return ""
	}
	return fileURL(strings.TrimPrefix(o.PublicOrigin, "http://"), o.RelativePath)
}

// ResolveDocument answers which host owns absPath. A path inside a folder that is
// being served (Serving true) is OwnerServed, compared with pathrel.SameOrBelow so a
// symlink or another letter case reaches the same answer. Otherwise a path under a
// live trusted folder is OwnerTrusted (a.trustedAnchor), else OwnerLoose.
//
// It never takes a.mu: registration asks this question while holding it (see
// buildSite's served-owner hook), so it reads only the config fact and the
// filesystem. a.trustedAnchor takes no app lock either, for the same reason.
func (a *app) ResolveDocument(absPath string) (Owner, error) {
	abs, err := filepath.Abs(absPath)
	if err != nil {
		return Owner{}, err
	}
	resolved, err := resolveSymlinks(abs)
	if err != nil {
		// The file does not exist yet, which is not a reason to answer wrongly:
		// resolve the symlinks that are already on disk and keep the rest.
		if resolved, err = pathrel.RealPath(abs); err != nil {
			return Owner{}, err
		}
	}

	for _, f := range a.rt.cfg.ServedFolderList() {
		if !f.Serving || !pathrel.SameOrBelow(resolved, f.Path) {
			continue
		}
		rel, _ := relativeWithin(resolved, f.Path)
		return Owner{
			Kind:         OwnerServed,
			FolderID:     f.ID,
			FolderPath:   f.Path,
			RelativePath: filepath.ToSlash(rel),
			PublicOrigin: "http://" + servedHostName(f.Name, a.rt.port),
		}, nil
	}

	if folder, ok := a.trustedAnchor(resolved); ok {
		rel, ok := relativeWithin(resolved, folder)
		if !ok {
			return Owner{}, fmt.Errorf("could not name %s under %s", resolved, folder)
		}
		return Owner{Kind: OwnerTrusted, FolderPath: folder, RelativePath: filepath.ToSlash(rel)}, nil
	}

	return Owner{Kind: OwnerLoose}, nil
}

// relativeWithin names path relative to dir. A plain lexical answer is enough
// whenever the two share a spelling; when they are the same folder spelled
// differently, as on a case-insensitive volume or through a link, the ancestor
// that is the same directory is found by identity instead.
func relativeWithin(path, dir string) (string, bool) {
	if rel, err := filepath.Rel(dir, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return rel, true
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		return "", false
	}
	for current := path; ; current = filepath.Dir(current) {
		if info, err := os.Stat(current); err == nil && os.SameFile(info, dirInfo) {
			rel, err := filepath.Rel(current, path)
			if err != nil {
				return "", false
			}
			return rel, true
		}
		if filepath.Dir(current) == current {
			return "", false
		}
	}
}

// startServed mounts every folder the config says is being served. A folder
// that is missing, is not a directory, or no longer matches its identity pin
// keeps its config entry but gets no mount, exactly as startSites treats a dead
// trusted folder.
func (a *app) startServed() {
	a.served = a.newServedHost()
	for _, f := range a.rt.cfg.ServedFolderList() {
		if !f.Serving {
			continue
		}
		if info, err := os.Stat(f.Path); err != nil || !info.IsDir() {
			a.rt.logger.Printf("Served folder %s is missing; its address answers with a recovery page", f.Path)
			continue
		}
		if !trust.IdentityOK(f.Path, f.Identity, a.rt.home) {
			a.rt.logger.Printf("Served folder %s failed its identity check; its address answers with a recovery page", f.Path)
			continue
		}
		if err := a.mountServed(f); err != nil {
			a.rt.logger.Printf("Could not serve folder %s: %v", f.Path, err)
			continue
		}
		a.rt.logger.Printf("Serving folder %s at http://%s", f.Path, servedHostName(f.Name, a.rt.port))
	}
	a.served.Start()
}

// mountServed adds one folder's handler to the host and records the mount the
// router looks up. The config entry must already exist.
func (a *app) mountServed(f config.ServedFolder) error {
	host := servedHostName(f.Name, a.rt.port)
	handler, err := a.served.AddRoot(served.RootSpec{ID: f.ID, Path: f.Path, PublicHost: host})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.servedMu.Lock()
	if a.servedByName == nil {
		a.servedByName = make(map[string]*servedMount)
	}
	a.servedByName[f.Name] = &servedMount{
		folder:  f,
		host:    host,
		handler: handler,
		ctx:     ctx,
		cancel:  cancel,
	}
	a.servedMu.Unlock()
	return nil
}

// serveFolder makes path a served folder under name, unless a rule in
// appcore.CheckServe refuses it. Serving is what gives every file inside it its one
// writer, so the older sites that could write those files are closed first (see
// closeSitesForServed).
func (a *app) serveFolder(path, name string) (config.ServedFolder, *appcore.Refusal, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return config.ServedFolder{}, nil, err
	}
	resolved, err := resolveSymlinks(abs)
	if err != nil {
		resolved = abs
	}

	localRoots := []string{}
	for _, root := range wireLocalRoots(wireLocalRootsPath()) {
		localRoots = append(localRoots, root.Path)
	}

	facts := appcore.GatherServeFacts(appcore.ServeInputs{
		Path:          resolved,
		Name:          name,
		Home:          a.rt.home,
		Protected:     []string{a.rt.configDir},
		Served:        a.rt.cfg.ServedFolderList(),
		Trusted:       a.rt.cfg.TrustedFolderList(),
		LocalServed:   localRoots,
		NameAvailable: a.rt.cfg.ServedNameAvailable,
	})
	if refusal := appcore.CheckServe(facts); refusal != nil {
		return config.ServedFolder{}, refusal, nil
	}

	a.closeSitesForServed(facts.Path)

	entry, err := a.rt.cfg.AddServedFolder(facts.Path, platform.DirIdentity(facts.Path), name)
	if err != nil {
		return config.ServedFolder{}, nil, err
	}
	a.saveConfig("serve " + name)

	if err := a.mountServed(entry); err != nil {
		a.rt.cfg.SetServing(entry.ID, false)
		a.saveConfig("failed serve " + name)
		return config.ServedFolder{}, nil, err
	}
	return entry, nil, nil
}

// closeSitesForServed closes every site that could still write a file inside p,
// before p becomes a served folder and its own host takes over those files.
//
// Two kinds qualify: a site whose folder (its key, or its anchor) is p or lies
// inside it, and a loose-file site whose every registration is inside p. Each is
// marked closed under a.mu first, so the router stops sending it new requests, and
// only then are the writes already in progress given a moment to finish: a save
// half-written while its origin is torn down is the very race this is here to
// prevent. The served host's own writes are atomic, so a save that outlives the
// wait leaves a whole file either way.
func (a *app) closeSitesForServed(p string) {
	a.mu.Lock()
	var older []*site
	for _, s := range a.sites {
		if s.closed.Load() {
			continue
		}
		if pathrel.SameOrBelow(s.key, p) || pathrel.SameOrBelow(s.anchor, p) {
			older = append(older, s)
			continue
		}
		regs := s.sessions.Registrations()
		if len(regs) == 0 {
			continue
		}
		inside := true
		for _, reg := range regs {
			if !pathrel.SameOrBelow(reg.Path, p) {
				inside = false
				break
			}
		}
		if inside {
			older = append(older, s)
		}
	}
	for _, s := range older {
		s.closed.Store(true)
	}
	a.mu.Unlock()

	if len(older) == 0 {
		return
	}

	if !a.waitWritesIdle(5 * time.Second) {
		a.rt.logger.Printf("Serving %s while a write to its files was still in progress", p)
	}

	a.mu.Lock()
	dropped := a.closeSitesLocked(older, p)
	a.mu.Unlock()
	for _, d := range dropped {
		a.rt.ls.DropSubscribers(d)
	}
	a.rt.logger.Printf("Closed %d older site(s) for %s: its files are served at its own address now", len(older), p)
}

// stopServing stops serving a folder but keeps it trusted and keeps its config
// entry with Serving false, so its name stays taken.
func (a *app) stopServing(id string) error {
	f, ok := a.rt.cfg.ServedFolderByID(id)
	if !ok {
		return fmt.Errorf("no served folder with id %q", id)
	}
	a.rt.cfg.SetServing(id, false)
	a.saveConfig("stop serving " + f.Name)

	a.servedMu.Lock()
	mount := a.servedByName[f.Name]
	delete(a.servedByName, f.Name)
	a.servedMu.Unlock()

	if mount != nil {
		if a.served != nil {
			a.served.RemoveRoot(id)
		}
		mount.cancel()
	}
	return nil
}

// serveServed answers a request naming <name>.htmlclay.localhost. A name that is
// not a valid served name is refused like any other bad label; a valid name with
// no mount gets the recovery page, the same answer a retired slug gets when it
// has no target.
func (a *app) serveServed(w http.ResponseWriter, r *http.Request, name string) {
	if config.ValidServedName(name) != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	a.servedMu.RLock()
	mount := a.servedByName[name]
	a.servedMu.RUnlock()
	if mount == nil {
		recoveryHeaders(w)
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		w.Write(recoveryPage)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(mount.ctx, cancel)
	defer stop()
	r = r.WithContext(ctx)
	mount.handler.ServeHTTP(w, r)
}

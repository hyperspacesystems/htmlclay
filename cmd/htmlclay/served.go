package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
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
// appcore.CheckServe refuses it. A live trusted-folder site for the same folder
// is not closed here: nothing in this build stops one without also untrusting
// the folder, and a served folder is always trusted.
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

	a.mu.Lock()
	live := a.siteAtLocked(facts.Path)
	a.mu.Unlock()
	if live != nil && live.trusted {
		return config.ServedFolder{}, nil, fmt.Errorf("a trusted-folder site is open for this folder; serving it waits for step 2.11d")
	}

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

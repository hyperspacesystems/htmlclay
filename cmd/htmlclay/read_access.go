package main

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"

	"github.com/panphora/htmlclay/internal/appcore"
)

// The read permissions list (Settings, Folders): every folder a page may read
// because the person allowed it on the read prompt, on a live site or kept for
// a site no request has reached since the restart, and Remove for each. The
// list reaches the screens through facts: watchFacts notices a new grant, and
// do() publishes after a Remove.

// readAccessID names one site's grant on one folder. It is stable for as long
// as the grant lives, and says nothing about either path.
func readAccessID(siteKey, folder string) string {
	sum := sha256.Sum256([]byte(siteKey + "\x00" + folder))
	return hex.EncodeToString(sum[:8])
}

type keyedSite struct {
	key string
	s   *site
}

// liveSites pairs each live site with its key, read under a.mu where the key
// is written (adoptLocked).
func (a *app) liveSites() []keyedSite {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]keyedSite, 0, len(a.sites))
	for _, s := range a.sites {
		out = append(out, keyedSite{key: s.key, s: s})
	}
	return out
}

func (a *app) readAccessFacts() []appcore.ReadAccess {
	var out []appcore.ReadAccess
	for _, ks := range a.liveSites() {
		if ks.s.closed.Load() {
			continue
		}
		grants := ks.s.sessions.ReadGrants()
		if len(grants) == 0 {
			continue
		}
		var pages []string
		for _, r := range ks.s.sessions.Registrations() {
			pages = append(pages, r.Path)
		}
		slices.Sort(pages)
		for _, folder := range grants {
			out = append(out, appcore.ReadAccess{ID: readAccessID(ks.key, folder), Folder: folder, Pages: pages})
		}
	}
	if t := a.tokens.Load(); t != nil {
		t.mu.Lock()
		for key, e := range t.kept {
			var pages []string
			for _, f := range e.Files {
				pages = append(pages, f.Path)
			}
			slices.Sort(pages)
			for _, folder := range e.GrantedRoots {
				out = append(out, appcore.ReadAccess{ID: readAccessID(key, folder), Folder: folder, Pages: pages})
			}
		}
		t.mu.Unlock()
	}
	slices.SortFunc(out, func(x, y appcore.ReadAccess) int {
		return cmp.Or(cmp.Compare(x.Folder, y.Folder), cmp.Compare(x.ID, y.ID))
	})
	return out
}

// removeReadAccess is readAccess.remove: it ends one grant on its live site, or
// takes it out of the kept record so the site's next request does not bring it
// back. A grant already gone is not an error: the list the person saw is older.
func (a *app) removeReadAccess(e appcore.Effect) *appcore.Refusal {
	if len(e.IDs) != 1 {
		return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "id"}
	}
	id := e.IDs[0]
	for _, ks := range a.liveSites() {
		for _, folder := range ks.s.sessions.ReadGrants() {
			if readAccessID(ks.key, folder) == id {
				ks.s.sessions.RevokeReadRoot(folder)
				return nil
			}
		}
	}
	a.tokens.Load().forgetKeptGrant(id)
	return nil
}

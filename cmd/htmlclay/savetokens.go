package main

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/session"
)

// Save permission that outlives a quit. A tab holds its document's save token,
// and before this record a quit or crash ended every token, so a tab left open
// across a restart could no longer save. save-tokens.json mirrors the live
// session tables: an addition reaches disk within keptWriteDelay, a revocation
// at once, so a crash or a forced quit loses at most that half second. After a
// restart a site takes its kept tokens back on the first request or open that
// reaches it, through the checks the update handoff uses (restoreHandoffFiles):
// the slug still mapped, the trust status unchanged, the file still the same
// file. A site nobody returns to is never rebuilt, so no folder is held open
// for it (an open read root pins a folder on Windows).
//
// The update handoff (handoff.go) is unchanged and still restores eagerly on
// an update restart; both records hold the same tokens, so whichever restores
// first wins and the other accumulates onto the same registrations.

const (
	saveTokensFile   = "save-tokens.json"
	saveTokensSchema = 1
	// keptUnusedDays: a token no request carried for this many days is not
	// carried across a restart. With unsaved-edit reports a tab holding unsaved
	// work reports every minute, so only clean, idle tabs ever reach it.
	keptUnusedDays = 30
	// keptMaxFiles bounds the record, least recently used dropped first: a
	// script that opens hundreds of documents would otherwise make every save
	// rewrite a large file.
	keptMaxFiles   = 1000
	keptWriteDelay = 500 * time.Millisecond
	keptRetryDelay = 2 * time.Second
)

type savedTokens struct {
	Schema int              `json:"schema"`
	Port   int              `json:"port"`
	Sites  []savedTokenSite `json:"sites"`
}

// savedTokenSite is one site's kept save tokens and the read grants its pages
// were given. A grant lives as long as its site keeps a file, and Stop trusting
// or serving the folder ends it here as it does on a live site.
type savedTokenSite struct {
	Key             string            `json:"key"`
	Slug            string            `json:"slug"`
	Anchor          string            `json:"anchor"`
	Trusted         bool              `json:"trusted"`
	Files           []savedTokenEntry `json:"files"`
	GrantedRoots    []string          `json:"grantedRoots,omitempty"`
	GrantIdentities map[string]string `json:"grantIdentities,omitempty"`
}

type savedTokenEntry struct {
	updateHandoffEntry
	// Used is the UTC day a request last carried the token, as 2006-01-02.
	Used string `json:"used"`
}

func (e savedTokenSite) handoff() updateHandoffSite {
	h := updateHandoffSite{Key: e.Key, Slug: e.Slug, Anchor: e.Anchor, Trusted: e.Trusted,
		GrantedRoots: e.GrantedRoots, GrantIdentities: e.GrantIdentities}
	for _, f := range e.Files {
		h.Files = append(h.Files, f.updateHandoffEntry)
	}
	return h
}

func dayString(day int64) string { return time.Unix(day*86400, 0).UTC().Format(time.DateOnly) }

func parseDay(s string) (int64, bool) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return 0, false
	}
	return session.UnixDay(t), true
}

// freshFiles keeps the entries used within keptUnusedDays of today.
func freshFiles(files []savedTokenEntry, today int64) []savedTokenEntry {
	var kept []savedTokenEntry
	for _, f := range files {
		if day, ok := parseDay(f.Used); ok && today-day <= keptUnusedDays {
			kept = append(kept, f)
		}
	}
	return kept
}

// tokenStore owns save-tokens.json: the dormant entries a restart has not
// restored yet, and the one goroutine that writes the file.
type tokenStore struct {
	a    *app
	path string

	soon    chan struct{} // an addition: write after keptWriteDelay
	now     chan struct{} // a revocation: write at once
	stop    chan struct{}
	done    chan struct{}
	stopped sync.Once

	mu sync.Mutex
	// kept holds the entries loaded at startup that no request has restored
	// yet, by site key. Restoring takes an entry out; writing prunes the ones
	// config no longer maps.
	kept map[string]savedTokenSite
	// restoreMu makes two requests that reach one dormant ad-hoc address at
	// once build its site once.
	restoreMu sync.Mutex
}

// startKeptTokens loads the record at path and starts the writer that keeps it
// current. main calls it after the update handoff is restored and before held
// requests are released, so the first request to any site finds its kept
// tokens.
func (a *app) startKeptTokens(path string) {
	t := &tokenStore{
		a:    a,
		path: path,
		soon: make(chan struct{}, 1),
		now:  make(chan struct{}, 1),
		stop: make(chan struct{}),
		done: make(chan struct{}),
		kept: map[string]savedTokenSite{},
	}
	t.load()
	a.tokens.Store(t)
	go t.run()
	// One write at once: it drops what load refused and records what the update
	// handoff restored.
	t.now <- struct{}{}
}

// keptTokensChanged is every site session manager's change hook. The manager
// calls it with its own lock held, so it only signals the writer: it never
// blocks and takes no lock.
func (a *app) keptTokensChanged(removed bool) {
	t := a.tokens.Load()
	if t == nil {
		return
	}
	ch := t.soon
	if removed {
		ch = t.now
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (t *tokenStore) load() {
	a := t.a
	data, err := os.ReadFile(t.path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		a.rt.logger.Printf("Kept save permission: cannot read %s: %v", t.path, err)
		return
	}
	var rec savedTokens
	if err := json.Unmarshal(data, &rec); err != nil {
		a.rt.logger.Printf("Kept save permission: %s is not readable (%v); starting without it", t.path, err)
		return
	}
	if rec.Schema != saveTokensSchema {
		a.rt.logger.Printf("Kept save permission: schema %d is not supported; starting without it", rec.Schema)
		return
	}
	// Every origin is slug.localhost:port, so on another port no open tab can
	// reach any of these addresses.
	if rec.Port != a.rt.port {
		a.rt.logger.Printf("Kept save permission is for port %d and this process serves %d; dropping it", rec.Port, a.rt.port)
		return
	}
	// A site the update handoff already restored holds the same tokens, and the
	// handoff, written at the update, is the newer of the two.
	restored := map[string]bool{}
	a.mu.Lock()
	for _, s := range a.sites {
		if len(s.sessions.Snapshot().Registrations) > 0 {
			restored[s.key] = true
		}
	}
	a.mu.Unlock()
	today := session.UnixDay(time.Now())
	files := 0
	for _, e := range rec.Sites {
		if restored[e.Key] {
			continue
		}
		if e.Files = freshFiles(e.Files, today); len(e.Files) == 0 {
			continue
		}
		t.kept[e.Key] = e
		files += len(e.Files)
	}
	if files > 0 {
		a.rt.logger.Printf("Kept save permission: %d file(s) in %d site(s), restored on each site's first request", files, len(t.kept))
	}
}

func (t *tokenStore) run() {
	defer close(t.done)
	var due <-chan time.Time
	for {
		select {
		case <-t.stop:
			return
		case <-t.soon:
			if due == nil {
				due = time.After(keptWriteDelay)
			}
			continue
		case <-t.now:
		case <-due:
		}
		due = nil
		if err := t.write(); err != nil {
			// On Windows a rename over a file an indexer or antivirus holds open
			// fails for a moment; the next attempt usually lands.
			t.a.rt.logger.Printf("Kept save permission: could not write %s, trying again: %v", t.path, err)
			due = time.After(keptRetryDelay)
		}
	}
}

func (t *tokenStore) write() error {
	t.a.mu.Lock()
	sites := append([]*site(nil), t.a.sites...)
	t.a.mu.Unlock()
	return t.writeSites(sites)
}

// halt stops the writer and waits for a write in progress. Safe to call twice
// and on a nil store.
func (t *tokenStore) halt() {
	if t == nil {
		return
	}
	t.stopped.Do(func() { close(t.stop) })
	<-t.done
}

// finish stops the writer and writes the record one last time from the sites
// shutdown captured, before shutdown revokes them, so the closing RevokeAll
// cannot erase what the next launch should bring back.
func (t *tokenStore) finish(sites []*site) {
	if t == nil {
		return
	}
	t.halt()
	if err := t.writeSites(sites); err != nil {
		t.a.rt.logger.Printf("Kept save permission: could not write %s at quit: %v", t.path, err)
	}
}

// writeSites writes every live registration and grant of sites, and every
// dormant entry config still maps, within the age and size bounds.
func (t *tokenStore) writeSites(sites []*site) error {
	a := t.a
	today := session.UnixDay(time.Now())

	// The site fields are written under a.mu (adoptLocked), so read them there.
	entries := make([]savedTokenSite, len(sites))
	a.mu.Lock()
	for i, s := range sites {
		entries[i] = savedTokenSite{Key: s.key, Slug: s.slug, Anchor: s.anchor, Trusted: s.trusted}
	}
	a.mu.Unlock()

	rec := savedTokens{Schema: saveTokensSchema, Port: a.rt.port, Sites: []savedTokenSite{}}
	live := map[string]bool{}
	for i, s := range sites {
		// A closed site's manager is revoked and returns nothing.
		snap := s.sessions.Snapshot()
		entry := entries[i]
		for _, r := range snap.Registrations {
			if today-r.Used > keptUnusedDays {
				continue
			}
			entry.Files = append(entry.Files, savedTokenEntry{
				updateHandoffEntry: updateHandoffEntry{Path: r.Path, Via: r.Via, Token: r.Token, Identity: platform.DirIdentity(r.Path)},
				Used:               dayString(r.Used),
			})
		}
		if len(entry.Files) == 0 {
			continue
		}
		if len(snap.GrantedRoots) > 0 {
			entry.GrantedRoots = snap.GrantedRoots
			entry.GrantIdentities = map[string]string{}
			for _, root := range snap.GrantedRoots {
				entry.GrantIdentities[root] = platform.DirIdentity(root)
			}
		}
		live[entry.Key] = true
		rec.Sites = append(rec.Sites, entry)
	}

	// The mapping check reads the disk, so it runs on a copy with t.mu released:
	// Stop trusting takes t.mu while holding a.mu (forgetKeptGrants), and must
	// never wait on a slow folder. An entry it changes meanwhile signals another
	// write, so this one may carry the old copy.
	t.mu.Lock()
	dormant := make(map[string]savedTokenSite, len(t.kept))
	maps.Copy(dormant, t.kept)
	t.mu.Unlock()
	var gone []string
	for key, e := range dormant {
		// Dormant entries keep the identity they were written with: reading it
		// again from disk would bless a file replaced while the app was down.
		e.Files = freshFiles(e.Files, today)
		if live[key] || len(e.Files) == 0 || !a.keptSiteStillMapped(e) {
			gone = append(gone, key)
			continue
		}
		rec.Sites = append(rec.Sites, e)
	}
	t.mu.Lock()
	for _, key := range gone {
		delete(t.kept, key)
	}
	for key, e := range t.kept {
		e.Files = freshFiles(e.Files, today)
		t.kept[key] = e
	}
	t.mu.Unlock()

	rec.Sites = capKeptFiles(rec.Sites, keptMaxFiles)
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return writePrivateFile(t.path, data)
}

// capKeptFiles keeps the max most recently used files across every site and
// drops a site left with none.
func capKeptFiles(sites []savedTokenSite, max int) []savedTokenSite {
	type ref struct{ site, file int }
	var all []ref
	for i, s := range sites {
		for j := range s.Files {
			all = append(all, ref{i, j})
		}
	}
	if len(all) <= max {
		return sites
	}
	// The dates are 2006-01-02, so they sort as strings.
	sort.SliceStable(all, func(x, y int) bool {
		return sites[all[x].site].Files[all[x].file].Used > sites[all[y].site].Files[all[y].file].Used
	})
	keep := map[ref]bool{}
	for _, r := range all[:max] {
		keep[r] = true
	}
	var out []savedTokenSite
	for i, s := range sites {
		var files []savedTokenEntry
		for j, f := range s.Files {
			if keep[ref{i, j}] {
				files = append(files, f)
			}
		}
		if len(files) > 0 {
			s.Files = files
			out = append(out, s)
		}
	}
	return out
}

// keptSiteStillMapped reports whether config still maps a dormant entry's
// folder the way it did when the token was kept: the same slug for the same
// key, trusted or not as before, and not served. Anything else ended that
// permission while the site was dormant, the same as it would have live.
func (a *app) keptSiteStillMapped(e savedTokenSite) bool {
	if a.rt.cfg.SiteSlug(e.Key) != e.Slug {
		return false
	}
	if f, ok := a.rt.cfg.ServedFolderForPath(e.Anchor); ok && f.Serving {
		return false
	}
	// An ad-hoc folder trusted since keeps its address (adoptLocked), so its
	// entry still belongs to it; a folder around it trusted since does not.
	anchor, inTrusted := a.trustedAnchor(e.Anchor)
	if e.Trusted {
		return inTrusted && anchor == e.Anchor
	}
	return !inTrusted || anchor == e.Anchor
}

// forgetKeptGrants takes folder's read grant out of every dormant entry, as
// closeSitesLocked takes it from every live site, so a grant Stop trusting
// ended cannot come back when its site's first request restores the entry.
// Caller holds a.mu; t.mu is only ever taken after it.
func (t *tokenStore) forgetKeptGrants(folder string) {
	if t == nil {
		return
	}
	same := func(_, root string) bool { return session.SamePath(root, folder) }
	t.forgetKeptGrantsWhere(same)
}

// forgetKeptGrant takes the one dormant grant whose read access id is id out
// of the record (readAccess.remove), and reports whether there was one.
func (t *tokenStore) forgetKeptGrant(id string) bool {
	if t == nil {
		return false
	}
	return t.forgetKeptGrantsWhere(func(key, root string) bool { return readAccessID(key, root) == id })
}

func (t *tokenStore) forgetKeptGrantsWhere(drop func(key, root string) bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	forgot := false
	for key, e := range t.kept {
		if !slices.ContainsFunc(e.GrantedRoots, func(root string) bool { return drop(key, root) }) {
			continue
		}
		// New slices and maps, never edits in place: a write in progress may be
		// encoding the old ones.
		var roots []string
		ids := map[string]string{}
		for _, root := range e.GrantedRoots {
			if !drop(key, root) {
				roots = append(roots, root)
				ids[root] = e.GrantIdentities[root]
			}
		}
		e.GrantedRoots, e.GrantIdentities = roots, ids
		t.kept[key] = e
		forgot = true
	}
	if forgot {
		t.a.keptTokensChanged(true)
	}
	return forgot
}

// keptSlugFor names the dormant ad-hoc address whose folder holds absPath, the
// most specific one, or "" when there is none.
func (t *tokenStore) keptSlugFor(absPath string) string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	anchor, slug := "", ""
	for _, e := range t.kept {
		if !e.Trusted && session.EqualOrUnder(absPath, e.Anchor) && len(e.Anchor) > len(anchor) {
			anchor, slug = e.Anchor, e.Slug
		}
	}
	return slug
}

// takeKept removes and returns the dormant entry for key, if there is one.
func (t *tokenStore) takeKept(key string) (savedTokenSite, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.kept[key]
	if ok {
		delete(t.kept, key)
	}
	return e, ok
}

// restoreKept gives a site back the kept tokens its tabs held before the last
// quit. The site's arm runs it once, on the first request or open that reaches
// the site, never under a.mu: restoring re-attaches helpers.
func (a *app) restoreKept(s *site) {
	t := a.tokens.Load()
	if t == nil {
		return
	}
	e, ok := t.takeKept(s.key)
	if !ok {
		return
	}
	a.mu.Lock()
	slug, trusted := s.slug, s.trusted
	a.mu.Unlock()
	if e.Slug != slug || (e.Trusted && !trusted) {
		a.rt.logger.Printf("Kept save permission for %s no longer matches its site; dropping it", e.Anchor)
		a.keptTokensChanged(true)
		return
	}
	h := e.handoff()
	h.Trusted = trusted
	n := a.restoreHandoffFiles(s, h, "Kept save permission")
	restoreUsedDays(s, e)
	a.rt.logger.Printf("Kept save permission: %d of %d file(s) restored for %s", n, len(e.Files), e.Anchor)
	// Files refused here leave the record now.
	a.keptTokensChanged(true)
}

// restoreKeptSlug rebuilds a dormant ad-hoc site when a request reaches its
// address and no site answers there yet, and returns it, or nil when there is
// nothing to restore. A trusted folder's site is built at startup, so only
// ad-hoc addresses reach here.
func (a *app) restoreKeptSlug(slug string) *site {
	t := a.tokens.Load()
	if t == nil {
		return nil
	}
	t.restoreMu.Lock()
	defer t.restoreMu.Unlock()
	a.hostMu.RLock()
	s := a.bySlug[slug]
	a.hostMu.RUnlock()
	if s != nil {
		// Another request rebuilt it while this one waited.
		return s
	}
	t.mu.Lock()
	var e savedTokenSite
	found := false
	for key, k := range t.kept {
		if k.Slug == slug && !k.Trusted {
			e, found = k, true
			delete(t.kept, key)
			break
		}
	}
	t.mu.Unlock()
	if !found {
		return nil
	}
	s, _ = a.restoreHandoffSite(e.handoff(), "Kept save permission")
	a.keptTokensChanged(true)
	if s == nil {
		return nil
	}
	restoreUsedDays(s, e)
	return s
}

// restoreUsedDays puts back each restored file's last-used day, so a restore
// does not count as a use and an idle tab still expires on time.
func restoreUsedDays(s *site, e savedTokenSite) {
	for _, f := range e.Files {
		day, ok := parseDay(f.Used)
		if !ok {
			continue
		}
		if live, found := s.sessions.LookupByPath(f.Path); found && live.Token == f.Token {
			live.SetUsedDay(day)
		}
	}
}

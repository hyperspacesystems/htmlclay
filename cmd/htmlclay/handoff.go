package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/panphora/htmlclay/internal/update"
	"os"
	"path/filepath"
	"time"

	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/session"
)

// updateHandoffFile is the name of the update handoff in the config directory.
const updateHandoffFile = "update-handoff.json"

// updateHandoffSchema is frozen from the first updater-capable release: version N
// writes the handoff that version N+1 reads, so a reader must keep accepting every
// schema a supported writer produced.
const updateHandoffSchema = 1

// updateHandoffMaxAge bounds how long a handoff stays usable. An update restart
// takes seconds; anything older is a leftover from an update that never finished,
// and replaying it on some later ordinary launch would resurrect capabilities the
// person had every reason to think were gone.
const updateHandoffMaxAge = 60 * time.Second

// updateHandoff is what an update restart carries from the old process to the
// new one, so every tab that was open keeps saving at the address it already has.
// It holds save tokens, so it is written 0600 and deleted on first read.
type updateHandoff struct {
	Schema        int                 `json:"schema"`
	Transaction   string              `json:"transaction"`
	Created       time.Time           `json:"created"`
	TargetVersion string              `json:"targetVersion"`
	Port          int                 `json:"port"`
	Sites         []updateHandoffSite `json:"sites"`
}

type updateHandoffSite struct {
	Key          string               `json:"key"`
	Slug         string               `json:"slug"`
	Anchor       string               `json:"anchor"`
	Trusted      bool                 `json:"trusted"`
	Files        []updateHandoffEntry `json:"files"`
	GrantedRoots []string             `json:"grantedRoots,omitempty"`
	// GrantIdentities maps each granted root to its directory's identity when
	// the handoff was written. A grant whose folder no longer has it, or that
	// has none recorded, is not restored.
	GrantIdentities map[string]string `json:"grantIdentities,omitempty"`
}

type updateHandoffEntry struct {
	Path  string             `json:"path"`
	Via   session.Provenance `json:"via"`
	Token string             `json:"token"`
	// Identity is the file's volume and file id (platform.DirIdentity) when the
	// handoff was written, after the drain. A file replaced at the same path
	// since then has another one, and its token is not restored. An entry
	// without one is refused.
	Identity string `json:"identity,omitempty"`
}

// writeUpdateHandoff captures every live site's registrations and grants into
// path, for the process that targetVersion will start. It preserves only
// capabilities that already exist; it never widens one. applyUpdate calls it
// after in-flight writes have landed and before it swaps the binary and stops
// this process.
func (a *app) writeUpdateHandoff(path, targetVersion string) error {
	txn := make([]byte, 16)
	if _, err := rand.Read(txn); err != nil {
		return fmt.Errorf("cannot generate a transaction id: %w", err)
	}
	h := updateHandoff{
		Schema:        updateHandoffSchema,
		Transaction:   hex.EncodeToString(txn),
		Created:       time.Now().UTC(),
		TargetVersion: targetVersion,
		Port:          a.rt.port,
	}

	a.mu.Lock()
	sites := append([]*site(nil), a.sites...)
	a.mu.Unlock()
	for _, s := range sites {
		if s.closed.Load() {
			continue
		}
		snap := s.sessions.Snapshot()
		if len(snap.Registrations) == 0 && len(snap.GrantedRoots) == 0 {
			continue
		}
		entry := updateHandoffSite{
			Key:             s.key,
			Slug:            s.slug,
			Anchor:          s.anchor,
			Trusted:         s.trusted,
			GrantedRoots:    snap.GrantedRoots,
			GrantIdentities: map[string]string{},
		}
		for _, root := range snap.GrantedRoots {
			entry.GrantIdentities[root] = platform.DirIdentity(root)
		}
		for _, r := range snap.Registrations {
			entry.Files = append(entry.Files, updateHandoffEntry{Path: r.Path, Via: r.Via, Token: r.Token, Identity: platform.DirIdentity(r.Path)})
		}
		h.Sites = append(h.Sites, entry)
	}

	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateFile(path, data)
}

// writePrivateFile writes data to path atomically with owner-only permissions:
// a temp file in the same directory, chmod 0600 before any token is visible
// under the final name, then rename.
func writePrivateFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Windows refuses to replace a file another process has open, and a virus
	// scanner opens a just-written file for a moment: a few short retries
	// outlast it, which matters most for the last write at quit.
	for _, wait := range []time.Duration{0, 5, 15, 40, 100} {
		time.Sleep(wait * time.Millisecond)
		if err = os.Rename(tmpPath, path); err == nil {
			return nil
		}
	}
	return err
}

// handoffPort returns the port in the handoff at path without consuming it, or
// 0 when there is no handoff written for this version.
func handoffPort(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var h updateHandoff
	if json.Unmarshal(data, &h) != nil || h.TargetVersion != version {
		return 0
	}
	return h.Port
}

// restoreUpdateHandoff reads the handoff at path, deletes it, and rebuilds the
// sites and registrations it describes with their original slugs and tokens. It
// runs at startup after startSites, so a trusted folder's site already exists
// and only gets its registrations back, and before argv is processed.
//
// A missing file is the ordinary launch and returns (0, nil). The file is
// deleted before it is parsed, so a handoff can be consumed at most once
// whatever happens next. A stale handoff, one for another version, or one for
// another port is ignored whole. Inside a usable handoff each site and each file
// is checked on its own, and anything that no longer validates is skipped and
// logged; restoring is never fatal. It returns how many registrations came back.
func (a *app) restoreUpdateHandoff(path string) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	// While an update is judged, the previous version's supervisor may still need
	// the file to hand the tabs back on a rollback, and it may read it after this
	// process starts: settleUpgradeSave removes it once the update is kept.
	if !update.Unjudged(update.UpdatesDir(a.rt.configDir)) {
		a.removeUpdateHandoff(path)
	}
	if err != nil {
		return 0, fmt.Errorf("cannot read the update handoff: %w", err)
	}

	var h updateHandoff
	if err := json.Unmarshal(data, &h); err != nil {
		return 0, fmt.Errorf("cannot parse the update handoff: %w", err)
	}
	if h.Schema != updateHandoffSchema {
		return 0, fmt.Errorf("update handoff schema %d is not supported", h.Schema)
	}
	if age := time.Since(h.Created); age < 0 || age > updateHandoffMaxAge {
		return 0, fmt.Errorf("update handoff %s is stale (created %s)", h.Transaction, h.Created.Format(time.RFC3339))
	}
	if h.TargetVersion != version {
		return 0, fmt.Errorf("update handoff %s targets version %s, this is %s", h.Transaction, h.TargetVersion, version)
	}
	// Every origin is slug.localhost:port. On another port none of the old
	// addresses exist, so restoring tokens would only re-arm capabilities no open
	// tab can reach.
	if h.Port != a.rt.port {
		return 0, fmt.Errorf("update handoff %s is for port %d, this process serves %d", h.Transaction, h.Port, a.rt.port)
	}

	restored := 0
	for _, entry := range h.Sites {
		_, n := a.restoreHandoffSite(entry, "Update handoff")
		restored += n
	}
	a.rt.logger.Printf("Update handoff %s restored %d registration(s)", h.Transaction, restored)
	return restored, nil
}

// removeUpdateHandoff deletes the handoff file; a file already gone is fine.
func (a *app) removeUpdateHandoff(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		a.rt.logger.Printf("Could not delete the update handoff %s: %v", path, err)
	}
}

// restoreHandoffSite brings one site back and returns it with how many of its
// registrations were restored (nil and 0 when none were). from names the record
// the entry came from in the log: the update handoff or the kept save tokens
// (savetokens.go), which share every check here. A trusted site must already be live, because
// startSites only builds a trusted folder that is still declared and still
// matches its identity pin; when it is not live the trust is gone and so is
// everything it granted. An ad-hoc site is rebuilt through buildSite, the same
// path route uses, and published only if at least one file came back.
func (a *app) restoreHandoffSite(entry updateHandoffSite, from string) (*site, int) {
	// The slug must still be the one config remembers for this key. That is what
	// makes the restored host the host the tab is on, and it refuses an entry
	// whose mapping changed (a retired or reassigned slug) while the app was down.
	if a.rt.cfg.SiteSlug(entry.Key) != entry.Slug {
		a.rt.logger.Printf("%s: %s is no longer remembered under %s; skipping it", from, entry.Key, entry.Slug)
		return nil, 0
	}

	if entry.Trusted {
		a.hostMu.RLock()
		s := a.bySlug[entry.Slug]
		a.hostMu.RUnlock()
		if s == nil || s.closed.Load() || !s.trusted || s.key != entry.Key {
			a.rt.logger.Printf("%s: trusted folder %s has no live site; skipping it", from, entry.Anchor)
			return nil, 0
		}
		return s, a.restoreHandoffFiles(s, entry, from)
	}

	a.hostMu.RLock()
	_, taken := a.bySlug[entry.Slug]
	a.hostMu.RUnlock()
	if taken {
		a.rt.logger.Printf("%s: %s is already live; skipping it", from, entry.Slug)
		return nil, 0
	}
	// A folder trusted since the old process registered these files belongs to
	// that folder's origin now, and an ad-hoc site for it would break invariant 4.
	// A stranger's own site inside a trusted folder is the exception: it stays
	// untrusted on purpose, and its tab keeps its origin and save token.
	if _, ok := a.trustedAnchor(entry.Anchor); ok && entry.Key == entry.Anchor {
		a.rt.logger.Printf("%s: %s is inside a trusted folder now; skipping it", from, entry.Anchor)
		return nil, 0
	}

	s := a.buildSite(entry.Anchor, entry.Key, false)
	n := a.restoreHandoffFiles(s, entry, from)

	a.mu.Lock()
	_, taken = a.bySlug[entry.Slug]
	if n == 0 || a.stopping || taken || s.slug != entry.Slug {
		a.discardPendingLocked(s)
		a.mu.Unlock()
		s.close()
		return nil, 0
	}
	a.publishLocked(s)
	a.mu.Unlock()
	a.rt.logger.Printf("%s: site restored for %s at http://%s", from, entry.Anchor, s.host)
	return s, n
}

// restoreHandoffFiles re-registers each file under its old token and then
// reinstalls the site's grants. Each file is validated on its own: it must sit
// under the site's anchor, must not have moved under a trusted folder's origin,
// must still be the same file (its recorded identity), and must pass
// RestoreRegistration, which re-runs every Register check.
func (a *app) restoreHandoffFiles(s *site, entry updateHandoffSite, from string) int {
	n := 0
	restored := make([]string, 0, len(entry.Files))
	for _, f := range entry.Files {
		if !session.EqualOrUnder(f.Path, s.anchor) {
			a.rt.logger.Printf("%s: %s is outside %s; skipping it", from, f.Path, s.anchor)
			continue
		}
		// The file must still belong to this site's folder: a folder trusted since,
		// or given its own address inside this one, owns its documents now.
		// A stranger's own site inside a trusted folder is the exception: it stays
		// untrusted on purpose, and its tab keeps its origin and save token.
		if anchor, ok := a.trustedAnchor(f.Path); s.key == s.anchor && (ok != entry.Trusted || (ok && anchor != s.anchor)) {
			a.rt.logger.Printf("%s: %s belongs to another folder's address now; skipping it", from, f.Path)
			continue
		}
		// A file opened again since the restart already has a token at another
		// address, and one file is held by one site.
		a.mu.Lock()
		other, _, open := a.lookupLocked(f.Path)
		a.mu.Unlock()
		if open && other != s {
			a.rt.logger.Printf("%s: %s is already open at another address; skipping it", from, f.Path)
			continue
		}
		// MatchDirIdentity, not equality: macOS renumbers a volume's device at each
		// mount, and the same file must still pass after a reboot.
		if f.Identity == "" || !matchesIdentity(f.Path, f.Identity, a.rt.home) {
			a.rt.logger.Printf("%s: %s is not the file the tab opened (identity changed or not recorded); skipping it", from, f.Path)
			continue
		}
		if _, err := s.sessions.RestoreRegistration(f.Path, f.Via, f.Token); err != nil {
			a.rt.logger.Printf("%s: could not restore %s: %v", from, f.Path, err)
			continue
		}
		restored = append(restored, f.Path)
		n++
	}
	if n == 0 {
		return 0
	}
	for _, root := range entry.GrantedRoots {
		// GrantCanonicalRoot deliberately does not re-resolve, so the check that
		// the folder is still the one the person approved happens here: a grant
		// whose path now resolves somewhere else is dropped, never followed.
		if resolved, err := resolveSymlinks(root); err != nil || resolved != root {
			a.rt.logger.Printf("%s: the grant on %s no longer resolves to itself; skipping it", from, root)
			continue
		}
		if id := entry.GrantIdentities[root]; id == "" || !matchesIdentity(root, id, a.rt.home) {
			a.rt.logger.Printf("%s: the folder granted at %s is not the one approved (identity changed or not recorded); skipping it", from, root)
			continue
		}
		if err := s.sessions.GrantCanonicalRoot(root); err != nil {
			a.rt.logger.Printf("%s: could not restore the grant on %s: %v", from, root, err)
		}
	}
	for _, document := range restored {
		a.applyHelperPlan(s.srv.Core, document, a.helpersForNavigation(document))
	}
	return n
}

func matchesIdentity(path, pinned, home string) bool {
	_, same := platform.MatchDirIdentity(path, pinned, home)
	return same
}

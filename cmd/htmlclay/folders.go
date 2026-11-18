package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/confirm"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/shellapi"
	"github.com/panphora/htmlclay/internal/tray"
	"github.com/panphora/htmlclay/internal/trust"
)

// trustFolder records dir as a trusted folder and brings it live. It returns a
// human-readable error for the caller to surface, nil on success.
func (a *app) trustFolder(dir string) error {
	canonical, err := a.rt.policy.Canonical(dir)
	if err != nil {
		return err
	}
	identity := platform.DirIdentity(canonical)

	a.mu.Lock()
	added := a.rt.cfg.AddTrustedFolder(canonical, identity)
	var previous string
	if !added {
		// Already listed. Approving for a folder whose stored identity no longer
		// matches — deleted and recreated by a restore, a sync client, or a clean
		// checkout — is a re-authorization of the folder now on disk, so re-pin it
		// and fall through to the save and the adopt below. Returning here instead
		// leaves the entry dead and the dialog asking on every page load, with
		// approving it never helping.
		previous, _ = a.rt.cfg.SetTrustedIdentity(canonical, identity)
	}
	// Only a folder trusted for the first time hands its trusted children their
	// own address. Approving a folder already on the list again (a re-pin) keeps
	// every child where it answers now, so no address in use moves.
	var owned []string
	if added {
		owned = a.takeOwnAddress(canonical)
	}
	if sErr := a.rt.cfg.Save(); sErr != nil {
		// Put the list back exactly as it was: drop an entry this call added,
		// restore the old pin on one it only re-pinned, and drop the addresses
		// this call handed out. Removing a pre-existing entry here would revoke a
		// folder the user still holds.
		if added {
			a.rt.cfg.RemoveTrustedFolder(canonical)
		} else {
			a.rt.cfg.SetTrustedIdentity(canonical, previous)
		}
		for _, path := range owned {
			a.rt.cfg.SetTrustedOwnAddress(path, false)
		}
		a.mu.Unlock()
		return fmt.Errorf("could not save config: %w", sErr)
	}
	a.adoptLocked(canonical)
	a.mu.Unlock()

	a.rt.logger.Printf("Trusted folder added: %s", canonical)
	if anchor, ok := a.trustedAnchor(canonical); ok && anchor != canonical {
		a.rt.logger.Printf("Trusted folder %s is covered by %s; remembered bookmarks use that origin after current folder approval", canonical, anchor)
	}
	a.bringLive(canonical)
	a.refreshHelperDispatchers()
	return nil
}

// takeOwnAddress keeps every trusted folder inside the newly trusted dir at the
// address its documents already answer at, so trusting a folder around it never
// moves them: what the browser stored for them is stored at that address.
//
// Only a folder with a remembered slug has an address to keep, and a slug in
// siteSlugs was never retired (retiring drops the entry), so its presence is the
// whole record. Nothing marks a folder at upgrade, only here, so every
// configuration written before this existed routes exactly as it did.
//
// It returns the folders it changed, for a caller whose save fails to put back.
func (a *app) takeOwnAddress(dir string) []string {
	var changed []string
	for _, tf := range a.rt.cfg.TrustedFolderList() {
		if tf.Path == dir || tf.OwnAddress || !session.EqualOrUnder(tf.Path, dir) {
			continue
		}
		if a.rt.cfg.SiteSlug(tf.Path) == "" {
			continue
		}
		if _, ok := a.rt.cfg.SetTrustedOwnAddress(tf.Path, true); ok {
			changed = append(changed, tf.Path)
		}
	}
	return changed
}

// bringLive gives a newly trusted folder its own origin now rather than at the
// next launch. It does nothing when a site is already anchored there, which is
// adoptLocked's case.
//
// The recovery page tells the user to add the folder under Trusted Folders, and
// until 1.4.0 following that instruction changed nothing until a restart:
// trustFolder only adopted an existing SITE anchored at the folder, and a parked
// port is not one. So the bookmark kept answering with the same page that had just
// told them how to fix it. Binding here is the state startSites reaches at boot
// anyway, arrived at when the user asked for it.
//
// It fires on all three doors, not only the tray, because trustFolder is the
// common tail of the picker, the page banner, and the read prompt's Trust button.
// A page-steered approval therefore binds a listener and remembers a port for a
// folder no file has been opened in. That is intended, since the folder is
// genuinely trusted either way, but it is why sitePortCap matters more than it did.
func (a *app) bringLive(canonical string) {
	a.bindLive(canonical, false)
}

// bindLive is bringLive for a caller that may have written the folder's label
// already, with the entry it belongs to: labelSaved skips the write that records
// it, so that caller's one write stays the only one.
func (a *app) bindLive(canonical string, labelSaved bool) {
	if !a.shouldBindLive(canonical) {
		return
	}

	// Build outside a.mu: it is the one call here that can block on a stalled mount.
	s := a.buildSite(canonical, canonical, true)

	a.mu.Lock()
	// Re-check everything the first pass established. An untrust or a quit can land
	// while the bind is in flight, and a concurrent open can publish a site at this
	// anchor, so publishing on the earlier answer could leave a trusted site for a
	// folder no longer in the list, which the tray cannot show or revoke.
	if !a.shouldBindLiveLocked(canonical) {
		a.mu.Unlock()
		s.close()
		return
	}
	a.publishLocked(s)
	// Persist the slug in the same critical section that publishes the site, and
	// so under the same lock shutdown takes to set stopping. Saving after the
	// unlock let a config write land after shutdown had already returned.
	if !labelSaved {
		a.saveConfig("trusting " + s.slug)
	}
	a.mu.Unlock()

	a.rt.logger.Printf("Trusted folder %s at http://%s", canonical, s.host)
}

func (a *app) shouldBindLive(canonical string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.shouldBindLiveLocked(canonical)
}

// shouldBindLiveLocked answers whether canonical still wants a site of its own.
// Caller holds a.mu.
//
// The trustedAnchor test is invariant 4, exactly as startSites applies it: only
// the broadest of a nested set gets a site. Without it, trusting a folder inside
// one already trusted builds a second origin over a tree that already has one, and
// remembers a port for a shadowed anchor. The next startSites parks that port
// instead of serving it, so the same URL would work now and be dead after a
// restart, which is the class of bug this whole item exists to remove.
func (a *app) shouldBindLiveLocked(canonical string) bool {
	if a.stopping || a.siteAtLocked(canonical) != nil {
		return false
	}
	anchor, ok := a.trustedAnchor(canonical)
	return ok && anchor == canonical
}

// adoptLocked brings a newly trusted folder live. Caller holds a.mu.
//
// This is all that remains of the three seeding loops. There is nothing to push
// into other sites: under broadest-wins anchoring a site holds at most its own
// anchor's trusted root (invariant 3), so the only site that can be affected is
// one anchored exactly here. That is also the common case, and the reason to
// handle it at all: the user opened a file, saw the read-only banner, and
// trusted that file's own folder, so the page they are looking at becomes
// editable on the same port and its address does not move.
//
// A site sitting strictly inside the new folder is deliberately left alone. Its
// files keep the tokens and the addresses they already have, and everything
// opened from now on anchors at the trusted folder, which is where the folder's
// own site gets built on the next open.
func (a *app) adoptLocked(canonical string) {
	s := a.siteAtLocked(canonical)
	if s == nil {
		return
	}
	s.trusted = true
	// Idempotent, so it does not matter whether the listener's lazy arm has
	// already fired for this site or is still to come.
	if err := s.sessions.InstallTrustedRoot(canonical); err != nil {
		a.rt.logger.Printf("Could not install trusted root %s: %v", canonical, err)
	}
}

// untrustFolder drops dir from the trusted list, closes its origin, and gives
// every file the user explicitly opened a new one.
//
// Closing the origin is what makes revocation real: the trusted root goes, the
// tokens minted under it die, and their live-sync streams are torn down. Files
// the user opened themselves survive that, because a double-click is its own
// capability and untrusting a folder was never meant to take it back; they are
// routed again, which lands them on a nested trusted folder if one covers them
// and on their own folder otherwise. That moves their port once, and it is why
// the tray asks before untrusting.
func (a *app) untrustFolder(dir string) error {
	a.mu.Lock()
	removed, ok := a.rt.cfg.RemoveTrustedFolder(dir)
	if !ok {
		a.mu.Unlock()
		return nil
	}
	// Forget the port in the same write that drops the trust, or re-homing below
	// binds it straight back and the survivor keeps the untrusted folder's origin.
	forgottenPort := a.rt.cfg.ForgetSitePort(dir)
	// Forget and retire the slug in the same write that drops the trust, so the files
	// re-homed below get a fresh address: their own folder IS this folder, and the old slug would
	// hand the untrusted folder's still-open pages a live save token on the same origin.
	forgottenSlug := a.rt.cfg.ForgetSiteSlug(dir)
	if err := a.rt.cfg.Save(); err != nil {
		// Restore what was taken out, exactly. Rebuilding the entry from the folder
		// on disk would re-pin a dead one to whatever is there now.
		a.rt.cfg.AddTrustedFolder(removed.Path, removed.Identity)
		if forgottenPort != 0 {
			a.rt.cfg.RememberSitePort(dir, forgottenPort)
		}
		if forgottenSlug != "" {
			a.rt.cfg.RestoreSiteSlug(dir, forgottenSlug)
		}
		a.mu.Unlock()
		return fmt.Errorf("could not save config: %w", err)
	}

	var reopen []session.RegistrationInfo
	var closing []*site
	if s := a.siteAtLocked(dir); s != nil {
		closing = append(closing, s)
		for _, reg := range s.sessions.Registrations() {
			if reg.Via.Has(session.ViaOsOpen) {
				reopen = append(reopen, session.RegistrationInfo{Path: reg.Path, Via: session.ViaOsOpen})
			}
		}
	}
	dropped := a.closeSitesLocked(closing, dir)
	a.mu.Unlock()

	for _, p := range dropped {
		a.rt.ls.DropSubscribers(p)
	}
	for _, tf := range a.rt.cfg.TrustedFolderList() {
		if tf.Path != dir && session.EqualOrUnder(tf.Path, dir) {
			a.bringLive(tf.Path)
		}
	}
	a.closeFolderRequests(dir)
	a.routeAgain(reopen, "untrusting "+dir)
	// A legacy per-folder port for this folder, if one is still bound, keeps answering the
	// recovery page for the rest of this run instead of redirecting: its folder is no longer
	// trusted. Its config entry was forgotten above.
	if freedPort := a.unpark(dir); freedPort != 0 {
		a.parkPortMode(dir, freedPort, recoveryPage, recoveryRevoked)
	}

	// Programs allowed only by this folder's trust stop for every document
	// still open, not just the ones re-homed above.
	a.refreshHelperDispatchers()
	a.rt.logger.Printf("Trusted folder removed: %s (%d registrations dropped, %d re-homed)", dir, len(dropped), len(reopen))
	return nil
}

// mergeFolder runs folder.merge: the child stops keeping its own address and its
// documents answer at the parent's address, after every open page has saved.
//
// Nothing in the folder moves and nothing in it is rewritten, and what the
// browser stored at the child's address stays there. The child's slug stays in
// the config as the label its old address answers with: serveRetired sends a
// navigation from it to the site that owns the file now, which is the parent.
func (a *app) mergeFolder(e appcore.Effect) *appcore.Refusal {
	childPath := a.folderPath(e.FolderID)
	parentPath := a.folderPath(e.ParentID)
	if childPath == "" || parentPath == "" {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchFolder, SubjectID: e.FolderID}
	}

	// Both folders are checked again: either can have been replaced, re-pinned or
	// untrusted since the plan, and the child can have stopped keeping its address.
	child, ok := a.trustedEntry(childPath)
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchFolder, SubjectID: e.FolderID}
	}
	parent, ok := a.trustedEntry(parentPath)
	if !ok || !child.OwnAddress || childPath == parentPath || !session.EqualOrUnder(childPath, parentPath) {
		return &appcore.Refusal{Code: appcore.RefuseNotAllowedNow, SubjectID: e.FolderID}
	}
	for _, entry := range []config.TrustedFolder{child, parent} {
		info, err := os.Stat(entry.Path)
		if err != nil || !info.IsDir() || !trust.IdentityOK(entry.Path, entry.Identity, a.rt.home) {
			return &appcore.Refusal{Code: appcore.WhyIdentityChanged, SubjectID: e.FolderID, Text: appcore.TextWhyServeIdentityChanged}
		}
	}

	if refusal := a.flushOpenPages(e.FolderID); refusal != nil {
		return refusal
	}

	a.mu.Lock()
	previousOwn, _ := a.rt.cfg.SetTrustedOwnAddress(childPath, false)
	// The port a site of the child's was parked on goes in the same write, or the
	// old address would answer from a listener the fold has no site for.
	forgottenPort := a.rt.cfg.ForgetSitePort(childPath)
	if err := a.rt.cfg.Save(); err != nil {
		a.rt.cfg.SetTrustedOwnAddress(childPath, previousOwn)
		if forgottenPort != 0 {
			a.rt.cfg.RememberSitePort(childPath, forgottenPort)
		}
		a.mu.Unlock()
		return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.FolderID}
	}
	var reopen []session.RegistrationInfo
	var closing []*site
	if s := a.siteAtLocked(childPath); s != nil {
		reopen = s.sessions.Registrations()
		closing = append(closing, s)
	}
	dropped := a.closeSitesLocked(closing, childPath)
	a.mu.Unlock()

	for _, p := range dropped {
		a.rt.ls.DropSubscribers(p)
	}
	a.routeAgain(reopen, "merging "+childPath)
	a.refreshHelperDispatchers()
	a.rt.logger.Printf("Merged trusted folder %s into %s (%d documents routed again)", childPath, parentPath, len(reopen))
	return nil
}

// warnInsideTrusted is the Trust sheet's code for a folder inside one already
// trusted: its documents answer at that folder's address now, and trusting it
// gives them an address of their own. The person has to acknowledge it before
// the trust.
const warnInsideTrusted = "inside-trusted"

// trustFromAction runs folder.trust, the Trust sheet's door. It is the one door
// that shows a warning and carries the answer back, so it is the one door where
// a folder inside a trusted folder gets its own address. The picker and the two
// page doors record such a folder under the folder around it, as they always
// have, and move nothing.
func (a *app) trustFromAction(e appcore.Effect) *appcore.Refusal {
	canonical, err := a.rt.policy.Canonical(e.Path)
	if err != nil {
		a.rt.logger.Printf("Could not trust folder %s: %v", e.Path, err)
		return &appcore.Refusal{Code: appcore.RefuseCannotTrust}
	}
	// Read again here, the way the relink and the program effects read their
	// warnings: a folder that came to sit inside a trusted one after the sheet
	// asked is never moved on the sheet's word.
	if around, inside := a.trustedAround(canonical); inside {
		if !slices.Contains(e.Acknowledged, warnInsideTrusted) {
			return &appcore.Refusal{Code: appcore.RefuseNeedsConfirmation}
		}
		return a.trustInsideTrusted(canonical, around)
	}
	if err := a.trustFolder(canonical); err != nil {
		a.rt.logger.Printf("Could not trust folder %s: %v", canonical, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	return nil
}

// trustedAround answers the live trusted folder whose address dir's documents
// answer at now, for a dir that is not on the trusted list. That is the one trust
// that moves documents. A folder already on the list keeps the address it has
// when it is approved again, and a served folder's documents stay at the served
// address whatever is trusted inside it, so neither has a folder around it here.
func (a *app) trustedAround(dir string) (string, bool) {
	if _, listed := a.trustedEntry(dir); listed {
		return "", false
	}
	owner, err := a.ResolveDocument(dir)
	if err != nil || owner.Kind != OwnerTrusted {
		return "", false
	}
	return owner.FolderPath, true
}

// trustInsideTrusted is mergeFolder in reverse: canonical, which sits inside the
// trusted folder around, is trusted with an address of its own, after every open
// page has saved.
//
// Nothing in the folder moves and nothing in it is rewritten, and what the
// browser stored at around's address stays there. One config write records the
// entry, that it keeps its own address, and the label that address answers with;
// when it fails, nothing has changed. After it, around's site lets go of the
// documents it had open from canonical, so around's address redirects their pages
// to the new one and no longer saves them.
func (a *app) trustInsideTrusted(canonical, around string) *appcore.Refusal {
	if refusal := a.flushOpenPages(""); refusal != nil {
		return refusal
	}
	identity := platform.DirIdentity(canonical)

	a.mu.Lock()
	// The flush ran without the lock, so the folder around is read once more.
	if now, inside := a.trustedAround(canonical); !inside || now != around {
		a.mu.Unlock()
		return &appcore.Refusal{Code: appcore.RefuseNotAllowedNow}
	}
	a.rt.cfg.AddTrustedFolder(canonical, identity)
	a.rt.cfg.SetTrustedOwnAddress(canonical, true)
	// A label remembered from before the folder around was trusted is the address
	// these documents had then, so it is the one they go back to.
	freshSlug := a.rt.cfg.SiteSlug(canonical) == ""
	a.slugForLocked(canonical)
	if err := a.rt.cfg.Save(); err != nil {
		a.rt.cfg.RemoveTrustedFolder(canonical)
		if freshSlug {
			a.rt.cfg.DropSiteSlug(canonical)
		}
		a.mu.Unlock()
		a.rt.logger.Printf("Could not trust folder %s: %v", canonical, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	a.adoptLocked(canonical)
	var moved []session.RegistrationInfo
	previous := a.siteAtLocked(around)
	if previous != nil {
		for _, reg := range previous.sessions.Registrations() {
			if session.EqualOrUnder(reg.Path, canonical) && previous.sessions.Unregister(reg.Path) {
				moved = append(moved, reg)
			}
		}
	}
	a.mu.Unlock()

	// What closing a whole site does for every document it held, done for the ones
	// that left a site that stays open.
	for _, reg := range moved {
		previous.srv.Core.Forget(reg.Path)
		a.rt.ls.DropSubscribers(reg.Path)
	}
	a.bindLive(canonical, true)
	a.routeAgain(moved, "trusting "+canonical)
	a.refreshHelperDispatchers()
	a.rt.logger.Printf("Trusted folder %s inside %s at its own address (%d documents routed again)", canonical, around, len(moved))
	return nil
}

// flushOpenPages is the flush host.quit uses, run before a change that moves open
// documents to another address: a page that still holds edits stops the change,
// and then nothing at all has changed.
func (a *app) flushOpenPages(subjectID string) *appcore.Refusal {
	if names, answered := a.unsavedDocuments(); !answered || len(names) > 0 {
		return &appcore.Refusal{Code: appcore.RefuseUnsavedEdits, SubjectID: subjectID, Documents: names}
	}
	return nil
}

// routeAgain registers documents again after a change took them from the site
// that held them, each with the provenance it is given, and attaches its programs
// where it lands. A document a served folder holds is left at its served address.
func (a *app) routeAgain(regs []session.RegistrationInfo, after string) {
	for _, reg := range regs {
		if owner, err := a.ResolveDocument(reg.Path); err == nil && owner.Kind == OwnerServed {
			a.rt.logger.Printf("Leaving %s at its served address %s", reg.Path, owner.URL())
			continue
		}
		s, _, ok := a.route(reg.Path, reg.Via)
		if !ok {
			a.rt.logger.Printf("Could not route %s again after %s", reg.Path, after)
			continue
		}
		a.applyHelperPlan(s.srv.Core, reg.Path, a.helpersForOpen(reg.Path, false))
	}
}

// trustedEntry answers the trusted folder at path, if there is one.
func (a *app) trustedEntry(path string) (config.TrustedFolder, bool) {
	for _, tf := range a.rt.cfg.TrustedFolderList() {
		if tf.Path == path {
			return tf, true
		}
	}
	return config.TrustedFolder{}, false
}

// closeSitesLocked closes every site in closing, records the registration paths they
// held, and takes folder's roots back from every site still live. Caller holds a.mu
// and has already marked each of them closed if the router must stop dispatching to
// it. The returned paths are the ones whose live-sync subscribers the caller must
// drop once it has let go of a.mu.
func (a *app) closeSitesLocked(closing []*site, folder string) []string {
	var dropped []string
	for _, s := range closing {
		for _, reg := range s.sessions.Registrations() {
			dropped = append(dropped, reg.Path)
		}
		s.close()
		a.removeSiteLocked(s)
	}
	// One "Trust this folder" click on the read prompt sets both the granted and
	// the trusted flag on a single read root, so clearing trust alone would leave
	// the granted flag holding the root open and the folder still readable.
	// Untrusting takes back everything that click gave. A folder the user
	// explicitly OPENED survives, because that root is what its page is served
	// from.
	for _, other := range a.sites {
		other.sessions.RevokeTrustedRoot(folder)
		other.sessions.RevokeReadRoot(folder)
	}
	return dropped
}

// removeSiteLocked drops s from the registry. Caller holds a.mu and has already
// closed s.
func (a *app) removeSiteLocked(s *site) {
	kept := a.sites[:0]
	for _, other := range a.sites {
		if other != s {
			kept = append(kept, other)
		}
	}
	a.sites = kept
	a.hostMu.Lock()
	delete(a.bySlug, s.slug)
	a.hostMu.Unlock()
}

// mayTrustFromPrompt gates the read prompt's third button. The dialog does not
// offer to trust a folder that trustFromPrompt would then refuse: before this,
// the button was always drawn and the refusal arrived afterwards as a
// notification, which asked the user to make a choice that could not be honored.
func (a *app) mayTrustFromPrompt(dir string) bool {
	return !a.rt.policy.RefuseSteered(dir)
}

// trustFromPrompt records a folder on behalf of a permission dialog the user
// answered with "Trust this folder". It is deliberately stricter than the tray's
// own picker: here the PAGE chooses the folder being offered, by picking which
// out-of-scope assets to request, so it can aim the common ancestor at one of
// the personal folders where files the user never wrote tend to land, and offer
// one-click trust for the whole tree. Trusting one of those from the tray is
// still allowed, because that takes a deliberate act with a folder picker.
//
// The refusal runs again here even though mayTrustFromPrompt already suppressed
// the button, because this grants write and the gate is a UI decision made
// earlier, on a path the server owns.
//
// The caller has already installed the session read root, so refusing here costs
// only durability: the page keeps working now and asks again next launch. The
// user is told, because they asked for something permanent and got something
// temporary.
func (a *app) trustFromPrompt(dir string) error {
	if a.rt.policy.RefuseSteered(dir) {
		a.reportTrustRefused(dir, "A page picked this folder, and it is one of your main personal folders.")
		return fmt.Errorf("%s cannot be trusted from a permission prompt", dir)
	}
	if err := a.trustFolder(dir); err != nil {
		a.reportTrustRefused(dir, err.Error())
		return err
	}
	return nil
}

// trustFromPage handles a page's request to trust its own folder. It is the one
// door behind both the read-only banner's button and an editable page's own
// request, because after the banner started trusting a folder rather than
// opening a single file, the two became the same act reached through different
// front doors (a nonce and a token).
//
// The folder is derived HERE from the requesting file: the page never names one,
// so it cannot steer the choice, and the looser refusal rule is the correct one.
// The server has already required that the user acted on the file, either from
// the desktop or through the banner. The dialog names the requesting file and
// the full folder path, states the consequence in full, and says up front when
// the file was reached by a link.
func (a *app) trustFromPage(requestingFile string, openedByUser bool) (string, bool) {
	// Already inside a trusted folder: trusting again grants nothing, so answer
	// with where the file serves and raise no dialog. A page that asks on every
	// load prompts once, not once per reload.
	if _, trusted := a.anchorFor(requestingFile); trusted {
		return a.serveURL(requestingFile)
	}

	folder := filepath.Dir(requestingFile)
	canonical, err := a.rt.policy.Canonical(folder)
	if err != nil {
		a.rt.logger.Printf("Trust request for %s refused: %v", folder, err)
		return "", false
	}
	if a.rt.policy.RefuseOwnFolder(canonical) {
		a.rt.logger.Printf("Trust request refused for protected folder %s", canonical)
		a.reportPageTrustRefused(canonical)
		return "", false
	}

	reached := ""
	if !openedByUser {
		reached = "You reached this file from a link rather than opening it yourself.\n\n"
	}
	msg := fmt.Sprintf("%s%s wants to trust its folder:\n\n%s\n\nEvery HTML Clay file in that folder becomes editable without asking, including files added later, and any file in it will be able to change any other. Only allow this for a folder you control completely.",
		reached, requestingFile, canonical)
	detail := ""
	if a.isMainFolder(canonical) {
		detail = "main-folder"
	}
	ask := confirm.Ask{
		Kind:     appcore.RequestFolderReadAgain,
		Document: requestingFile,
		Folder:   filepath.Base(canonical),
		Path:     canonical,
		Detail:   detail,
		Answers:  []string{confirm.ChoiceDeny, readChoiceTrustFolder},
	}
	reply, err := a.trustConfirmer("HTML Clay", msg).Ask(context.Background(), ask)
	if err != nil {
		a.rt.logger.Printf("Trust dialog error for %s: %v", canonical, err)
		return "", false
	}
	if reply.Choice != readChoiceTrustFolder {
		return "", false
	}
	if err := a.trustFolder(canonical); err != nil {
		a.rt.logger.Printf("Could not trust folder %s: %v", canonical, err)
		return "", false
	}
	a.rt.logger.Printf("Folder %s trusted from a page request by %s", canonical, requestingFile)
	return a.serveURL(requestingFile)
}

// serveURL routes absPath, attaches its helper dispatcher, and reports where it
// is served. A file already registered keeps its origin; one that is not gets
// the origin its anchor now implies, which after a trust is the trusted
// folder's. Attaching here rather than only in openFile is what lets a document
// reached by navigation, a bookmark or the banner's trust button use a helper it
// already has permission for: those paths never pass through openFile, so
// without this the document reports every declared name as unavailable and a
// reload cannot change it.
func (a *app) serveURL(absPath string) (string, bool) {
	// A served file is reached at its served address; it is not a site's to register.
	if owner, err := a.ResolveDocument(absPath); err == nil && owner.Kind == OwnerServed {
		return owner.URL(), true
	}
	s, rel, ok := a.route(absPath, session.ViaTrusted)
	if !ok {
		return "", false
	}
	// Browser navigation restores stored permissions; only an explicit file
	// open should ask the user to approve a new program.
	a.applyHelperPlan(s.srv.Core, absPath, a.helpersForNavigation(absPath))
	return s.url(rel), true
}

// confirmTrustRequest raises a two-button dialog through its test seam,
// defaulting to the real native one. It is a separate seam from the read
// prompt's confirm so a test that approves one class of dialog can never
// silently approve the other.
//
// affirmative is what the button the user clicks actually says, and it is a
// parameter because this helper serves both directions: the same dialog asks to
// trust a folder and to stop trusting one. A fixed "Trust Folder" label shipped
// on the removal dialog in 1.3.0, so "Stop trusting this folder?" was answered
// by clicking a button that said Trust Folder, and Deny was the one that kept
// the trust. Every label was the opposite of its action.
func (a *app) confirmTrustRequest(title, message, affirmative string) (bool, error) {
	if a.rt.confirmTrust != nil {
		return a.rt.confirmTrust(title, message, affirmative)
	}
	return a.dialogs().confirmWithButtons(title, message, affirmative)
}

// confirmTrayChange is confirmTrustRequest for a change the user started from
// the tray, so the refusing button says Cancel rather than Deny. It shares the
// confirmTrust seam: the tests that drive it answer the dialog, not its labels.
func (a *app) confirmTrayChange(title, message, affirmative string) (bool, error) {
	if a.rt.confirmTrust != nil {
		return a.rt.confirmTrust(title, message, affirmative)
	}
	return a.dialogs().confirmChange(title, message, affirmative)
}

// reportTrustRefused tells the user why a folder was not remembered. Staying
// silent is its own bug: they asked for something durable and got something that
// lasts until they quit, so without this the folder simply starts asking again
// with no explanation. It covers ordinary failures too, such as a config file
// that cannot be written, not only the protected-folder policy refusal.
//
// The notification is detached because this runs on the broker's prompt goroutine,
// which must not block: a notification that wedges (on Windows it is a modal that
// waits to be clicked) would leave the broker marked as prompting, and the site
// would never raise another permission dialog for the rest of its life.
func (a *app) reportTrustRefused(dir, reason string) {
	msg := dir + " was not added to your trusted folders.\n\n" + reason +
		"\n\nIt stays readable until you quit. You can add it yourself from the HTML Clay menu."
	go func() {
		if nErr := a.notifyUser("HTML Clay", msg); nErr != nil {
			a.rt.logger.Printf("Could not notify about the refused trust of %s: %v", dir, nErr)
		}
	}()
}

// reportPageTrustRefused tells the user a page asked for a protected folder.
// Detached for the same reason as reportTrustRefused: this runs on a prompt path
// that must never block.
func (a *app) reportPageTrustRefused(dir string) {
	msg := dir + " was not trusted.\n\nA page requested it, and it is one of your main personal folders. You can add a trusted folder yourself from the HTML Clay menu."
	go func() {
		if nErr := a.notifyUser("HTML Clay", msg); nErr != nil {
			a.rt.logger.Printf("Could not notify about the refused trust of %s: %v", dir, nErr)
		}
	}()
}

// trustedFolderRows returns the tray's rows. The label carries the dead marker
// and the path is carried alongside it, so removal never has to recover a path
// by stripping a suffix off a label the user might have produced themselves by
// naming a folder that way.
func (a *app) trustedFolderRows() []tray.Row {
	list := a.rt.cfg.TrustedFolderList()
	out := make([]tray.Row, 0, len(list))
	for _, tf := range list {
		label := tf.Path
		if info, err := os.Stat(tf.Path); err != nil || !info.IsDir() || !trust.IdentityOK(tf.Path, tf.Identity, a.rt.home) {
			// The entry stays listed: it is the record of a standing write grant,
			// and it must surface as dead rather than silently vanish.
			label += " (missing or replaced)"
		} else if outer, ok := a.shadowedBy(tf.Path); ok {
			// Dead wins over covered when both apply: a dead entry grants nothing
			// whatever encloses it.
			label += " (covered by " + filepath.Base(outer) + ")"
		}
		out = append(out, tray.Row{Path: tf.Path, Label: label})
	}
	return out
}

// shadowedBy names the live trusted folder that owns dir's tree, when that is not
// dir itself.
//
// Broadest-wins means an enclosing trusted folder anchors everything under it, so
// the inner entry is a declaration with no present effect: untrusting it closes
// nothing and its files stay editable. The tray said the opposite, promising that
// files would stop opening editable and open pages would need reopening, and a
// brand new file created in the folder afterwards still served with a save token.
//
// It asks trustedAnchor rather than testing containment lexically, so an outer
// folder whose identity pin no longer matches does not count: the inner entry is
// then genuinely in charge and its removal really does revoke.
func (a *app) shadowedBy(dir string) (string, bool) {
	anchor, ok := a.trustedAnchor(dir)
	if !ok || anchor == dir {
		return "", false
	}
	return anchor, true
}

// pickAndTrustFolder pops the native folder picker and trusts the choice. The
// picker route deliberately skips the page-request refusal list: a deliberate
// act with a folder picker may choose anything Canonical accepts. It does warn
// once for a personal folder, because picking ~/Documents by accident from a
// file dialog is easy and the consequence is the whole tree.
func (a *app) pickAndTrustFolder() []tray.Row {
	dir, ok, err := a.selectFolder("Choose a folder to trust. Every HTML Clay file inside it (including files added later) opens editable with no prompts, and any file in it can change any other.")
	if err != nil {
		a.rt.logger.Printf("Folder picker failed: %v", err)
		return a.trustedFolderRows()
	}
	if !ok {
		return a.trustedFolderRows()
	}
	canonical, cErr := a.rt.policy.Canonical(dir)
	if cErr == nil && a.rt.policy.IsPersonal(canonical) {
		msg := fmt.Sprintf("%s is one of your main personal folders.\n\nTrusting it makes every HTML Clay file anywhere inside it editable with no prompts, and lets any of them change any other. Most people want a single project folder instead.", canonical)
		proceed, cfErr := a.confirmTrayChange("HTML Clay", msg, "Trust folder")
		if cfErr != nil || !proceed {
			return a.trustedFolderRows()
		}
	}
	if cErr == nil {
		cErr = a.trustFolder(canonical)
	}
	if cErr != nil {
		a.rt.logger.Printf("Could not trust folder %s: %v", dir, cErr)
		go func() {
			if nErr := a.notifyUser("HTML Clay can't trust this folder", cErr.Error()); nErr != nil {
				a.rt.logger.Printf("Could not show notification: %v", nErr)
			}
		}()
	}
	return a.trustedFolderRows()
}

// removeTrustedFolder is the tray's remove hook. It asks first, because a
// misclick now closes a live origin and retires the address a bookmark points at.
//
// A folder covered by a broader trusted folder gets different wording, because
// for it none of that is true: the outer folder still anchors the tree, so nothing
// closes and nothing stops opening editable. Removing it is still worth allowing,
// since it is the user's own declaration and it takes effect the moment the outer
// folder goes, but the dialog must not claim a revocation it will not perform.
func (a *app) removeTrustedFolder(dir string) []tray.Row {
	msg := fmt.Sprintf("Stop trusting this folder?\n\n%s\n\nFiles in it will stop opening editable, and any of its pages you have open now will need to be opened again.", dir)
	affirmative := "Stop Trusting"
	if outer, ok := a.shadowedBy(dir); ok {
		a.mu.Lock()
		ownSite := a.siteAtLocked(dir) != nil
		a.mu.Unlock()
		msg = fmt.Sprintf("Remove this folder from your trusted list?\n\n%s\n\nIt sits inside %s, which you also trust, so its files stay editable and its pages stay open. Removing it here takes effect if you stop trusting that folder too.", dir, outer)
		if ownSite {
			// Trusted before the outer folder, so it still has its own address
			// this run, and untrustFolder closes it.
			msg = fmt.Sprintf("Remove this folder from your trusted list?\n\n%s\n\nIt sits inside %s, which you also trust, so its files stay editable. Pages you have open from it now use this folder's own address, which closes, so they will need to be opened again.", dir, outer)
		}
		affirmative = "Remove"
	}
	proceed, err := a.confirmTrayChange("HTML Clay", msg, affirmative)
	if err != nil || !proceed {
		return a.trustedFolderRows()
	}
	if err := a.untrustFolder(dir); err != nil {
		a.rt.logger.Printf("Could not untrust folder %s: %v", dir, err)
	}
	return a.trustedFolderRows()
}

// warnDifferentFolder is the Find it sheet's code for a folder at the new path that
// is not the one that moved. The person has to acknowledge it before the relink.
const warnDifferentFolder = "different-folder"

// folderRelinkPreview is the Find it sheet's answer: the folder as it will be
// stored, and the codes the person has to acknowledge. The effect reads the same
// warnings, so a folder that changed between the sheet and the click is never
// relinked on the sheet's word.
func (a *app) folderRelinkPreview(folderID, path string) (shellapi.Preview, *appcore.Refusal) {
	entry, ok := trustedFolderByID(a.rt.cfg.TrustedFolderList(), folderID)
	if !ok {
		return shellapi.Preview{}, &appcore.Refusal{Code: appcore.RefuseNoSuchFolder, SubjectID: folderID}
	}
	canonical, refusal := a.relinkCanonical(path, entry.ID)
	if refusal != nil {
		return shellapi.Preview{}, refusal
	}
	if served, isServed := a.rt.cfg.ServedFolderByID(entry.ID); isServed {
		if _, refusal := a.serveFactsFor(canonical, served); refusal != nil {
			return shellapi.Preview{}, refusal
		}
	}
	return shellapi.Preview{
		Path:     canonical,
		Name:     filepath.Base(canonical),
		Warnings: a.relinkWarnings(canonical, a.relinkPin(entry)),
	}, nil
}

// relinkFolder runs folder.relink: the entry keeps its id, name, address and frame
// setting and points at the folder's new path. A folder that moved keeps its
// identity, so nothing needs acknowledging. A different folder there needs every
// warning the sheet showed. A team folder stays one: the new path joins the team
// record first and the config is saved second, so a stop between the two leaves the
// stricter state.
func (a *app) relinkFolder(e appcore.Effect) *appcore.Refusal {
	entry, ok := trustedFolderByID(a.rt.cfg.TrustedFolderList(), e.FolderID)
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchFolder, SubjectID: e.FolderID}
	}
	served, isServed := a.rt.cfg.ServedFolderByID(entry.ID)
	canonical, refusal := a.relinkCanonical(e.Path, entry.ID)
	if refusal != nil {
		return refusal
	}
	if a.syncDownloadRunning(entry.ID) {
		return &appcore.Refusal{Code: appcore.RefuseSyncDownloading, SubjectID: entry.ID}
	}
	// A served folder is served from its new path, so the new path obeys every rule a folder
	// served there for the first time would: never inside or around another served folder.
	if isServed {
		if _, refusal := a.serveFactsFor(canonical, served); refusal != nil {
			return refusal
		}
	}
	for _, warning := range a.relinkWarnings(canonical, a.relinkPin(entry)) {
		if !slices.Contains(e.Acknowledged, warning.Code) {
			return &appcore.Refusal{Code: appcore.RefuseNeedsConfirmation, SubjectID: entry.ID}
		}
	}
	identity := platform.DirIdentity(canonical)
	if refusal := a.carryTeamFolder(entry.Path, canonical, entry.ID, identity); refusal != nil {
		return refusal
	}

	previousTrusted, ok := a.rt.cfg.RelinkTrustedFolder(entry.ID, canonical, identity)
	if !ok {
		return &appcore.Refusal{Code: appcore.RefuseNoSuchFolder, SubjectID: entry.ID}
	}
	var previousServed config.ServedFolder
	if isServed {
		var servedOK bool
		previousServed, servedOK = a.rt.cfg.RelinkServedFolder(entry.ID, canonical, identity)
		if !servedOK {
			a.rt.cfg.RelinkTrustedFolder(previousTrusted.ID, previousTrusted.Path, previousTrusted.Identity)
			return &appcore.Refusal{Code: appcore.RefuseNoSuchFolder, SubjectID: entry.ID}
		}
	}
	if err := a.rt.cfg.Save(); err != nil {
		a.rt.cfg.RelinkTrustedFolder(previousTrusted.ID, previousTrusted.Path, previousTrusted.Identity)
		if isServed {
			a.rt.cfg.RelinkServedFolder(previousServed.ID, previousServed.Path, previousServed.Identity)
		}
		a.rt.logger.Printf("Could not relink the folder %s: %v", entry.Path, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: entry.ID}
	}
	a.rt.logger.Printf("Folder relinked: id=%s from=%s to=%s", entry.ID, entry.Path, canonical)

	a.bringLive(canonical)
	if isServed {
		served.Path, served.Identity = canonical, identity
		a.moveRelinkedMount(served)
	}
	a.refreshHelperDispatchers()
	a.changed()
	return nil
}

// syncDownloadRunning: a folder's first download holds its old path in the
// service until it ends, so the folder is found again only after that.
func (a *app) syncDownloadRunning(folderID string) bool {
	a.rt.syncMu.Lock()
	defer a.rt.syncMu.Unlock()
	record, err := a.syncRecordLocked()
	if err != nil {
		return false
	}
	for _, session := range record.SyncSessions {
		if session.RootID == folderID && a.rt.syncBinding[session.ID] {
			return true
		}
	}
	return false
}

// relinkCanonical is the folder a relink would point an entry at, as it will be
// stored, or why the path cannot be used: a folder that is not there, or one
// another entry already holds.
func (a *app) relinkCanonical(path, folderID string) (string, *appcore.Refusal) {
	canonical, err := a.rt.policy.Canonical(path)
	if err != nil {
		return "", &appcore.Refusal{Code: appcore.RefuseCannotTrust}
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return "", &appcore.Refusal{Code: appcore.RefuseNotAFolder}
	}
	for _, other := range a.rt.cfg.TrustedFolderList() {
		if other.ID != folderID && other.Path == canonical {
			return "", &appcore.Refusal{Code: appcore.RefuseNotAllowedNow, SubjectID: folderID, OtherPath: canonical}
		}
	}
	return canonical, nil
}

// relinkPin is the identity the entry is pinned to. A served entry carries its own
// copy of the pin and the facts read that one, so it wins when the two disagree.
func (a *app) relinkPin(entry config.TrustedFolder) string {
	if served, ok := a.rt.cfg.ServedFolderByID(entry.ID); ok && served.Identity != "" {
		return served.Identity
	}
	return entry.Identity
}

// relinkWarnings is what a person should know before an entry is pointed at
// canonical: whether the folder there is the one that moved, then the Trust sheet's
// own warnings about the new location.
func (a *app) relinkWarnings(canonical, pin string) []shellapi.Warning {
	var warnings []shellapi.Warning
	if pin == "" || !trust.IdentityOK(canonical, pin, a.rt.home) {
		warnings = append(warnings, shellapi.Warning{Code: warnDifferentFolder, Text: appcore.TextWarnDifferentFolder})
	}
	return append(warnings, a.folderWarnings(canonical)...)
}

// carryTeamFolder keeps a relinked team folder a team folder. A record entry the new
// folder's identity matches already covers it, wherever the folder has gone.
// Otherwise the new path joins the team record before the config changes: a stop
// between the two leaves the old path marked too, which grants no more than before.
func (a *app) carryTeamFolder(oldPath, canonical, folderID, identity string) *appcore.Refusal {
	team, ok := a.teamFolderFor(oldPath)
	if !ok {
		return nil
	}
	for _, candidate := range a.rt.cfg.TeamFolderList() {
		if candidate.Kind == config.TeamFolderKindTeam && identity != "" && candidate.Identity == identity {
			return nil
		}
	}
	entry := config.TeamFolder{
		FolderID: folderID,
		Path:     canonical,
		Identity: identity,
		Kind:     config.TeamFolderKindTeam,
		Key:      team.Key,
	}
	if err := a.rt.cfg.MarkTeamFolder(entry); err != nil {
		a.rt.logger.Printf("Could not keep folder %s in the team record: %v", canonical, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: folderID}
	}
	a.rt.logger.Printf("Team folder relinked: id=%s from=%s to=%s", folderID, team.Path, canonical)
	return nil
}

// teamFolderFor answers the team record entry that makes path a team folder, by the
// same rules program decisions use: the folder whose path holds it, or whose
// identity it carries.
func (a *app) teamFolderFor(path string) (config.TeamFolder, bool) {
	scope := a.scopeOf(path)
	if scope.Kind != config.ScopeTeam {
		return config.TeamFolder{}, false
	}
	for _, folder := range a.rt.cfg.TeamFolderList() {
		if folder.Kind == config.TeamFolderKindTeam && folder.Scope() == scope {
			return folder, true
		}
	}
	return config.TeamFolder{}, false
}

// moveRelinkedMount moves a served folder's address to its new path, so it answers
// from the folder that moved rather than from the one that is no longer there. A
// mount the folder already has is dropped first, because a root is served once: its
// files moved with the folder, so its copy is taken away the way a stop serving
// takes it, and the service is asked to let go before that.
func (a *app) moveRelinkedMount(entry config.ServedFolder) {
	if a.served == nil || !entry.Serving {
		return
	}
	a.servedMu.Lock()
	mount := a.servedByName[entry.Name]
	delete(a.servedByName, entry.Name)
	a.servedMu.Unlock()
	if mount != nil {
		if _, err := a.service.removeRoot(entry.ID); err != nil {
			a.rt.logger.Printf("The live-room service did not answer while relinking %s: %v", entry.Name, err)
		}
		a.served.RemoveRoot(entry.ID)
		mount.cancel()
		if mount.lock != nil {
			mount.lock.Release()
		}
	}
	if err := a.mountServed(entry); err != nil {
		a.rt.logger.Printf("Could not serve the folder %s at its new path: %v", entry.Path, err)
	}
}

// trustedFolderByID answers the trusted entry with id.
func trustedFolderByID(folders []config.TrustedFolder, id string) (config.TrustedFolder, bool) {
	for _, folder := range folders {
		if folder.ID == id {
			return folder, true
		}
	}
	return config.TrustedFolder{}, false
}

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/confirm"
	"github.com/panphora/htmlclay/internal/pathrel"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/recents"
	"github.com/panphora/htmlclay/internal/trust"
)

// Channel is where an action came from. It decides which classes it may send.
type Channel int

const (
	ChannelTray      Channel = iota + 1 // the tray menu, in this process
	ChannelShell                        // the Mac app over the private listener
	ChannelDashboard                    // a browser page on the dashboard origin
)

// coreAnswerLimit is how many action ids the core remembers an answer for.
const coreAnswerLimit = 256

// coreChangeDelay is how long a change waits for company before the core builds
// one snapshot and tells the subscribers.
const coreChangeDelay = 100 * time.Millisecond

const (
	factsWatchEvery = 2 * time.Second
	docCountTTL     = 30 * time.Second
	docCountMaxWalk = 20000 // entries; a folder past this shows the count so far
)

// coreState is the core's own state on app: rev, epoch, seen waiting ids,
// recent action answers, subscribers.
type coreState struct {
	mu          sync.Mutex
	rev         uint64
	epoch       string                // 16 lowercase hex, random at startup
	seen        map[string]bool       // waiting ids marked seen this run
	answers     map[string]coreAnswer // last 256 action ids
	answerOrder []string
	subs        map[int]chan appcore.Snapshot
	nextSub     int
	pending     *time.Timer       // coalescing timer, nil when idle
	requests    *confirm.Registry // the questions waiting for the person

	watching   bool                // the facts watcher runs while anyone subscribes
	lastFacts  [32]byte            // digest of the facts the watcher last saw
	countCache map[string]docCount // folder path -> its document count, refreshed every docCountTTL
	// versionsKept remembers how many versions each document has kept, with the
	// document's stat when it was read: either host keeps a version only while
	// writing the document, so a document that changed has a count to read again.
	versionsKept map[string]keptVersions
	watchEvery   time.Duration // tests shorten it; 0 means factsWatchEvery

	// recents is the record of documents a person opened before: the list is
	// built from it, joined with the live registrations. forgotten holds the
	// documents taken off the list while they were open, which stay off until
	// their next explicit open. recorded is when each document was last noted,
	// so a page load or a registration writes at most once a minute.
	recents   *recents.Store
	forgotten map[string]bool
	recorded  map[string]time.Time

	// beforeEffect runs before each effect. Tests only.
	beforeEffect func(appcore.Effect)
}

type docCount struct {
	n  int
	at time.Time
}

// keptVersions is one document's remembered version count, the stat it was read
// at, and the newest version the store held then.
type keptVersions struct {
	count  int
	at     documentStat
	newest string
}

// documentStat is a document's size and modification time in nanoseconds, the
// stamp that drops a remembered version count.
type documentStat struct {
	size int64
	mod  int64
}

type coreAnswer struct {
	Snapshot appcore.Snapshot
	Refusal  *appcore.Refusal
	Running  bool
}

// initCore fills the core's maps and epoch the first time anything asks for
// them, so an app built in a test starts whole without a constructor.
func (a *app) initCore() {
	a.core.mu.Lock()
	defer a.core.mu.Unlock()
	if a.core.seen == nil {
		a.core.seen = map[string]bool{}
	}
	if a.core.answers == nil {
		a.core.answers = map[string]coreAnswer{}
	}
	if a.core.subs == nil {
		a.core.subs = map[int]chan appcore.Snapshot{}
	}
	if a.core.forgotten == nil {
		a.core.forgotten = map[string]bool{}
	}
	if a.core.recorded == nil {
		a.core.recorded = map[string]time.Time{}
	}
	// A launch that never resolved a config folder has nowhere to keep the
	// record, and must not write one beside whatever folder it was started in.
	if a.core.recents == nil && a.rt.configDir != "" {
		store, err := recents.Open(a.rt.configDir)
		if err != nil {
			a.rt.logger.Printf("Could not read the record of recently opened documents: %v", err)
		}
		a.core.recents = store
	}
	if a.core.countCache == nil {
		a.core.countCache = map[string]docCount{}
	}
	if a.core.versionsKept == nil {
		a.core.versionsKept = map[string]keptVersions{}
	}
	if a.core.epoch == "" {
		a.core.epoch = newEpoch()
	}
	if a.core.requests == nil {
		a.core.requests = confirm.New(a.changed)
	}
}

// newEpoch answers 16 lowercase hex characters no other run of the core has.
func newEpoch() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b[:])
}

// facts gathers everything the pure rules read. It holds the core's lock only to
// copy its own numbers and holds a.mu only while reading the live sites.
func (a *app) facts() appcore.Facts {
	a.initCore()
	a.core.mu.Lock()
	f := appcore.Facts{
		Now:   time.Now().UTC(),
		Rev:   a.core.rev,
		Epoch: a.core.epoch,
		Home:  a.rt.home,
	}
	if len(a.core.seen) > 0 {
		f.Seen = make([]string, 0, len(a.core.seen))
		for id := range a.core.seen {
			f.Seen = append(f.Seen, id)
		}
		slices.Sort(f.Seen)
	}
	a.core.mu.Unlock()

	f.Host = appcore.HostFacts{
		Version:     version,
		OS:          runtime.GOOS,
		Port:        a.rt.port,
		Service:     a.service.state(),
		Implemented: a.implemented(),
	}
	f.Requests = a.requestFacts()
	f.Folders = a.folderFacts()
	var syncFolders []appcore.FolderFact
	var synced map[string]syncedFolder
	f.Sync, syncFolders, f.Events, synced = a.syncFacts()
	for i := range f.Folders {
		if s, ok := synced[f.Folders[i].ID]; ok {
			f.Folders[i].Synced = s.Enabled
			f.Folders[i].Origin = s.Origin
			f.Folders[i].Viewer = s.Viewer
			f.Folders[i].Sync = s.Sync
		}
	}
	f.Folders = append(f.Folders, syncFolders...)
	f.Documents = a.documentFacts()
	f.Programs = a.programFacts()
	f.Update = appcore.UpdateState{
		Current:   version,
		Automatic: !a.rt.cfg.SelfUpdateOff(),
	}
	a.updateFacts(&f.Update)
	profile := a.rt.cfg.ProfileState()
	f.Preferences = appcore.Preferences{
		OpenAtLogin:       a.rt.cfg.StartOnLoginEnabled(),
		OpenAtLoginChosen: true,
		AIEditing:         a.rt.cfg.AIEditEnabled(),
		Appearance:        a.rt.cfg.Appearance(),
		YourName:          profile.Name,
		UseName:           profile.Enabled,
		AIEditTool:        a.aiEditTool(),
		AIEditTools:       a.aiEditTools(),
	}
	f.FileTypes = a.fileTypeFacts()
	f.Move = a.moveFacts()
	f.RecoveredChanges = int(a.recovered.n.Load())
	return f
}

// folderFacts lists the trusted folders in config order, then any served folder
// that is not trusted.
func (a *app) folderFacts() []appcore.FolderFact {
	now := time.Now()
	trusted := a.rt.cfg.TrustedFolderList()
	localRoots := []string{}
	for _, root := range wireLocalRoots(wireLocalRootsPath()) {
		localRoots = append(localRoots, root.Path)
	}

	folders := make([]appcore.FolderFact, 0, len(trusted))
	servedSeen := make(map[string]bool, len(trusted))
	for _, tf := range trusted {
		ff := appcore.FolderFact{
			ID:             tf.ID,
			Name:           filepath.Base(tf.Path),
			Path:           tf.Path,
			Origin:         appcore.Origin{Kind: appcore.OriginLocal},
			Trusted:        true,
			OnThisComputer: true,
			OwnAddress:     tf.OwnAddress,
		}
		identity := tf.Identity
		if entry, ok := a.rt.cfg.ServedFolderForPath(tf.Path); ok {
			ff.Served = entry.Serving
			ff.ServedName = entry.Name
			ff.AllowsFraming = entry.AllowFraming
			identity = entry.Identity
			servedSeen[entry.ID] = true
		}
		markFolderOnDisk(&ff, identity, a.rt.home)
		ff.ServedByLocal = servedByLocal(ff.Path, localRoots)
		folders = append(folders, ff)
	}
	for _, entry := range a.rt.cfg.ServedFolderList() {
		if servedSeen[entry.ID] {
			continue
		}
		ff := appcore.FolderFact{
			ID:             entry.ID,
			Name:           filepath.Base(entry.Path),
			Path:           entry.Path,
			Origin:         appcore.Origin{Kind: appcore.OriginLocal},
			Trusted:        true,
			Served:         entry.Serving,
			ServedName:     entry.Name,
			AllowsFraming:  entry.AllowFraming,
			OnThisComputer: true,
		}
		markFolderOnDisk(&ff, entry.Identity, a.rt.home)
		ff.ServedByLocal = servedByLocal(ff.Path, localRoots)
		folders = append(folders, ff)
	}
	for i := range folders {
		// Only a folder that keeps its own address inside another trusted folder
		// is nested in it: one that does not answers at the enclosing folder's
		// address and is part of it already.
		if folders[i].OwnAddress {
			folders[i].ParentID = nearestFolderAbove(folders[i].Path, folders)
		}
		_, folders[i].Team = a.teamFolderFor(folders[i].Path)
	}
	for i := range folders {
		if !folders[i].Missing {
			folders[i].DocumentCount = a.documentCount(folders[i].Path, now)
		}
	}
	return folders
}

// countDocuments answers how many documents dir holds, skipping hidden folders
// such as .hyperclay and stopping after docCountMaxWalk entries.
func countDocuments(dir string) int {
	n, visited := 0, 0
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		visited++
		if visited > docCountMaxWalk {
			return fs.SkipAll
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".html", ".htm", ".htmlclay":
			n++
		}
		return nil
	})
	return n
}

// documentCount answers a folder's document count, walking it at most once every
// docCountTTL.
func (a *app) documentCount(path string, now time.Time) int {
	a.initCore()
	a.core.mu.Lock()
	if cached, ok := a.core.countCache[path]; ok && now.Sub(cached.at) < docCountTTL {
		a.core.mu.Unlock()
		return cached.n
	}
	a.core.mu.Unlock()
	n := countDocuments(path)
	a.core.mu.Lock()
	a.core.countCache[path] = docCount{n: n, at: now}
	a.core.mu.Unlock()
	return n
}

// nearestFolderAbove answers the id of the closest other folder that contains
// path, excluding path itself, or "" when none does.
func nearestFolderAbove(path string, folders []appcore.FolderFact) string {
	id, longest := "", ""
	for _, other := range folders {
		if other.Path == path {
			continue
		}
		if !pathrel.SameOrBelow(path, other.Path) {
			continue
		}
		if len(other.Path) > len(longest) {
			id, longest = other.ID, other.Path
		}
	}
	return id
}

// markFolderOnDisk records whether the folder is still there and still the one
// that was trusted.
func markFolderOnDisk(ff *appcore.FolderFact, identity, home string) {
	info, err := os.Stat(ff.Path)
	if err != nil || !info.IsDir() {
		ff.Missing = true
		return
	}
	if !trust.IdentityOK(ff.Path, identity, home) {
		ff.IdentityChanged = true
	}
}

// servedByLocal reports whether Hyperclay Local serves path now, the relation
// appcore.GatherServeFacts refuses on: same, inside, or above.
func servedByLocal(path string, roots []string) bool {
	for _, root := range roots {
		if pathrel.Relate(path, root) != pathrel.Unrelated {
			return true
		}
	}
	return false
}

// documentFacts lists the documents a person opened, from the record of real
// opens, joined with the registrations live sites hold now so a row says whether
// its document is open. A row's id is its record's id, so it keeps the same id
// when its tab closes. A document that is open without a row (one opened under a
// build that did not record, or whose record could not be written) is still
// listed, and a document a person took off the list while it was open stays off
// it.
func (a *app) documentFacts() []appcore.DocumentFact {
	var regs []registration
	a.mu.Lock()
	for _, s := range a.sites {
		if s.closed.Load() {
			continue
		}
		for _, reg := range s.sessions.Registrations() {
			f, ok := s.sessions.LookupByPath(reg.Path)
			if !ok {
				continue
			}
			regs = append(regs, registration{path: reg.Path, url: fileURL(s.host, f.RelPath)})
		}
	}
	a.mu.Unlock()

	openURLs := make(map[string]string, len(regs))
	for _, reg := range regs {
		if _, ok := openURLs[reg.path]; !ok {
			openURLs[reg.path] = reg.url
		}
	}

	entries := a.recentEntries()
	folders := a.folderFacts()
	forgotten := a.forgottenPaths()
	docs := make([]appcore.DocumentFact, 0, len(entries)+len(openURLs))
	seen := make(map[string]bool, len(entries)+len(openURLs))
	add := func(id, path string, openedAt *time.Time) {
		if seen[path] || forgotten[path] {
			return
		}
		seen[path] = true
		doc := appcore.DocumentFact{
			ID:       id,
			Name:     filepath.Base(path),
			Path:     path,
			FolderID: nearestFolderAbove(path, folders),
			OpenedAt: openedAt,
		}
		if url, open := openURLs[path]; open {
			doc.URL = url
			doc.OpenNow = true
		}
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			doc.FileMissing = true
		}
		doc.VersionsKept = a.versionsKept(path, folders)
		docs = append(docs, doc)
	}

	for _, entry := range entries {
		openedAt := entry.OpenedAt
		add(entry.ID, entry.Path, &openedAt)
	}
	for _, reg := range regs {
		add(recentID(reg.path), reg.path, nil)
	}
	return docs
}

// versionsKept answers how many versions a document has kept, remembering the
// count per document: one read lists a whole history, and the count only moves
// when a store keeps one. A kept version does not always move the document, so
// the count is remembered with the newest version the store held, and read again
// once the history holds a version the count was not read with.
func (a *app) versionsKept(path string, folders []appcore.FolderFact) int {
	at := documentStat{}
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		at = documentStat{size: info.Size(), mod: info.ModTime().UnixNano()}
	}
	doc := historyDocumentFor(path, folders)
	a.initCore()
	a.core.mu.Lock()
	kept, ok := a.core.versionsKept[path]
	a.core.mu.Unlock()
	if ok && kept.at == at && a.historyUnchanged(doc, kept.newest) {
		return kept.count
	}
	count, newest, err := a.countHistoryNewest(doc)
	if err != nil {
		a.rt.logger.Printf("Could not read how many versions %s has kept: %v", path, err)
		if ok {
			return kept.count
		}
	}
	a.core.mu.Lock()
	a.core.versionsKept[path] = keptVersions{count: count, at: at, newest: newest}
	a.core.mu.Unlock()
	return count
}

// historyUnchanged answers whether doc's history still holds the newest version a
// remembered count was read with: the same version is still the newest, or
// nothing is kept at all, which keeps no version and leaves the count alone.
func (a *app) historyUnchanged(doc historyDocument, newest string) bool {
	items, err := a.readHistoryPage(doc, "", 1)
	if err != nil {
		return false
	}
	return len(items) == 0 || items[0].ID == newest
}

// historyDocumentFor names the store a document's versions are kept in: a served
// document under its folder's id, and every other document by its own path.
func historyDocumentFor(path string, folders []appcore.FolderFact) historyDocument {
	if folder, rel, ok := servedFolderAbove(path, folders); ok {
		return historyDocument{RootID: folder.ID, Path: rel}
	}
	return historyDocument{Path: path}
}

// versionsRoute is the dashboard page that shows one document's versions: the
// document's own page under its served folder, or the recent document's own
// versions page.
func (a *app) versionsRoute(doc appcore.DocumentFact) (string, *appcore.Refusal) {
	for _, folder := range a.folderFacts() {
		if folder.ID != doc.FolderID || !folder.Served {
			continue
		}
		if !pathrel.SameOrBelow(doc.Path, folder.Path) {
			continue
		}
		rel, err := filepath.Rel(folder.Path, doc.Path)
		if err != nil {
			break
		}
		return "/roots/" + folder.ID + "/document?path=" + url.QueryEscape(filepath.ToSlash(rel)) + "&panel=versions", nil
	}
	return "/recent/" + url.PathEscape(doc.ID) + "/versions", nil
}

// servedFolderAbove answers the closest served folder that holds path, and the
// path's place under it.
func servedFolderAbove(path string, folders []appcore.FolderFact) (appcore.FolderFact, string, bool) {
	found := appcore.FolderFact{}
	rel := ""
	for _, folder := range folders {
		if !folder.Served || !pathrel.SameOrBelow(path, folder.Path) {
			continue
		}
		if found.ID != "" && len(folder.Path) <= len(found.Path) {
			continue
		}
		under, err := filepath.Rel(folder.Path, path)
		if err != nil {
			continue
		}
		found, rel = folder, filepath.ToSlash(under)
	}
	return found, rel, found.ID != ""
}

// registration is one document a live site holds now, and where it is served.
type registration struct {
	path string
	url  string
}

// recentInterval is how often a page load or a registration may record the same
// document: a reload is not a new open, and the record is a file on disk.
const recentInterval = time.Minute

// recentID is the id a document's row starts with: the hash of its path, in the
// shape the record keeps. A row that already has an id keeps it, so a restart
// never renames a row.
func recentID(path string) string {
	return "d" + shortHash(path)
}

// recentsStore is the record of documents opened before.
func (a *app) recentsStore() *recents.Store {
	a.initCore()
	a.core.mu.Lock()
	defer a.core.mu.Unlock()
	return a.core.recents
}

// recentEntries is the record, most recently opened first.
func (a *app) recentEntries() []recents.Entry {
	store := a.recentsStore()
	if store == nil {
		return nil
	}
	return store.List()
}

// forgottenPaths is the documents a person took off the list.
func (a *app) forgottenPaths() map[string]bool {
	a.core.mu.Lock()
	defer a.core.mu.Unlock()
	forgotten := make(map[string]bool, len(a.core.forgotten))
	for path := range a.core.forgotten {
		forgotten[path] = true
	}
	return forgotten
}

// noteOpen records filePath as a document a person opened. An explicit open is a
// person's own: it always writes the row, and it is the one thing that brings a
// forgotten document back onto the list. A page load or a registration only notes
// a document it has not noted in the last minute, and never brings a forgotten
// one back.
func (a *app) noteOpen(filePath, how string, explicit bool) {
	path, err := resolveSymlinks(filePath)
	if err != nil {
		path = filePath
	}
	store := a.recentsStore()

	a.core.mu.Lock()
	if explicit {
		delete(a.core.forgotten, path)
	} else {
		_, wasForgotten := a.core.forgotten[path]
		last := a.core.recorded[path]
		if wasForgotten || (!last.IsZero() && time.Since(last) < recentInterval) {
			a.core.mu.Unlock()
			return
		}
		a.core.recorded[path] = time.Now()
	}
	a.core.mu.Unlock()

	if store == nil {
		return
	}
	entry := recents.Entry{
		ID:       recentID(path),
		Path:     path,
		Identity: platform.DirIdentity(path),
		FolderID: nearestFolderAbove(path, a.folderFacts()),
		How:      how,
	}
	// A row keeps the identity it was recorded with: a file replaced since is a
	// different document, and the record still says which one the person opened.
	if existing, ok := store.Find(entry.ID); ok && existing.Path == path && existing.Identity != "" {
		entry.Identity = existing.Identity
	}
	if err := store.Touch(entry, time.Now().UTC()); err != nil {
		a.rt.logger.Printf("Could not record %s as recently opened: %v", path, err)
	}
}

// forgetRecent takes a document's row off the list. The document stays off it
// until its next explicit open: a page load or a registration is not a person
// asking for it back, and a document that is open keeps its tab without its row.
func (a *app) forgetRecent(id string) *appcore.Refusal {
	for _, doc := range a.facts().Documents {
		if doc.ID != id {
			continue
		}
		store := a.recentsStore()
		if store == nil {
			return &appcore.Refusal{Code: appcore.RefuseNoSuchDocument, SubjectID: id}
		}
		if err := store.Forget(id); err != nil {
			a.rt.logger.Printf("Could not remove %s from the recently opened documents: %v", doc.Path, err)
			return &appcore.Refusal{Code: appcore.RefuseFailed}
		}
		a.core.mu.Lock()
		a.core.forgotten[doc.Path] = true
		a.core.mu.Unlock()
		return nil
	}
	return &appcore.Refusal{Code: appcore.RefuseNoSuchDocument, SubjectID: id}
}

// reopenRefusal refuses a document.open from a page whose file is no longer the
// one the record holds: an id names what the person opened before, not whatever
// is at that path now. A missing file is left to the open itself, which answers
// file-missing.
func (a *app) reopenRefusal(id string) *appcore.Refusal {
	store := a.recentsStore()
	if store == nil {
		return nil
	}
	entry, ok := store.Find(id)
	if !ok {
		return nil
	}
	if info, err := os.Stat(entry.Path); err != nil || !info.Mode().IsRegular() {
		return nil
	}
	if platform.DirIdentity(entry.Path) != entry.Identity {
		return &appcore.Refusal{Code: appcore.RefuseNeedsConfirmation, SubjectID: id}
	}
	return nil
}

// programFacts maps the helper programs in config with the documents their
// decisions name. A document is listed by the path its decision is keyed on,
// which is what program.removeAccess takes back, and a removal is listed under
// every registration of the name the document was removed from.
func (a *app) programFacts() []appcore.Program {
	list := a.rt.cfg.HelperProgramList()
	decisions := a.rt.cfg.HelperDecisionList()
	programs := append(a.builtinProgramFacts(), make([]appcore.Program, 0, len(list))...)
	for _, program := range list {
		allowed, removed := []string{}, []string{}
		for _, decision := range decisions {
			switch {
			case decision.Allowed && decision.Program == program.ID:
				allowed = append(allowed, decision.Document)
			case decision.Revoked && decision.Name == program.Name:
				removed = append(removed, decision.Document)
			}
		}
		slices.Sort(allowed)
		slices.Sort(removed)
		programs = append(programs, appcore.Program{
			ID:                program.ID,
			Name:              program.Name,
			Path:              program.Path,
			FileMissing:       helperProgramMissing(program.Path),
			AllowedDocuments:  allowed,
			AllTrustedFolders: program.AnyDocument,
			RemovedDocuments:  removed,
		})
	}
	if aiEditing, ok := a.aiEditProgramFact(); ok {
		programs = append(programs, aiEditing)
	}
	return programs
}

// shortHash is the stable id suffix the core gives a path: 15 hex characters of
// its sha256.
func shortHash(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])[:15]
}

// snapshot builds the one snapshot every door and every subscriber reads.
func (a *app) snapshot() appcore.Snapshot { return appcore.Build(a.facts()) }

// do runs one action: the pure rules decide the effects, the app runs the code
// it already has, and the answer is a fresh snapshot or the reason nothing
// happened.
func (a *app) do(ctx context.Context, action appcore.Action, ch Channel) (appcore.Snapshot, *appcore.Refusal) {
	a.initCore()
	if action.ID != "" {
		a.core.mu.Lock()
		answer, ok := a.core.answers[action.ID]
		if !ok {
			a.reserveLocked(action.ID)
		}
		a.core.mu.Unlock()
		if ok {
			if answer.Running {
				return a.snapshot(), &appcore.Refusal{Code: appcore.RefuseStillRunning}
			}
			return answer.Snapshot, answer.Refusal
		}
	}

	if ch == ChannelDashboard && appcore.ClassOf(action) == appcore.ClassGrant {
		// A page never runs a grant. On Windows and Linux the host asks the
		// person in a dialog of its own and answers pending (grants.go).
		refusal := a.dashboardGrant(action)
		snapshot := a.snapshot()
		a.remember(action.ID, coreAnswer{Snapshot: snapshot, Refusal: refusal})
		return snapshot, refusal
	}
	if ch != ChannelShell && (action.Type == "document.recover" || action.Type == "update.prepared") {
		refusal := &appcore.Refusal{Code: appcore.RefuseNeedsConfirmation}
		snapshot := a.snapshot()
		a.remember(action.ID, coreAnswer{Snapshot: snapshot, Refusal: refusal})
		return snapshot, refusal
	}

	effects, refusal := appcore.Plan(a.facts(), action)
	if refusal == nil && ch == ChannelDashboard && action.Type == "document.open" && len(effects) == 1 {
		refusal = a.reopenRefusal(effects[0].DocumentID)
	}
	if refusal != nil {
		snapshot := a.snapshot()
		a.remember(action.ID, coreAnswer{Snapshot: snapshot, Refusal: refusal})
		return snapshot, refusal
	}
	for _, effect := range effects {
		if hook := a.core.beforeEffect; hook != nil {
			hook(effect)
		}
		if refusal := a.runEffect(ctx, effect); refusal != nil {
			a.bumpRev()
			a.changed()
			snapshot := a.snapshot()
			a.remember(action.ID, coreAnswer{Snapshot: snapshot, Refusal: refusal})
			return snapshot, refusal
		}
	}

	a.bumpRev()
	a.changed()
	answer := coreAnswer{Snapshot: a.snapshot()}
	a.remember(action.ID, answer)
	return answer.Snapshot, nil
}

// remember keeps the answer to one action id, dropping the oldest past the limit.
func (a *app) remember(id string, answer coreAnswer) {
	if id == "" {
		return
	}
	a.core.mu.Lock()
	defer a.core.mu.Unlock()
	a.rememberLocked(id, answer)
}

// rememberLocked is remember with the lock already held.
func (a *app) rememberLocked(id string, answer coreAnswer) {
	if a.core.answers == nil {
		a.core.answers = map[string]coreAnswer{}
	}
	if _, ok := a.core.answers[id]; ok {
		a.core.answers[id] = answer
		return
	}
	a.core.answers[id] = answer
	a.core.answerOrder = append(a.core.answerOrder, id)
	for len(a.core.answerOrder) > coreAnswerLimit {
		oldest := a.core.answerOrder[0]
		a.core.answerOrder = a.core.answerOrder[1:]
		delete(a.core.answers, oldest)
	}
}

// reserveLocked marks an action id as running, so a repeated id while its
// effects run is refused rather than run again.
func (a *app) reserveLocked(id string) {
	a.rememberLocked(id, coreAnswer{Running: true})
}

// bumpRev raises the revision for a change this answer reports, so no two
// different states ever carry one revision. publish raises it again for the
// coalesced send; a revision is only ever skipped, never shared.
func (a *app) bumpRev() {
	a.core.mu.Lock()
	a.core.rev++
	a.core.mu.Unlock()
}

// changed tells the subscribers that state moved. Changes within
// coreChangeDelay share one snapshot and one revision.
func (a *app) changed() {
	a.core.mu.Lock()
	defer a.core.mu.Unlock()
	if a.core.pending != nil {
		return
	}
	a.core.pending = time.AfterFunc(coreChangeDelay, a.publish)
}

// publish is the coalescing timer's body: one new revision, one snapshot, one
// send per subscriber.
func (a *app) publish() {
	a.core.mu.Lock()
	a.core.pending = nil
	a.core.rev++
	a.core.mu.Unlock()

	snapshot := a.snapshot()

	a.core.mu.Lock()
	defer a.core.mu.Unlock()
	for _, ch := range a.core.subs {
		// A subscriber that has not read the last snapshot gets the new one
		// instead of waiting behind it.
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- snapshot:
		default:
		}
	}
}

// watchFacts publishes changes no action caused (a document opened from the
// browser, a folder deleted in Finder): every factsWatchEvery it digests the
// facts, leaving out the clock and the revision, and calls changed when the
// digest moved. It stops when the last subscriber leaves.
func (a *app) watchFacts() {
	every := factsWatchEvery
	a.core.mu.Lock()
	if a.core.watchEvery > 0 {
		every = a.core.watchEvery
	}
	a.core.mu.Unlock()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for range ticker.C {
		a.core.mu.Lock()
		if len(a.core.subs) == 0 {
			a.core.watching = false
			a.core.mu.Unlock()
			return
		}
		a.core.mu.Unlock()

		f := a.facts()
		f.Now, f.Rev = time.Time{}, 0
		raw, err := json.Marshal(f)
		if err != nil {
			continue
		}
		digest := sha256.Sum256(raw)
		a.core.mu.Lock()
		moved := digest != a.core.lastFacts
		a.core.lastFacts = digest
		a.core.mu.Unlock()
		if moved {
			a.changed()
		}
	}
}

// subscribe hands back a one-slot stream of snapshots and the function that ends
// it.
func (a *app) subscribe() (<-chan appcore.Snapshot, func()) {
	a.initCore()
	a.core.mu.Lock()
	id := a.core.nextSub
	a.core.nextSub++
	ch := make(chan appcore.Snapshot, 1)
	a.core.subs[id] = ch
	start := !a.core.watching
	if start {
		a.core.watching = true
	}
	a.core.mu.Unlock()
	if start {
		go a.watchFacts()
	}
	return ch, func() {
		a.core.mu.Lock()
		defer a.core.mu.Unlock()
		if sub, ok := a.core.subs[id]; ok {
			delete(a.core.subs, id)
			close(sub)
		}
	}
}

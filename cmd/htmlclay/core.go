package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
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

	watching   bool                 // the facts watcher runs while anyone subscribes
	lastFacts  [32]byte             // digest of the facts the watcher last saw
	openedAt   map[string]time.Time // document path -> when the core first saw it open
	countCache map[string]docCount  // folder path -> its document count, refreshed every docCountTTL
	watchEvery time.Duration        // tests shorten it; 0 means factsWatchEvery

	// beforeEffect runs before each effect. Tests only.
	beforeEffect func(appcore.Effect)
}

type docCount struct {
	n  int
	at time.Time
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
	if a.core.openedAt == nil {
		a.core.openedAt = map[string]time.Time{}
	}
	if a.core.countCache == nil {
		a.core.countCache = map[string]docCount{}
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
	f.Documents = a.documentFacts()
	f.Programs = a.programFacts()
	f.Update = appcore.UpdateState{
		Current:   version,
		Automatic: !a.rt.cfg.SelfUpdateOff(),
	}
	profile := a.rt.cfg.ProfileState()
	f.Preferences = appcore.Preferences{
		OpenAtLogin:       a.rt.cfg.StartOnLoginEnabled(),
		OpenAtLoginChosen: true,
		AIEditing:         a.rt.cfg.AIEditEnabled(),
		Appearance:        a.rt.cfg.Appearance(),
		YourName:          profile.Name,
		UseName:           profile.Enabled,
	}
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
			ID:             "t" + shortHash(tf.Path),
			Name:           filepath.Base(tf.Path),
			Path:           tf.Path,
			Origin:         appcore.Origin{Kind: appcore.OriginLocal},
			Trusted:        true,
			OnThisComputer: true,
		}
		identity := tf.Identity
		if entry, ok := a.rt.cfg.ServedFolderForPath(tf.Path); ok {
			ff.ID = entry.ID
			ff.Served = entry.Serving
			ff.ServedName = entry.Name
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
			OnThisComputer: true,
		}
		markFolderOnDisk(&ff, entry.Identity, a.rt.home)
		ff.ServedByLocal = servedByLocal(ff.Path, localRoots)
		folders = append(folders, ff)
	}
	for i := range folders {
		folders[i].ParentID = nearestFolderAbove(folders[i].Path, folders)
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

// documentFacts lists every document registered in a live site now. Documents a
// person opened recently and closed again are not tracked yet.
func (a *app) documentFacts() []appcore.DocumentFact {
	type registration struct {
		path string
		url  string
	}
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

	folders := a.folderFacts()
	docs := make([]appcore.DocumentFact, 0, len(regs))
	seen := make(map[string]bool, len(regs))
	for _, reg := range regs {
		if seen[reg.path] {
			continue
		}
		seen[reg.path] = true
		doc := appcore.DocumentFact{
			ID:      "d" + shortHash(reg.path),
			Name:    filepath.Base(reg.path),
			Path:    reg.path,
			URL:     reg.url,
			OpenNow: true,
		}
		doc.FolderID = nearestFolderAbove(reg.path, folders)
		if info, err := os.Stat(reg.path); err != nil || !info.Mode().IsRegular() {
			doc.FileMissing = true
		}
		docs = append(docs, doc)
	}

	a.core.mu.Lock()
	defer a.core.mu.Unlock()
	for path := range a.core.openedAt {
		if !seen[path] {
			delete(a.core.openedAt, path)
		}
	}
	for i := range docs {
		at, ok := a.core.openedAt[docs[i].Path]
		if !ok {
			at = time.Now().UTC()
			a.core.openedAt[docs[i].Path] = at
		}
		opened := at
		docs[i].OpenedAt = &opened
	}
	return docs
}

// programFacts maps the helper programs in config. Allowed documents live in the
// helper decisions, not in config, so they are left empty here.
func (a *app) programFacts() []appcore.Program {
	list := a.rt.cfg.HelperProgramList()
	programs := make([]appcore.Program, 0, len(list))
	for _, program := range list {
		programs = append(programs, appcore.Program{
			ID:                program.ID,
			Name:              program.Name,
			Path:              program.Path,
			FileMissing:       helperProgramMissing(program.Path),
			AllTrustedFolders: program.AnyDocument,
		})
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
		refusal := &appcore.Refusal{Code: appcore.RefuseNeedsConfirmation}
		snapshot := a.snapshot()
		a.remember(action.ID, coreAnswer{Snapshot: snapshot, Refusal: refusal})
		return snapshot, refusal
	}

	effects, refusal := appcore.Plan(a.facts(), action)
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

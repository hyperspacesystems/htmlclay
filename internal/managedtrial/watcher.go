//go:build managedtrial

package managedtrial

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The managed trial host's watcher, ported from Local's
// src/main/root-observer.js: the same ignored paths, the same awaitWriteFinish
// stabilization, and the same outside-edit pass for a document someone else
// wrote. Local's observer is chokidar; this one polls, because the host has no
// inotify binding and the walk is what chokidar's own poller does anyway.

const (
	// watchPollInterval is SYNC_CONFIG.FILE_STABILIZATION.pollInterval.
	watchPollInterval = 100 * time.Millisecond

	// watchStability is SYNC_CONFIG.FILE_STABILIZATION.stabilityThreshold: a
	// path is reported only after it has looked the same for this long, so the
	// halfway state of a slow write is never published and an outside edit is
	// judged once.
	watchStability = 1000 * time.Millisecond
)

// watchIgnoredNames is root-observer's IGNORED list, which names folders and
// files a served root may hold but never reports; every leading-dot path is
// covered by the dot rule below.
var watchIgnoredNames = map[string]bool{
	"node_modules":   true,
	"sites-versions": true,
	"tailwindcss":    true,
	"Thumbs.db":      true,
}

// watcherIgnored is root-observer's ignored test for one root-relative name.
func watcherIgnored(rel string) bool {
	for _, segment := range strings.Split(rel, "/") {
		if segment == "" {
			continue
		}
		if strings.HasPrefix(segment, ".") || watchIgnoredNames[segment] {
			return true
		}
	}
	return false
}

// watchEntry is what one path looked like: enough to tell a rewrite from a new
// file, and to know a write has settled.
type watchEntry struct {
	isDir bool
	size  int64
	mtime int64
}

// watchPending is a change that has not been stable for long enough to publish.
type watchPending struct {
	entry watchEntry
	event string
	since time.Time
}

// rootWatcher polls one served root and reports what changed.
type rootWatcher struct {
	rootID  string
	baseDir string
	events  *eventRing
	// guardLive is where an outside edit's data-loss event goes, the way
	// root-observer hands its own root live to the guard.
	guardLive guardLive
	known     map[string]watchEntry
	pending   map[string]watchPending
	halt      chan struct{}
	done      chan struct{}
}

func newRootWatcher(rootID, baseDir string, events *eventRing, guardLive guardLive) *rootWatcher {
	return &rootWatcher{
		rootID:    rootID,
		baseDir:   baseDir,
		events:    events,
		guardLive: guardLive,
		known:     make(map[string]watchEntry),
		pending:   make(map[string]watchPending),
	}
}

// start takes the root's current state without reporting it (chokidar's
// ignoreInitial) and polls from there. It is a no-op on a running watcher.
func (w *rootWatcher) start() {
	if w.halt != nil {
		return
	}
	w.known = w.snapshot()
	halt := make(chan struct{})
	done := make(chan struct{})
	w.halt = halt
	w.done = done
	go func() {
		defer close(done)
		ticker := time.NewTicker(watchPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-halt:
				return
			case now := <-ticker.C:
				w.examine(now)
			}
		}
	}()
}

// stop ends the poll and waits for the walk in flight.
func (w *rootWatcher) stop() {
	if w.halt == nil {
		return
	}
	close(w.halt)
	<-w.done
	w.halt = nil
	w.done = nil
}

// snapshot walks the root into a map of root-relative names, skipping every
// ignored path and whole ignored folder.
func (w *rootWatcher) snapshot() map[string]watchEntry {
	entries := make(map[string]watchEntry)
	_ = filepath.WalkDir(w.baseDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if path == w.baseDir {
				return err
			}
			return nil
		}
		if path == w.baseDir {
			return nil
		}
		rel, relErr := filepath.Rel(w.baseDir, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if watcherIgnored(rel) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		entries[rel] = watchEntry{isDir: entry.IsDir(), size: info.Size(), mtime: info.ModTime().UnixNano()}
		return nil
	})
	return entries
}

// examine is one poll: a path that has not been seen is an add, one whose stat
// moved is a change, and a published path keeps its place until it moves again.
func (w *rootWatcher) examine(now time.Time) {
	current := w.snapshot()
	for rel, entry := range current {
		known, seen := w.known[rel]
		if seen && known == entry {
			delete(w.pending, rel)
			continue
		}
		waiting, waitingOK := w.pending[rel]
		if !waitingOK || waiting.entry != entry {
			w.pending[rel] = watchPending{entry: entry, event: watchEventName(seen, entry.isDir), since: now}
			continue
		}
		if now.Sub(waiting.since) < watchStability {
			continue
		}
		delete(w.pending, rel)
		w.known[rel] = entry
		w.publish(waiting.event, rel, entry)
	}
	for rel, known := range w.known {
		if _, ok := current[rel]; ok {
			continue
		}
		delete(w.known, rel)
		event := "unlink"
		if known.isDir {
			event = "unlinkDir"
		}
		w.publish(event, rel, known)
	}
}

// watchEventName is chokidar's name for an observation: a path it had not seen
// is added, one it had is changed.
func watchEventName(seen, isDir bool) string {
	if seen {
		return "change"
	}
	if isDir {
		return "addDir"
	}
	return "add"
}

// publish appends the observation's frame and, for a document someone else
// wrote, runs the data-loss guard's outside-edit pass, which reaches the tabs
// through the root's live channel. Root-observer's own external-change notice
// and its saved-lane broadcast are the full-host stage's: the adoption
// classifier that decides between them is not ported yet.
func (w *rootWatcher) publish(event, rel string, entry watchEntry) {
	w.events.changeFrame(w.rootID, event, rel)
	if entry.isDir || !isDocumentName(rel) {
		return
	}
	// Root-observer runs the guard for a changed document only: an added one
	// has no last-seen bytes to lose.
	if event != "change" {
		return
	}
	bytes, err := os.ReadFile(filepath.Join(w.baseDir, filepath.FromSlash(rel)))
	if err != nil {
		return
	}
	catchUpOutsideEdit(w.baseDir, rel, bytes, string(bytes), w.guardLive)
}

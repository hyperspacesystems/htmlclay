package main

import (
	"sort"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/session"
)

// What open tabs say about their unsaved edits (the unsaved-report-v1
// extension), so every Quit path can ask about a page that saves only on
// Ctrl+S, which the live-room flush never sees. Memory only: a report is about
// a tab that exists now.

const (
	// unsavedReportTTL forgets a tab that stopped reporting: a tab with unsaved
	// edits repeats its report every minute, so only a crashed browser goes
	// quiet this long. Browsers slow a hidden tab's timers to about one wake in
	// five minutes, so a shorter time would drop real unsaved tabs.
	unsavedReportTTL = 10 * time.Minute
	// unsavedFrozenTTL keeps a tab the browser froze: it said so as it froze,
	// runs no timers until it wakes, and reports again then. A day bounds a tab
	// the browser later discarded without a word.
	unsavedFrozenTTL  = 24 * time.Hour
	unsavedTabsPerDoc = 32
)

type tabReport struct {
	seq     int64
	unsaved bool
	frozen  bool
	at      time.Time
}

func (r tabReport) expired(now time.Time) bool {
	ttl := unsavedReportTTL
	if r.frozen {
		ttl = unsavedFrozenTTL
	}
	return now.Sub(r.at) >= ttl
}

type unsavedReports struct {
	mu   sync.Mutex
	docs map[string]map[string]tabReport // absolute path, then tab
}

// report records one tab's state. A report no newer than the tab's last one is
// ignored, so a late "unsaved" cannot override a later "saved".
func (u *unsavedReports) report(path, tab string, seq int64, unsaved, frozen bool, now time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.docs == nil {
		u.docs = map[string]map[string]tabReport{}
	}
	tabs := u.docs[path]
	if tabs == nil {
		tabs = map[string]tabReport{}
		u.docs[path] = tabs
	}
	if prev, ok := tabs[tab]; ok && prev.seq >= seq && !prev.expired(now) {
		return
	}
	tabs[tab] = tabReport{seq: seq, unsaved: unsaved, frozen: frozen, at: now}
	for len(tabs) > unsavedTabsPerDoc {
		oldest := ""
		for id, r := range tabs {
			if oldest == "" || r.at.Before(tabs[oldest].at) {
				oldest = id
			}
		}
		delete(tabs, oldest)
	}
}

// unsavedPaths answers every document at least one live tab says holds unsaved
// edits, sorted, and forgets tabs that went quiet.
func (u *unsavedReports) unsavedPaths(now time.Time) []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	var out []string
	for path, tabs := range u.docs {
		unsaved := false
		for id, r := range tabs {
			if r.expired(now) {
				delete(tabs, id)
				continue
			}
			unsaved = unsaved || r.unsaved
		}
		if len(tabs) == 0 {
			delete(u.docs, path)
		}
		if unsaved {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// forget drops every report about a document whose open tabs lost their
// address: nothing they hold can be saved there, and they cannot say so as
// they close.
func (u *unsavedReports) forget(path string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.docs, path)
}

// unsavedTabsUnder names, for a person, the documents in dir an open tab says
// hold unsaved edits.
func (a *app) unsavedTabsUnder(dir string) []string {
	var names []string
	for _, p := range a.unsaved.unsavedPaths(time.Now()) {
		if session.EqualOrUnder(p, dir) {
			names = append(names, a.unsavedPathLine(p))
		}
	}
	return names
}

// noteUnsavedReport is both hosts' report hook.
func (a *app) noteUnsavedReport(path, tab string, seq int64, unsaved, frozen bool) {
	a.unsaved.report(path, tab, seq, unsaved, frozen, time.Now())
}

package main

import (
	"testing"
	"time"
)

func TestUnsavedReportsOrderTabsAndExpiry(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)

	// (a) one tab's report lists its document.
	a := &unsavedReports{}
	a.report("/doc-a", "A", 1, true, false, now)
	if got := a.unsavedPaths(now); len(got) != 1 || got[0] != "/doc-a" {
		t.Fatalf("(a) unsavedPaths = %v, want [/doc-a]", got)
	}

	// (b) a second tab saving does not clear the first tab's edits.
	b := &unsavedReports{}
	b.report("/doc-b", "A", 1, true, false, now)
	b.report("/doc-b", "B", 1, false, false, now)
	if got := b.unsavedPaths(now); len(got) != 1 || got[0] != "/doc-b" {
		t.Fatalf("(b) unsavedPaths = %v, want [/doc-b]", got)
	}

	// (c) a late older report never overrides a newer one.
	c := &unsavedReports{}
	c.report("/doc-c", "A", 3, false, false, now)
	c.report("/doc-c", "A", 2, true, false, now)
	if got := c.unsavedPaths(now); len(got) != 0 {
		t.Fatalf("(c) unsavedPaths = %v, want none", got)
	}

	// (d) a tab that goes quiet expires.
	d := &unsavedReports{}
	d.report("/doc-d", "C", 1, true, false, now)
	if got := d.unsavedPaths(now.Add(unsavedReportTTL)); len(got) != 0 {
		t.Fatalf("(d) unsavedPaths = %v, want none", got)
	}

	// (e) a tab the browser froze cannot repeat its report, so it is kept past
	// the quiet limit, and forgotten only after a day.
	e := &unsavedReports{}
	e.report("/doc-e", "F", 1, true, true, now)
	if got := e.unsavedPaths(now.Add(2 * time.Hour)); len(got) != 1 {
		t.Fatalf("(e) a frozen tab two hours on: unsavedPaths = %v, want /doc-e", got)
	}
	if got := e.unsavedPaths(now.Add(unsavedFrozenTTL)); len(got) != 0 {
		t.Fatalf("(e) a frozen tab a day on: unsavedPaths = %v, want none", got)
	}
}

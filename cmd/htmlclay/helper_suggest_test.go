package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeSuggestFile(t *testing.T, path string, mode os.FileMode) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeSuggestDocument(t *testing.T, path, meta string) string {
	t.Helper()
	writeSuggestFile(t, path, 0o644)
	if err := os.WriteFile(path, []byte("<!doctype html><html><head>"+meta+"</head><body></body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func addSuggestProgram(t *testing.T, a *app, name, path string) {
	t.Helper()
	if _, err := a.rt.cfg.AddHelperProgram(name, path); err != nil {
		t.Fatal(err)
	}
}

// settled is a clock an hour ahead, so files this test just wrote count as
// having sat unchanged long enough to be offered.
func settled() time.Time { return time.Now().Add(time.Hour) }

// newSuggestApp builds an app whose risky folders sit inside its own home,
// since the real temp folder holds every test's home.
func newSuggestApp(t *testing.T) (*app, string) {
	t.Helper()
	previous := helperSuggestRiskyFolders
	helperSuggestRiskyFolders = func(home string) []string {
		return []string{filepath.Join(home, "Downloads"), filepath.Join(home, "tmp")}
	}
	t.Cleanup(func() { helperSuggestRiskyFolders = previous })
	home := t.TempDir()
	return buildTestApp(t, home, t.TempDir()), home
}

func TestSuggestHelperProgramFindsExpectedFileInKnownFolder(t *testing.T) {
	a, home := newSuggestApp(t)
	bin := filepath.Join(home, "apps", "bin")
	addSuggestProgram(t, a, "other", writeSuggestFile(t, filepath.Join(bin, "other"), 0o755))
	want := writeSuggestFile(t, filepath.Join(bin, "clay-search"), 0o755)
	doc := writeSuggestDocument(t, filepath.Join(home, "apps", "search", "search.htmlclay"),
		`<meta name="htmlclay-helper" content="search" data-program-folder="~/apps/bin" data-program-file="clay-search">`)

	got := a.suggestHelperProgram(doc, "search", settled())
	if got.path != want || got.program.ID != "" {
		t.Fatalf("suggestion = %+v, want unregistered %s", got, want)
	}
}

func TestSuggestHelperProgramReusesRegistrationNamedAsExpected(t *testing.T) {
	a, home := newSuggestApp(t)
	path := writeSuggestFile(t, filepath.Join(home, "apps", "bin", "clay-search"), 0o755)
	addSuggestProgram(t, a, "clay-search", path)
	doc := writeSuggestDocument(t, filepath.Join(home, "apps", "search", "search.htmlclay"),
		`<meta name="htmlclay-helper" content="search" data-program-file="clay-search">`)

	got := a.suggestHelperProgram(doc, "search", settled())
	if got.path != path || got.program.Name != "clay-search" {
		t.Fatalf("suggestion = %+v, want the clay-search registration", got)
	}
}

func TestSuggestHelperProgramPrefersSameNameRegistration(t *testing.T) {
	a, home := newSuggestApp(t)
	path := writeSuggestFile(t, filepath.Join(home, "tools", "anything"), 0o755)
	addSuggestProgram(t, a, "search", path)
	doc := writeSuggestDocument(t, filepath.Join(home, "docs", "s.htmlclay"), `<meta name="htmlclay-helper" content="search">`)

	got := a.suggestHelperProgram(doc, "search", settled())
	if got.path != path || got.program.Name != "search" {
		t.Fatalf("suggestion = %+v", got)
	}
}

func TestSuggestHelperProgramNeverLooksBesideTheDocument(t *testing.T) {
	a, home := newSuggestApp(t)
	docs := filepath.Join(home, "Shared", "pack")
	addSuggestProgram(t, a, "other", writeSuggestFile(t, filepath.Join(docs, "tools", "other"), 0o755))
	writeSuggestFile(t, filepath.Join(docs, "tools", "search"), 0o755)
	for _, risky := range []string{"Downloads", "tmp"} {
		addSuggestProgram(t, a, "x"+strings.ToLower(risky), writeSuggestFile(t, filepath.Join(home, risky, "x"+strings.ToLower(risky)), 0o755))
		writeSuggestFile(t, filepath.Join(home, risky, "search"), 0o755)
	}
	doc := writeSuggestDocument(t, filepath.Join(docs, "s.htmlclay"), `<meta name="htmlclay-helper" content="search" data-program-folder="tools">`)

	if got := a.suggestHelperProgram(doc, "search", settled()); got.path != "" {
		t.Fatalf("suggested %s from a folder the document could have arrived with", got.path)
	}
}

func TestSuggestHelperProgramWithholdsJustChangedFiles(t *testing.T) {
	a, home := newSuggestApp(t)
	bin := filepath.Join(home, "apps", "bin")
	addSuggestProgram(t, a, "other", writeSuggestFile(t, filepath.Join(bin, "other"), 0o755))
	writeSuggestFile(t, filepath.Join(bin, "search"), 0o755)
	doc := writeSuggestDocument(t, filepath.Join(home, "docs", "s.htmlclay"), `<meta name="htmlclay-helper" content="search">`)

	got := a.suggestHelperProgram(doc, "search", time.Now())
	if got.path != "" {
		t.Fatalf("suggested a file written moments ago: %s", got.path)
	}
	if len(got.withheld) != 1 || !strings.Contains(got.withheld[0], "~/apps/bin/search") || !strings.Contains(got.withheld[0], "changed") {
		t.Fatalf("withheld = %v", got.withheld)
	}
}

func TestSuggestHelperProgramOffersNothingWhenAmbiguous(t *testing.T) {
	a, home := newSuggestApp(t)
	for _, dir := range []string{"one", "two"} {
		addSuggestProgram(t, a, "x"+dir, writeSuggestFile(t, filepath.Join(home, dir, "x"+dir), 0o755))
		writeSuggestFile(t, filepath.Join(home, dir, "search"), 0o755)
	}
	doc := writeSuggestDocument(t, filepath.Join(home, "docs", "s.htmlclay"), `<meta name="htmlclay-helper" content="search">`)

	if got := a.suggestHelperProgram(doc, "search", settled()); got.path != "" {
		t.Fatalf("picked %s between two candidates", got.path)
	}
}

func TestSuggestHelperProgramSkipsNonExecutable(t *testing.T) {
	a, home := newSuggestApp(t)
	bin := filepath.Join(home, "bin")
	addSuggestProgram(t, a, "other", writeSuggestFile(t, filepath.Join(bin, "other"), 0o755))
	writeSuggestFile(t, filepath.Join(bin, "search"), 0o644)
	doc := writeSuggestDocument(t, filepath.Join(home, "docs", "s.htmlclay"), `<meta name="htmlclay-helper" content="search">`)

	if got := a.suggestHelperProgram(doc, "search", settled()); got.path != "" {
		t.Fatalf("suggested a file that cannot run: %s", got.path)
	}
}

func TestMatchesProgramFile(t *testing.T) {
	for _, c := range []struct {
		base, expected string
		want           bool
	}{
		{"clay-search", "clay-search", true},
		{"search.py", "search", true},
		{"search.txt", "search", false},
		{"search-old", "search", false},
		{"my-search", "search", false},
	} {
		if got := matchesProgramFile(c.base, c.expected); got != c.want {
			t.Errorf("matchesProgramFile(%q, %q) = %v", c.base, c.expected, got)
		}
	}
}

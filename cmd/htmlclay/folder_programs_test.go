package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const helperPage = `<!doctype html><html><head><meta name="htmlclay-helper" content="search"></head><body><p>Old</p></body></html>`

func writeFolderProgram(t *testing.T, home, name, body string) string {
	t.Helper()
	dir := programsFolder(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFolderProgramsAnswerOwnPagesAndStrangersOnlyByOptIn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a folder program there is an .exe, .cmd or .bat file")
	}
	s := newTierStand(t)
	home := s.app.rt.home
	mine := filepath.Join(s.folder, "mine.html")
	stranger := filepath.Join(s.folder, "stranger.html")
	writeTestFile(t, mine, helperPage)
	writeTestFile(t, stranger, helperPage)
	madeHere(t, s.app, mine)
	downloadedHere(t, s.app, stranger)
	program := writeFolderProgram(t, home, "search", "#!/bin/sh\necho ok\n")
	resolved := real(t, program)

	allowed, _ := s.app.resolvedHelpers(mine, []string{"search"})
	if got := allowed["search"]; got.Path != resolved || got.ID != folderProgramPrefix+"search" {
		t.Fatalf("the person's own page resolves search to %+v, want %s", got, resolved)
	}
	if !s.app.helperGranted(mine, "search", folderProgramPrefix+"search") {
		t.Fatal("the person's own page may not start the folder program")
	}
	if allowed, _ := s.app.resolvedHelpers(stranger, []string{"search"}); len(allowed) != 0 {
		t.Fatalf("a stranger's page reached a program that did not opt in: %+v", allowed)
	}
	if s.app.helperGranted(stranger, "search", folderProgramPrefix+"search") {
		t.Fatal("a stranger's page may start a program that did not opt in")
	}

	asked := s.askForUse(stranger, "search")
	s.awaitWarning(t, real(t, stranger))
	select {
	case <-asked:
		t.Fatal("asking for the program finished before the warning was answered")
	default:
	}
	s.answer(t, real(t, stranger), helperAnswerAllowDocument)
	if allowed, _ := s.app.resolvedHelpers(stranger, []string{"search"}); len(allowed) != 0 {
		t.Fatalf("after the warning a stranger's page reached a program that did not opt in: %+v", allowed)
	}

	writeFolderProgram(t, home, "search", "#!/bin/sh\n# htmlclay-callers: any\necho ok\n")
	allowed, _ = s.app.resolvedHelpers(stranger, []string{"search"})
	if got := allowed["search"]; got.Path != resolved {
		t.Fatalf("an allowed stranger's page resolves %+v, want the opted-in folder program", got)
	}
	other := filepath.Join(s.folder, "other.html")
	writeTestFile(t, other, helperPage)
	downloadedHere(t, s.app, other)
	if allowed, _ := s.app.resolvedHelpers(other, []string{"search"}); len(allowed) != 0 {
		t.Fatalf("an opted-in program answered a stranger before the warning: %+v", allowed)
	}
	if s.app.helperGranted(other, "search", folderProgramPrefix+"search") {
		t.Fatal("an opted-in program may start for a stranger before the warning")
	}
	s.askForUse(other, "search")
	s.awaitWarning(t, real(t, other))

	writeFolderProgram(t, home, "search.sh", "#!/bin/sh\necho two\n")
	if allowed, _ := s.app.resolvedHelpers(mine, []string{"search"}); allowed["search"].ID == folderProgramPrefix+"search" {
		t.Fatal("two programs answering one name still counted as one")
	}
	os.Remove(filepath.Join(programsFolder(home), "search.sh"))

	s.app.tierStore().SetDeveloper(false)
	if s.app.helperGranted(mine, "search", folderProgramPrefix+"search") {
		t.Fatal("the folder program still starts with developer mode and the programs folder off")
	}
	s.app.tierStore().SetPrograms(true)
	if !s.app.helperGranted(mine, "search", folderProgramPrefix+"search") {
		t.Fatal("htmlclay programs enable did not switch the folder on")
	}
}

func TestCallersAny(t *testing.T) {
	dir := t.TempDir()
	for body, want := range map[string]bool{
		"#!/bin/sh\n# htmlclay-callers: any\n":                   true,
		"// HTMLClay-Callers: Any\n":                             true,
		"/* htmlclay-callers: any */\n":                          true,
		"rem htmlclay-callers: any\r\n":                          true,
		"#!/bin/sh\necho htmlclay-callers: any\n":                false,
		"# htmlclay-callers: mine\n":                             false,
		string(make([]byte, 4096)) + "# htmlclay-callers: any\n": false,
	} {
		path := filepath.Join(dir, "p")
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		if got := callersAny(path); got != want {
			t.Errorf("callersAny(%q) = %v, want %v", body[:min(len(body), 40)], got, want)
		}
	}
}

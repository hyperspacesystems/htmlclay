package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeTestProgram(t *testing.T, dir, name string, mode os.FileMode) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho ok\n"), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHelperProgramRowsCarryIDsAndStates(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	dir := t.TempDir()
	normalPath := writeTestProgram(t, dir, "normal", 0755)
	anyPath := writeTestProgram(t, dir, "any", 0755)

	if _, err := a.rt.cfg.AddHelperProgram("search", normalPath); err != nil {
		t.Fatal(err)
	}
	any, err := a.rt.cfg.AddHelperProgram("index", anyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.rt.cfg.SetHelperAnyDocument(any.ID, true); !ok {
		t.Fatal("any-document program was not found")
	}
	if got := a.helperProgramsTitle(); got != "Programs" {
		t.Fatalf("title with every file present = %q, want %q", got, "Programs")
	}
	if rows := a.helperProgramRows(); len(rows) != 0 {
		t.Fatalf("rows with every file present = %+v, want none", rows)
	}

	missing, err := a.rt.cfg.AddHelperProgram("search", filepath.Join(dir, "gone"))
	if err != nil {
		t.Fatal(err)
	}
	if got := a.helperProgramsTitle(); got != "Programs (1 missing)" {
		t.Fatalf("title with one file missing = %q, want %q", got, "Programs (1 missing)")
	}

	labels := map[string]string{}
	for _, row := range a.helperProgramRows() {
		labels[row.Path] = row.Label
	}
	want := map[string]string{
		missing.ID: "Locate “search” (file missing)…",
	}
	if len(labels) != len(want) {
		t.Fatalf("rows = %#v, want %d", labels, len(want))
	}
	for id, label := range want {
		if labels[id] != label {
			t.Errorf("row %q label = %q, want %q", id, labels[id], label)
		}
	}
}

func TestPrepareHelperProgramWindowsAcceptsOnlyLaunchableExtensions(t *testing.T) {
	// PATHEXT lists .js and Windows would run it under Windows Script Host
	// rather than node, so a node helper with a correct shebang must be refused
	// here instead of failing strangely at launch.
	dir := t.TempDir()
	refuse := func(string, string, string) (bool, error) {
		t.Fatal("the Windows branch must never reach the executable-bit prompt")
		return false, nil
	}
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"helper.exe", true},
		{"helper.bat", true},
		{"helper.cmd", true},
		{"HELPER.CMD", true},
		{"helper.js", false},
		{"helper.ps1", false},
		{"helper.py", false},
		{"helper", false},
	} {
		path := filepath.Join(dir, tc.name)
		if err := os.WriteFile(path, []byte("echo hi\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		ok, err := prepareHelperProgramOn("windows", path, refuse)
		if ok != tc.want {
			t.Errorf("prepareHelperProgramOn(windows, %s) = %v, want %v (err %v)", tc.name, ok, tc.want, err)
		}
		if !tc.want && err == nil {
			t.Errorf("%s was refused with no explanation; the refusal must say to use a .cmd wrapper", tc.name)
		}
		if !tc.want && err != nil && !strings.Contains(err.Error(), ".cmd wrapper") {
			t.Errorf("%s refusal does not say what to do: %v", tc.name, err)
		}
	}
}

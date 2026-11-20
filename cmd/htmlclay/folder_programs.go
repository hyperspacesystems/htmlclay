package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/tier"
)

const (
	folderProgramPrefix = "folder:"
	folderCallersAny    = "htmlclay-callers: any"
	folderCallersScan   = 4096
)

// programsFolder is ~/htmlclay/programs/, the person's own programs. A page
// names one the way it names any program, by the file name without its
// extension.
func programsFolder(home string) string {
	return filepath.Join(home, "htmlclay", "programs")
}

// folderProgram is the programs folder's answer for name on behalf of a
// document whose tier is v. The folder answers only while it is on (developer
// mode, or htmlclay programs enable). It answers the person's own and signed
// documents with no question. Any other page reaches a program only when the
// program opts in with the line "htmlclay-callers: any" in its first 4 KiB,
// and then only after the warning or the team question; waits reports that.
// A program that is missing, downloaded, or not open to this caller leaves the
// name to the registered programs.
func (a *app) folderProgram(name string, v tier.Verdict) (program config.HelperProgram, ok, waits bool) {
	store := a.tierStore()
	if store == nil || !store.Programs() || !config.ValidHelperName(name) {
		return config.HelperProgram{}, false, false
	}
	path, found := findFolderProgram(programsFolder(a.rt.home), name)
	if !found {
		return config.HelperProgram{}, false, false
	}
	program = config.HelperProgram{ID: folderProgramPrefix + name, Name: name, Path: path}
	switch {
	case v.Trusted():
		return program, true, false
	case !callersAny(path):
		return config.HelperProgram{}, false, false
	case v.Warn():
		return config.HelperProgram{}, false, true
	}
	return program, true, false
}

// findFolderProgram finds the one executable file in dir named name, or name
// plus one usual program extension, as the program search does. A file that
// carries a download mark, or that is or links to a shell or script
// interpreter, never counts: a program gets the page's request on its standard
// input. Two matches count as none, so the page never picks between them.
func findFolderProgram(dir, name string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var matches []string
	for _, entry := range entries {
		if !matchesProgramFile(entry.Name(), name) || scriptInterpreter(entry.Name()) {
			continue
		}
		candidate := filepath.Join(dir, entry.Name())
		if runtime.GOOS == "windows" && !windowsHelperProgramExtension(candidate) {
			continue
		}
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode()&0o111 == 0) {
			continue
		}
		resolved, err := resolveSymlinks(candidate)
		if err != nil || scriptInterpreter(resolved) || platform.Downloaded(candidate) || platform.Downloaded(resolved) {
			continue
		}
		if !containsSameFile(matches, resolved) {
			matches = append(matches, resolved)
		}
	}
	if len(matches) != 1 {
		return "", false
	}
	return matches[0], true
}

// callersAny reports a program that opts in to every caller: a line in its
// first 4 KiB that reads "htmlclay-callers: any", alone or after a comment
// marker.
func callersAny(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head, err := io.ReadAll(io.LimitReader(f, folderCallersScan))
	if err != nil {
		return false
	}
	lines := bufio.NewScanner(bytes.NewReader(head))
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		for _, marker := range []string{"#", "//", "--", "/*", "*", ";", "rem ", "REM ", "::"} {
			if rest, ok := strings.CutPrefix(line, marker); ok {
				line = strings.TrimSpace(rest)
				break
			}
		}
		if strings.EqualFold(strings.TrimSpace(strings.TrimSuffix(line, "*/")), folderCallersAny) {
			return true
		}
	}
	return false
}

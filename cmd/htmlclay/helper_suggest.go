package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/htmlutil"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/session"
)

// helperSuggestSettle is how long a program file must sit unchanged before
// HTML Clay offers it: a file that just arrived next to a document is exactly
// what an attacker would plant.
const helperSuggestSettle = 10 * time.Minute

// helperSuggestScanCap bounds how many folder entries one suggestion reads.
const helperSuggestScanCap = 2000

// helperSuggestRiskyFolders are the folders a document can arrive in together
// with a program: Downloads and the temp folder. Tests replace it, because their
// home folder sits inside the temp folder.
var helperSuggestRiskyFolders = func(home string) []string {
	return []string{filepath.Join(home, "Downloads"), os.TempDir()}
}

// helperSuggestion is the program HTML Clay offers for one name. program is set
// when the file is already registered; path is the file to run, empty when
// nothing qualifies and the person picks one. withheld lists matches that were
// left out, in plain words, so the dialog can say why.
type helperSuggestion struct {
	program  config.HelperProgram
	path     string
	withheld []string
}

func readDocumentHelperFiles(document string) map[string]string {
	f, err := os.Open(document)
	if err != nil {
		return nil
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, helperDeclarationReadLimit))
	if err != nil {
		return nil
	}
	return htmlutil.ReadHelperFiles(data)
}

// suggestHelperProgram finds the one file to offer for name in document. It
// looks, in order, at programs already registered under name, at registered
// programs whose file is named as the document expects, and then at files with
// that name directly inside folders the person already keeps programs in. It
// never looks in the document's own folder, Downloads or the temp folder, never
// offers a downloaded file or one changed in the last few minutes, and offers
// nothing when more than one file qualifies. The page chooses only the name it
// expects; where HTML Clay looks is the person's.
func (a *app) suggestHelperProgram(document, name string, now time.Time) helperSuggestion {
	expected := readDocumentHelperFiles(document)[name]
	if expected == "" {
		expected = name
	}
	programs := a.rt.cfg.HelperProgramList()

	var sameName []config.HelperProgram
	for _, p := range programs {
		if p.Name == name && !helperProgramMissing(p.Path) && !scriptInterpreter(p.Path) {
			sameName = append(sameName, p)
		}
	}
	if len(sameName) == 1 {
		return helperSuggestion{program: sameName[0], path: sameName[0].Path}
	}
	if len(sameName) > 1 {
		return helperSuggestion{}
	}

	var byFile []config.HelperProgram
	for _, p := range programs {
		if matchesProgramFile(filepath.Base(p.Path), expected) && !helperProgramMissing(p.Path) && !scriptInterpreter(p.Path) && !containsSameFile(programPaths(byFile), p.Path) {
			byFile = append(byFile, p)
		}
	}
	if len(byFile) == 1 {
		return helperSuggestion{program: byFile[0], path: byFile[0].Path}
	}
	if len(byFile) > 1 {
		return helperSuggestion{}
	}

	found, withheld := a.scanKnownProgramFolders(a.knownProgramFolders(document, programs), expected, now)
	if hinted := a.helperPickerFolder(document, name); hinted != filepath.Dir(document) {
		var inHint []string
		for _, path := range found {
			if filepath.Dir(path) == hinted {
				inHint = append(inHint, path)
			}
		}
		if len(inHint) > 0 {
			found = inHint
		}
	}
	if len(found) == 1 {
		return helperSuggestion{path: found[0], withheld: withheld}
	}
	return helperSuggestion{withheld: withheld}
}

// scanKnownProgramFolders lists the settled, runnable files named expected
// directly inside folders, and says in plain words why it left out the others.
func (a *app) scanKnownProgramFolders(folders []string, expected string, now time.Time) (found, withheld []string) {
	scanned := 0
	for _, folder := range folders {
		entries, err := os.ReadDir(folder)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if scanned++; scanned > helperSuggestScanCap {
				return found, withheld
			}
			if !matchesProgramFile(entry.Name(), expected) || scriptInterpreter(entry.Name()) {
				continue
			}
			path := filepath.Join(folder, entry.Name())
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode()&0111 == 0) {
				continue
			}
			switch {
			case platform.Downloaded(path):
				withheld = append(withheld, fmt.Sprintf("%s was downloaded from the internet, so it is not suggested.", a.tildePath(path)))
			case platform.ChangedWithin(path, helperSuggestSettle, now):
				withheld = append(withheld, fmt.Sprintf("%s changed in the last few minutes, so it is not suggested.", a.tildePath(path)))
			case !containsSameFile(found, path):
				found = append(found, path)
			}
		}
	}
	return found, withheld
}

// knownProgramFolders is every folder that holds a registered program file,
// minus the folders a document could have arrived with: its own folder and
// those below it (only the folder itself for a document loose in home; none
// when document is empty, as for relinking from the Programs page),
// Downloads, and the temp folder.
func (a *app) knownProgramFolders(document string, programs []config.HelperProgram) []string {
	docDir := filepath.Dir(document)
	excluded := func(folder string) bool {
		if document != "" && (folder == docDir || (docDir != a.rt.home && session.EqualOrUnder(folder, docDir))) {
			return true
		}
		for _, risky := range helperSuggestRiskyFolders(a.rt.home) {
			if session.EqualOrUnder(folder, risky) {
				return true
			}
			if real, err := filepath.EvalSymlinks(risky); err == nil && session.EqualOrUnder(folder, real) {
				return true
			}
		}
		return false
	}
	var folders []string
	seen := make(map[string]bool)
	for _, p := range programs {
		if helperProgramMissing(p.Path) {
			continue
		}
		folder := filepath.Dir(filepath.Clean(p.Path))
		if seen[folder] || excluded(folder) {
			continue
		}
		seen[folder] = true
		folders = append(folders, folder)
	}
	return folders
}

// scriptInterpreters are programs that run their input as code. A helper gets
// the page's request on stdin, so offering one of these would hand the page a
// shell. They are never suggested, and picking one by hand gets a warning.
var scriptInterpreters = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "mksh": true, "fish": true, "csh": true, "tcsh": true,
	"pwsh": true, "powershell": true, "cmd": true, "wscript": true, "cscript": true, "osascript": true, "env": true,
	"node": true, "deno": true, "bun": true, "ruby": true, "irb": true, "perl": true, "php": true, "lua": true, "tclsh": true,
	"python": true, "python2": true, "python3": true,
}

// scriptInterpreter reports whether path names a shell or script interpreter,
// ignoring case, a Windows extension and a version suffix such as python3.12.
func scriptInterpreter(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	for _, ext := range []string{".exe", ".cmd", ".bat"} {
		base = strings.TrimSuffix(base, ext)
	}
	if scriptInterpreters[base] {
		return true
	}
	stem := strings.TrimRight(base, "0123456789.")
	return stem != base && scriptInterpreters[stem]
}

// helperFileRisk says, in plain words, why a file someone picked by hand is
// one HTML Clay would not have suggested, or "" when nothing stands out.
// document is "" when no document is involved, as for a relink.
func (a *app) helperFileRisk(document, path string, now time.Time) string {
	dir := filepath.Dir(filepath.Clean(path))
	switch {
	case scriptInterpreter(path):
		return "is a shell or script interpreter, so it would run whatever a page sends it as code"
	case document != "" && session.EqualOrUnder(dir, filepath.Dir(document)) && filepath.Dir(document) != a.rt.home:
		return "sits in the document's own folder, where a downloaded document can bring its own program"
	}
	for _, risky := range helperSuggestRiskyFolders(a.rt.home) {
		if session.EqualOrUnder(dir, risky) {
			return "is in a downloads or temporary folder"
		}
		if real, err := filepath.EvalSymlinks(risky); err == nil && session.EqualOrUnder(dir, real) {
			return "is in a downloads or temporary folder"
		}
	}
	switch {
	case platform.Downloaded(path):
		return "was downloaded from the internet"
	case platform.ChangedWithin(path, helperSuggestSettle, now):
		return "changed in the last few minutes"
	}
	return ""
}

func helperProgramExtensions() []string {
	if runtime.GOOS == "windows" {
		return []string{".exe", ".cmd", ".bat"}
	}
	return []string{".sh", ".py", ".mjs", ".js", ".rb", ".pl"}
}

// matchesProgramFile reports whether a file named base is the program expected:
// the exact name, or the name plus one usual program extension. Case is ignored
// where the file system ignores it.
func matchesProgramFile(base, expected string) bool {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		base, expected = strings.ToLower(base), strings.ToLower(expected)
	}
	if base == expected {
		return true
	}
	stem, ok := strings.CutPrefix(base, expected)
	if !ok {
		return false
	}
	for _, ext := range helperProgramExtensions() {
		if stem == ext {
			return true
		}
	}
	return false
}

func programPaths(programs []config.HelperProgram) []string {
	paths := make([]string, len(programs))
	for i, p := range programs {
		paths[i] = p.Path
	}
	return paths
}

func containsSameFile(paths []string, path string) bool {
	info, err := os.Stat(path)
	for _, existing := range paths {
		if existing == path {
			return true
		}
		if err != nil {
			continue
		}
		if other, err := os.Stat(existing); err == nil && os.SameFile(info, other) {
			return true
		}
	}
	return false
}

// tildePath shows a path under home as ~/..., the way the person thinks of it.
func (a *app) tildePath(path string) string {
	if _, ok := session.ContainWithinHome(a.rt.home, path); !ok {
		return path
	}
	rest, err := filepath.Rel(a.rt.home, path)
	if err != nil {
		return path
	}
	return "~" + string(os.PathSeparator) + rest
}

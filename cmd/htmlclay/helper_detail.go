package main

import (
	"path/filepath"
	"time"

	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/session"
)

const (
	helperDetailInDownloads       = "in-downloads"
	helperDetailInTemp            = "in-temp"
	helperDetailJustArrived       = "just-arrived"
	helperDetailDownloaded        = "downloaded"
	helperDetailNotExecutable     = "not-executable"
	helperDetailScriptInterpreter = "script-interpreter"
	helperDetailScopeUnknown      = "scope-unknown"
	helperDetailMainFolder        = "main-folder"
)

var helperDetailConditions = map[string]string{
	helperDetailInDownloads:       "the file sits in the first risky folder, Downloads",
	helperDetailInTemp:            "the file sits in the second risky folder, the temporary folder",
	helperDetailJustArrived:       "the file changed within helperSuggestSettle",
	helperDetailDownloaded:        "platform.Downloaded reports the file came from the internet",
	helperDetailNotExecutable:     "prepareHelperProgramOn found the file without an executable bit",
	helperDetailScriptInterpreter: "scriptInterpreter matches the file name",
	helperDetailScopeUnknown:      "config.ScopeOf returned ScopeUnknown for the document",
	helperDetailMainFolder:        "the file sits in the document's own folder, which is not home",
}

// helperFileRiskDetail is helperFileRisk as the closed detail codes, so the
// request window can say why a file is risky without quoting prose.
func (a *app) helperFileRiskDetail(document, path string, now time.Time) string {
	dir := filepath.Dir(filepath.Clean(path))
	switch {
	case scriptInterpreter(path):
		return helperDetailScriptInterpreter
	case document != "" && session.EqualOrUnder(dir, filepath.Dir(document)) && filepath.Dir(document) != a.rt.home:
		return helperDetailMainFolder
	}
	for i, risky := range helperSuggestRiskyFolders(a.rt.home) {
		if helperUnderRiskyFolder(dir, risky) {
			if i == 0 {
				return helperDetailInDownloads
			}
			return helperDetailInTemp
		}
	}
	switch {
	case platform.Downloaded(path):
		return helperDetailDownloaded
	case platform.ChangedWithin(path, helperSuggestSettle, now):
		return helperDetailJustArrived
	}
	return ""
}

func helperUnderRiskyFolder(dir, risky string) bool {
	if session.EqualOrUnder(dir, risky) {
		return true
	}
	real, err := filepath.EvalSymlinks(risky)
	return err == nil && session.EqualOrUnder(dir, real)
}

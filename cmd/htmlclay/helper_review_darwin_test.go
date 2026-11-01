//go:build darwin

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/platform"
	"golang.org/x/sys/unix"
)

func TestReviewPickedDownloadGetsFinalWarning(t *testing.T) {
	a, home := newSuggestApp(t)
	path := writeSuggestFile(t, filepath.Join(home, "Downloads", "search"), 0755)
	if err := unix.Setxattr(path, "com.apple.quarantine", []byte("0081;6720a3b1;Safari;"), 0); err != nil {
		t.Fatal(err)
	}
	doc := writeSuggestDocument(t, filepath.Join(home, "docs", "page.htmlclay"), `<meta name="htmlclay-helper" content="search">`)
	picked := false
	warnings := 0
	dialogs := helperApprovalDialogs{
		setup: func(_, message, _, _, _ string) (platform.Choice, error) {
			if picked && strings.Contains(message, "download") {
				warnings++
			}
			return platform.ChoiceAffirm, nil
		},
		choose: func(string, string) (string, bool, error) { picked = true; return path, true, nil },
		prepare: func(path string) (bool, error) {
			return prepareHelperProgram(path, func(_, message, _ string) (bool, error) {
				if strings.Contains(message, "download") {
					warnings++
				}
				return true, nil
			})
		},
	}
	plan := a.helpersForUseWith(doc, "search", dialogs)
	if _, ok := plan.allowed["search"]; ok && warnings == 0 {
		t.Error("downloaded executable granted after picker with no warning or final confirmation")
	}
}

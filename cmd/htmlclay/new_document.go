package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/server"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/tier"
)

// newDocumentAllowFirst is the page's answer while its document waits for
// the person's answer to the warning.
const newDocumentAllowFirst = "Allow this document first. HTML Clay is asking you about it now."

// newDocumentForPage is the page menu's "New blank document". The page builds
// the blank copy; this writes it beside its source as <name>-new, then
// <name>-new-2 and on, never replacing a file, and returns the address the new
// file serves at, which the page opens. Only a document that may reach its
// folder makes one: any other gets its warning instead, so a stranger's script
// cannot fill a folder. The copy takes the source's download mark, first sight
// and provenance, so a stranger's blank copy is a stranger too. It is recorded
// as the person's own only when the source is and the page changed none of its
// code: the page's script wrote these bytes.
func (a *app) newDocumentForPage(source string, via session.Provenance, html []byte) (string, error) {
	source = closedPath(source)
	if !a.documentTrust(source).Ready {
		a.askDocument(source, servedOpenProgram)
		return "", server.ActionRefusal{Message: newDocumentAllowFirst}
	}
	ext := filepath.Ext(source)
	base := strings.TrimSuffix(filepath.Base(source), ext) + "-new"
	path, err := createFreeDocument(filepath.Dir(source), numberedName(base, "-", ext), html)
	if err != nil {
		return "", err
	}
	a.rt.logger.Printf("Created %s, a new blank document from %s", path, source)
	if err := platform.KeepDownloadMark(source, path); err != nil {
		a.rt.logger.Printf("Could not copy the download mark of %s to %s: %v", source, path, err)
	}
	if store := a.tierStore(); store != nil {
		if err := store.NoteCopy(source, path); err != nil {
			a.rt.logger.Printf("Could not record %s as a copy of %s: %v", path, source, err)
		}
	}
	if a.tierOf(source).Tier == tier.Mine && a.sameCode(source, path) {
		if err := a.recordMadeDocument(path, tier.HowNewDocument); err != nil {
			a.rt.logger.Printf("Could not record %s as yours: %v", path, err)
		}
	}
	url, ok := a.serveFile(path, via, a.helpersForNavigation(path))
	if !ok {
		return "", fmt.Errorf("could not serve %s", path)
	}
	return url, nil
}

// sameCode reports whether the documents at x and y run the same code.
func (a *app) sameCode(x, y string) bool {
	sx, ok := a.codeSetOf(x)
	if !ok {
		return false
	}
	sy, ok := a.codeSetOf(y)
	return ok && sx.Hash == sy.Hash
}

package main

import (
	"fmt"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/platform"
)

// deniedDocumentFacts lists documents with a stored "Don't allow" so the
// Programs page can offer "Ask again".
func (a *app) deniedDocumentFacts() []string {
	store := a.tierStore()
	if store == nil {
		return nil
	}
	return store.DeniedDocuments()
}

// clearDocumentDeny removes a stored "Don't allow" and quiet window for
// document so its next action asks again.
func (a *app) clearDocumentDeny(document string) error {
	path := closedPath(document)
	store := a.tierStore()
	if store == nil {
		return fmt.Errorf("HTML Clay has no settings folder to record this in")
	}
	if err := store.ClearDeny(path); err != nil {
		return err
	}
	a.helperStateMu.Lock()
	delete(a.documentQuiet, path)
	a.helperStateMu.Unlock()
	a.refreshHelperDispatchers()
	a.changed()
	return nil
}

func (a *app) askDocumentAgainEffect(e appcore.Effect) *appcore.Refusal {
	if e.DocumentID == "" {
		return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "documentID"}
	}
	if err := a.clearDocumentDeny(e.DocumentID); err != nil {
		a.rt.logger.Printf("Could not clear Don't allow for %s: %v", e.DocumentID, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.DocumentID}
	}
	return nil
}

func (a *app) askDocumentAgainFromPrograms(document string, dialogs helperApprovalDialogs) (string, error) {
	store := a.tierStore()
	if store == nil {
		return "failed", nil
	}
	path := closedPath(document)
	ans, ok := store.Answer(path)
	if !ok || !ans.Denied {
		return "unchanged", nil
	}
	message := fmt.Sprintf("Let %s ask for permission again the next time you use it?\n\n    %s\n\nThis clears your earlier Don't allow answer without granting access yet.", filepath.Base(path), a.tildePath(path))
	choice, err := dialogs.setup("Let "+filepath.Base(path)+" ask again?", message, helperSetupCancel, "Keep Blocked", "Ask Again")
	if err != nil || choice != platform.ChoiceAffirm {
		return "unchanged", err
	}
	if err := a.clearDocumentDeny(path); err != nil {
		return "", err
	}
	return "ask-again", nil
}

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/confirm"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/tier"
)

// aiEditProgramID is the id the built-in AI editing goes by wherever a program
// id is expected: the team question, the Programs list, and Remove Access. A
// registered program's id is 32 hex digits, so the two never meet.
const aiEditProgramID = appcore.AIEditProgram

const (
	aiEditDialogTitle   = "Let %s use AI editing?"
	aiEditDialogMessage = "Other people can edit HTML files in this team folder later. Allowing applies to this HTML file only, never the whole folder. After allowing, go back to the page and send your request again."
)

// aiEditFor is both hosts' answer to whether document may use AI editing now.
// The switch decides first. The person's own and signed documents need nothing
// more. A document nobody vouches for needs the person's allow on the unknown
// document warning. A team document, or one whose team record cannot be read,
// needs the person's allow for that document in that scope, and for its
// current code; without both the page's request is refused and the person is
// asked.
func (a *app) aiEditFor(document string) (on, ask bool) {
	if !a.rt.cfg.AIEditEnabled() {
		return false, false
	}
	switch v := a.tierOf(document); {
	case v.Trusted():
		return true, false
	case v.Tier == tier.Unknown:
		return v.Allowed, !v.Allowed
	default:
		on = v.Allowed && a.rt.cfg.AIEditDocumentAllowed(document, a.scopeOf(document))
		return on, !on
	}
}

// askAIEdit is both hosts' AskAIEdit: it raises the question document needs
// and returns at once, because it runs on the page's own request: the warning
// for a document nobody vouches for, the AI editing question for a team
// document never allowed it, and the team question for one whose code changed
// since. One question per document is open at a time, and after one that
// allowed nothing the document stays quiet for a minute.
func (a *app) askAIEdit(document string) {
	v := a.tierOf(document)
	switch {
	case v.Trusted():
		return
	case v.Tier == tier.Unknown, a.rt.cfg.AIEditDocumentAllowed(document, a.scopeOf(document)):
		a.askDocument(document, aiEditProgramID)
		return
	}
	if !a.reserveAIEditAsk(document) {
		return
	}
	go a.runAIEditAsk(document, a.scopeOf(document), v)
}

func (a *app) reserveAIEditAsk(document string) bool {
	a.helperStateMu.Lock()
	defer a.helperStateMu.Unlock()
	if a.aiEditAsking[document] || time.Now().Before(a.aiEditQuiet[document]) {
		return false
	}
	if a.aiEditAsking == nil {
		a.aiEditAsking = make(map[string]bool)
	}
	a.aiEditAsking[document] = true
	return true
}

// runAIEditAsk asks the question and stores the answer. Only allow-document
// stores anything, under the scope the person was asked about.
func (a *app) runAIEditAsk(document string, scope config.Scope, v tier.Verdict) {
	allowed := false
	defer func() {
		a.helperStateMu.Lock()
		delete(a.aiEditAsking, document)
		if allowed {
			delete(a.aiEditQuiet, document)
		} else {
			if a.aiEditQuiet == nil {
				a.aiEditQuiet = make(map[string]time.Time)
			}
			a.aiEditQuiet[document] = time.Now().Add(pageAskQuietFor)
		}
		a.helperStateMu.Unlock()
	}()

	presenter := a.presenter(aiEditDialog{app: a})
	reply, err := presenter.Ask(context.Background(), a.aiEditAsk(document, scope))
	if err != nil {
		a.rt.logger.Printf("Could not ask about AI editing for %s: %v", document, err)
		return
	}
	defer presenter.Close(reply.ID)
	if reply.Choice != helperAnswerAllowDocument {
		a.rt.logger.Printf("AI editing was not allowed for %s; its page can ask again in a minute", document)
		return
	}
	if err := a.allowAIEdit(document, scope); err != nil {
		a.rt.logger.Printf("Could not allow AI editing for %s: %v", document, err)
		return
	}
	a.allowTeamCode(document, v)
	allowed = true
	a.rt.logger.Printf("AI editing allowed: document=%s scope=%s", document, scope.Key)
	a.changed()
}

// aiEditAsk is the team question for AI editing: the document and its folder, no
// file, and the two answers a team document can get.
func (a *app) aiEditAsk(document string, scope config.Scope) confirm.Ask {
	ask := confirm.Ask{
		Kind:     appcore.RequestTeamDocument,
		Document: document,
		Program:  aiEditProgramID,
		Folder:   filepath.Base(filepath.Dir(document)),
		Identity: platform.DirIdentity(document),
		Answers:  []string{helperAnswerDeny, helperAnswerAllowDocument},
	}
	if scope.Kind == config.ScopeUnknown {
		ask.Detail = helperDetailScopeUnknown
	} else if a.teamCodeChanged(document, a.tierOf(document)) {
		ask.Detail = teamCodeDetail
	}
	return ask
}

func (a *app) allowAIEdit(document string, scope config.Scope) error {
	a.helperStateMu.Lock()
	defer a.helperStateMu.Unlock()
	previous, had, err := a.rt.cfg.AllowAIEditDocument(document, scope)
	if err != nil {
		return err
	}
	if err := a.rt.cfg.Save(); err != nil {
		a.rt.cfg.ForgetAIEditDocument(document)
		if had {
			a.rt.cfg.RestoreAIEditDocument(previous)
		}
		return fmt.Errorf("could not save the AI editing decision: %w", err)
	}
	return nil
}

// aiEditDialog presents the AI editing question with today's two-button dialog.
type aiEditDialog struct {
	app *app
}

func (d aiEditDialog) Ask(_ context.Context, ask confirm.Ask) (confirm.Reply, error) {
	show := d.app.rt.confirm
	if show == nil {
		show = d.app.readPrompt
	}
	choice, err := show(fmt.Sprintf(aiEditDialogTitle, filepath.Base(ask.Document)), aiEditDialogMessage, false)
	if err != nil || choice == platform.ConfirmDeny {
		return confirm.Reply{ID: ask.ID, Choice: confirm.ChoiceDeny}, err
	}
	return confirm.Reply{ID: ask.ID, Choice: helperAnswerAllowDocument}, nil
}

func (aiEditDialog) Close(string) {}

// aiEditProgramFact lists AI editing among the programs when a document has its
// allow, so the Programs list shows who may use it and Remove Access takes it
// back. It is absent when nobody was ever asked.
func (a *app) aiEditProgramFact() (appcore.Program, bool) {
	rows := a.rt.cfg.AIEditDocumentList()
	if len(rows) == 0 {
		return appcore.Program{}, false
	}
	documents := make([]string, 0, len(rows))
	for _, row := range rows {
		documents = append(documents, row.Document)
	}
	slices.Sort(documents)
	return appcore.Program{
		ID:               aiEditProgramID,
		Name:             aiEditProgramID,
		Builtin:          true,
		On:               a.rt.cfg.AIEditEnabled(),
		AllowedDocuments: documents,
		RemovedDocuments: []string{},
	}, true
}

// removeAIEditAccess is program.removeAccess and program.removeAllAccess for AI
// editing. The allow is removed, so the document asks again, and a request
// running on it is stopped.
func (a *app) removeAIEditAccess(e appcore.Effect) *appcore.Refusal {
	all := e.Type == "program.removeAllAccess"
	a.helperStateMu.Lock()
	var removed []config.AIEditDocument
	for _, row := range a.rt.cfg.AIEditDocumentList() {
		if !all && row.Document != e.DocumentID {
			continue
		}
		if forgotten, ok := a.rt.cfg.ForgetAIEditDocument(row.Document); ok {
			removed = append(removed, forgotten)
		}
	}
	err := a.rt.cfg.Save()
	if err != nil {
		for _, row := range removed {
			a.rt.cfg.RestoreAIEditDocument(row)
		}
	}
	a.helperStateMu.Unlock()
	if err != nil {
		a.rt.logger.Printf("Could not remove access to AI editing: %v", err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
	for _, row := range removed {
		if core, ok := a.wireCoreFor(row.Document); ok {
			core.CancelAIFor(row.Document)
		}
		a.rt.logger.Printf("AI editing access removed: document=%s", row.Document)
	}
	return nil
}

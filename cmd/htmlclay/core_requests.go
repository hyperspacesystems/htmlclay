package main

import (
	"errors"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/confirm"
)

// registry answers the one request registry this host has, building it the first
// time anything needs it. Every ask and every answer goes through it.
func (a *app) registry() *confirm.Registry {
	a.initCore()
	a.core.mu.Lock()
	defer a.core.mu.Unlock()
	return a.core.requests
}

// requestFacts lists the questions waiting for the person, in ask order, so the
// snapshot shows them and Plan can check an answer against them.
func (a *app) requestFacts() []appcore.RequestFact {
	pending := a.registry().Pending()
	facts := make([]appcore.RequestFact, 0, len(pending))
	for _, p := range pending {
		facts = append(facts, appcore.RequestFact{
			ID:         p.ID,
			Kind:       p.Ask.Kind,
			Revision:   p.Revision,
			DocumentID: p.Ask.Identity,
			Document:   p.Ask.Document,
			Program:    p.Ask.Program,
			Folder:     p.Ask.Folder,
			Path:       p.Ask.Path,
			Detail:     p.Ask.Detail,
			Answers:    p.Ask.Answers,
		})
	}
	return facts
}

// answerRequest hands one answer to the registry as the person gave it. The
// revision is the one the snapshot showed, so a request that changed since is
// refused, and a choice this request never offered is refused.
func (a *app) answerRequest(e appcore.Effect) *appcore.Refusal {
	err := a.registry().Answer(e.RequestID, e.Revision, e.Answer, e.Path)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, confirm.ErrNoSuchRequest):
		return &appcore.Refusal{Code: appcore.RefuseNoSuchRequest, SubjectID: e.RequestID}
	case errors.Is(err, confirm.ErrRequestChanged):
		return &appcore.Refusal{Code: appcore.RefuseRequestChanged, SubjectID: e.RequestID}
	case errors.Is(err, confirm.ErrAnswerNotOffered):
		return &appcore.Refusal{Code: appcore.RefuseAnswerNotOffered, Field: "answer", SubjectID: e.RequestID}
	default:
		a.rt.logger.Printf("Could not answer request %s: %v", e.RequestID, err)
		return &appcore.Refusal{Code: appcore.RefuseBadArguments, Field: "path"}
	}
}

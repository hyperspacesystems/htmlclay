package main

import (
	"errors"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/confirm"
	"github.com/panphora/htmlclay/internal/pathrel"
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
			Attention:  p.Attention,
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

// showRequest raises one question in the Mac app's request window. It changes
// nothing else about the question: its answers, its revision and its path stay
// what they were. An id nobody is waiting on any more is no-such-request.
func (a *app) showRequest(e appcore.Effect) *appcore.Refusal {
	err := a.registry().Show(e.RequestID)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, confirm.ErrNoSuchRequest):
		return &appcore.Refusal{Code: appcore.RefuseNoSuchRequest, SubjectID: e.RequestID}
	default:
		a.rt.logger.Printf("Could not show request %s: %v", e.RequestID, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed}
	}
}

// closeFolderRequests denies and drops every question about a folder this host
// no longer trusts, once the trust is gone: the page that asked is closed with
// it, so nobody is waiting for the answer. A question about a folder that is
// still trusted below dir stays where it is.
func (a *app) closeFolderRequests(dir string) {
	trusted := a.rt.cfg.TrustedFolderList()
	requests := a.registry()
	for _, p := range requests.Pending() {
		if p.Ask.Kind != appcore.RequestFolderRead && p.Ask.Kind != appcore.RequestFolderReadAgain {
			continue
		}
		if p.Ask.Path == "" || !pathrel.SameOrBelow(p.Ask.Path, dir) {
			continue
		}
		kept := false
		for _, tf := range trusted {
			if pathrel.SameOrBelow(p.Ask.Path, tf.Path) {
				kept = true
				break
			}
		}
		if !kept {
			requests.Close(p.ID)
		}
	}
}

// stopRequests denies every question still waiting: the host is going, so no
// answer can arrive, and a question nobody can answer grants nothing.
func (a *app) stopRequests() {
	a.core.mu.Lock()
	requests := a.core.requests
	a.core.mu.Unlock()
	if requests != nil {
		requests.Stop()
	}
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

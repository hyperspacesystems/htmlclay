package main

import (
	"context"
	"strings"

	"github.com/panphora/htmlclay/internal/appcore"
)

// resolveConflict is conflict.resolve: the person's choice for one held
// conflict goes to the service, whose engine keeps that copy (C3
// /sync/v1/resolve). Go writes nothing here; the engine's writes go through
// Go's commit path as every sync write does. The held list is read again
// right away, so an open screen drops the row without waiting for a report.
func (a *app) resolveConflict(e appcore.Effect) *appcore.Refusal {
	if a.runningService() == nil {
		return &appcore.Refusal{Code: appcore.RefuseServiceNeeded, SubjectID: e.FolderID}
	}
	session, _, _ := strings.Cut(e.Conflict, ":")
	ctx, cancel := context.WithTimeout(context.Background(), syncCallTimeout)
	defer cancel()
	body := struct {
		SessionID string `json:"sessionId"`
		Path      string `json:"path"`
		Keep      string `json:"keep"`
	}{SessionID: session, Path: e.Path, Keep: e.Keep}
	var answer struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := a.served.CallService(ctx, "/sync/v1/resolve", body, &answer); err != nil {
		a.rt.logger.Printf("Could not resolve the conflict on %s: %v", e.Path, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.FolderID}
	}
	defer func() {
		a.readHeldConflicts()
		a.changed()
	}()
	if answer.OK {
		return nil
	}
	switch answer.Error {
	case "no-conflict", "unknown":
		return &appcore.Refusal{Code: appcore.RefuseConflictGone, SubjectID: e.FolderID}
	case "changed", "stale-etag":
		// stale-etag: the server's copy moved on; the engine has kept the newer
		// one, so the person looks again before choosing.
		return &appcore.Refusal{Code: appcore.RefuseConflictChanged, SubjectID: e.FolderID}
	case "not-text":
		return &appcore.Refusal{Code: appcore.RefuseNotText, SubjectID: e.FolderID}
	}
	a.rt.logger.Printf("Could not resolve the conflict on %s: %s", e.Path, answer.Error)
	return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.FolderID}
}

// resolveRemoval is folder.resolveRemoval: the person's choice for a removal of
// many attachments the engine holds (C3 /sync/v1/removal). restore puts them
// back here; delete removes them on the server too.
func (a *app) resolveRemoval(e appcore.Effect) *appcore.Refusal {
	if a.runningService() == nil {
		return &appcore.Refusal{Code: appcore.RefuseServiceNeeded, SubjectID: e.FolderID}
	}
	session := ""
	a.rt.syncMu.Lock()
	if record, err := a.syncRecordLocked(); err == nil {
		for _, s := range record.SyncSessions {
			if s.RootID == e.FolderID {
				session = s.ID
			}
		}
	}
	a.rt.syncMu.Unlock()
	if session == "" {
		return &appcore.Refusal{Code: appcore.RefuseRemovalGone, SubjectID: e.FolderID}
	}
	ctx, cancel := context.WithTimeout(context.Background(), syncCallTimeout)
	defer cancel()
	body := struct {
		SessionID string `json:"sessionId"`
		Choice    string `json:"choice"`
	}{SessionID: session, Choice: e.Choice}
	var answer struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := a.served.CallService(ctx, "/sync/v1/removal", body, &answer); err != nil {
		a.rt.logger.Printf("Could not answer the held removal in folder %s: %v", e.FolderID, err)
		return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.FolderID}
	}
	if answer.OK {
		return nil
	}
	if answer.Error == "no-removal" || answer.Error == "unknown" {
		return &appcore.Refusal{Code: appcore.RefuseRemovalGone, SubjectID: e.FolderID}
	}
	a.rt.logger.Printf("Could not answer the held removal in folder %s: %s", e.FolderID, answer.Error)
	return &appcore.Refusal{Code: appcore.RefuseFailed, SubjectID: e.FolderID}
}

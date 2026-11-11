//go:build managedtrial

package managedtrial

import (
	"log"
	"os"
	"regexp"
	"time"

	"github.com/panphora/htmlclay/internal/specwire"
)

// The three lanes this stage ports from Local's document host: `exact` (a sync
// download), `create` (a new document) and `save` (a browser save and a data
// API write). All three run with no local authority, so every branch that needs
// a room, a target or a token is the full-host stage's to add.

// commitExact is Local's commitExact: the bytes a sync download brought, stored
// as they are, with the versions and blockeditor-state record that came with them.
func (h *commitHost) commitExact(request *commitRequest, filePath string) (commitResult, error) {
	isDocument := request.Text && isDocumentName(request.Name)
	var consult *consultation
	if isDocument {
		var refusal *commitResult
		consult, refusal = h.beginLane(request, filePath, "exact", request.Kind)
		if refusal != nil {
			return *refusal, nil
		}
	}
	var previous *string
	if isDocument {
		if data, err := os.ReadFile(filePath); err == nil {
			text := string(data)
			previous = &text
		}
	}

	if isDocument {
		CreateBackupIfExists(filePath, backupSiteName(request.Name), h.baseDir, h.observer())
		if request.PageRecord != nil {
			if err := writePageState(h.baseDir, request.Name, request.PageRecord, false); err != nil {
				return commitResult{}, err
			}
		}
	} else {
		CreateBinaryBackupIfExists(filePath, request.Name, h.baseDir, h.observer())
	}

	stored := any(request.Bytes)
	content := any(request.Bytes)
	storedBytes := request.Bytes
	if isDocument {
		text, err := storePageState(h.baseDir, request.Name, string(request.Bytes), false)
		if err != nil {
			return commitResult{}, err
		}
		stored = text
		content = text
		storedBytes = []byte(text)
		h.hooks.MarkPending(h.baseDir, request.Name, text)
	}

	if err := atomicWriteFile(filePath, storedBytes, nil, true); err != nil {
		if consult != nil {
			consult.settleFailed(err, "sync-engine")
		}
		return commitResult{}, err
	}
	h.hooks.MarkBrowserSave(request.Name)

	if request.ModifiedAt != nil {
		stamp := modifiedTime(*request.ModifiedAt)
		if err := os.Chtimes(filePath, stamp, stamp); err != nil {
			if consult != nil {
				_ = consult.settle(settleArgs{Outcome: "indeterminate", Sender: "sync-engine"})
			}
			return commitResult{}, indefinite(err)
		}
	}
	if consult != nil {
		// Local's confirmExact: a confirmation that fails leaves the write
		// indeterminate, as Local's does, and the document unconfirmed.
		logical := string(request.Bytes)
		file := stored.(string)
		if err := consult.settle(settleArgs{Outcome: "committed", Logical: &logical, File: &file, Sender: "sync-engine"}); err != nil {
			return commitResult{}, indefinite(err)
		}
	}

	if isDocument {
		if err := h.hooks.RefreshDerivedArtifacts(h.baseDir, request.Name, string(request.Bytes)); err != nil {
			if consult == nil {
				return commitResult{}, indefinite(err)
			}
			log.Printf("[SYNC] derived artifact refresh failed: %v", err)
		}
		h.hooks.RunDataLossGuard(dataLossGuardArgs{
			BaseDir:     h.baseDir,
			Name:        request.Name,
			NewHTML:     string(request.Bytes),
			PrevContent: previous,
			Prov:        "external",
		})
	}

	checksum := specwire.Etag(request.Bytes)
	if text, ok := stored.(string); ok {
		checksum = specwire.Etag([]byte(text))
	}
	return commitResult{
		Outcome:     "committed",
		Checksum:    checksum,
		Stored:      stored,
		Content:     content,
		HasChecksum: true,
	}, nil
}

// commitCreate is Local's commitCreate: a brand-new document, its page state
// split off first.
func (h *commitHost) commitCreate(request *commitRequest, filePath string) (commitResult, error) {
	var consult *consultation
	if request.Text {
		var refusal *commitResult
		consult, refusal = h.beginLane(request, filePath, "create", request.Kind)
		if refusal != nil {
			return *refusal, nil
		}
	}
	stored, err := storePageState(h.baseDir, request.Name, string(request.Bytes), false)
	if err != nil {
		return commitResult{}, err
	}
	h.hooks.MarkPending(h.baseDir, request.Name, stored)
	if err := atomicWriteFile(filePath, []byte(stored), nil, true); err != nil {
		if consult != nil {
			consult.settleFailed(err, "server-save")
		}
		return commitResult{}, err
	}
	if consult != nil {
		logical := string(request.Bytes)
		etag := specwire.Etag(request.Bytes)
		if err := consult.settle(settleArgs{Outcome: "committed", Logical: &logical, File: &stored, Etag: &etag, Sender: "server-save"}); err != nil {
			return commitResult{}, indefinite(err)
		}
	}
	content := any(stored)
	if !request.Text {
		content = request.Bytes
	}
	return commitResult{
		Outcome:     "committed",
		Etag:        specwire.Etag(request.Bytes),
		Stored:      stored,
		Content:     content,
		HasChecksum: true,
	}, nil
}

// commitSave is Local's commitSave for kinds `browser-save` and `data-api`.
func (h *commitHost) commitSave(request *commitRequest, policy commitLane, filePath string) (commitResult, error) {
	backupName := backupSiteName(request.Name)

	var consult *consultation
	var content, fileContent string
	lane, origin := "save", "browser"
	if policy.origin == "api" {
		lane, origin = "api", "data-api"
	}
	consult, refusal := h.beginLane(request, filePath, lane, origin)
	if refusal != nil {
		return *refusal, nil
	}
	if consult != nil {
		// The full-host stage: the service supplies Local's normalization, the
		// stale rebase and the room's admission, and confirms the write once it
		// lands (protocol v1, admit and settle).
		var refusedAnswer *commitResult
		content, fileContent, refusedAnswer = h.admitted(consult, admitArgs{Incoming: request.Bytes, DataLossPrev: request.DataLossPrev, Headers: request.Headers})
		if refusedAnswer != nil {
			return *refusedAnswer, nil
		}
	} else {
		// Normalized once, before anything is backed up: a refusal must leave no
		// backup and no written byte behind.
		content = h.canonicalForWrite(string(request.Bytes))

		// AD2: where Local rebases a save made against an older page, this host
		// refuses it, before anything is written.
		stale, err := stalePageLineage(h.baseDir, request.Name, content)
		if err != nil {
			return commitResult{}, err
		}
		if stale {
			return refused("stale-blockeditor-lineage"), nil
		}

		var logical string
		fileContent, logical, err = h.splitWrite(request.Name, content, true)
		if err != nil {
			return commitResult{}, err
		}
		content = logical
	}

	// Check if this is the first save (no versions exist yet).
	siteVersionsDir := upathJoin(h.baseDir, versionsDirName, backupName)
	isFirstSave := true
	if entries, err := os.ReadDir(siteVersionsDir); err == nil {
		isFirstSave = len(entries) == 0
	}

	// The bytes already on disk, read once: the first-save backup stores their
	// file part, and a save that changed only the merge state leaves them alone.
	var existingContent *string
	if data, err := os.ReadFile(filePath); err == nil {
		text := string(data)
		existingContent = &text
	}

	if isFirstSave && existingContent != nil {
		existingFile, _ := splitPage(*existingContent)
		CreateBackup(h.baseDir, backupName, []byte(existingFile), nil)
	}
	if existingContent == nil || *existingContent != fileContent {
		CreateBackup(h.baseDir, backupName, []byte(fileContent), nil)
	}

	// The sidecar lands before the file, and only the file part reaches disk.
	stored, err := h.placeBytes(request.Name, filePath, content, request.BeforeRename)
	if err != nil {
		if consult != nil {
			consult.settleFailed(err, saveSender(policy))
		}
		return commitResult{}, err
	}

	// Recorded from the bytes actually written.
	h.store.setEtag(filePath, etagRecord{
		Etag:    specwire.Etag([]byte(content)),
		MtimeNs: mtimeNs(filePath),
		Writer:  policy.origin,
		SaveID:  request.SaveID,
	})

	sender := saveSender(policy)
	if consult != nil {
		// Local's confirmation point. The service publishes the data API notice
		// and the saved broadcast; a failed settle leaves the document
		// unconfirmed and its abort retried, and the write still committed,
		// because the bytes are on disk.
		etag := specwire.Etag([]byte(content))
		var userDriven *bool
		if request.UserDrivenSet {
			value := request.UserDriven
			userDriven = &value
		}
		var saveID *string
		if request.SaveID != "" {
			saveID = &request.SaveID
		}
		_ = consult.settle(settleArgs{
			Outcome: "committed", Logical: &content, File: &fileContent, Etag: &etag,
			Sender: sender, UserDriven: userDriven, CaptureID: request.CaptureID, SaveID: saveID,
		})
	} else {
		// A data API write has no tab behind it, so it reaches edit-mode tabs the
		// way an external disk change does. Whether the document needs adoption is
		// ClayJS's checkpoint classifier, which this stage has not ported: no
		// observer is attached yet and the classifier answers false here. The
		// full-host stage adds it.
		adopt := false
		if policy.origin == "api" && !adopt {
			change := externalChange{Kind: "external-change", Sender: "data-api"}
			if len(fileContent) <= maxExternalHTML {
				change.HTML = fileContent
				change.Etag = specwire.Etag([]byte(content))
				change.HasHTML = true
			}
			h.hooks.Notify(request.Name, liveNotification{
				MsgType: "warning",
				Msg:     upathBase(request.Name) + " was updated through the data API",
				Action:  "reload",
				Data:    change,
			})
		}
		h.hooks.Broadcast(request.Name, liveMessage{HTML: fileContent, Sender: sender}, broadcastOptions{Lane: "saved"})
	}

	// The API data sidecar before the fallible Tailwind compile, and non-fatal:
	// a sidecar error must never fail the save.
	writeAPISidecar(h.baseDir, request.Name, content)

	prov := "external"
	if policy.origin != "api" {
		prov = "ui-unknown"
		if request.UserDrivenSet {
			prov = h.hooks.ProvenanceForLocalSave(request.UserDriven)
		}
	}
	h.hooks.RunDataLossGuard(dataLossGuardArgs{
		BaseDir:     h.baseDir,
		Name:        request.Name,
		NewHTML:     content,
		PrevContent: request.DataLossPrev,
		Prov:        prov,
	})

	// Generate the site's Tailwind CSS when it uses one. Non-fatal, for the same
	// reason as the sidecar: the file is already written, backed up and
	// broadcast, so a compiler error must not report the save as failed.
	if tailwindCSSName(content) != "" {
		_ = h.hooks.RefreshTailwind(content)
	}

	// The provenance bit for platform sync, merged rather than replaced with the
	// snapshot /live-sync/save contributes.
	previous, _ := h.store.snapshot(request.Name)
	entry := snapshotEntry{UserDriven: request.UserDriven, UserDrivenSet: request.UserDrivenSet, Timestamp: time.Now().UnixMilli()}
	if previous.HTML != nil {
		entry.HTML = previous.HTML
	}
	h.store.setSnapshot(request.Name, entry)

	return commitResult{
		Outcome: "committed",
		Etag:    specwire.Etag([]byte(content)),
		Stored:  stored,
		Content: content,
		File:    fileContent,
		HasFile: true,
	}, nil
}

// saveSender is who a save lane write is from, as its broadcast names it.
func saveSender(policy commitLane) string {
	if policy.origin == "api" {
		return "data-api"
	}
	return "server-save"
}

// tailwindHref is tailwind-hyperclay's extractHrefs: every href value, in order.
var tailwindHref = regexp.MustCompile(`(?i)href\s*=\s*["']([^"']+)["']`)

// tailwindNamePattern is tailwind-hyperclay's getTailwindName: a relative
// /tailwindcss/<name>.css or the same path on any origin, with an optional
// query or fragment.
var tailwindNamePattern = regexp.MustCompile(`^(?:https?://[^/]+)?/tailwindcss/([^?#]+)\.css(?:[?#].*)?$`)

// tailwindCSSName is tailwind-hyperclay's getTailwindCssName: the first Tailwind
// stylesheet the document links, or nothing when it links none.
func tailwindCSSName(html string) string {
	for _, match := range tailwindHref.FindAllStringSubmatch(html, -1) {
		if name := tailwindNamePattern.FindStringSubmatch(match[1]); name != nil {
			return name[1]
		}
	}
	return ""
}

// indefinite marks a failure after the bytes landed: Local sets
// `error.indeterminate = true` on these and the dispatcher reports them as
// indeterminate, never committed.
func indefinite(err error) error {
	return &indeterminateError{err: err, code: errnoCode(err)}
}

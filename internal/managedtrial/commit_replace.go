//go:build managedtrial

package managedtrial

import (
	"errors"
	"os"

	"github.com/panphora/htmlclay/internal/specwire"
)

// Local's `replace` lane: the bytes of a stored version are canonicalized,
// versioned as a plain backup, split, and written. A restore the data-loss guard
// makes to repair lost data arrives as `guard-restore` and is the same write.
//
// AD1: Local runs formatHtml and scopeTailwindLink over these bytes here. The
// trial announces neither capability, so the bytes reach disk as they came,
// exactly as the save lane stores them.

// currentLogical is the logical document already on disk: Local's
// `logicalBytes(baseDir, name, await fs.readFile(filePath))`. A missing file is
// nil, and any other failure is the error Local refuses the replace on.
func currentLogical(baseDir, name, filePath string) (*string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	logical, err := logicalBytes(baseDir, name, data)
	if err != nil {
		return nil, err
	}
	text := string(logical)
	return &text, nil
}

// commitReplace is Local's commitReplace.
func (h *commitHost) commitReplace(request *commitRequest, filePath string) (commitResult, error) {
	html := h.canonicalForWrite(stripSaveToken(string(request.Bytes)))
	backupName := backupSiteName(request.Name)
	fileContent, content, err := h.splitWrite(request.Name, html, true)
	if err != nil {
		return commitResult{}, err
	}

	if request.Safety {
		current, readErr := currentLogical(h.baseDir, request.Name, filePath)
		if readErr != nil {
			// Local refuses here when the live room cannot be read; without rooms
			// the only way to this branch is bytes this host cannot read, and a
			// safety backup of nothing is no safety at all.
			return refused("room-unavailable"), nil
		}
		if current != nil {
			// Without a live room there is no page to ask, so the safety copy is
			// the plain document Local writes for an `edit` restore.
			if CreateBackup(h.baseDir, backupName, []byte(*current), nil) == nil {
				return refused("backup-failed"), nil
			}
		}
	}

	CreateBackup(h.baseDir, backupName, []byte(fileContent), nil)

	stored, err := h.placeBytes(request.Name, filePath, content)
	if err != nil {
		return commitResult{}, err
	}

	// Morph view-mode tabs with the persisted on-disk HTML.
	h.hooks.Broadcast(request.Name, liveMessage{HTML: fileContent, Sender: "server-save"}, broadcastOptions{Lane: "saved"})

	// The API data sidecar, then the Tailwind compile: both non-fatal, because
	// the bytes are already written, backed up and broadcast.
	writeAPISidecar(h.baseDir, request.Name, content)
	if tailwindCSSName(content) != "" {
		_ = h.hooks.RefreshTailwind(content)
	}

	return commitResult{
		Outcome: "committed",
		Etag:    specwire.Etag([]byte(content)),
		Stored:  stored,
		Content: content,
	}, nil
}

// committedRebase is Local's answer for the `external-rebase` lane: committed,
// with a null checksum and the bytes it was handed as the content.
func committedRebase(bytes []byte, stored string) commitResult {
	return commitResult{
		Outcome:     "committed",
		Etag:        specwire.Etag(bytes),
		Stored:      stored,
		Content:     string(bytes),
		HasChecksum: true,
	}
}

// commitRebase is Local's commitRebase: an outside edit, once the watcher has
// decided the page it touched must be rebased onto it.
//
// AD2 note: with a local authority Local recovers the room's unsaved edits and
// confirms the write through it first. There are no rooms here, so the write is
// the backup, the page state and the bytes.
func (h *commitHost) commitRebase(request *commitRequest, filePath string) (commitResult, error) {
	html := string(request.Bytes)
	fileContent, _ := splitPage(html)
	CreateBackup(h.baseDir, backupSiteName(request.Name), []byte(fileContent), nil)

	stored, err := storePageState(h.baseDir, request.Name, html, false)
	if err != nil {
		return commitResult{}, err
	}
	h.hooks.MarkPending(h.baseDir, request.Name, stored)
	if err := atomicWriteFile(filePath, []byte(stored), nil, false); err != nil {
		return commitResult{}, err
	}
	return committedRebase(request.Bytes, stored), nil
}

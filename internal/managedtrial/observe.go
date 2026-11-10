//go:build managedtrial

package managedtrial

import "log"

// observeOutside tells the service what the watcher saw happen to a page
// document: Local's observer, run as a consultation under the document's lock.
// A rebase answer is written through the external-rebase lane, against the
// checksum the service judged, so an edit that landed meanwhile refuses
// `changed` and is observed on its own. Failures are logged: the watcher has
// no caller to answer.
func (h *commitHost) observeOutside(event, rel string) {
	if h.service == nil || !isDocumentName(rel) {
		return
	}
	filePath, err := h.resolveWrite(rel)
	if err != nil {
		log.Printf("observe %s: %v", rel, err)
		return
	}
	unlock := h.locks.lock(filePath)
	defer unlock()
	if h.service.barred(filePath) {
		// The service reconciles an unconfirmed document from a fresh read.
		return
	}
	current, err := readSnapshot(h.baseDir, rel, filePath)
	if err != nil {
		log.Printf("observe %s: %v", rel, err)
		return
	}
	consult := h.service.begin(newOpID(), nil, h.rootID, h.baseDir, rel, filePath, "observe", "external")
	answer, err := consult.observe(event, current)
	if err != nil {
		consult.end(err)
		log.Printf("observe %s: %v", rel, err)
		return
	}
	if answer.Action != "rebase" {
		return
	}
	logical, _, err := answer.Logical.decode()
	if err != nil {
		log.Printf("observe %s: %v", rel, err)
		return
	}
	checksum, _ := snapshotField(current, "checksum").(string)
	request := &commitRequest{
		Kind: "external-rebase", Name: rel, Bytes: logical, Text: true,
		Expected: &commitExpectation{Type: "checksum", Value: checksum},
	}
	if _, err := h.commitLocked(request, commitKinds["external-rebase"], filePath); err != nil {
		log.Printf("observe %s: rebase: %v", rel, err)
	}
}

// snapshotField is one field of a snapshot, or nil.
func snapshotField(snapshot orderedObject, key string) any {
	for _, field := range snapshot {
		if field.Key == key {
			return field.Value
		}
	}
	return nil
}

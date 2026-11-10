//go:build managedtrial

package managedtrial

import "sync"

// What each open file owes the platform on its next sync upload, keyed by
// filename within its own root's store. Ported from Local's
// src/main/utils/snapshot-store.js: two lanes fill it, /live-sync/save
// contributes the unstripped snapshot and /save contributes the provenance bit,
// so both halves are optional and neither lane clears the other's.

// etagRecord is the stamp this host last wrote to one canonical path, with who
// wrote it and the request's receipt.
type etagRecord struct {
	Etag    string
	MtimeNs *int64
	Writer  string
	SaveID  string
}

// snapshotEntry is what one document owes platform sync. HTML is nil when only
// the provenance half is known.
type snapshotEntry struct {
	HTML       *string
	UserDriven bool
	Timestamp  int64
}

// snapshotStore is one root's receipts store.
type snapshotStore struct {
	mu        sync.Mutex
	etags     map[string]etagRecord
	snapshots map[string]snapshotEntry
}

var (
	rootStoresMu sync.Mutex
	rootStores   = map[string]*snapshotStore{}
)

// snapshotStoreFor is the one store for a root id, created on first use.
func snapshotStoreFor(rootID string) *snapshotStore {
	rootStoresMu.Lock()
	defer rootStoresMu.Unlock()
	store, ok := rootStores[rootID]
	if !ok {
		store = &snapshotStore{etags: map[string]etagRecord{}, snapshots: map[string]snapshotEntry{}}
		rootStores[rootID] = store
	}
	return store
}

// setEtag records the stamp of the bytes this host just wrote to filePath.
func (s *snapshotStore) setEtag(filePath string, record etagRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.etags[filePath] = record
}

// etag is the record for one canonical path, if this host wrote it.
func (s *snapshotStore) etag(filePath string) (etagRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.etags[filePath]
	return record, ok
}

// saveReceipt is Local's saveReceiptFor: the id of the save whose body produced
// the bytes on disk right now, or nothing when this host cannot prove that
// pairing. The proof is answer-time and needs no invalidation hooks: the id was
// recorded beside the etag of the bytes that save wrote, so a stamp that no
// longer equals what is on disk means something else has written since.
func (s *snapshotStore) saveReceipt(filePath, currentEtag string) string {
	record, ok := s.etag(filePath)
	if !ok || record.SaveID == "" || record.Etag != currentEtag {
		return ""
	}
	return record.SaveID
}

// setSnapshot merges one lane's half of a document's sync entry.
func (s *snapshotStore) setSnapshot(name string, entry snapshotEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[name] = entry
}

// snapshot is one document's sync entry, if any lane wrote one.
func (s *snapshotStore) snapshot(name string) (snapshotEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.snapshots[name]
	return entry, ok
}

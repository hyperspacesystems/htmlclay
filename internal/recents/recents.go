// Package recents keeps HTML Clay's own list of recently opened documents. It is
// history, not capability: a row says a document was opened and never grants it
// anything, and nothing is reopened at start. The record lives in its own file in
// the config folder, never inside a person's document or folder, so no mark is ever
// written into their own files and an older build never rewrites it away.
package recents

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/panphora/htmlclay/internal/pathrel"
)

// fileName is the record, beside config.json in the config folder.
const fileName = "recents.json"

// recordCap bounds the record. Every open adds a row and only forgetting removes
// one, so the list has to have a ceiling.
const recordCap = 200

// The ways a document was opened.
const (
	// HowFile is a loose document opened from the app.
	HowFile = "file"
	// HowTrusted is a document that registered under a trusted folder.
	HowTrusted = "trusted"
	// HowServed is a served document that answered a real page load.
	HowServed = "served"
)

// idLength is the number of hex characters in an entry id.
const idLength = 16

// Entry is one recently opened document. ID is opaque and made once per document,
// so a row keeps its id when its tab closes. Path, Identity, FolderID and Key are
// what the document was when it was opened, so a moved or replaced document is
// still recognisable.
type Entry struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Identity string    `json:"identity,omitempty"`
	FolderID string    `json:"folderID,omitempty"`
	Key      string    `json:"key,omitempty"`
	OpenedAt time.Time `json:"openedAt"`
	// How is HowFile, HowTrusted or HowServed.
	How string `json:"how,omitempty"`
}

type recordFile struct {
	Entries []Entry `json:"entries"`
}

// Store is the recents record. Every method is safe to call from any goroutine,
// and every change is written before it is visible, so a failed write leaves both
// the old file and the old list.
type Store struct {
	mu      sync.Mutex
	path    string
	entries []Entry
}

// Open reads the record in dir, which is the config folder. A record that does not
// parse is moved aside the way a corrupt config is and the record starts empty, so
// a bad file never stops the app. Ids come from the file and are never made again
// at a restart.
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, fileName)}

	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		s.setAside(err)
		return s, nil
	}

	var file recordFile
	if err := json.Unmarshal(data, &file); err != nil {
		s.setAside(err)
		return s, nil
	}

	s.entries = sanitize(file.Entries)
	return s, nil
}

// setAside moves a record that cannot be used out of the way so it stays
// recoverable and the next write does not overwrite it.
func (s *Store) setAside(cause error) {
	quarantine := fmt.Sprintf("%s.corrupt-%d", s.path, time.Now().Unix())
	if err := os.Rename(s.path, quarantine); err != nil {
		fmt.Fprintf(os.Stderr, "[htmlclay] recents.json is unusable (%v) and could not be set aside (%v), starting empty\n", cause, err)
		return
	}
	fmt.Fprintf(os.Stderr, "[htmlclay] recents.json is unusable (%v); saved it as %s and starting empty\n", cause, quarantine)
}

// sanitize drops rows the rest of the app cannot use and caps the record. A row
// whose id is missing or malformed gets one, which is the only case where an id is
// made at open.
func sanitize(entries []Entry) []Entry {
	clean := make([]Entry, 0, len(entries))
	seenID := make(map[string]bool, len(entries))
	seenPath := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.Path == "" {
			continue
		}
		if seenID[entry.ID] && entry.ID != "" {
			continue
		}
		if seenPath[entry.Path] {
			continue
		}
		if !validID(entry.ID) {
			id, err := newID()
			if err != nil {
				continue
			}
			entry.ID = id
		}
		seenID[entry.ID] = true
		seenPath[entry.Path] = true
		clean = append(clean, entry)
		if len(clean) == recordCap {
			break
		}
	}
	return clean
}

// List returns a copy of the record, most recent first.
func (s *Store) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Entry(nil), s.entries...)
}

// Find reports the entry with this id.
func (s *Store) Find(id string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range s.entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return Entry{}, false
}

// Touch moves the entry for this document to the front, or adds it there when the
// record does not have it yet, and drops the rows past the cap. The id is the
// record's: a row already in the record keeps its id whatever the caller passes.
func (s *Store) Touch(entry Entry, now time.Time) error {
	if entry.Path == "" {
		return errors.New("recents entry has no path")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current := entry
	current.OpenedAt = now
	if current.How == "" {
		current.How = HowFile
	}

	kept := make([]Entry, 0, len(s.entries))
	for _, existing := range s.entries {
		match := existing.Path == entry.Path || (entry.ID != "" && existing.ID == entry.ID)
		if !match {
			kept = append(kept, existing)
			continue
		}
		current.ID = existing.ID
	}
	if !validID(current.ID) {
		id, err := newID()
		if err != nil {
			return err
		}
		current.ID = id
	}

	next := make([]Entry, 0, len(s.entries)+1)
	next = append(next, current)
	next = append(next, kept...)
	if len(next) > recordCap {
		next = next[:recordCap]
	}

	if err := writeRecord(s.path, next); err != nil {
		return err
	}
	s.entries = next
	return nil
}

// Forget removes the row for this id and nothing else: no file is deleted and no
// other record is touched. A forgotten document stays forgotten until its next
// explicit open.
func (s *Store) Forget(id string) error {
	if id == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := make([]Entry, 0, len(s.entries))
	found := false
	for _, entry := range s.entries {
		if entry.ID == id {
			found = true
			continue
		}
		next = append(next, entry)
	}
	if !found {
		return nil
	}

	if err := writeRecord(s.path, next); err != nil {
		return err
	}
	s.entries = next
	return nil
}

// Rebind moves the rows at oldPath, and the rows below it, onto newPath, so a
// document or a folder that was renamed or moved keeps its history. When two rows
// land on the same path the one nearer the front is kept.
func (s *Store) Rebind(oldPath, newPath string) error {
	if oldPath == "" || newPath == "" || oldPath == newPath {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := make([]Entry, 0, len(s.entries))
	seenPath := make(map[string]bool, len(s.entries))
	changed := false
	for _, entry := range s.entries {
		switch {
		case entry.Path == oldPath:
			entry.Path = newPath
			changed = true
		case pathrel.Within(entry.Path, oldPath):
			rel := entry.Path[len(oldPath):]
			entry.Path = filepath.Join(newPath, rel)
			changed = true
		}
		if seenPath[entry.Path] {
			changed = true
			continue
		}
		seenPath[entry.Path] = true
		next = append(next, entry)
	}
	if !changed {
		return nil
	}

	if err := writeRecord(s.path, next); err != nil {
		return err
	}
	s.entries = next
	return nil
}

// writeRecord replaces the record with entries through a temporary file in the
// same folder and a rename, the way config.json's own save does, so a write that
// fails partway leaves the old file exactly as it was.
func writeRecord(path string, entries []Entry) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if entries == nil {
		entries = []Entry{}
	}
	data, err := json.MarshalIndent(recordFile{Entries: entries}, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".recents-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// newID mints an entry id: 8 random bytes as 16 hex characters.
func newID() (string, error) {
	b := make([]byte, idLength/2)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// validID reports whether id has the shape this package makes.
func validID(id string) bool {
	if len(id) != idLength {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

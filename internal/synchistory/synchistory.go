// Package synchistory keeps each synced folder's history: what sync uploaded,
// downloaded, renamed, removed, held as a conflict or settled, one JSON line per
// entry in <config>/sync-history/<folder id>.jsonl, oldest first. Only the
// newest Keep lines stay. An entry whose id the file already holds is ignored,
// so a report the service sends again adds nothing.
package synchistory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// Keep is how many lines each folder's file holds.
const Keep = 5000

// Entry kinds: the Mac app's four (HistoryEntry.Kind) plus removed and conflict.
const (
	Uploaded   = "uploaded"
	Downloaded = "downloaded"
	Renamed    = "renamed"
	Removed    = "removed"
	Conflict   = "conflict"
	Settled    = "settled"
)

type Entry struct {
	ID     string `json:"id"`
	At     string `json:"at"`
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Detail string `json:"detail"`
}

var folderID = regexp.MustCompile(`^[0-9a-f]{16}$`)

// ErrFolderID is a folder id that is not one the app makes.
var ErrFolderID = errors.New("synchistory: not a folder id")

// Page is one page of a folder's history as the dashboard and the Mac app read
// it: newest first, and the cursor for the next page or null.
type Page struct {
	Entries []Entry `json:"entries"`
	Next    *string `json:"next"`
}

// Store is the only writer of its files, so it keeps each folder's entries in
// memory after the first read.
type Store struct {
	dir   string
	keep  int
	mu    sync.Mutex
	cache map[string][]Entry
}

// New keeps history in dir, which is made on the first append.
func New(dir string) *Store { return &Store{dir: dir, keep: Keep, cache: map[string][]Entry{}} }

// entriesLocked answers the folder's entries, oldest first, from memory or the file.
func (s *Store) entriesLocked(folder, file string) ([]Entry, error) {
	if kept, ok := s.cache[folder]; ok {
		return kept, nil
	}
	kept, err := s.read(file)
	if err != nil {
		return nil, err
	}
	s.cache[folder] = kept
	return kept, nil
}

func (s *Store) file(folder string) (string, error) {
	if !folderID.MatchString(folder) {
		return "", fmt.Errorf("%w: %q", ErrFolderID, folder)
	}
	return filepath.Join(s.dir, folder+".jsonl"), nil
}

// read answers the folder's entries, oldest first. A line that does not read is
// skipped; a missing file is no entries.
func (s *Store) read(file string) ([]Entry, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []Entry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		var e Entry
		if json.Unmarshal(scanner.Bytes(), &e) == nil && e.ID != "" {
			entries = append(entries, e)
		}
	}
	return entries, scanner.Err()
}

// Append adds the entries the folder's file does not hold yet, keeps the newest
// Keep lines, and answers how many it added.
func (s *Store) Append(folder string, entries ...Entry) (int, error) {
	file, err := s.file(folder)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	kept, err := s.entriesLocked(folder, file)
	if err != nil {
		return 0, err
	}
	kept = append([]Entry(nil), kept...)
	seen := make(map[string]bool, len(kept)+len(entries))
	for _, e := range kept {
		seen[e.ID] = true
	}
	added := 0
	for _, e := range entries {
		if e.ID == "" || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		kept = append(kept, e)
		added++
	}
	if added == 0 {
		return 0, nil
	}
	if len(kept) > s.keep {
		kept = kept[len(kept)-s.keep:]
	}
	var buf bytes.Buffer
	for _, e := range kept {
		line, err := json.Marshal(e)
		if err != nil {
			return 0, err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(s.dir, "."+folder+"-*.tmp")
	if err != nil {
		return 0, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmp.Name(), file); err != nil {
		return 0, err
	}
	s.cache[folder] = kept
	return added, nil
}

// Page answers up to limit entries, newest first, older than the entry before
// names (from the newest when before is empty), and the cursor for the next
// page, or "" when no older entry is kept. A cursor no longer kept answers an
// empty page.
func (s *Store) Page(folder, before string, limit int) ([]Entry, string, error) {
	file, err := s.file(folder)
	if err != nil {
		return nil, "", err
	}
	s.mu.Lock()
	kept, err := s.entriesLocked(folder, file)
	s.mu.Unlock()
	if err != nil {
		return nil, "", err
	}
	end := len(kept)
	if before != "" {
		end = -1
		for i, e := range kept {
			if e.ID == before {
				end = i
			}
		}
		if end < 0 {
			return []Entry{}, "", nil
		}
	}
	if limit <= 0 {
		limit = 1
	}
	start := max(end-limit, 0)
	page := make([]Entry, 0, end-start)
	for i := end - 1; i >= start; i-- {
		page = append(page, kept[i])
	}
	next := ""
	if start > 0 {
		next = kept[start].ID
	}
	return page, next, nil
}

// Today counts the folder's uploads and downloads on now's calendar day, in
// now's time zone. A folder with no history, or one that does not read, is 0.
func (s *Store) Today(folder string, now time.Time) (uploaded, downloaded int) {
	file, err := s.file(folder)
	if err != nil {
		return 0, 0
	}
	s.mu.Lock()
	kept, err := s.entriesLocked(folder, file)
	s.mu.Unlock()
	if err != nil {
		return 0, 0
	}
	y, m, d := now.Date()
	for _, e := range kept {
		at, err := time.Parse(time.RFC3339Nano, e.At)
		if err != nil {
			continue
		}
		if ey, em, ed := at.In(now.Location()).Date(); ey != y || em != m || ed != d {
			continue
		}
		switch e.Kind {
		case Uploaded:
			uploaded++
		case Downloaded:
			downloaded++
		}
	}
	return uploaded, downloaded
}

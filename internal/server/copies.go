package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/versions"
)

// Recovery copies (plan 09 H-r3, the copy wire in plan-v2/c9a-clayjs-live-room.md): a live-room
// tab stores the edits it could not save as a copy page, so they outlive the window that made them.
// Copies sit beside the document's versions, in the same private per-document folder, and only the
// document's token routes reach them. A copy is written once and never overwritten.
const (
	copyLimit    = 6 << 20
	docCopyLimit = 32 << 20
	copiesDir    = "copies"
	copiesIndex  = "index.json"
)

var copyIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

type copyMark struct {
	E    json.RawMessage `json:"e"`
	Kind string          `json:"kind"`
	At   int64           `json:"at"`
}

type copyEntry struct {
	ID   string     `json:"id"`
	Time int64      `json:"time"`
	Size int64      `json:"size"`
	Done []copyMark `json:"done"`
}

func writeCopyJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func copyError(w http.ResponseWriter, status int, msg string) {
	writeCopyJSON(w, status, map[string]string{"error": msg})
}

// copyKey resolves the document's history key through the one seam allowed to make it, so a copy
// request arriving before the page is served again (after an update restart) never pins a path key
// on a .htmlclay sheet whose versions are kept by id. Caller holds f.Lock.
func (s *Server) copyKey(f *session.File) string {
	if key := f.HistoryKey(); key != "" {
		return key
	}
	data, _ := os.ReadFile(f.AbsPath)
	key, _ := s.ensureHistoryKeyLocked(f, data)
	return key
}

// copiesRoot opens the document's copies folder; create makes it (and the versions folder) when
// missing. A stored copy makes the document's history durable, or the sweep of provisional
// histories would delete the copy with it.
func (s *Server) copiesRoot(key, absPath string, create bool) (*os.Root, error) {
	var folder string
	var err error
	if create {
		folder, err = s.versions.Folder(key, absPath)
	} else {
		folder, err = s.versions.ExistingFolder(key, absPath)
	}
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(folder)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if create {
		if err := root.Mkdir(copiesDir, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	return root.OpenRoot(copiesDir)
}

// copyMeta is what the index keeps per copy: when it was stored and its decision marks. Which
// copies exist is the set of <id>.html files, so a crash between writing a copy and its index entry
// never hides a copy.
type copyMeta struct {
	Time int64      `json:"time"`
	Done []copyMark `json:"done"`
}

// readCopies lists the stored copies from their files, with their index entries. An index that
// cannot be read (a torn write) is logged and read as no marks, so one bad write never refuses every
// later copy of the document.
func (s *Server) readCopies(root *os.Root, relPath string) (map[string]*copyEntry, error) {
	meta := map[string]copyMeta{}
	if data, err := root.ReadFile(copiesIndex); err == nil {
		if jErr := json.Unmarshal(data, &meta); jErr != nil {
			s.Logger.Printf("The recovery copy index of %s is unreadable, reading it as no marks: %v", relPath, jErr)
			meta = map[string]copyMeta{}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	dirents, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	copies := map[string]*copyEntry{}
	for _, d := range dirents {
		id, ok := strings.CutSuffix(d.Name(), ".html")
		if !ok || !copyIDPattern.MatchString(id) || !d.Type().IsRegular() {
			continue
		}
		info, err := d.Info()
		if err != nil {
			continue
		}
		m, known := meta[id]
		if !known {
			m.Time = info.ModTime().UnixMilli()
		}
		if m.Done == nil {
			m.Done = []copyMark{}
		}
		copies[id] = &copyEntry{ID: id, Time: m.Time, Size: info.Size(), Done: m.Done}
	}
	return copies, nil
}

func writeCopiesIndex(root *os.Root, copies map[string]*copyEntry) error {
	meta := make(map[string]copyMeta, len(copies))
	for id, e := range copies {
		meta[id] = copyMeta{Time: e.Time, Done: e.Done}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(meta); err != nil {
		return err
	}
	return versions.PublishBytes(root, copiesIndex, buf.Bytes())
}

func sortedCopies(copies map[string]*copyEntry) []*copyEntry {
	list := make([]*copyEntry, 0, len(copies))
	for _, e := range copies {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Time != list[j].Time {
			return list[i].Time < list[j].Time
		}
		return list[i].ID < list[j].ID
	})
	return list
}

// handleCopies serves GET /_/copies/{token}: every copy of the document, oldest first.
func (s *Server) handleCopies(w http.ResponseWriter, r *http.Request) {
	f, ok := s.lookupSession(w, r)
	if !ok {
		return
	}
	f.Lock()
	defer f.Unlock()
	list := []*copyEntry{}
	root, err := s.copiesRoot(s.copyKey(f), f.AbsPath, false)
	if err == nil {
		defer root.Close()
		copies, rErr := s.readCopies(root, f.RelPath)
		if rErr != nil {
			s.Logger.Printf("Could not read the recovery copies of %s: %v", f.RelPath, rErr)
			copyError(w, http.StatusInternalServerError, "could not read the copies")
			return
		}
		list = sortedCopies(copies)
	} else if !errors.Is(err, versions.ErrNoHistory) && !errors.Is(err, fs.ErrNotExist) {
		s.Logger.Printf("Could not open the recovery copies of %s: %v", f.RelPath, err)
		copyError(w, http.StatusInternalServerError, "could not read the copies")
		return
	}
	writeCopyJSON(w, http.StatusOK, map[string]any{"copies": list})
}

// handleCopy serves GET and PUT /_/copies/{token}/{id}. A PUT creates the copy once: the same bytes
// again answer 200, different bytes 409, and nothing is ever overwritten. A GET answers what is
// stored, never the request just taken.
func (s *Server) handleCopy(w http.ResponseWriter, r *http.Request) {
	f, ok := s.lookupSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !copyIDPattern.MatchString(id) {
		copyError(w, http.StatusBadRequest, "a copy id is letters, digits, _ and -")
		return
	}
	name := id + ".html"

	if r.Method == http.MethodGet {
		f.Lock()
		defer f.Unlock()
		root, err := s.copiesRoot(s.copyKey(f), f.AbsPath, false)
		if err != nil {
			if !errors.Is(err, versions.ErrNoHistory) && !errors.Is(err, fs.ErrNotExist) {
				s.Logger.Printf("Could not open the recovery copies of %s: %v", f.RelPath, err)
				copyError(w, http.StatusInternalServerError, "could not read the copy")
				return
			}
			copyError(w, http.StatusNotFound, "no such copy")
			return
		}
		defer root.Close()
		data, err := root.ReadFile(name)
		if errors.Is(err, fs.ErrNotExist) {
			copyError(w, http.StatusNotFound, "no such copy")
			return
		}
		if err != nil {
			s.Logger.Printf("Could not read recovery copy %s of %s: %v", id, f.RelPath, err)
			copyError(w, http.StatusInternalServerError, "could not read the copy")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(data)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, copyLimit+1))
	if err != nil {
		copyError(w, http.StatusInternalServerError, "could not read the copy")
		return
	}
	if len(body) > copyLimit {
		copyError(w, http.StatusRequestEntityTooLarge, "the copy does not fit")
		return
	}

	f.Lock()
	defer f.Unlock()
	key := s.copyKey(f)
	root, err := s.copiesRoot(key, f.AbsPath, true)
	if err != nil {
		s.Logger.Printf("Could not open the recovery copies of %s: %v", f.RelPath, err)
		copyError(w, http.StatusInternalServerError, "could not store the copy")
		return
	}
	defer root.Close()
	copies, err := s.readCopies(root, f.RelPath)
	if err != nil {
		s.Logger.Printf("Could not read the recovery copies of %s: %v", f.RelPath, err)
		copyError(w, http.StatusInternalServerError, "could not store the copy")
		return
	}
	stored, rErr := root.ReadFile(name)
	if rErr == nil {
		if bytes.Equal(stored, body) {
			writeCopyJSON(w, http.StatusOK, map[string]any{"id": id, "size": len(stored)})
		} else {
			copyError(w, http.StatusConflict, "a different copy is stored under this id")
		}
		return
	}
	if !errors.Is(rErr, fs.ErrNotExist) {
		s.Logger.Printf("Could not read recovery copy %s of %s: %v", id, f.RelPath, rErr)
		copyError(w, http.StatusInternalServerError, "could not store the copy")
		return
	}
	var used int64
	for _, e := range copies {
		used += e.Size
	}
	if used+int64(len(body)) > docCopyLimit {
		copyError(w, http.StatusRequestEntityTooLarge, "the copy does not fit")
		return
	}
	out, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	created := err == nil
	if created {
		_, err = out.Write(body)
		if sErr := out.Sync(); err == nil {
			err = sErr
		}
		if cErr := out.Close(); err == nil {
			err = cErr
		}
		if err == nil {
			err = versions.SyncRoot(root)
		}
	}
	if err == nil {
		copies[id] = &copyEntry{ID: id, Time: time.Now().UnixMilli(), Size: int64(len(body)), Done: []copyMark{}}
		if iErr := writeCopiesIndex(root, copies); iErr != nil {
			s.Logger.Printf("Stored recovery copy %s of %s without its index entry: %v", id, f.RelPath, iErr)
		}
	}
	if err != nil {
		if created {
			root.Remove(name)
		}
		s.Logger.Printf("Could not store recovery copy %s of %s: %v", id, f.RelPath, err)
		copyError(w, http.StatusInternalServerError, "could not store the copy")
		return
	}
	if pErr := s.versions.SetProvisional(key, f.AbsPath, false); pErr != nil {
		s.Logger.Printf("Could not keep the history of %s for its recovery copy: %v", f.RelPath, pErr)
	}
	writeCopyJSON(w, http.StatusCreated, map[string]any{"id": id, "size": len(body)})
}

// handleCopyDone serves POST /_/copies/{token}/{id}/done: one decision mark { e, kind }, appended
// unless the same e and kind are already there. It answers the whole list.
func (s *Server) handleCopyDone(w http.ResponseWriter, r *http.Request) {
	f, ok := s.lookupSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !copyIDPattern.MatchString(id) {
		copyError(w, http.StatusBadRequest, "a copy id is letters, digits, _ and -")
		return
	}
	var mark copyMark
	data, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err == nil {
		err = json.Unmarshal(data, &mark)
	}
	var e string
	if err != nil || json.Unmarshal(mark.E, &e) != nil || e == "" || mark.Kind == "" {
		copyError(w, http.StatusBadRequest, "a mark is { e, kind }, with e an edit id")
		return
	}

	f.Lock()
	defer f.Unlock()
	root, err := s.copiesRoot(s.copyKey(f), f.AbsPath, false)
	if err != nil {
		// A client treats 404 as final, so only a missing history or folder may answer it.
		if !errors.Is(err, versions.ErrNoHistory) && !errors.Is(err, fs.ErrNotExist) {
			s.Logger.Printf("Could not open the recovery copies of %s: %v", f.RelPath, err)
			copyError(w, http.StatusInternalServerError, "could not record the decision")
			return
		}
		copyError(w, http.StatusNotFound, "no such copy")
		return
	}
	defer root.Close()
	copies, err := s.readCopies(root, f.RelPath)
	if err != nil {
		copyError(w, http.StatusInternalServerError, "could not read the copies")
		return
	}
	entry, ok := copies[id]
	if !ok {
		copyError(w, http.StatusNotFound, "no such copy")
		return
	}
	for _, m := range entry.Done {
		var seen string
		if m.Kind == mark.Kind && json.Unmarshal(m.E, &seen) == nil && seen == e {
			writeCopyJSON(w, http.StatusOK, map[string]any{"id": id, "done": entry.Done})
			return
		}
	}
	mark.At = time.Now().UnixMilli()
	mark.E, _ = json.Marshal(e)
	entry.Done = append(entry.Done, mark)
	if err := writeCopiesIndex(root, copies); err != nil {
		s.Logger.Printf("Could not record a decision on recovery copy %s of %s: %v", id, f.RelPath, err)
		copyError(w, http.StatusInternalServerError, "could not record the decision")
		return
	}
	writeCopyJSON(w, http.StatusOK, map[string]any{"id": id, "done": entry.Done})
}

package main

import (
	"errors"
	"io/fs"
	"os"
	"time"
)

// historyPageSize is how many versions one page of history holds.
const historyPageSize = 50

// historyItem is one kept version of a document, in the one shape the app and
// the dashboard read whether the document is loose or served.
type historyItem struct {
	ID         string `json:"id"`
	CreatedAt  string `json:"createdAt"`
	Size       int64  `json:"size"`
	Kind       string `json:"kind"`
	Pinned     bool   `json:"pinned"`
	Restorable bool   `json:"restorable"`
}

// historyDocument names the document whose kept versions are read: a served
// document when RootID is set, with Path as its path under that root, and a
// loose file otherwise, with Path as its absolute path.
type historyDocument struct {
	RootID string
	Path   string
}

// readHistory returns one page of doc's kept versions, newest first, starting
// after the before cursor. It only reads: no version, history folder, pin or
// document is created, and a document that is gone still answers its history.
func (a *app) readHistory(doc historyDocument, before string) ([]historyItem, error) {
	return a.readHistoryPage(doc, before, historyPageSize)
}

// countHistory is how many versions doc has kept across both stores, read the one
// way readHistory reads them. A negative limit asks for the whole history.
func (a *app) countHistory(doc historyDocument) (int, error) {
	items, err := a.readHistoryPage(doc, "", -1)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

// readHistoryPage is readHistory with the page size a caller wants.
func (a *app) readHistoryPage(doc historyDocument, before string, limit int) ([]historyItem, error) {
	if doc.RootID != "" {
		return a.readServedHistory(doc.RootID, doc.Path, before, limit)
	}
	return a.readLooseHistory(doc.Path, before, limit)
}

// readLooseHistory reads a loose document's history from the version store. The
// document's bytes only name its history: the store keeps the same one after the
// file is gone, and a read never writes the file back.
func (a *app) readLooseHistory(absPath, before string, limit int) ([]historyItem, error) {
	if a.rt.versions == nil {
		return nil, errors.New("no version store")
	}
	present := true
	data, err := os.ReadFile(absPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		present = false
		data = nil
	}
	entries, err := a.rt.versions.Page(absPath, data, before, limit)
	if err != nil {
		return nil, err
	}
	items := make([]historyItem, 0, len(entries))
	for _, entry := range entries {
		items = append(items, historyItem{
			ID:         entry.Name,
			CreatedAt:  historyInstant(entry.Time),
			Size:       entry.Size,
			Kind:       "saved",
			Pinned:     entry.Pinned,
			Restorable: present,
		})
	}
	return items, nil
}

// readServedHistory reads a served document's history from the folder's own
// store, in the same shape readLooseHistory answers.
func (a *app) readServedHistory(rootID, rel, before string, limit int) ([]historyItem, error) {
	if a.served == nil {
		return nil, errors.New("no served folders")
	}
	rows, err := a.served.Versions(rootID, rel, before, limit)
	if err != nil {
		return nil, err
	}
	items := make([]historyItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, historyItem{
			ID:         row.ID,
			CreatedAt:  historyInstant(row.CreatedAt),
			Size:       row.Size,
			Kind:       row.Kind,
			Pinned:     row.Pinned,
			Restorable: row.Restorable,
		})
	}
	return items, nil
}

// historyInstant is createdAt in the one form both stores answer: UTC, to the
// millisecond, the way the served version routes already write a time.
func historyInstant(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

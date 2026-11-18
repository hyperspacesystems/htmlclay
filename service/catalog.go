// Package servicefiles is the live-room service's starter templates as the Go
// host reads them. service/local/src/main/document-templates/ stays the one
// source: the service reads it at run time, and this package embeds the same
// files at build time, so both come from the same commit.
package servicefiles

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"sync"
)

//go:embed local/src/main/document-templates/manifest.json
//go:embed local/src/main/document-templates/thumbnails/*.svg
//go:embed local/src/main/document-templates/blank.html
//go:embed local/src/main/document-templates/kanban.html
//go:embed local/src/main/document-templates/journal.html
//go:embed local/src/main/document-templates/freelance-rate-calculator.html
var files embed.FS

const root = "local/src/main/document-templates/"

// singleFile is the templates that work as one document outside a served
// folder. The block editor saves through the served-folder code, so it is not
// one. Each template listed here is embedded above.
var singleFile = map[string]bool{
	"blank":                     true,
	"kanban":                    true,
	"journal":                   true,
	"freelance-rate-calculator": true,
}

// Card is one template card, in the service's bootstrap shape plus SingleFile.
type Card struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	SizeBytes     int64  `json:"sizeBytes"`
	StartsPrivate bool   `json:"startsPrivate"`
	Thumbnail     string `json:"thumbnail"`
	SingleFile    bool   `json:"singleFile"`
}

type entry struct {
	ID            string `json:"id"`
	File          string `json:"file"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Thumbnail     string `json:"thumbnail"`
	Listed        bool   `json:"listed"`
	StartsPrivate bool   `json:"startsPrivate"`
	Bytes         int64  `json:"bytes"`
	SHA256        string `json:"sha256"`
}

var manifest = sync.OnceValue(func() []entry {
	data, err := files.ReadFile(root + "manifest.json")
	if err != nil {
		panic(err)
	}
	var all []entry
	if err := json.Unmarshal(data, &all); err != nil {
		panic(err)
	}
	return all
})

func listed(id string) (entry, bool) {
	for _, e := range manifest() {
		if e.ID == id && e.Listed {
			return e, true
		}
	}
	return entry{}, false
}

// Cards is the listed templates in the manifest's order.
func Cards() []Card {
	cards := make([]Card, 0, len(manifest()))
	for _, e := range manifest() {
		if !e.Listed {
			continue
		}
		cards = append(cards, Card{
			ID: e.ID, Name: e.Name, Description: e.Description, SizeBytes: e.Bytes,
			StartsPrivate: e.StartsPrivate, Thumbnail: "/templates/" + e.ID + ".svg", SingleFile: singleFile[e.ID],
		})
	}
	return cards
}

// Picture is a listed template's preview image, looked up by its manifest id.
func Picture(id string) ([]byte, bool) {
	e, ok := listed(id)
	if !ok || e.Thumbnail == "" {
		return nil, false
	}
	data, err := files.ReadFile(root + e.Thumbnail)
	return data, err == nil
}

// Template is a listed single-file template's bytes, when they match the
// manifest's sha256.
func Template(id string) ([]byte, bool) {
	e, ok := listed(id)
	if !ok || !singleFile[id] {
		return nil, false
	}
	data, err := files.ReadFile(root + e.File)
	if err != nil {
		return nil, false
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != e.SHA256 {
		return nil, false
	}
	return data, true
}

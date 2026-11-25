package servicefiles

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func TestCatalogIsTheServiceTreesListedTemplates(t *testing.T) {
	cards := Cards()
	var ids, single []string
	for _, c := range cards {
		ids = append(ids, c.ID)
		if c.SingleFile {
			single = append(single, c.ID)
		}
		if c.Thumbnail != "/templates/"+c.ID+".svg" {
			t.Errorf("%s: thumbnail %q", c.ID, c.Thumbnail)
		}
	}
	if want := []string{"blank", "kanban", "journal", "freelance-rate-calculator", "blockeditor"}; !slices.Equal(ids, want) {
		t.Fatalf("cards = %v, want the manifest's listed templates in order %v", ids, want)
	}
	if want := []string{"blank", "kanban", "journal", "freelance-rate-calculator"}; !slices.Equal(single, want) {
		t.Fatalf("single-file cards = %v, want %v (the block editor saves through a served folder)", single, want)
	}

	for _, c := range cards {
		picture, ok := Picture(c.ID)
		onDisk, err := os.ReadFile("local/src/main/document-templates/thumbnails/" + c.ID + ".svg")
		if !ok || err != nil || !bytes.Equal(picture, onDisk) {
			t.Fatalf("%s: the embedded picture differs from the service's file (%v)", c.ID, err)
		}
		template, ok := Template(c.ID)
		if !c.SingleFile {
			if ok {
				t.Fatalf("%s is no single-file template, yet Template answered it", c.ID)
			}
			continue
		}
		onDisk, err = os.ReadFile("local/src/main/document-templates/" + c.ID + ".html")
		if !ok || err != nil || !bytes.Equal(template, onDisk) {
			t.Fatalf("%s: a card Home offers as a single file has no template the host can write (%v)", c.ID, err)
		}
	}

	for _, id := range []string{"../manifest", "thumbnails/kanban", "devlog", "landing", "nope", ""} {
		if _, ok := Picture(id); ok {
			t.Errorf("Picture(%q) answered an id that is no listed template", id)
		}
		if _, ok := Template(id); ok {
			t.Errorf("Template(%q) answered an id that is no listed template", id)
		}
	}
}

func TestOfflineRuntimeMatchesGoLock(t *testing.T) {
	type lock struct {
		Selected string   `json:"selected"`
		Versions []string `json:"versions"`
	}
	read := func(path string) lock {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var out lock
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		return out
	}
	goLock := read("../offline-runtime.lock.json")
	svcLock := read("local/src/main/offline-runtime/manifest.json")
	if goLock.Selected != svcLock.Selected || !slices.Equal(goLock.Versions, svcLock.Versions) {
		t.Fatalf("service offline-runtime (%+v) does not match offline-runtime.lock.json (%+v)", svcLock, goLock)
	}
}

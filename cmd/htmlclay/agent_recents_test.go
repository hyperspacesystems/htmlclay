package main

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
)

// An agent's read and save of a trusted folder's document the person never
// opened leaves the Recent list alone.
func TestM3AgentSaveLeavesRecent(t *testing.T) {
	f := newAgentLaneFixture(t)
	a := f.app
	store := a.recentsStore()
	if store == nil {
		t.Fatal("no recents store")
	}
	before := len(store.List())
	trustedDir := filepath.Dir(f.trusted)
	var paths []string
	for i := 0; i < 5; i++ {
		p := filepath.Join(trustedDir, fmt.Sprintf("bulk-%d.html", i))
		writeTestFile(t, p, documentAccessDocument)
		paths = append(paths, p)
	}
	for i, p := range paths {
		doc, rec := agentLaneGet(t, a, p)
		if rec.Code != http.StatusOK {
			t.Fatalf("read %s = %d %s", p, rec.Code, rec.Body.String())
		}
		html := doc.HTML + fmt.Sprintf("<!-- edit %d -->", i)
		rec = agentLaneSave(t, a, p, html, fmt.Sprintf("op-%d", i), doc.ETag)
		if rec.Code != http.StatusOK {
			t.Fatalf("save %s = %d %s", p, rec.Code, rec.Body.String())
		}
	}
	after := store.List()
	recorded := 0
	for _, e := range after {
		for _, p := range paths {
			if e.Path == p || filepath.Base(e.Path) == filepath.Base(p) {
				recorded++
			}
		}
	}
	if recorded > 0 || len(after) != before {
		t.Errorf("agent saves added %d Recent rows (before %d, after %d)", recorded, before, len(after))
	}
}

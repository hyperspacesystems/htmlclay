package dashboard

import (
	"net/http"
	"strings"
	"testing"
)

// TestM3PagePath is step 7.24: the pages the dashboard's own Go routes read are
// admitted, and an address that is not one of them still is not.
func TestM3PagePath(t *testing.T) {
	s, _ := newTestServer(t, nil)
	c := enterWith(t, s, "/")
	cookie := map[string]string{"Cookie": c.Name + "=" + c.Value}

	t.Run("the new pages are admitted", func(t *testing.T) {
		for _, to := range []string{
			"/recent",
			"/needs-you",
			"/recent/d0123456789abcd/versions",
			"/trusted/d0123456789abcd/files",
			"/settings",
			"/settings/general",
			"/settings/files",
			"/settings/folders",
			"/settings/folders/d0123456789abcd",
			"/settings/programs",
			"/settings/programs/p0123456789abcd",
			"/settings/programs/file-search",
			"/settings/updates",
			"/settings/advanced",
			"/settings/sync",
			"/panel",
		} {
			entry, err := s.EntryURL(to)
			if err != nil {
				t.Fatalf("EntryURL(%q): %v", to, err)
			}
			w := do(s, http.MethodGet, pathOf(t, entry), cookie)
			if w.Code != http.StatusFound {
				t.Fatalf("entering %q = %d", to, w.Code)
			}
			page := do(s, http.MethodGet, to, cookie)
			if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `<div id="workspace-root"></div>`) {
				t.Fatalf("the page %q = %d: %s", to, page.Code, page.Body.String())
			}
		}
	})

	t.Run("an address that is not a page is not admitted", func(t *testing.T) {
		for _, to := range []string{
			"/recent/",
			"/recent/only-an-id",
			"/recent/d0123456789abcd/versions/extra",
			"/recent/d0123456789abcd/versions/extra/more",
			"/trusted/d0123456789abcd",
			"/trusted/d0123456789abcd/files/extra",
			"/needs-you/x",
			"/settings/other",
			"/settings/name",
			"/settings/sync/x",
			"/settings/folders/a/b",
			"/settingsx",
			"/panel/x",
		} {
			if _, err := s.EntryURL(to); err == nil {
				t.Fatalf("EntryURL(%q) succeeded", to)
			}
			if w := do(s, http.MethodGet, to, cookie); w.Code != http.StatusNotFound {
				t.Fatalf("%q = %d, want 404", to, w.Code)
			}
		}
	})

	t.Run("a page with a query opens with its query", func(t *testing.T) {
		to := "/recent/d0123456789abcd/versions?before=2026-01-02-03-04-05-006Z.html"
		entry, err := s.EntryURL(to)
		if err != nil {
			t.Fatal(err)
		}
		w := do(s, http.MethodGet, pathOf(t, entry), nil)
		if w.Code != http.StatusFound || w.Header().Get("Location") != to {
			t.Fatalf("entering %q = %d to %q", to, w.Code, w.Header().Get("Location"))
		}
	})
}

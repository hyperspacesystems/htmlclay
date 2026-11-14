package dashboard

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dashboard/assets"
)

// TestM3DashboardBundle is step 7.25: the build the dashboard serves is the
// workspace's new one, so its entry script and stylesheets are the files the
// entry page names, and the pages they render are admitted.
func TestM3DashboardBundle(t *testing.T) {
	lock, err := assets.LoadLock()
	if err != nil {
		t.Fatal(err)
	}
	var scripts, styles []string
	for name := range lock.Files {
		switch {
		case strings.HasPrefix(name, "main-") && strings.HasSuffix(name, ".js"):
			scripts = append(scripts, name)
		case strings.HasPrefix(name, "main-") && strings.HasSuffix(name, ".css"):
			styles = append(styles, name)
		}
	}
	sort.Strings(scripts)
	sort.Strings(styles)

	t.Run("the packaged entry loads", func(t *testing.T) {
		if len(scripts) != 1 || len(styles) != 1 {
			t.Fatalf("the lock names %v scripts and %v stylesheets, want one of each", scripts, styles)
		}
		s, _ := newTestServer(t, nil)
		c := enterWith(t, s, "/")
		entry := do(s, http.MethodGet, "/", map[string]string{"Cookie": c.Name + "=" + c.Value})
		if entry.Code != http.StatusOK {
			t.Fatalf("the entry page = %d", entry.Code)
		}
		body := entry.Body.String()
		for _, name := range []string{scripts[0], styles[0]} {
			if !strings.Contains(body, "/"+name) {
				t.Fatalf("the entry page does not name /%s", name)
			}
		}
		for _, want := range []struct {
			name, typed, header string
		}{
			{scripts[0], "javascript", "javascript"},
			{styles[0], "css", "css"},
		} {
			w := do(s, http.MethodGet, "/"+want.name, nil)
			ctype := w.Header().Get("Content-Type")
			if w.Code != http.StatusOK || !strings.Contains(ctype, want.header) || w.Body.Len() == 0 {
				t.Fatalf("/%s = %d %q with %d bytes, want 200 with a %s content type", want.name, w.Code, ctype, w.Body.Len(), want.typed)
			}
		}
	})

	t.Run("recent and needs-you are admitted and served", func(t *testing.T) {
		s, _ := newTestServer(t, nil)
		c := enterWith(t, s, "/")
		cookie := map[string]string{"Cookie": c.Name + "=" + c.Value}
		root := do(s, http.MethodGet, "/", cookie)
		if root.Code != http.StatusOK {
			t.Fatalf("the entry page = %d", root.Code)
		}
		for _, to := range []string{"/recent", "/needs-you"} {
			page := do(s, http.MethodGet, to, cookie)
			if page.Code != http.StatusOK || page.Body.String() != root.Body.String() {
				t.Fatalf("%q = %d, want the entry page", to, page.Code)
			}
			typed := do(s, http.MethodGet, to, map[string]string{"Sec-Fetch-Site": "none", "Sec-Fetch-Dest": "document"})
			if typed.Code != http.StatusFound || !strings.HasPrefix(typed.Header().Get("Location"), "/enter?ticket=") {
				t.Fatalf("%q without a session = %d to %q", to, typed.Code, typed.Header().Get("Location"))
			}
			linked := do(s, http.MethodGet, to, map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Dest": "document"})
			if linked.Code != http.StatusForbidden || !strings.Contains(linked.Body.String(), TextHowToOpen) {
				t.Fatalf("%q from another site = %d", to, linked.Code)
			}
			if strings.Contains(linked.Body.String(), s.token) {
				t.Fatalf("%q from another site got the workspace token", to)
			}
		}
	})
}

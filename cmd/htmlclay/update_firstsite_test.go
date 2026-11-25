package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

// A tab left open on the first site across the update that moved it off
// htmlclay.localhost cannot save at the dashboard's address. Its writes are
// refused with site-moved, the site's new address and a message telling the
// person to copy the change and reload, instead of the dashboard's 405.
func TestUpdateRefusesFirstSiteTabClearly(t *testing.T) {
	f := newAddressFixture(t, "notes")
	f.seed(t, f.firstSite(f.docs))
	a := f.launch(t)
	slug := a.rt.cfg.SiteSlug(f.docs)
	if slug == "" || slug == "htmlclay" {
		t.Fatalf("the first site was not moved: label %q", slug)
	}
	want := "http://" + siteHost(slug, nestedTestPort) + "/"
	before, err := os.ReadFile(f.doc)
	if err != nil {
		t.Fatal(err)
	}
	tab := map[string]string{
		"Origin":         "http://" + dashboardAddress,
		"Sec-Fetch-Site": "same-origin",
		"Content-Type":   "text/html",
	}

	for _, path := range []string{"/_/save/abc", "/_/save", "/_/sync"} {
		rec := servedRequest(t, a, http.MethodPost, dashboardAddress, path, "<html><body>changed</body></html>", tab)
		var got struct{ Code, Msg, To string }
		if rec.Code != http.StatusConflict || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
			t.Fatalf("POST %s = %d %q, want 409 site-moved", path, rec.Code, rec.Body.String())
		}
		if got.Code != "site-moved" || got.To != want || got.Msg != siteMovedMessage {
			t.Fatalf("POST %s answered %+v, want site-moved to %s", path, got, want)
		}
	}
	if after, _ := os.ReadFile(f.doc); string(after) != string(before) {
		t.Fatal("a refused write changed the document")
	}

	for _, other := range []map[string]string{
		{"Origin": "http://evil.example", "Sec-Fetch-Site": "cross-site"},
		{"Origin": "http://" + siteHost("htmlclay1", nestedTestPort), "Sec-Fetch-Site": "same-site"},
		{},
	} {
		if rec := servedRequest(t, a, http.MethodPost, dashboardAddress, "/_/save/abc", "x", other); rec.Code == http.StatusConflict || strings.Contains(rec.Body.String(), want) {
			t.Fatalf("a write from %v was told where the site moved: %d %q", other, rec.Code, rec.Body.String())
		}
	}
	if rec := servedRequest(t, a, http.MethodGet, dashboardAddress, "/_/save/abc", "", tab); rec.Code == http.StatusConflict {
		t.Fatal("a GET was refused as a moved-site write")
	}
	if rec := servedRequest(t, a, http.MethodPost, dashboardAddress, "/api/folders", "{}", tab); strings.Contains(rec.Body.String(), "site-moved") {
		t.Fatal("the dashboard's own API was refused as a moved-site write")
	}

	plain := newAddressFixture(t, "notes")
	b := plain.launch(t)
	if rec := servedRequest(t, b, http.MethodPost, dashboardAddress, "/_/save/abc", "x", tab); rec.Code == http.StatusConflict {
		t.Fatal("a config that never moved a site refused a write as site-moved")
	}
}

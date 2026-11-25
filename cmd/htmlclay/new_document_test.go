package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/server"
	"github.com/panphora/htmlclay/internal/session"
	"github.com/panphora/htmlclay/internal/tier"
)

// A blank copy lands beside its source under a free -new name and keeps the
// source's standing: the person's own document makes another of theirs, a
// download makes another download, and a copy whose code the page changed is
// judged on its own, because the page's script wrote it.
func TestNewDocumentKeepsItsSourcesStanding(t *testing.T) {
	a := newTestApp(t, evalTempHome(t))
	if err := a.tierStore().SetDeveloper(false); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(a.rt.home, "notes")
	mine := filepath.Join(folder, "list.html")
	stranger := filepath.Join(folder, "shared.htmlclay")
	writeTestFile(t, mine, tierTestPage)
	writeTestFile(t, stranger, tierTestPage)
	madeHere(t, a, mine)
	downloadedHere(t, a, stranger)
	folder = real(t, folder)

	blank := strings.Replace(tierTestPage, "<p>Old</p>", "<p></p>", 1)
	newDocument := func(source, html, want string) string {
		t.Helper()
		url, err := a.newDocumentForPage(source, session.ViaOsOpen, []byte(html))
		if err != nil {
			t.Fatalf("new document from %s: %v", source, err)
		}
		path := filepath.Join(folder, want)
		got, err := os.ReadFile(path)
		if err != nil || string(got) != html {
			t.Fatalf("%s holds %q (%v), want the page's copy", want, got, err)
		}
		if !strings.HasPrefix(url, "http://") || !strings.HasSuffix(url, "/"+want) {
			t.Fatalf("new document %s serves at %q", want, url)
		}
		return path
	}

	if v := a.tierOf(newDocument(mine, blank, "list-new.html")); v.Tier != tier.Mine {
		t.Fatalf("a blank copy of the person's own document is %s %s", v.Tier, v.Reason)
	}
	newDocument(mine, blank, "list-new-2.html")
	if got, _ := os.ReadFile(mine); string(got) != tierTestPage {
		t.Fatalf("the source changed: %q", got)
	}

	changed := strings.Replace(blank, "</body>", "<script>fetch('/x')</script></body>", 1)
	if v := a.tierOf(newDocument(mine, changed, "list-new-3.html")); v.Tier == tier.Mine {
		t.Fatal("a copy whose code the page changed is recorded as the person's own")
	}

	// A stranger's script cannot make files until the person answers its
	// warning, and Don't allow keeps it so.
	if _, err := a.newDocumentForPage(stranger, session.ViaOsOpen, []byte(blank)); !errors.As(err, new(server.ActionRefusal)) {
		t.Fatalf("a stranger nobody allowed made a copy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, "shared-new.htmlclay")); err == nil {
		t.Fatal("the refused copy was written")
	}
	v := a.tierOf(stranger)
	if err := a.allowDocument(real(t, stranger), v.Code); err != nil {
		t.Fatal(err)
	}
	if v := a.tierOf(newDocument(stranger, blank, "shared-new.htmlclay")); v.Tier != tier.Unknown || v.Reason != tier.ReasonDownloaded {
		t.Fatalf("a blank copy of a download is %s %s, want unknown and downloaded", v.Tier, v.Reason)
	}
}

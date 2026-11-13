package appcore

import (
	"path/filepath"
	"testing"
)

func TestDisplayPath(t *testing.T) {
	cases := []struct {
		name string
		home string
		path string
		want string
	}{
		{"home itself", "/Users/sam", "/Users/sam", "~"},
		{"under home", "/Users/sam", "/Users/sam/Documents/Notes", "~/Documents/Notes"},
		{"sibling sharing a prefix", "/Users/sam", "/Users/sam2/Documents", "/Users/sam2/Documents"},
		{"outside home", "/Users/sam", "/Volumes/Backup/site", "/Volumes/Backup/site"},
		{"empty", "/Users/sam", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// The system's own paths: Windows spells these with backslashes.
			c.home, c.path, c.want = filepath.FromSlash(c.home), filepath.FromSlash(c.path), filepath.FromSlash(c.want)
			if got := displayPath(c.home, c.path); got != c.want {
				t.Fatalf("displayPath(%q, %q) = %q, want %q", c.home, c.path, got, c.want)
			}
		})
	}
}

func TestHostCanListsEveryType(t *testing.T) {
	snapshot := Build(Facts{Host: HostFacts{Implemented: []string{"folder.serve", "made-up.type"}}})
	if len(snapshot.Host.Can) != len(ActionClasses) {
		t.Fatalf("host.can has %d keys, want %d", len(snapshot.Host.Can), len(ActionClasses))
	}
	if !snapshot.Host.Can["folder.serve"] {
		t.Fatal("host.can says folder.serve is not implemented")
	}
	for _, typ := range []string{"document.open", "file.open", "waiting.markSeen"} {
		if snapshot.Host.Can[typ] {
			t.Fatalf("host.can says %s is implemented, but the facts do not list it", typ)
		}
	}
	for typ, can := range snapshot.Host.Can {
		if typ != "folder.serve" && can {
			t.Fatalf("host.can says %s is implemented, but the facts do not list it", typ)
		}
	}
	if _, ok := snapshot.Host.Can["made-up.type"]; ok {
		t.Fatal("host.can lists made-up.type, which is no action type")
	}
}

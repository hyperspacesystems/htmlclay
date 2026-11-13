package appcore

import "testing"

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
			if got := displayPath(c.home, c.path); got != c.want {
				t.Fatalf("displayPath(%q, %q) = %q, want %q", c.home, c.path, got, c.want)
			}
		})
	}
}

package server

import (
	"path/filepath"
	"testing"

	"github.com/panphora/htmlclay/internal/versions"
)

// M3: keeping bytes that history already holds must name that entry, not add the
// old bytes back as a new newest version.
func TestGuardHistoryKeepReusesTheExistingVersion(t *testing.T) {
	dir := t.TempDir()
	v := versions.New(filepath.Join(dir, "versions"))
	doc := filepath.Join(dir, "board.htmlclay")
	key := versions.Key(doc, nil)
	body := []byte("<html><body>old answer</body></html>")
	if _, err := v.Backup(key, doc, body); err != nil {
		t.Fatal(err)
	}
	before, err := v.List(key, doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 {
		t.Fatalf("versions before Keep = %d, want 1", len(before))
	}

	h := NewGuardHistory(v, key, doc)
	name, err := h.Keep(body)
	if err != nil {
		t.Fatalf("Keep: %v", err)
	}
	if name != before[0].Name {
		t.Fatalf("Keep returned %q, want the existing entry %q", name, before[0].Name)
	}
	after, err := v.List(key, doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("Keep added a version: %d after, %d before", len(after), len(before))
	}
}

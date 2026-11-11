//go:build managedtrial

package managedtrial

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A document written later in the same millisecond as its sidecar is newer than
// the sidecar, as Local's mtimeMs comparison sees it, so the sidecar is stale.
func TestReadFreshSidecarSubMillisecond(t *testing.T) {
	root := t.TempDir()
	doc := filepath.Join(root, "stale.html")
	sidecar := filepath.Join(root, ".hyperclay", "api-v2", "stale.html.json")
	if err := os.MkdirAll(filepath.Dir(sidecar), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doc, []byte("<!doctype html><title>Stale</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecar, []byte(`{"title":"Sidecar"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	sidecarTime := base.Add(100 * time.Microsecond)
	docTime := base.Add(400 * time.Microsecond)
	if err := os.Chtimes(sidecar, sidecarTime, sidecarTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(doc, docTime, docTime); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(doc)
	if err != nil {
		t.Fatal(err)
	}
	if info.ModTime().Nanosecond() != docTime.Nanosecond() {
		t.Skipf("this filesystem keeps no sub-millisecond times (%v)", info.ModTime())
	}
	if text, ok := readFreshSidecar(root, "stale.html", nodeMtimeMs(info.ModTime())); ok {
		t.Fatalf("a sidecar older than its document within one millisecond was served as fresh: %q", text)
	}
	if err := os.Chtimes(doc, sidecarTime, sidecarTime); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(doc)
	if _, ok := readFreshSidecar(root, "stale.html", nodeMtimeMs(info.ModTime())); !ok {
		t.Fatal("a sidecar as new as its document was not served")
	}
}

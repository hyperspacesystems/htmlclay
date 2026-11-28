package mirror_test

import (
	"testing"

	"github.com/panphora/htmlclay/internal/htmlutil"
	"github.com/panphora/htmlclay/internal/mirror"
)

// The served document carries the tag right after its head, and the bytes that were
// read from disk are left alone: a caller hashes them after this call.
func TestInjectPutsTheTagAfterHeadAndLeavesTheInputAlone(t *testing.T) {
	s := []byte("<html><head><title>t</title>")
	buf := make([]byte, len(s), len(s)+64)
	copy(buf, s)
	got := mirror.Inject(buf)
	if spare := string(buf[:cap(buf)][len(s) : len(s)+len(htmlutil.OfflineBootTag)]); spare != string(make([]byte, len(htmlutil.OfflineBootTag))) {
		t.Fatalf("the call wrote into the input's backing array: %q", spare)
	}
	if string(buf) != string(s) {
		t.Fatalf("the input slice changed: %q", buf)
	}
	want := "<html><head>" + string(htmlutil.OfflineBootTag) + "<title>t</title>"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

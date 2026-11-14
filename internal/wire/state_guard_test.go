package wire

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// stateSelector matches a read of the wire's live state from outside this
// package: one of the Core's state fields reached through the loose Server
// (receiver s or srv, which embeds *Core), or the Hub's own fields. Those fields
// are exported only so the loose host's tests can inspect them; product code
// must go through the Core's methods.
var stateSelector = regexp.MustCompile(`\b(s|srv)\.(HelperMu|Helpers|HelperOffline|HelperAsking|HelperSetting|HelperAskQuiet|HelperBindings|AIEdit)\.|\b(s|srv)\.(HelperMu|Helpers|HelperOffline|HelperAsking|HelperSetting|HelperAskQuiet)\b|\.Hub\.(Mu|Chans|TerminalBytes)\b`)

func TestWireStateSelectorCatchesAPlantedRead(t *testing.T) {
	for _, line := range []string{"n := len(s.Helpers)", "s.AIEdit.Mu.Lock()", "srv.Hub.Chans[path]", "s.HelperBindings.mu.RLock()"} {
		if !stateSelector.MatchString(line) {
			t.Errorf("the guard misses %q", line)
		}
	}
	for _, line := range []string{"return c.AIEdit.Default, engines", "meta.Document.Helpers = x", "s.Hub.Shutdown()", "s.WhileUnbound(path, fn)"} {
		if stateSelector.MatchString(line) {
			t.Errorf("the guard flags %q, which is not wire state", line)
		}
	}
}

func TestWireStateStaysInside(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(here), "..", "..")
	scanned := 0
	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(repo, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path == filepath.Join(repo, "internal", "wire") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			scanned++
			for i, line := range strings.Split(string(data), "\n") {
				code, _, _ := strings.Cut(line, "//")
				if stateSelector.MatchString(code) {
					t.Errorf("%s:%d reads the wire's state directly: %s", path, i+1, strings.TrimSpace(line))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if scanned < 50 {
		t.Fatalf("scanned only %d files; the walk did not reach the product code", scanned)
	}
}

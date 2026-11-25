package served

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/aitools"
	"github.com/panphora/htmlclay/internal/wire"
)

// folderTools is an ai-tools host where only one document has a folder.
type folderTools struct{ document string }

func (folderTools) Enabled() bool                          { return true }
func (folderTools) Service() *aitools.Service              { return nil }
func (folderTools) Preferred() (string, map[string]string) { return "", nil }
func (h folderTools) HasFolder(path string) bool           { return path == h.document }
func (folderTools) Folder(context.Context, string, bool, string, func()) (string, bool, error) {
	return "", false, nil
}

type toolsPolicy struct {
	framesOnly
	tools folderTools
}

func (p toolsPolicy) AITools() wire.AIToolsHost { return p.tools }

func TestTheRelayRefusesASnapshotIntoAnAIToolsOrProgramsDocument(t *testing.T) {
	const blockEditor = `<!doctype html><script src="https://clayjs.com/v1/clay.js?plugins=blockeditor"></script><p>wire</p>`
	const aiPage = `<!doctype html><script src="https://clayjs.com/v1/clay.js?plugins=ai"></script><p>wire</p>`
	const carriedPage = `<!doctype html><script type="text/x-clay-program" name="notes">async function main() {}</script><p>wire</p>`
	dir := t.TempDir()
	real := filepath.Join(dir, wireDocumentName)
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		real = filepath.Join(r, wireDocumentName)
	}
	for _, tc := range []struct {
		name   string
		page   string
		policy wire.Policy
		bind   bool
		want   int
	}{
		{"a Block Editor document", blockEditor, framesOnly{}, false, http.StatusOK},
		{"a document with an AI with tools folder", blockEditor, toolsPolicy{tools: folderTools{document: real}}, false, http.StatusForbidden},
		{"a document that loads the ai plugin", aiPage, framesOnly{}, false, http.StatusForbidden},
		{"a document that carries a program", carriedPage, framesOnly{}, false, http.StatusForbidden},
		{"a document with programs bound", blockEditor, framesOnly{}, true, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, wireDocumentName), []byte(tc.page), 0o644); err != nil {
				t.Fatal(err)
			}
			host := wireTestHost(t)
			stand := &wireStand{host: host, rootID: wireRootID, dir: dir, publicHost: wireRootHost, server: wireAddRoot(t, host, wireRootID, dir, wireRootHost, tc.policy)}
			if tc.bind {
				core, ok := host.Wire(wireRootID)
				if !ok {
					t.Fatal("no wire for the root")
				}
				core.BindHelpers(real)
			}
			for _, body := range []string{`{"snapshot":"<!doctype html><html><body>peer</body></html>"}`, `{"document":"<!doctype html><html><body>peer</body></html>"}`} {
				request, err := http.NewRequest(http.MethodPost, stand.server.URL+"/_/sync", strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				request.Host = stand.publicHost
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Sec-Fetch-Site", "same-origin")
				request.Header.Set("Document-URL", stand.documentURL(wireDocumentName))
				response := stand.do(t, request)
				answer, _ := io.ReadAll(response.Body)
				if response.StatusCode != tc.want {
					t.Fatalf("%s: status %d, want %d: %s", body, response.StatusCode, tc.want, answer)
				}
			}
		})
	}
}

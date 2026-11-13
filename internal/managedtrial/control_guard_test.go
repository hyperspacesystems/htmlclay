//go:build managedtrial

package managedtrial

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dataguard"
	"github.com/panphora/htmlclay/internal/specwire"
)

func TestControlGuardIfMatch(t *testing.T) {
	testControlGuardIfMatch(t, false)
}

func TestControlGuardAdapterIfMatch(t *testing.T) {
	if os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE") == "" {
		t.Skip("MANAGED_TRIAL_LOCAL_SOURCE is required for the real JS adapter")
	}
	testControlGuardIfMatch(t, true)
}

func testControlGuardIfMatch(t *testing.T, adapter bool) {
	original := `<html><head><script data-rules-name="api" data-rules-version="1">{"note":"h1"}</script></head><body><h1>Saved</h1></body></html>`
	for _, c := range []struct {
		name, header, value, kind string
		fire                      bool
	}{
		{"exact", "exact", "Updated", "data-api", false},
		{"stale", `"stale"`, "Updated", "data-api", true},
		{"wildcard", "*", "Updated", "data-api", true},
		{"absent", "", "Updated", "data-api", true},
		{"cleared", "exact", "", "data-api", true},
		{"download", "exact", "Updated", "sync-download", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := controlTestRoot(t, "guard-root")
			server := controlTestServer(t, root)
			if err := os.WriteFile(filepath.Join(root.baseDir, "doc.html"), []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			runDataLossGuard(runGuardArgs{BaseDir: root.baseDir, Name: "doc.html", NewHTML: original, PrevContent: &original, Prov: dataguard.UIGestured})
			state, err := readGuard(root.baseDir, "doc.html")
			if err != nil || state == nil || !state.UIWorkPending {
				t.Fatalf("seed = %+v, %v", state, err)
			}
			header := c.header
			if header == "exact" {
				header = specwire.Etag([]byte(original))
			}
			body := map[string]any{
				"opId": "0123456789abcdef", "root": "guard-root", "name": "doc.html", "kind": c.kind,
				"bytes":    map[string]any{"encoding": "utf8", "data": strings.Replace(original, "Saved", c.value, 1)},
				"ifMatch":  header,
				"expected": map[string]any{"type": "checksum", "value": specwire.Etag([]byte(original))},
			}
			if adapter {
				encoded, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				script := `const path = require('node:path');
const [repo, url, secret, baseDir, encoded] = process.argv.slice(1);
require(path.join(repo, 'trial/go-process.cjs')).setConnection({url, secret, roots: {[baseDir]:'guard-root'}});
const host = require(path.join(repo, 'trial/go-document-host.cjs')).createGoDocumentHost({ctx:{root:{id:'guard-root',path:baseDir}}});
const body=JSON.parse(encoded); body.bytes=body.bytes.data;
host.commit(body).then(result=>{if(result.outcome!=='committed')throw new Error(JSON.stringify(result));}).catch(e=>{console.error(e);process.exitCode=1;});`
				cmd := exec.Command("node", "-e", script, os.Getenv("MANAGED_TRIAL_LOCAL_SOURCE"), server.URL, controlSecret, root.baseDir, string(encoded))
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("adapter: %v\n%s", err, output)
				}
			} else {
				status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", body)
				if status != http.StatusOK || answer["outcome"] != "committed" {
					t.Fatalf("commit: %d %+v", status, answer)
				}
			}
			state, err = readGuard(root.baseDir, "doc.html")
			if err != nil || state == nil || state.UIWorkPending || (state.Event != nil) != c.fire {
				t.Fatalf("guard = %+v, %v; want warning %v", state, err, c.fire)
			}
		})
	}
}

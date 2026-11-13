package served

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// structureLostScript is a service that plans every restructure and then never
// confirms its settle, as one that is slow past the deadline or restarting.
func structureLostScript() *consultScript {
	return newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/structure/prepare":
			return map[string]any{"decision": "proceed", "holdId": "h1"}
		case "/v1/structure/settle":
			return map[string]any{"ok": false, "error": "lost"}
		}
		return nil
	})
}

func unconfirmedNames(t *testing.T, get func() map[string]any) []string {
	t.Helper()
	answer := get()
	documents, _ := answer["documents"].([]any)
	names := []string{}
	for _, raw := range documents {
		entry, _ := raw.(map[string]any)
		name, _ := entry["name"].(string)
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TestStructureSettleLost is the recorded gap (protocol v1, "Gaps recorded"): a
// person renames a page while the service cannot confirm the settle. Go marks
// the document unconfirmed at both its old and new path, and the abort tells
// the service how many steps moved, so it can settle late and reconcile both.
func TestStructureSettleLost(t *testing.T) {
	consultShortAborts(t)
	script := structureLostScript()
	server, dir := consultStructureServer(t, consultStructurePeer(t, script))
	consultWriteFile(t, dir, "a.html", []byte("<p>a</p>"))
	consultWriteFile(t, dir, "style.css", []byte("p{}"))

	answer := consultStructureRestructure(t, server, "move", []map[string]any{{"from": "a.html", "to": "b.html"}})
	if answer["outcome"] != "committed" {
		t.Fatalf("move answer = %v, want committed", answer)
	}
	consultStructureWaitFor(t, script, "/v1/abort", func(body map[string]any) bool {
		return body["reason"] == "settle-failed" && body["structure"] == true &&
			body["completed"] == float64(1)
	})

	get := func() map[string]any {
		status, answer := controlJSON(t, server, controlSecret, http.MethodGet, "/v1/unconfirmed", nil)
		if status != http.StatusOK {
			t.Fatalf("GET /v1/unconfirmed = %d", status)
		}
		return answer
	}
	if names := unconfirmedNames(t, get); len(names) != 2 || names[0] != "a.html" || names[1] != "b.html" {
		t.Fatalf("unconfirmed after a lost rename = %v, want [a.html b.html]", names)
	}
	for _, name := range []string{"a.html", "b.html"} {
		status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/reconciled", map[string]string{"root": "root-consult", "name": name})
		if status != http.StatusOK || answer["ok"] != true {
			t.Fatalf("reconciled %s = %d %v", name, status, answer)
		}
	}
	if names := unconfirmedNames(t, get); len(names) != 0 {
		t.Fatalf("unconfirmed after reconciling both = %v, want none", names)
	}
}

// TestStructureSettleLostFolder is the same for a folder rename: every page
// document under it is marked at its old and new path, and the old paths,
// whose folder no longer exists, still resolve when the service reports them
// reconciled.
func TestStructureSettleLostFolder(t *testing.T) {
	consultShortAborts(t)
	script := structureLostScript()
	server, dir := consultStructureServer(t, consultStructurePeer(t, script))
	if err := os.MkdirAll(filepath.Join(dir, "old", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	consultWriteFile(t, dir, "old/x.html", []byte("<p>x</p>"))
	consultWriteFile(t, dir, "old/sub/y.html", []byte("<p>y</p>"))
	consultWriteFile(t, dir, "old/sub/z.png", []byte("png"))

	answer := consultStructureRestructure(t, server, "move", []map[string]any{{"from": "old", "to": "new"}})
	if answer["outcome"] != "committed" {
		t.Fatalf("folder move answer = %v, want committed", answer)
	}
	consultStructureWaitFor(t, script, "/v1/abort", func(body map[string]any) bool {
		return body["reason"] == "settle-failed" && body["structure"] == true
	})
	get := func() map[string]any {
		_, answer := controlJSON(t, server, controlSecret, http.MethodGet, "/v1/unconfirmed", nil)
		return answer
	}
	want := []string{"new/sub/y.html", "new/x.html", "old/sub/y.html", "old/x.html"}
	names := unconfirmedNames(t, get)
	if len(names) != len(want) {
		t.Fatalf("unconfirmed after a lost folder rename = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("unconfirmed after a lost folder rename = %v, want %v", names, want)
		}
	}
	for _, name := range want {
		status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/reconciled", map[string]string{"root": "root-consult", "name": name})
		if status != http.StatusOK || answer["ok"] != true {
			t.Fatalf("reconciled %s = %d %v", name, status, answer)
		}
	}
	if names := unconfirmedNames(t, get); len(names) != 0 {
		t.Fatalf("unconfirmed after reconciling every path = %v, want none", names)
	}
}

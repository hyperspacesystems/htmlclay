//go:build managedtrial

package managedtrial

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/specwire"
)

// controlSecret is the secret every control test authenticates with.
const controlSecret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// controlTestRoot builds one served root the way the binary does, without the
// Local checkout the document routes need: the control API only ever talks to
// the commit core.
func controlTestRoot(t *testing.T, id string) *controlRoot {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := newConsentRegistry(dir, nil)
	registry.Ready()
	baseDir := registry.BaseReal()
	host := newCommitHost(baseDir, id, registry, newGuardHooks(nil), nil)
	return newControlRoot(id, baseDir, registry, host, newEventRing())
}

func controlTestServer(t *testing.T, roots ...*controlRoot) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(newControlHandler(controlSecret, roots))
	t.Cleanup(server.Close)
	return server
}

// controlJSON sends one authenticated request and decodes the answer.
func controlJSON(t *testing.T, server *httptest.Server, secret, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "" {
		request.Header.Set("Authorization", "Bearer "+secret)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	answer := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &answer); err != nil {
			t.Fatalf("%s %s answered %q, which is not JSON: %v", method, path, raw, err)
		}
	}
	return response.StatusCode, answer
}

// controlText is one payload's string, from either encoding.
func controlText(t *testing.T, value any) (string, bool) {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		return "", false
	}
	data, _ := object["data"].(string)
	switch object["encoding"] {
	case "utf8":
		return data, true
	case "base64":
		return data, false
	}
	t.Fatalf("payload %v names no encoding", value)
	return "", false
}

// TestControlRequiresSecret is G1's authentication over the whole API: no
// header and a wrong secret are unauthorized, the right one is not.
func TestControlRequiresSecret(t *testing.T) {
	root := controlTestRoot(t, "root-0")
	server := controlTestServer(t, root)

	if status, answer := controlJSON(t, server, "", http.MethodGet, "/health", nil); status != http.StatusUnauthorized {
		t.Errorf("no header: status = %d (%v), want 401", status, answer)
	}
	if status, answer := controlJSON(t, server, "wrong", http.MethodGet, "/health", nil); status != http.StatusUnauthorized {
		t.Errorf("wrong secret: status = %d (%v), want 401", status, answer)
	}
	if status, answer := controlJSON(t, server, "", http.MethodPost, "/v1/commit", map[string]any{"kind": "create"}); status != http.StatusUnauthorized {
		t.Errorf("no header on /v1/commit: status = %d (%v), want 401", status, answer)
	}
	if status, answer := controlJSON(t, server, controlSecret, http.MethodGet, "/health", nil); status != http.StatusOK {
		t.Fatalf("right secret: status = %d (%v), want 200", status, answer)
	}
}

// TestControlCommitRoundTrip is the protocol Local's adapter is written
// against: a create, an exact binary download, a conditional save, a
// restructure, a page-state move, a read, the op states and the three requests
// the API refuses.
func TestControlCommitRoundTrip(t *testing.T) {
	root := controlTestRoot(t, "root-0")
	server := controlTestServer(t, root)

	doc := "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<title>Doc</title>\n</head>\n<body>\n<h1>One</h1>\n</body>\n</html>\n"
	status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"opId":  "create-1",
		"root":  "root-0",
		"name":  "a.html",
		"kind":  "create",
		"bytes": map[string]any{"encoding": "utf8", "data": doc},
	})
	if status != http.StatusOK {
		t.Fatalf("create: status = %d (%v)", status, answer)
	}
	if answer["outcome"] != "committed" || answer["opId"] != "create-1" {
		t.Fatalf("create: answer = %v, want a committed create-1", answer)
	}
	if answer["reason"] != nil || answer["cursor"] == "" || answer["etag"] == "" {
		t.Fatalf("create: answer = %v, want a cursor, an etag and a null reason", answer)
	}
	if stored, text := controlText(t, answer["stored"]); !text || stored != doc {
		t.Fatalf("create: stored = %v, want the utf8 document back", answer["stored"])
	}

	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R', 0xff, 0xfe, 0x00, 0x01}
	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"opId":  "exact-1",
		"root":  "root-0",
		"name":  "image.png",
		"kind":  "sync-download",
		"bytes": map[string]any{"encoding": "base64", "data": base64.StdEncoding.EncodeToString(png)},
	})
	if status != http.StatusOK || answer["outcome"] != "committed" {
		t.Fatalf("exact: status = %d, answer = %v", status, answer)
	}
	if stored, text := controlText(t, answer["stored"]); text || stored != base64.StdEncoding.EncodeToString(png) {
		t.Fatalf("exact: stored = %v, want the base64 bytes back", answer["stored"])
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodGet, "/v1/read?root=root-0&name=image.png", nil)
	if status != http.StatusOK || answer["exists"] != true {
		t.Fatalf("read image.png: status = %d, answer = %v", status, answer)
	}
	if data, text := controlText(t, answer["bytes"]); text || data != base64.StdEncoding.EncodeToString(png) {
		t.Fatalf("read image.png: bytes = %v, want the same base64", answer["bytes"])
	}
	if answer["checksum"] != specwire.Etag(png) {
		t.Fatalf("read image.png: checksum = %v, want %s", answer["checksum"], specwire.Etag(png))
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodGet, "/v1/read?root=root-0&name=a.html", nil)
	if status != http.StatusOK || answer["exists"] != true {
		t.Fatalf("read a.html: status = %d, answer = %v", status, answer)
	}
	checksum, _ := answer["checksum"].(string)
	if checksum == "" {
		t.Fatalf("read a.html: answer = %v, want a checksum", answer)
	}
	if data, text := controlText(t, answer["bytes"]); !text || data != doc {
		t.Fatalf("read a.html: bytes = %v, want the document", answer["bytes"])
	}

	edited := strings.Replace(doc, "<h1>One</h1>", "<h1>Two</h1>", 1)
	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"opId":     "save-1",
		"root":     "root-0",
		"name":     "a.html",
		"kind":     "browser-save",
		"bytes":    map[string]any{"encoding": "utf8", "data": edited},
		"expected": map[string]any{"type": "checksum", "value": checksum},
	})
	if status != http.StatusOK || answer["outcome"] != "committed" {
		t.Fatalf("save with the right checksum: status = %d, answer = %v", status, answer)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"opId":     "save-2",
		"root":     "root-0",
		"name":     "a.html",
		"kind":     "browser-save",
		"bytes":    map[string]any{"encoding": "utf8", "data": doc},
		"expected": map[string]any{"type": "checksum", "value": "0000000000000000"},
	})
	if status != http.StatusOK || answer["outcome"] != "refused" || answer["reason"] != "changed" {
		t.Fatalf("save with a stale checksum: status = %d, answer = %v, want a changed refusal", status, answer)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/restructure", map[string]any{
		"opId":  "move-1",
		"root":  "root-0",
		"kind":  "move",
		"steps": []map[string]any{{"from": "a.html", "to": "c.html"}},
	})
	if status != http.StatusOK || answer["outcome"] != "committed" || answer["completed"] != float64(1) {
		t.Fatalf("restructure: status = %d, answer = %v", status, answer)
	}
	if answer["opId"] != "move-1" || answer["cursor"] == "" {
		t.Fatalf("restructure: answer = %v, want its opId and a cursor", answer)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodGet, "/v1/read?root=root-0&name=a.html", nil)
	if status != http.StatusOK || answer["exists"] != false {
		t.Fatalf("read a.html after the move: status = %d, answer = %v", status, answer)
	}
	status, answer = controlJSON(t, server, controlSecret, http.MethodGet, "/v1/read?root=root-0&name=c.html", nil)
	if status != http.StatusOK || answer["exists"] != true || answer["checksum"] != specwire.Etag([]byte(edited)) {
		t.Fatalf("read c.html after the move: status = %d, answer = %v", status, answer)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/page-state", map[string]any{
		"opId": "state-1", "root": "root-0", "op": "write", "name": "c.html",
		"record": map[string]any{"v": 1, "rev": "abc", "tag": "tag"},
	})
	if status != http.StatusOK {
		t.Fatalf("page-state write: status = %d, answer = %v", status, answer)
	}
	from, err := pageStatePath(root.baseDir, "c.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(from); err != nil {
		t.Fatalf("page-state write left no sidecar: %v", err)
	}
	if status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/page-state", map[string]any{
		"opId": "state-2", "root": "root-0", "op": "move", "name": "c.html", "to": "d.html",
	}); status != http.StatusOK {
		t.Fatalf("page-state move: status = %d, answer = %v", status, answer)
	}
	to, err := pageStatePath(root.baseDir, "d.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(to); err != nil {
		t.Fatalf("page-state move left no sidecar at the destination: %v", err)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Fatalf("page-state move left the source sidecar behind: %v", err)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodGet, "/v1/op/create-1", nil)
	if status != http.StatusOK || answer["state"] != "committed" || answer["checksum"] == nil || answer["cursor"] == nil {
		t.Fatalf("op create-1: status = %d, answer = %v", status, answer)
	}
	if status, answer = controlJSON(t, server, controlSecret, http.MethodGet, "/v1/op/never-asked", nil); status != http.StatusOK || answer["state"] != "unknown" {
		t.Fatalf("op never-asked: status = %d, answer = %v", status, answer)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"root": "root-0", "name": "a.html", "kind": "browser-save", "hooks": true,
		"bytes": map[string]any{"encoding": "utf8", "data": doc},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("hooks: status = %d, answer = %v, want 400", status, answer)
	}
	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"root": "root-0", "name": "a.html", "kind": "bogus",
	})
	if status != http.StatusBadRequest || answer["error"] != "Unknown document write kind: bogus" {
		t.Fatalf("unknown kind: status = %d, answer = %v", status, answer)
	}
	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"root": "root-0", "name": "a.html", "kind": "checkpoint",
	})
	if status != http.StatusBadRequest || answer["error"] != "checkpoint needs rooms" {
		t.Fatalf("checkpoint: status = %d, answer = %v", status, answer)
	}
}

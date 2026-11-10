//go:build managedtrial

package managedtrial

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	server := httptest.NewServer(newControlHandler(controlSecret, roots, nil))
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
		"opId":  "1111111111111111",
		"root":  "root-0",
		"name":  "a.html",
		"kind":  "create",
		"bytes": map[string]any{"encoding": "utf8", "data": doc},
	})
	if status != http.StatusOK {
		t.Fatalf("create: status = %d (%v)", status, answer)
	}
	if answer["outcome"] != "committed" || answer["opId"] != "1111111111111111" {
		t.Fatalf("create: answer = %v, want a committed 1111111111111111", answer)
	}
	if answer["reason"] != nil || answer["cursor"] == "" || answer["etag"] == "" {
		t.Fatalf("create: answer = %v, want a cursor, an etag and a null reason", answer)
	}
	if stored, text := controlText(t, answer["stored"]); !text || stored != doc {
		t.Fatalf("create: stored = %v, want the utf8 document back", answer["stored"])
	}

	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R', 0xff, 0xfe, 0x00, 0x01}
	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"opId":  "2222222222222222",
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
		"opId":     "3333333333333333",
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
		"opId":     "4444444444444444",
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
		"opId":  "5555555555555555",
		"root":  "root-0",
		"kind":  "move",
		"steps": []map[string]any{{"from": "a.html", "to": "c.html"}},
	})
	if status != http.StatusOK || answer["outcome"] != "committed" || answer["completed"] != float64(1) {
		t.Fatalf("restructure: status = %d, answer = %v", status, answer)
	}
	if answer["opId"] != "5555555555555555" || answer["cursor"] == "" {
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
		"opId": "6666666666666666", "root": "root-0", "op": "write", "name": "c.html",
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
		"opId": "7777777777777777", "root": "root-0", "op": "move", "name": "c.html", "to": "d.html",
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

	status, answer = controlJSON(t, server, controlSecret, http.MethodGet, "/v1/op/1111111111111111", nil)
	if status != http.StatusOK || answer["state"] != "committed" || answer["checksum"] == nil || answer["cursor"] == nil {
		t.Fatalf("op 1111111111111111: status = %d, answer = %v", status, answer)
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

// TestControlRestructureWorkspaceKinds proves the route's shape check and the
// two workspace kinds: a page state its kind cannot carry or an empty-trash with
// steps is a 400 before anything runs, and a folder step makes its directory
// and answers no page state.
func TestControlRestructureWorkspaceKinds(t *testing.T) {
	root := controlTestRoot(t, "root-0")
	server := controlTestServer(t, root)

	status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/restructure", map[string]any{
		"root": "root-0", "kind": "move",
		"steps": []map[string]any{{"from": "a.html", "to": "b.html", "pageState": "drop"}},
	})
	if status != http.StatusBadRequest || answer["error"] == nil {
		t.Fatalf("move with a page-state drop: status = %d, answer = %v, want 400", status, answer)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/restructure", map[string]any{
		"root": "root-0", "kind": "empty-trash",
		"steps": []map[string]any{{"from": "a.html", "to": "b.html"}},
	})
	if status != http.StatusBadRequest || answer["error"] == nil {
		t.Fatalf("empty-trash with a step: status = %d, answer = %v, want 400", status, answer)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/restructure", map[string]any{
		"root": "root-0", "kind": "folder",
		"steps": []map[string]any{{"to": "made"}},
	})
	if status != http.StatusOK || answer["outcome"] != "committed" || answer["pageState"] != nil {
		t.Fatalf("folder: status = %d, answer = %v, want committed with no page state", status, answer)
	}
	if info, err := os.Stat(filepath.Join(root.baseDir, "made")); err != nil || !info.IsDir() {
		t.Fatalf("folder made no directory: %v", err)
	}
}

// TestControlOpReuseRefused is the opId rule every mutating route shares: a
// reused id is refused with its earlier outcome kept, an id that is not 16 to 64
// lowercase hex is a 400, and no id at all is one Go makes up.
func TestControlOpReuseRefused(t *testing.T) {
	root := controlTestRoot(t, "root-0")
	server := controlTestServer(t, root)

	doc := "<!doctype html>\n<html lang=\"en\">\n<body>\n<h1>One</h1>\n</body>\n</html>\n"
	const opID = "0123456789abcdef"
	status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"opId":  opID,
		"root":  "root-0",
		"name":  "r.html",
		"kind":  "create",
		"bytes": map[string]any{"encoding": "utf8", "data": doc},
	})
	if status != http.StatusOK || answer["outcome"] != "committed" {
		t.Fatalf("create with %s: status = %d, answer = %v", opID, status, answer)
	}

	edited := strings.Replace(doc, "<h1>One</h1>", "<h1>Two</h1>", 1)
	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"opId":  opID,
		"root":  "root-0",
		"name":  "r.html",
		"kind":  "browser-save",
		"bytes": map[string]any{"encoding": "utf8", "data": edited},
	})
	if status != http.StatusConflict || answer["error"] != "op-reused" {
		t.Fatalf("reused opId: status = %d, answer = %v, want 409 op-reused", status, answer)
	}
	if stored, err := os.ReadFile(filepath.Join(root.baseDir, "r.html")); err != nil || string(stored) != doc {
		t.Fatalf("the refused reuse left %q / %v, want the first commit's bytes", stored, err)
	}
	if record, ok := root.events.lookupOp(opID); !ok || record.state != "committed" {
		t.Fatalf("the refused reuse left the op as %+v, want the first commit's outcome", record)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"opId":  "NOT-HEX",
		"root":  "root-0",
		"name":  "r.html",
		"kind":  "browser-save",
		"bytes": map[string]any{"encoding": "utf8", "data": edited},
	})
	if status != http.StatusBadRequest || answer["error"] != "invalid opId" {
		t.Fatalf("non-hex opId: status = %d, answer = %v, want 400 invalid opId", status, answer)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/commit", map[string]any{
		"root":  "root-0",
		"name":  "r.html",
		"kind":  "browser-save",
		"bytes": map[string]any{"encoding": "utf8", "data": edited},
	})
	if status != http.StatusOK || answer["outcome"] != "committed" {
		t.Fatalf("no opId: status = %d, answer = %v, want a committed save", status, answer)
	}
	made, _ := answer["opId"].(string)
	if !controlOpID.MatchString(made) {
		t.Fatalf("no opId: answer names %q, want a 16-char lowercase hex opId", made)
	}

	const moveOpID = "fedcba9876543210"
	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/restructure", map[string]any{
		"opId":  moveOpID,
		"root":  "root-0",
		"kind":  "move",
		"steps": []map[string]any{{"from": "r.html", "to": "s.html"}},
	})
	if status != http.StatusOK || answer["outcome"] != "committed" {
		t.Fatalf("restructure with %s: status = %d, answer = %v", moveOpID, status, answer)
	}
	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/restructure", map[string]any{
		"opId":  moveOpID,
		"root":  "root-0",
		"kind":  "move",
		"steps": []map[string]any{{"from": "s.html", "to": "t.html"}},
	})
	if status != http.StatusConflict || answer["error"] != "op-reused" {
		t.Fatalf("reused restructure opId: status = %d, answer = %v, want 409 op-reused", status, answer)
	}
	if _, err := os.Stat(filepath.Join(root.baseDir, "t.html")); !os.IsNotExist(err) {
		t.Fatalf("the refused restructure still moved the file: %v", err)
	}

	const stateOpID = "0011223344556677"
	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/page-state", map[string]any{
		"opId": stateOpID, "root": "root-0", "op": "write", "name": "s.html",
		"record": map[string]any{"v": 1, "rev": "abc", "tag": "tag"},
	})
	if status != http.StatusOK {
		t.Fatalf("page-state write with %s: status = %d, answer = %v", stateOpID, status, answer)
	}
	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/page-state", map[string]any{
		"opId": stateOpID, "root": "root-0", "op": "move", "name": "s.html", "to": "u.html",
	})
	if status != http.StatusConflict || answer["error"] != "op-reused" {
		t.Fatalf("reused page-state opId: status = %d, answer = %v, want 409 op-reused", status, answer)
	}
}

// TestControlReadCoherent is the read the JS service sees a document through: the
// merge state the stored file lacks, the sidecar it came from, the bytes' sha256
// and the file's identity, all taken under the document's lock.
func TestControlReadCoherent(t *testing.T) {
	root := controlTestRoot(t, "root-0")
	server := controlTestServer(t, root)

	rev := "093360993130d818"
	stored := saveGoldenPage(rev)
	if strings.Contains(stored, `merge="page"`) {
		t.Fatalf("the fixture already carries the merge state: %q", stored)
	}
	if err := os.WriteFile(filepath.Join(root.baseDir, "page.html"), []byte(stored), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writePageState(root.baseDir, "page.html", &PageRecord{V: 1, Rev: rev, Tag: saveGoldenYJSTag}, false); err != nil {
		t.Fatal(err)
	}

	status, answer := controlJSON(t, server, controlSecret, http.MethodGet, "/v1/read?root=root-0&name=page.html", nil)
	if status != http.StatusOK || answer["exists"] != true {
		t.Fatalf("read page.html: status = %d, answer = %v", status, answer)
	}
	logical, text := controlText(t, answer["logical"])
	if !text || !strings.Contains(logical, saveGoldenYJSTag) {
		t.Fatalf("read page.html: logical = %v, want the stored file with its merge state back", answer["logical"])
	}
	sidecar, ok := answer["sidecar"].(map[string]any)
	if !ok || sidecar["rev"] != rev {
		t.Fatalf("read page.html: sidecar = %v, want the record for %s", answer["sidecar"], rev)
	}
	sum := sha256.Sum256([]byte(stored))
	if answer["fileSha256"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("read page.html: fileSha256 = %v, want %s", answer["fileSha256"], hex.EncodeToString(sum[:]))
	}
	identity, ok := answer["identity"].(map[string]any)
	if !ok {
		t.Fatalf("read page.html: identity = %v, want an object", answer["identity"])
	}
	if identity["size"] != float64(len(stored)) {
		t.Fatalf("read page.html: identity.size = %v, want %d", identity["size"], len(stored))
	}
	ino, _ := identity["ino"].(string)
	if ino == "" || strings.Trim(ino, "0123456789") != "" {
		t.Fatalf("read page.html: identity.ino = %v, want a decimal string", identity["ino"])
	}

	if err := os.WriteFile(filepath.Join(root.baseDir, "notes.txt"), []byte("plain text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, answer = controlJSON(t, server, controlSecret, http.MethodGet, "/v1/read?root=root-0&name=notes.txt", nil)
	if status != http.StatusOK || answer["exists"] != true {
		t.Fatalf("read notes.txt: status = %d, answer = %v", status, answer)
	}
	if answer["logical"] != nil || answer["sidecar"] != nil {
		t.Fatalf("read notes.txt: logical = %v, sidecar = %v, want both null", answer["logical"], answer["sidecar"])
	}
	if _, ok := answer["identity"].(map[string]any); !ok {
		t.Fatalf("read notes.txt: identity = %v, want an object", answer["identity"])
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodGet, "/v1/read?root=root-0&name=gone.html", nil)
	if status != http.StatusOK || answer["exists"] != false {
		t.Fatalf("read gone.html: status = %d, answer = %v", status, answer)
	}
	for _, key := range []string{"fileSha256", "identity", "logical", "sidecar"} {
		if answer[key] != nil {
			t.Fatalf("read gone.html: %s = %v, want null", key, answer[key])
		}
	}
}

// TestControlReadWaitsForTheLock is the read's coherence: a save holding the
// document's lock keeps the read waiting, so it can never see a sidecar written
// for bytes not yet renamed into place.
func TestControlReadWaitsForTheLock(t *testing.T) {
	root := controlTestRoot(t, "root-0")
	server := controlTestServer(t, root)
	if err := os.WriteFile(filepath.Join(root.baseDir, "held.html"), []byte("<!doctype html>\n<html><body><h1>Held</h1></body></html>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	filePath, err := resolveWritePath(root.registry, "held.html")
	if err != nil {
		t.Fatal(err)
	}

	unlock := fileLocks.lock(filePath)
	answered := make(chan int, 1)
	go func() {
		request, err := http.NewRequest(http.MethodGet, server.URL+"/v1/read?root=root-0&name=held.html", nil)
		if err != nil {
			answered <- 0
			return
		}
		request.Header.Set("Authorization", "Bearer "+controlSecret)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			answered <- 0
			return
		}
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		answered <- response.StatusCode
	}()

	select {
	case status := <-answered:
		unlock()
		t.Fatalf("the read answered %d while the document's lock was held", status)
	case <-time.After(100 * time.Millisecond):
	}
	unlock()

	select {
	case status := <-answered:
		if status != http.StatusOK {
			t.Fatalf("the read answered %d after the lock was released, want 200", status)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the read never answered after the lock was released")
	}
}

// TestControlDurable is POST /v1/durable: the bytes the caller names are fsynced
// with their sidecar and directories, other bytes answer `changed`, and a
// directory sync that fails for a reason the platform does not excuse is a 500.
func TestControlDurable(t *testing.T) {
	root := controlTestRoot(t, "root-0")
	server := controlTestServer(t, root)

	doc := []byte("<!doctype html>\n<html><body><h1>Durable</h1></body></html>\n")
	if err := os.WriteFile(filepath.Join(root.baseDir, "d.html"), doc, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writePageState(root.baseDir, "d.html", &PageRecord{V: 1, Rev: "rev-1", Tag: "<script></script>"}, false); err != nil {
		t.Fatal(err)
	}
	checksum := specwire.Etag(doc)

	status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/durable", map[string]any{
		"root": "root-0", "name": "d.html", "checksum": checksum,
	})
	if status != http.StatusOK || answer["durable"] != true || answer["checksum"] != checksum {
		t.Fatalf("durable with the right checksum: status = %d, answer = %v", status, answer)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/durable", map[string]any{
		"root": "root-0", "name": "d.html", "checksum": "0000000000000000",
	})
	if status != http.StatusOK || answer["changed"] != true || answer["checksum"] != checksum {
		t.Fatalf("durable with a stale checksum: status = %d, answer = %v, want changed with the current checksum", status, answer)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/durable", map[string]any{
		"root": "root-0", "name": "gone.html", "checksum": checksum,
	})
	if status != http.StatusOK || answer["changed"] != true || answer["checksum"] != nil {
		t.Fatalf("durable on a missing file: status = %d, answer = %v, want changed with a null checksum", status, answer)
	}

	original := atomicDirSync
	atomicDirSync = func(string) error { return errors.New("boom") }
	t.Cleanup(func() { atomicDirSync = original })
	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/durable", map[string]any{
		"root": "root-0", "name": "d.html", "checksum": checksum,
	})
	if status != http.StatusInternalServerError {
		t.Fatalf("durable with a failing directory sync: status = %d, answer = %v, want 500", status, answer)
	}
}

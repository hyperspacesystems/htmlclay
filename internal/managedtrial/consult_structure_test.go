//go:build managedtrial

package managedtrial

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// consultStructurePeer starts the fake service and answers its address.
func consultStructurePeer(t *testing.T, script *consultScript) servicePeer {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(script.handle))
	t.Cleanup(server.Close)
	return servicePeer{URL: server.URL, Secret: strings.Repeat("ab", 32), Epoch: "aaaaaaaaaaaaaaaa"}
}

// consultStructureServer serves one root with a service attached, the way the
// binary builds them, and answers its real folder.
func consultStructureServer(t *testing.T, peer servicePeer) (*httptest.Server, string) {
	t.Helper()
	dir := consultTestRoot(t)
	registry := newConsentRegistry(dir, nil)
	registry.Ready()
	baseDir := registry.BaseReal()
	events := newEventRing()
	link := newServiceLink(events, peer)
	link.setRoot("root-consult", baseDir)
	host := newCommitHost(baseDir, "root-consult", registry, newGuardHooks(nil), nil)
	host.service = link
	root := newControlRoot("root-consult", baseDir, registry, host, events)
	server := httptest.NewServer(newControlHandler(controlSecret, []*controlRoot{root}, link))
	t.Cleanup(server.Close)
	return server, baseDir
}

// consultStructureRestructure posts one /v1/restructure and answers its body.
func consultStructureRestructure(t *testing.T, server *httptest.Server, kind string, steps []map[string]any) map[string]any {
	t.Helper()
	status, answer := controlJSON(t, server, controlSecret, http.MethodPost, "/v1/restructure", map[string]any{
		"root":  "root-consult",
		"kind":  kind,
		"steps": steps,
	})
	if status != http.StatusOK {
		t.Fatalf("%s: status = %d, answer = %v", kind, status, answer)
	}
	return answer
}

// consultStructureWaitFor waits for one call the host makes off its own
// goroutine that matches the predicate.
func consultStructureWaitFor(t *testing.T, script *consultScript, path string, matches func(map[string]any) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, exchange := range script.find(path) {
			if matches(exchange.Body) {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s never arrived", path)
}

// consultStructureStep is the one step a prepare answered.
func consultStructureStep(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	raw, ok := body["steps"].([]any)
	if !ok || len(raw) != 1 {
		t.Fatalf("prepare body steps = %v, want one step", body["steps"])
	}
	return consultTestObject(t, raw[0], "step")
}

// TestConsultStructureMove is one move consulted: the prepare inside the
// structure gate with the step's names, locations and canonical paths, then
// the settle with the one completed step.
func TestConsultStructureMove(t *testing.T) {
	var baseDir string
	prepareSawSource := false
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/structure/prepare":
			_, err := os.Stat(filepath.Join(baseDir, "a.html"))
			prepareSawSource = err == nil
			return map[string]any{"decision": "proceed", "holdId": "h1"}
		case "/v1/structure/settle":
			return map[string]any{"ok": true}
		}
		return nil
	})
	server, dir := consultStructureServer(t, consultStructurePeer(t, script))
	baseDir = dir
	consultWriteFile(t, dir, "a.html", []byte("<p>a</p>"))

	answer := consultStructureRestructure(t, server, "move", []map[string]any{{"from": "a.html", "to": "b.html"}})
	if answer["outcome"] != "committed" || answer["completed"] != float64(1) {
		t.Fatalf("move answer = %v, want committed with one completed step", answer)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.html")); err != nil {
		t.Fatalf("b.html after the move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.html")); !os.IsNotExist(err) {
		t.Fatalf("a.html after the move: %v", err)
	}
	if !prepareSawSource {
		t.Fatal("prepare did not come before the move: a.html was already gone")
	}

	prepare := script.only(t, "/v1/structure/prepare")
	if prepare.Body["origin"] != "move" {
		t.Fatalf("prepare origin = %v, want move", prepare.Body["origin"])
	}
	step := consultStructureStep(t, prepare.Body)
	if step["kind"] != "move" {
		t.Fatalf("step kind = %v, want move", step["kind"])
	}
	from := consultTestObject(t, step["from"], "from")
	if from["name"] != "a.html" || from["isDir"] != false || from["isSymlink"] != false {
		t.Fatalf("step from = %v, want a.html as a plain file", from)
	}
	if from["canonicalPath"] != filepath.Join(dir, "a.html") {
		t.Fatalf("step from canonical path = %v, want %q", from["canonicalPath"], filepath.Join(dir, "a.html"))
	}
	to := consultTestObject(t, step["to"], "to")
	if to["name"] != "b.html" || to["canonicalPath"] != filepath.Join(dir, "b.html") {
		t.Fatalf("step to = %v, want b.html", to)
	}

	settle := script.only(t, "/v1/structure/settle")
	if settle.Body["holdId"] != "h1" || settle.Body["completed"] != float64(1) || settle.Body["outcome"] != "committed" {
		t.Fatalf("settle body = %v, want the completed move", settle.Body)
	}
	copied, ok := settle.Body["copiedSteps"].([]any)
	if !ok || len(copied) != 0 {
		t.Fatalf("settle copiedSteps = %v, want none", settle.Body["copiedSteps"])
	}
}

// TestConsultStructureRefused is a prepare's refusal: the service's answer
// reaches the client as it was decided, nothing moves and the consultation is
// aborted.
func TestConsultStructureRefused(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/structure/prepare" {
			return map[string]any{"decision": "refuse", "status": 409, "code": "peers-editing"}
		}
		return nil
	})
	server, dir := consultStructureServer(t, consultStructurePeer(t, script))
	consultWriteFile(t, dir, "a.html", []byte("<p>a</p>"))

	answer := consultStructureRestructure(t, server, "move", []map[string]any{{"from": "a.html", "to": "b.html"}})
	if answer["outcome"] != "refused" || answer["reason"] != "peers-editing" {
		t.Fatalf("move answer = %v, want a peers-editing refusal", answer)
	}
	refusal := consultTestObject(t, answer["refusal"], "refusal")
	if refusal["status"] != float64(409) || refusal["code"] != "peers-editing" {
		t.Fatalf("refusal = %v, want status 409 and code peers-editing", refusal)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.html")); err != nil {
		t.Fatalf("a.html after the refusal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.html")); !os.IsNotExist(err) {
		t.Fatalf("b.html after the refusal: %v", err)
	}

	consultStructureWaitFor(t, script, "/v1/abort", func(body map[string]any) bool {
		return body["reason"] == "refused"
	})
	if count := script.count("/v1/structure/settle"); count != 0 {
		t.Fatalf("settle was called %d times after a refusal, want none", count)
	}
}

// TestConsultStructureUnreachable is a service that cannot be reached: the
// host's own retryable refusal, and nothing moved.
func TestConsultStructureUnreachable(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	peer := servicePeer{URL: closed.URL, Secret: strings.Repeat("ab", 32), Epoch: "aaaaaaaaaaaaaaaa"}
	closed.Close()
	server, dir := consultStructureServer(t, peer)
	consultWriteFile(t, dir, "a.html", []byte("<p>a</p>"))

	answer := consultStructureRestructure(t, server, "move", []map[string]any{{"from": "a.html", "to": "b.html"}})
	if answer["outcome"] != "refused" || answer["reason"] != "service-unavailable" {
		t.Fatalf("move answer = %v, want a service-unavailable refusal", answer)
	}
	refusal := consultTestObject(t, answer["refusal"], "refusal")
	if refusal["status"] != float64(503) || refusal["code"] != "service-unavailable" {
		t.Fatalf("refusal = %v, want status 503 and code service-unavailable", refusal)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.html")); err != nil {
		t.Fatalf("a.html after the refusal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.html")); !os.IsNotExist(err) {
		t.Fatalf("b.html after the refusal: %v", err)
	}
}

// TestConsultStructureDuplicate is a duplicate: every completed step is a
// copied step the settle names.
func TestConsultStructureDuplicate(t *testing.T) {
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/structure/prepare" {
			return map[string]any{"decision": "proceed", "holdId": "h1"}
		}
		return nil
	})
	server, dir := consultStructureServer(t, consultStructurePeer(t, script))
	consultWriteFile(t, dir, "a.html", []byte("<p>a</p>"))

	answer := consultStructureRestructure(t, server, "duplicate", []map[string]any{{"from": "a.html", "to": "c.html"}})
	if answer["outcome"] != "committed" || answer["completed"] != float64(1) {
		t.Fatalf("duplicate answer = %v, want committed with one completed step", answer)
	}
	for _, name := range []string{"a.html", "c.html"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s after the duplicate: %v", name, err)
		}
	}
	settle := script.only(t, "/v1/structure/settle")
	if settle.Body["completed"] != float64(1) || settle.Body["outcome"] != "committed" {
		t.Fatalf("settle body = %v, want the completed duplicate", settle.Body)
	}
	copied, ok := settle.Body["copiedSteps"].([]any)
	if !ok || len(copied) != 1 || copied[0] != float64(0) {
		t.Fatalf("settle copiedSteps = %v, want [0]", settle.Body["copiedSteps"])
	}
}

// TestConsultStructurePartial is a move whose second step fails: the settle
// names the one step that completed and the partial outcome.
func TestConsultStructurePartial(t *testing.T) {
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/structure/prepare" {
			return map[string]any{"decision": "proceed", "holdId": "h1"}
		}
		return nil
	})
	server, dir := consultStructureServer(t, consultStructurePeer(t, script))
	consultWriteFile(t, dir, "a.html", []byte("<p>a</p>"))

	answer := consultStructureRestructure(t, server, "move", []map[string]any{
		{"from": "a.html", "to": "b.html"},
		{"from": "missing.html", "to": "d.html"},
	})
	if answer["outcome"] != "partial" || answer["completed"] != float64(1) {
		t.Fatalf("move answer = %v, want partial with one completed step", answer)
	}
	settle := script.only(t, "/v1/structure/settle")
	if settle.Body["completed"] != float64(1) || settle.Body["outcome"] != "partial" {
		t.Fatalf("settle body = %v, want completed 1 and partial", settle.Body)
	}
}

// TestConsultStructureSettleFails is a settle the service answers badly: the
// move is still answered as it happened, and the consultation is aborted.
func TestConsultStructureSettleFails(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/structure/prepare":
			return map[string]any{"decision": "proceed", "holdId": "h1"}
		case "/v1/structure/settle":
			return map[string]any{"ok": false, "error": "x"}
		}
		return nil
	})
	server, dir := consultStructureServer(t, consultStructurePeer(t, script))
	consultWriteFile(t, dir, "a.html", []byte("<p>a</p>"))

	answer := consultStructureRestructure(t, server, "move", []map[string]any{{"from": "a.html", "to": "b.html"}})
	if answer["outcome"] != "committed" || answer["completed"] != float64(1) {
		t.Fatalf("move answer = %v, want the move committed anyway", answer)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.html")); err != nil {
		t.Fatalf("b.html after the move: %v", err)
	}
	consultStructureWaitFor(t, script, "/v1/abort", func(body map[string]any) bool {
		return body["reason"] == "settle-failed"
	})
}

// TestConsultStructureFolderUnconsulted is a workspace kind: v1 composes no
// workspace command in the service, so a folder restructure sends no prepare.
func TestConsultStructureFolderUnconsulted(t *testing.T) {
	script := newConsultScript(func(path string, body map[string]any, call int) any { return nil })
	server, dir := consultStructureServer(t, consultStructurePeer(t, script))

	answer := consultStructureRestructure(t, server, "folder", []map[string]any{{"to": "made"}})
	if answer["outcome"] != "committed" {
		t.Fatalf("folder answer = %v, want committed", answer)
	}
	if info, err := os.Stat(filepath.Join(dir, "made")); err != nil || !info.IsDir() {
		t.Fatalf("folder made no directory: %v", err)
	}
	if count := script.count("/v1/structure/prepare"); count != 0 {
		t.Fatalf("prepare was called %d times for a folder, want none", count)
	}
}

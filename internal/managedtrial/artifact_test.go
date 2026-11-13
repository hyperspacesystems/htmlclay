//go:build managedtrial

package managedtrial

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/dataguard"
)

// consultArtifactServer serves one root with a service attached that this route
// never calls.
func consultArtifactServer(t *testing.T) (*rootServer, string) {
	t.Helper()
	script := newConsultScript(func(path string, body map[string]any, call int) any { return nil })
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	return handler, dir
}

// TestControlArtifactWritesRecoveredCopy is the recovered copy outside any
// consultation: written under the document's lock with Local's naming, and
// answered with the receipt that names it.
func TestControlArtifactWritesRecoveredCopy(t *testing.T) {
	handler, dir := consultArtifactServer(t)
	control := controlTestServer(t, handler.control)

	status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/artifact", map[string]any{
		"opId":       strings.Repeat("1", 32),
		"root":       "root-consult",
		"name":       "page.html",
		"artifactId": strings.Repeat("a", 16),
		"type":       "recovered-copy",
		"logical":    map[string]any{"encoding": "utf8", "data": "<p>unsaved</p>"},
	})
	if status != http.StatusOK {
		t.Fatalf("artifact answered %d: %v", status, answer)
	}
	if answer["error"] != nil {
		t.Fatalf("artifact error = %v, want null", answer["error"])
	}
	if answer["root"] != "root-consult" || answer["name"] != "page.html" {
		t.Fatalf("artifact receipt = %v, want the named root and document", answer)
	}
	if answer["type"] != "recovered-copy" || answer["artifactId"] != strings.Repeat("a", 16) {
		t.Fatalf("artifact receipt = %v, want the artifact it was asked for", answer)
	}
	version, ok := answer["version"].(string)
	if !ok || version == "" {
		t.Fatalf("artifact receipt version = %v, want a file name", answer["version"])
	}
	copies := consultTestFiles(t, filepath.Join(dir, ".hyperclay", "recovered", "page"))
	found := false
	for _, path := range copies {
		if filepath.Base(path) != version {
			continue
		}
		found = true
		if data := consultTestDisk(t, path); data != "<p>unsaved</p>" {
			t.Fatalf("recovered copy = %q, want the unsaved edits", data)
		}
	}
	if !found {
		t.Fatalf("the receipt names %s, which is not among %v", version, copies)
	}
	sum := sha256.Sum256([]byte("<p>unsaved</p>"))
	if answer["sha256"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("artifact sha256 = %v, want %s", answer["sha256"], hex.EncodeToString(sum[:]))
	}
}

// TestControlArtifactInvalidID is a receipt that names why nothing was written.
func TestControlArtifactInvalidID(t *testing.T) {
	handler, dir := consultArtifactServer(t)
	control := controlTestServer(t, handler.control)

	status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/artifact", map[string]any{
		"opId":       strings.Repeat("2", 32),
		"root":       "root-consult",
		"name":       "page.html",
		"artifactId": "xyz",
		"type":       "recovered-copy",
		"logical":    map[string]any{"encoding": "utf8", "data": "<p>unsaved</p>"},
	})
	if status != http.StatusOK || answer["error"] != "invalid artifactId" {
		t.Fatalf("artifact answered %d %v, want 200 with invalid artifactId", status, answer)
	}
	if _, err := os.Stat(filepath.Join(dir, ".hyperclay", "recovered", "page")); !os.IsNotExist(err) {
		t.Fatalf("an invalid artifactId still wrote a recovered copy: %v", err)
	}
}

// TestControlArtifactSafetyVersionRefused is the one type this route does not
// write: a safety version exists only inside the write it protects.
func TestControlArtifactSafetyVersionRefused(t *testing.T) {
	handler, dir := consultArtifactServer(t)
	control := controlTestServer(t, handler.control)

	status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/artifact", map[string]any{
		"opId":       strings.Repeat("3", 32),
		"root":       "root-consult",
		"name":       "page.html",
		"artifactId": strings.Repeat("b", 16),
		"type":       "safety-version",
		"logical":    map[string]any{"encoding": "utf8", "data": "<p>safe</p>"},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("safety-version answered %d %v, want 400", status, answer)
	}
	if _, err := os.Stat(filepath.Join(dir, ".hyperclay", "versions", "page")); !os.IsNotExist(err) {
		t.Fatalf("a refused safety version still wrote: %v", err)
	}
}

// TestControlArtifactNoService is the root with no service to name a recovered
// copy against.
func TestControlArtifactNoService(t *testing.T) {
	root := controlTestRoot(t, "root-noservice")
	control := controlTestServer(t, root)

	status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/artifact", map[string]any{
		"opId":       strings.Repeat("4", 32),
		"root":       "root-noservice",
		"name":       "page.html",
		"artifactId": strings.Repeat("c", 16),
		"type":       "recovered-copy",
		"logical":    map[string]any{"encoding": "utf8", "data": "<p>unsaved</p>"},
	})
	if status != http.StatusNotFound {
		t.Fatalf("artifact without a service answered %d %v, want 404", status, answer)
	}
}

// TestControlArtifactOpReused is the opId rule every control write shares.
func TestControlArtifactOpReused(t *testing.T) {
	handler, _ := consultArtifactServer(t)
	control := controlTestServer(t, handler.control)
	body := map[string]any{
		"opId":       strings.Repeat("5", 32),
		"root":       "root-consult",
		"name":       "page.html",
		"artifactId": strings.Repeat("d", 16),
		"type":       "recovered-copy",
		"logical":    map[string]any{"encoding": "utf8", "data": "<p>unsaved</p>"},
	}

	status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/artifact", body)
	if status != http.StatusOK {
		t.Fatalf("the first artifact answered %d: %v", status, answer)
	}
	status, answer = controlJSON(t, control, controlSecret, http.MethodPost, "/v1/artifact", body)
	if status != http.StatusConflict || answer["error"] != "op-reused" {
		t.Fatalf("the reused opId answered %d %v, want 409 op-reused", status, answer)
	}
}

// consultRefusingPeer is a service whose admission refuses every write.
func consultRefusingPeer(t *testing.T) servicePeer {
	t.Helper()
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		if path == "/v1/admit" {
			return map[string]any{"decision": "refuse", "status": 409, "code": "peers-editing"}
		}
		return nil
	})
	return consultSavePeer(t, script)
}

// TestRestoreRefusedByService is a restore the service refused: the answer is
// its status and the document keeps its bytes.
func TestRestoreRefusedByService(t *testing.T) {
	consultShortAborts(t)
	server, _, dir := consultSaveServer(t, consultRefusingPeer(t))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	versionsDir := filepath.Join(dir, ".hyperclay", "versions", "page")
	if err := os.MkdirAll(versionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	restoredVersion, err := PublishVersion(versionsDir, ".html", []byte(consultSavePage+"<!-- v -->"))
	if err != nil {
		t.Fatal(err)
	}

	request, err := http.NewRequest(http.MethodPost, server.URL+"/_/restore/page.html/"+restoredVersion.Filename, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("a refused restore answered %d, want the service's 409", response.StatusCode)
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != consultSavePage {
		t.Fatalf("page.html = %q, want the bytes unchanged", onDisk)
	}
}

// TestGuardRestoreRefusedByService is a data-loss restore the service refused:
// the guard keeps its incident open and says the write-back failed.
func TestGuardRestoreRefusedByService(t *testing.T) {
	consultShortAborts(t)
	server, _, dir := consultSaveServer(t, consultRefusingPeer(t))

	full := watcherDocument("alpha", "bravo", "charlie")
	lost := watcherDocument("alpha")
	previous := full
	runDataLossGuard(runGuardArgs{
		BaseDir: dir, Name: "page.html", NewHTML: full, PrevContent: &previous,
		Prov: dataguard.UIGestured, Live: nil,
	})
	consultWriteFile(t, dir, "page.html", []byte(lost))
	if raised := runDataLossGuard(runGuardArgs{
		BaseDir: dir, Name: "page.html", NewHTML: lost, PrevContent: nil,
		Prov: dataguard.External, Live: nil,
	}); raised == nil {
		t.Fatal("the outside edit raised no data-loss event")
	}

	choose := func(choice string) (int, map[string]any) {
		t.Helper()
		body := strings.NewReader(`{"file":"page.html","choice":"` + choice + `"}`)
		request, err := http.NewRequest(http.MethodPost, server.URL+"/_/data-loss", body)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		answer := map[string]any{}
		if err := json.NewDecoder(response.Body).Decode(&answer); err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, answer
	}

	status, answer := choose("restore")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("the refused restore answered %d %v, want 422", status, answer)
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != lost {
		t.Fatalf("page.html = %q, want the bytes unchanged", onDisk)
	}
	response, err := http.Get(server.URL + "/_/data-loss?file=page.html")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	pending := map[string]any{}
	if err := json.NewDecoder(response.Body).Decode(&pending); err != nil {
		t.Fatal(err)
	}
	if pending["event"] == nil {
		t.Fatalf("the refused restore cleared the incident: %v", pending)
	}
}

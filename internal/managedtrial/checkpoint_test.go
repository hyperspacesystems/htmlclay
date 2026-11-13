//go:build managedtrial

package managedtrial

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestControlCheckpointRender(t *testing.T) {
	consultShortAborts(t)
	admitted := strings.Replace(consultSavePage, "  </body>", "    <!-- admitted -->\n  </body>", 1)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/render-checkpoint":
			return map[string]any{
				"kind":      "render",
				"logical":   map[string]any{"encoding": "utf8", "data": consultSavePage},
				"revision":  4,
				"captureId": "cap1",
				"artifacts": []any{},
			}
		case "/v1/admit":
			return map[string]any{
				"decision":    "proceed",
				"logical":     map[string]any{"encoding": "utf8", "data": admitted},
				"file":        map[string]any{"encoding": "utf8", "data": admitted},
				"reservation": nil,
			}
		case "/v1/settle":
			return map[string]any{"ok": true}
		}
		return nil
	})
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	control := controlTestServer(t, handler.control)

	opID := strings.Repeat("a", 32)
	status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/checkpoint", map[string]any{
		"opId":         opID,
		"root":         "root-consult",
		"name":         "page.html",
		"roomIdentity": "7",
		"generation":   nil,
	})
	if status != http.StatusOK {
		t.Fatalf("checkpoint answered %d: %v", status, answer)
	}
	if answer["kind"] != "ok" {
		t.Fatalf("kind = %v, want ok", answer["kind"])
	}
	if answer["revision"] != float64(4) {
		t.Fatalf("revision = %v, want 4", answer["revision"])
	}
	if written := consultTestData(t, answer["written"], "written"); written != admitted {
		t.Fatalf("written = %q, want the admitted file", written)
	}
	if answer["opId"] != opID {
		t.Fatalf("opId = %v, want %s", answer["opId"], opID)
	}
	if cursor, ok := answer["cursor"].(string); !ok || cursor == "" {
		t.Fatalf("cursor = %v, want a non-null cursor", answer["cursor"])
	}

	render := script.only(t, "/v1/render-checkpoint").Body
	for key, want := range map[string]any{"opHandle": opID, "lane": "save", "origin": "checkpoint", "roomIdentity": "7", "generation": nil} {
		if render[key] != want {
			t.Fatalf("render-checkpoint %s = %v, want %v", key, render[key], want)
		}
	}
	consultTestObject(t, render["current"], "render-checkpoint current")

	admit := script.only(t, "/v1/admit").Body
	for key, want := range map[string]any{"origin": "checkpoint", "opHandle": opID} {
		if admit[key] != want {
			t.Fatalf("admit %s = %v, want %v", key, admit[key], want)
		}
	}

	settle := script.only(t, "/v1/settle").Body
	for key, want := range map[string]any{"captureId": "cap1", "userDriven": false, "outcome": "committed"} {
		if settle[key] != want {
			t.Fatalf("settle %s = %v, want %v", key, settle[key], want)
		}
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != admitted {
		t.Fatalf("page.html = %q, want the admitted file", onDisk)
	}
}

func TestControlCheckpointGenerationNumber(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/render-checkpoint":
			return map[string]any{"kind": "clean"}
		}
		return nil
	})
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	control := controlTestServer(t, handler.control)

	status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/checkpoint", map[string]any{
		"opId":         strings.Repeat("b", 32),
		"root":         "root-consult",
		"name":         "page.html",
		"roomIdentity": "7",
		"generation":   12,
	})
	if status != http.StatusOK {
		t.Fatalf("checkpoint answered %d: %v", status, answer)
	}
	if got := script.only(t, "/v1/render-checkpoint").Body["generation"]; got != float64(12) {
		t.Fatalf("render-checkpoint generation = %v, want 12", got)
	}
}

func TestControlCheckpointPassThrough(t *testing.T) {
	cases := []struct {
		kind   string
		reason any
	}{
		{"clean", nil},
		{"current", nil},
		{"refused", "checkpoint-unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			consultShortAborts(t)
			script := newConsultScript(func(path string, body map[string]any, call int) any {
				if path == "/v1/render-checkpoint" {
					answer := map[string]any{"kind": tc.kind}
					if tc.reason != nil {
						answer["reason"] = tc.reason
					}
					return answer
				}
				return nil
			})
			_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
			consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
			control := controlTestServer(t, handler.control)

			status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/checkpoint", map[string]any{
				"opId":         strings.Repeat("c", 32),
				"root":         "root-consult",
				"name":         "page.html",
				"roomIdentity": "7",
				"generation":   nil,
			})
			if status != http.StatusOK {
				t.Fatalf("checkpoint answered %d: %v", status, answer)
			}
			if answer["kind"] != tc.kind {
				t.Fatalf("kind = %v, want %s", answer["kind"], tc.kind)
			}
			if tc.reason != nil && answer["reason"] != tc.reason {
				t.Fatalf("reason = %v, want %v", answer["reason"], tc.reason)
			}
			if calls := script.count("/v1/admit"); calls != 0 {
				t.Fatalf("admit was called %d times, want none", calls)
			}
			if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != consultSavePage {
				t.Fatalf("page.html = %q, want the bytes unchanged", onDisk)
			}
		})
	}
}

func TestControlCheckpointIdentityChanged(t *testing.T) {
	consultShortAborts(t)
	outside := "<!doctype html><p>outside write, longer than nothing</p>"
	dir := ""
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/render-checkpoint":
			return map[string]any{
				"kind":      "render",
				"logical":   map[string]any{"encoding": "utf8", "data": consultSavePage},
				"revision":  4,
				"captureId": "cap1",
				"artifacts": []any{},
			}
		case "/v1/admit":
			if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(outside), 0o644); err != nil {
				t.Errorf("writing outside bytes: %v", err)
			}
			return map[string]any{
				"decision":    "proceed",
				"logical":     map[string]any{"encoding": "utf8", "data": consultSavePage},
				"file":        map[string]any{"encoding": "utf8", "data": consultSavePage},
				"reservation": nil,
			}
		case "/v1/settle":
			return map[string]any{"ok": true}
		}
		return nil
	})
	_, handler, served := consultSaveServer(t, consultSavePeer(t, script))
	dir = served
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	control := controlTestServer(t, handler.control)

	status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/checkpoint", map[string]any{
		"opId":         strings.Repeat("d", 32),
		"root":         "root-consult",
		"name":         "page.html",
		"roomIdentity": "7",
		"generation":   nil,
	})
	if status != http.StatusOK {
		t.Fatalf("checkpoint answered %d: %v", status, answer)
	}
	if answer["kind"] != "refused" || answer["reason"] != "invalid-identity" {
		t.Fatalf("answer = %v, want a refused invalid-identity", answer)
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != outside {
		t.Fatalf("page.html = %q, want the outside bytes", onDisk)
	}
	if settle := script.only(t, "/v1/settle").Body; settle["outcome"] != "failed" {
		t.Fatalf("settle outcome = %v, want failed", settle["outcome"])
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("a temp file was left: %s", entry.Name())
		}
	}
}

func TestControlCheckpointUnconfirmed(t *testing.T) {
	consultShortAborts(t)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		return nil
	})
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	filePath, err := handler.commit.resolveWrite("page.html")
	if err != nil {
		t.Fatal(err)
	}
	handler.commit.service.markUnconfirmed(filePath, "root-consult", "page.html")
	control := controlTestServer(t, handler.control)

	status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/checkpoint", map[string]any{
		"opId":         strings.Repeat("e", 32),
		"root":         "root-consult",
		"name":         "page.html",
		"roomIdentity": "7",
		"generation":   nil,
	})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("checkpoint answered %d: %v", status, answer)
	}
	if answer["reason"] != "unconfirmed" {
		t.Fatalf("reason = %v, want unconfirmed", answer["reason"])
	}
	if calls := script.count("/v1/render-checkpoint"); calls != 0 {
		t.Fatalf("render-checkpoint was called %d times, want none", calls)
	}
}

func TestControlCheckpointServiceDown(t *testing.T) {
	consultShortAborts(t)
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closed.Close()
	_, handler, dir := consultSaveServer(t, servicePeer{
		URL: closed.URL, Secret: strings.Repeat("ab", 32), Epoch: "aaaaaaaaaaaaaaaa",
	})
	consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
	control := controlTestServer(t, handler.control)

	status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/checkpoint", map[string]any{
		"opId":         strings.Repeat("f", 32),
		"root":         "root-consult",
		"name":         "page.html",
		"roomIdentity": "7",
		"generation":   nil,
	})
	if status != http.StatusServiceUnavailable {
		t.Fatalf("checkpoint answered %d: %v", status, answer)
	}
	if answer["reason"] != "service-unavailable" {
		t.Fatalf("reason = %v, want service-unavailable", answer["reason"])
	}
	if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != consultSavePage {
		t.Fatalf("page.html = %q, want the bytes unchanged", onDisk)
	}
}

func TestControlCheckpointNoService(t *testing.T) {
	root := controlTestRoot(t, "root-noservice")
	control := controlTestServer(t, root)

	status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/checkpoint", map[string]any{
		"opId":         strings.Repeat("1", 32),
		"root":         "root-noservice",
		"name":         "page.html",
		"roomIdentity": "7",
		"generation":   nil,
	})
	if status != http.StatusNotFound {
		t.Fatalf("checkpoint answered %d: %v", status, answer)
	}
}

// TestControlCheckpointAdmitRefused is a checkpoint whose save the service's
// admission refuses: Local's checkpoint throws that refusal (commitSave), so
// Go answers it with its status and code, never as ok, and writes nothing.
func TestControlCheckpointAdmitRefused(t *testing.T) {
	consultShortAborts(t)
	for _, tc := range []struct {
		status int
		code   string
	}{
		{http.StatusConflict, "moved-or-deleted"},
		{http.StatusConflict, "unsaved-edits"},
		{http.StatusInternalServerError, "failed"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			script := newConsultScript(func(path string, body map[string]any, call int) any {
				switch path {
				case "/v1/render-checkpoint":
					return map[string]any{
						"kind":      "render",
						"logical":   map[string]any{"encoding": "utf8", "data": consultSavePage + "<!-- rendered -->"},
						"revision":  nil,
						"captureId": nil,
						"artifacts": []any{},
					}
				case "/v1/admit":
					return map[string]any{
						"decision": "refuse",
						"status":   tc.status,
						"code":     tc.code,
						"body":     map[string]any{"msg": "refused", "msgType": "error", "code": tc.code},
					}
				}
				return nil
			})
			_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
			consultWriteFile(t, dir, "page.html", []byte(consultSavePage))
			control := controlTestServer(t, handler.control)

			status, answer := controlJSON(t, control, controlSecret, http.MethodPost, "/v1/checkpoint", map[string]any{
				"opId":         strings.Repeat("d", 32),
				"root":         "root-consult",
				"name":         "page.html",
				"roomIdentity": "7",
				"generation":   nil,
			})
			if status != tc.status || answer["code"] != tc.code {
				t.Fatalf("checkpoint answered %d %v, want %d with code %s", status, answer, tc.status, tc.code)
			}
			if onDisk := consultTestDisk(t, filepath.Join(dir, "page.html")); onDisk != consultSavePage {
				t.Fatalf("page.html = %q, want the bytes unchanged", onDisk)
			}
		})
	}
}

// TestControlCheckpointLogicalPrev is the document a checkpoint's save is
// judged against: the logical page, sidecar state included, as Local's
// renderCheckpoint passes it, not the stored file part.
func TestControlCheckpointLogicalPrev(t *testing.T) {
	consultShortAborts(t)
	logical := strings.Replace(saveGoldenPage(saveGoldenPageRev), "<article", saveGoldenYJSTag+"\n<article", 1)
	script := newConsultScript(func(path string, body map[string]any, call int) any {
		switch path {
		case "/v1/render-checkpoint":
			return map[string]any{
				"kind":      "render",
				"logical":   map[string]any{"encoding": "utf8", "data": logical},
				"revision":  nil,
				"captureId": nil,
				"artifacts": []any{},
			}
		case "/v1/admit":
			return map[string]any{"decision": "refuse", "status": 409, "code": "unsaved-edits", "body": map[string]any{"code": "unsaved-edits"}}
		}
		return nil
	})
	_, handler, dir := consultSaveServer(t, consultSavePeer(t, script))
	stored, err := storePageState(dir, "page.html", logical, false)
	if err != nil {
		t.Fatal(err)
	}
	if stored == logical {
		t.Fatalf("the fixture kept its page state in the file: %q", stored)
	}
	consultWriteFile(t, dir, "page.html", []byte(stored))
	want, err := logicalBytes(dir, "page.html", []byte(stored))
	if err != nil {
		t.Fatal(err)
	}
	if string(want) == stored {
		t.Fatalf("the logical page equals the stored file: %q", stored)
	}
	control := controlTestServer(t, handler.control)

	controlJSON(t, control, controlSecret, http.MethodPost, "/v1/checkpoint", map[string]any{
		"opId":         strings.Repeat("e", 32),
		"root":         "root-consult",
		"name":         "page.html",
		"roomIdentity": "7",
		"generation":   nil,
	})
	admit := script.only(t, "/v1/admit").Body
	if prev := consultTestData(t, admit["dataLossPrev"], "admit dataLossPrev"); prev != string(want) {
		t.Fatalf("admit dataLossPrev = %q, want the logical page %q", prev, want)
	}
}

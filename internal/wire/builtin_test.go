package wire

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/panphora/htmlclay/internal/builtin"
	"github.com/panphora/htmlclay/internal/config"
)

func builtinRequest(t *testing.T, core *Core, policy *fakePolicy, file, id, helper, payload string) *Sub {
	t.Helper()
	srv := httptest.NewServer(core.WireMux())
	t.Cleanup(srv.Close)
	sub := observer(t, core, file)
	body := fmt.Sprintf(`{"type":"wire/request","id":%q,"helper":%q,"payload":%s}`, id, helper, payload)
	if resp := sendFrame(t, srv, body, "doc.htmlclay"); resp.StatusCode != http.StatusOK {
		t.Fatalf("page request refused: %d", resp.StatusCode)
	}
	return sub
}

func askedBuiltins(policy *fakePolicy) []string {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	return append([]string(nil), policy.askedBuiltin...)
}

func revealedFiles(policy *fakePolicy) []string {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	return append([]string(nil), policy.revealed...)
}

func TestBuiltinOffIsRefused(t *testing.T) {
	core, _, policy, file := newFakeCore(t)
	policy.builtin = builtin.Access{State: builtin.Off}
	sub := builtinRequest(t, core, policy, file, "b1", builtin.FileSearch, `{"query":"x"}`)
	refusal := receiveHelperType(t, sub, "wire/error", "b1")
	if got := helperErrorCode(t, refusal); got != "helper_off" {
		t.Fatalf("code = %q, want helper_off", got)
	}
}

func TestBuiltinAskRaisesTheQuestion(t *testing.T) {
	core, _, policy, file := newFakeCore(t)
	policy.builtin = builtin.Access{State: builtin.Ask}
	sub := builtinRequest(t, core, policy, file, "b1", builtin.FileSearch, `{"query":"x"}`)
	refusal := receiveHelperType(t, sub, "wire/error", "b1")
	if got := helperErrorCode(t, refusal); got != "helper_not_granted" {
		t.Fatalf("code = %q, want helper_not_granted", got)
	}
	if got, want := askedBuiltins(policy), []string{"file-search:" + file}; !slices.Equal(got, want) {
		t.Fatalf("asked = %v, want %v", got, want)
	}
}

func TestBuiltinSearchAnswers(t *testing.T) {
	core, _, policy, file := newFakeCore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a needle here"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy.builtin = builtin.Access{State: builtin.Ready, Root: dir}
	sub := builtinRequest(t, core, policy, file, "b1", builtin.FileSearch, `{"query":"needle"}`)
	receiveHelperType(t, sub, "wire/ack", "b1")
	done := receiveHelperType(t, sub, "wire/done", "b1")
	var payload struct {
		Matches []builtin.SearchMatch `json:"matches"`
	}
	if err := json.Unmarshal(done.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Matches) == 0 || payload.Matches[0].Path != "a.txt" {
		t.Fatalf("matches = %+v, want a.txt first", payload.Matches)
	}
}

func TestBuiltinRemovedIsRefusedQuietly(t *testing.T) {
	core, _, policy, file := newFakeCore(t)
	policy.builtin = builtin.Access{State: builtin.Removed}
	sub := builtinRequest(t, core, policy, file, "b1", builtin.FileSearch, `{"query":"x"}`)
	refusal := receiveHelperType(t, sub, "wire/error", "b1")
	if got := helperErrorCode(t, refusal); got != "helper_not_granted" {
		t.Fatalf("code = %q, want helper_not_granted", got)
	}
	if got := askedBuiltins(policy); len(got) != 0 {
		t.Fatalf("asked = %v, want nothing", got)
	}
}

func TestBuiltinSearchReveals(t *testing.T) {
	core, _, policy, file := newFakeCore(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy.builtin = builtin.Access{State: builtin.Ready, Root: dir}
	sub := builtinRequest(t, core, policy, file, "b1", builtin.FileSearch, `{"reveal":"a.txt"}`)
	receiveHelperType(t, sub, "wire/ack", "b1")
	done := receiveHelperType(t, sub, "wire/done", "b1")
	var payload struct {
		Revealed string `json:"revealed"`
	}
	if err := json.Unmarshal(done.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Revealed != "a.txt" {
		t.Fatalf("revealed = %q, want a.txt", payload.Revealed)
	}
	if got, want := revealedFiles(policy), []string{filepath.Join(dir, "a.txt")}; !slices.Equal(got, want) {
		t.Fatalf("revealed files = %v, want %v", got, want)
	}

	sub = builtinRequest(t, core, policy, file, "b2", builtin.FileSearch, `{"reveal":"../x"}`)
	refusal := receiveHelperType(t, sub, "wire/error", "b2")
	if got := helperErrorCode(t, refusal); got != "invalid_request" {
		t.Fatalf("code = %q, want invalid_request", got)
	}
	if got := revealedFiles(policy); len(got) != 1 {
		t.Fatalf("revealed = %v, want only the first file", got)
	}
}

func TestBuiltinMalformedPayload(t *testing.T) {
	core, _, policy, file := newFakeCore(t)
	policy.builtin = builtin.Access{State: builtin.Ready, Root: t.TempDir()}
	sub := builtinRequest(t, core, policy, file, "b1", builtin.FileSearch, `{"query": 5}`)
	refusal := receiveHelperType(t, sub, "wire/error", "b1")
	if got := helperErrorCode(t, refusal); got != "invalid_request" {
		t.Fatalf("code = %q, want invalid_request", got)
	}
}

func TestBuiltinWatchCancels(t *testing.T) {
	core, _, policy, file := newFakeCore(t)
	policy.builtin = builtin.Access{State: builtin.Ready, Root: t.TempDir()}
	srv := httptest.NewServer(core.WireMux())
	t.Cleanup(srv.Close)
	sub := observer(t, core, file)
	body := fmt.Sprintf(`{"type":"wire/request","id":"w1","helper":%q,"payload":{}}`, builtin.FolderWatch)
	if resp := sendFrame(t, srv, body, "doc.htmlclay"); resp.StatusCode != http.StatusOK {
		t.Fatalf("page request refused: %d", resp.StatusCode)
	}
	receiveHelperType(t, sub, "wire/ack", "w1")
	cancel := `{"type":"wire/cancel","id":"w1"}`
	if resp := sendFrame(t, srv, cancel, "doc.htmlclay"); resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel refused: %d", resp.StatusCode)
	}
	refusal := receiveHelperType(t, sub, "wire/error", "w1")
	if got := helperErrorCode(t, refusal); got != "helper_cancelled" {
		t.Fatalf("code = %q, want helper_cancelled", got)
	}
}

func TestDocumentCannotShadowABuiltin(t *testing.T) {
	core, _, policy, file := newFakeCore(t)
	policy.builtin = builtin.Access{State: builtin.Ask}
	if err := core.AttachHelpers(file, map[string]config.HelperProgram{builtin.FileSearch: helperProgram(builtin.FileSearch, "/nowhere")}, nil); err != nil {
		t.Fatal(err)
	}
	_, meta := core.HelperDiscovery(file)
	count := 0
	for _, entry := range meta {
		if entry.Name != builtin.FileSearch {
			continue
		}
		count++
		if entry.State != "ready" {
			t.Fatalf("state = %q, want ready for a document that may ask", entry.State)
		}
	}
	if count != 1 {
		t.Fatalf("file-search listed %d times, want once", count)
	}
}

package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/session"
)

const pageToolID = "3f1b9c62-0000-4000-8000-1234567890ab"

// withPageToolPrompts puts known tool-generation prompts in front of the server,
// so a test does not depend on the bundled runtime's prompt text.
func withPageToolPrompts(t *testing.T) {
	t.Helper()
	original := pageToolPrompts
	pageToolPrompts = func() (string, bool) { return "HOUSE RULES\n", true }
	t.Cleanup(func() { pageToolPrompts = original })
}

// withoutPageToolPrompts hides the bundled runtime's tool-generation prompts, for a
// test about a server whose runtime carries none.
func withoutPageToolPrompts(t *testing.T) {
	t.Helper()
	original := pageToolPrompts
	pageToolPrompts = func() (string, bool) { return "", false }
	t.Cleanup(func() { pageToolPrompts = original })
}

func pageToolBody(t *testing.T, id, prompt string) string {
	t.Helper()
	encoded, err := json.Marshal(prompt)
	if err != nil {
		t.Fatal(err)
	}
	return `{"type":"wire/request","id":"` + id + `","helper":"blockeditor-tool","document":"none","payload":{"protocol":"blockeditor-tool/1","requestId":"` + id + `","prompt":` + string(encoded) + `,"context":{"title":"Research","outline":[],"tables":[]},"target":null,"current":null}}`
}

func pageToolServer(t *testing.T) (*Server, *session.File) {
	t.Helper()
	t.Setenv("MOCK_MODEL", "1")
	srv, f := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.wire.shutdown() })
	t.Cleanup(srv.CancelAIEdits)
	var on atomic.Bool
	on.Store(true)
	srv.SetHooks(aiEditSwitch(&on))
	withPageToolPrompts(t)
	return srv, f
}

func TestPageToolListing(t *testing.T) {
	srv, f := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.wire.shutdown() })

	// A server with no AI editing hook offers neither builtin.
	for _, entry := range aiEditDiscoveryMeta(t, srv, f.AbsPath) {
		if entry.Name == "blockeditor-tool" {
			t.Fatal("a server with no AI editing hooks offered blockeditor-tool")
		}
	}

	var on atomic.Bool
	on.Store(true)
	srv.SetHooks(aiEditSwitch(&on))

	// The bundled runtime carries the tool prompt files, so the helper is offered.
	if _, ok := readPageToolPrompts(); !ok {
		t.Fatal("the bundled runtime carries no tool-generation prompts")
	}

	// A runtime without the prompt files leaves the helper absent rather than
	// listed unavailable.
	withoutPageToolPrompts(t)
	if meta := aiEditDiscoveryMeta(t, srv, f.AbsPath); len(meta) != 1 || meta[0].Name != "ai-edit" {
		t.Fatalf("discovery without prompt files = %+v", meta)
	}

	withPageToolPrompts(t)
	want := []helperMeta{{Name: "ai-edit", State: "ready"}, {Name: "blockeditor-tool", State: "ready"}}
	if meta := aiEditDiscoveryMeta(t, srv, f.AbsPath); len(meta) != 2 || meta[0] != want[0] || meta[1] != want[1] {
		t.Fatalf("enabled discovery = %+v, want %+v", meta, want)
	}

	on.Store(false)
	want = []helperMeta{{Name: "ai-edit", State: "unavailable"}, {Name: "blockeditor-tool", State: "unavailable"}}
	if meta := aiEditDiscoveryMeta(t, srv, f.AbsPath); len(meta) != 2 || meta[0] != want[0] || meta[1] != want[1] {
		t.Fatalf("disabled discovery = %+v, want %+v", meta, want)
	}
	on.Store(true)

	// A document cannot shadow the builtin by declaring the name.
	srv.helperMu.Lock()
	if srv.helperOffline == nil {
		srv.helperOffline = make(map[string][]string)
	}
	srv.helperOffline[f.AbsPath] = []string{"search", "blockeditor-tool"}
	srv.helperMu.Unlock()
	meta := aiEditDiscoveryMeta(t, srv, f.AbsPath)
	if len(meta) != 3 || meta[0].Name != "search" || meta[1].Name != "ai-edit" || meta[2].Name != "blockeditor-tool" {
		t.Fatalf("declared-plus-built-in discovery = %+v", meta)
	}
}

func TestPageToolSuccessThroughTheHandler(t *testing.T) {
	srv, f := pageToolServer(t)
	sub := helperObserver(t, srv, f.AbsPath)

	aiEditDelivered(t, aiEditSend(t, srv, f.Token, pageToolBody(t, pageToolID, "Make a tally for completed interviews")))

	ack := receiveHelperType(t, sub, "wire/ack", pageToolID)
	var ackPayload struct {
		Mode     string `json:"mode"`
		BudgetMs int64  `json:"budgetMs"`
	}
	if err := json.Unmarshal(ack.Payload, &ackPayload); err != nil {
		t.Fatal(err)
	}
	if ackPayload.Mode != "jsonl" || ackPayload.BudgetMs != pageToolBudget.Milliseconds() {
		t.Fatalf("ack payload = %+v", ackPayload)
	}
	status := receiveHelperEnvelope(t, sub)
	if status.Type != "wire/status" || !strings.HasPrefix(status.Text, "Making your tool") {
		t.Fatalf("first frame after the ack = %s %q", status.Type, status.Text)
	}

	done := receiveHelperType(t, sub, "wire/done", pageToolID)
	var payload struct {
		RequestID string          `json:"requestId"`
		Artifact  json.RawMessage `json:"artifact"`
		State     json.RawMessage `json:"state"`
		Model     string          `json:"model"`
	}
	if err := json.Unmarshal(done.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.RequestID != pageToolID || payload.Model != "mock(claude-opus-5-5)" {
		t.Fatalf("done payload = %+v", payload)
	}
	if !strings.Contains(string(payload.Artifact), `"title":"Counter"`) {
		t.Fatalf("artifact = %s", payload.Artifact)
	}
	if string(payload.State) != `{"count":0}` {
		t.Fatalf("state = %s", payload.State)
	}
}

func TestPageToolAndAIEditShareOneRun(t *testing.T) {
	srv, f := pageToolServer(t)
	sub := helperObserver(t, srv, f.AbsPath)

	aiEditDelivered(t, aiEditSend(t, srv, f.Token, pageToolBody(t, pageToolID, "make a tally [mock:hang]")))
	receiveHelperType(t, sub, "wire/ack", pageToolID)

	// A second tool cannot start either, and the same id is a duplicate.
	second := "3f1b9c62-0000-4000-8000-1234567890ac"
	aiEditDelivered(t, aiEditSend(t, srv, f.Token, pageToolBody(t, second, "make another tally")))
	if env := aiEditEnvelopeType(t, sub, second, 8); env.Type != "wire/error" || helperErrorCode(t, env) != "helper_busy" {
		t.Fatalf("second tool while the first runs = %s %s", env.Type, env.Payload)
	}
	aiEditDelivered(t, aiEditSend(t, srv, f.Token, pageToolBody(t, pageToolID, "make a tally again")))
	if env := aiEditEnvelopeType(t, sub, pageToolID, 8); env.Type != "wire/error" || helperErrorCode(t, env) != "duplicate_request" {
		t.Fatalf("same id while it runs = %s %s", env.Type, env.Payload)
	}

	// An edit cannot start while a tool is being made on the same document.
	aiEditDelivered(t, aiEditSend(t, srv, f.Token, aiEditBody("e1", "tighten")))
	if env := aiEditEnvelopeType(t, sub, "e1", 8); env.Type != "wire/error" || helperErrorCode(t, env) != "helper_busy" {
		t.Fatalf("edit while a tool runs = %s %s", env.Type, env.Payload)
	}

	aiEditDelivered(t, aiEditSend(t, srv, f.Token, `{"type":"wire/cancel","id":"`+pageToolID+`"}`))
	if env := aiEditEnvelopeType(t, sub, pageToolID, 8); env.Type != "wire/error" || helperErrorCode(t, env) != "helper_cancelled" {
		t.Fatalf("cancel answer = %s %s", env.Type, env.Payload)
	}

	// The tool's slot is free again, so an edit runs and then a tool runs.
	aiEditDelivered(t, aiEditSend(t, srv, f.Token, aiEditBody("e2", "tighten [mock:end_turn]")))
	if env := aiEditEnvelopeType(t, sub, "e2", 8); env.Type != "wire/done" {
		t.Fatalf("edit after the tool's slot was released = %s %s", env.Type, env.Payload)
	}
	aiEditDelivered(t, aiEditSend(t, srv, f.Token, pageToolBody(t, pageToolID, "make a tally")))
	if env := aiEditEnvelopeType(t, sub, pageToolID, 8); env.Type != "wire/done" {
		t.Fatalf("tool after the edit = %s %s", env.Type, env.Payload)
	}
}

func TestPageToolTimesOut(t *testing.T) {
	srv, f := pageToolServer(t)
	sub := helperObserver(t, srv, f.AbsPath)

	original := pageToolBudget
	pageToolBudget = 150 * time.Millisecond
	t.Cleanup(func() { pageToolBudget = original })

	aiEditDelivered(t, aiEditSend(t, srv, f.Token, pageToolBody(t, pageToolID, "make a tally [mock:hang]")))
	if env := aiEditEnvelopeType(t, sub, pageToolID, 8); env.Type != "wire/error" || helperErrorCode(t, env) != "helper_timeout" {
		t.Fatalf("budget expiry = %s %s", env.Type, env.Payload)
	}
}

func TestPageToolRefusesAMalformedRequest(t *testing.T) {
	srv, f := pageToolServer(t)
	sub := helperObserver(t, srv, f.AbsPath)

	body := `{"type":"wire/request","id":"m1","helper":"blockeditor-tool","document":"none","payload":{"protocol":"blockeditor-tool/9","requestId":"m1","prompt":"x"}}`
	aiEditDelivered(t, aiEditSend(t, srv, f.Token, body))
	env := aiEditEnvelopeType(t, sub, "m1", 8)
	if env.Type != "wire/error" || helperErrorCode(t, env) != "invalid_request" {
		t.Fatalf("malformed request = %s %s", env.Type, env.Payload)
	}
}

func TestPageToolRefusedWhileAIEditingIsOff(t *testing.T) {
	srv, f := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.wire.shutdown() })
	t.Cleanup(srv.CancelAIEdits)
	var on atomic.Bool
	srv.SetHooks(aiEditSwitch(&on))
	withPageToolPrompts(t)
	sub := helperObserver(t, srv, f.AbsPath)

	aiEditDelivered(t, aiEditSend(t, srv, f.Token, pageToolBody(t, pageToolID, "make a tally")))
	env := aiEditEnvelopeType(t, sub, pageToolID, 8)
	if env.Type != "wire/error" || helperErrorCode(t, env) != "helper_not_granted" {
		t.Fatalf("answer with AI editing off = %s %s", env.Type, env.Payload)
	}
}

// The tool store is a second live store beside `page`, and nothing in it needs a
// plugin: two clients on the local lane write it and read it back, and end on the
// same bytes.
func TestTwoClientsConvergeOnAPageToolStore(t *testing.T) {
	srv, f := setupLiveSyncTest(t)
	pageURL := helperPageURL(srv)

	store := func(tools string) string {
		return `{"tools":` + tools + `}`
	}
	document := func(tools string) string {
		return `<html><head>` + pageYJS +
			`<script type="application/json" merge="blockeditor.tools" clay="live">` + store(tools) + `</script>` +
			`</head><body><article class="blockeditor" data-blockeditor="1"><h1 data-id="title"></h1></article></body></html>`
	}

	tabA := newSubscriber(f.AbsPath, laneLive)
	tabB := newSubscriber(f.AbsPath, laneLive)
	srv.hub.add(tabA)
	srv.hub.add(tabB)

	if w := postLiveSync(t, srv, pageURL, `{"snapshot":`+quote(t, document(`{"t1":{"title":"Tally"}}`))+`,"sender":"tab-a"}`); w.Code != http.StatusOK {
		t.Fatalf("tab a's write = %d %s", w.Code, w.Body.String())
	}
	for name, tab := range map[string]*subscriber{"a": tabA, "b": tabB} {
		html, _ := waitFrame(t, tab, 2*time.Second)["html"].(string)
		if !strings.Contains(html, `merge="blockeditor.tools"`) || !strings.Contains(html, `"t1"`) {
			t.Fatalf("tab %s read %q", name, html)
		}
	}

	last := document(`{"t1":{"title":"Tally"},"t2":{"title":"Interview tally"}}`)
	if w := postLiveSync(t, srv, pageURL, `{"snapshot":`+quote(t, last)+`,"sender":"tab-b"}`); w.Code != http.StatusOK {
		t.Fatalf("tab b's write = %d %s", w.Code, w.Body.String())
	}
	seen := make(map[string]string, 2)
	for name, tab := range map[string]*subscriber{"a": tabA, "b": tabB} {
		html, _ := waitFrame(t, tab, 2*time.Second)["html"].(string)
		seen[name] = html
		if !strings.Contains(html, `"t2"`) {
			t.Fatalf("tab %s read %q", name, html)
		}
	}
	if seen["a"] != seen["b"] || seen["a"] != last {
		t.Fatalf("the tabs did not converge: %q vs %q", seen["a"], seen["b"])
	}
}

func quote(t *testing.T, document string) string {
	t.Helper()
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

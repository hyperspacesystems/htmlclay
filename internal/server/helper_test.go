package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/panphora/htmlclay/internal/wire"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/helper"
)

func writeHelperDocument(t *testing.T, path string, names ...string) {
	t.Helper()
	var metas strings.Builder
	for _, name := range names {
		fmt.Fprintf(&metas, "<meta name=\"htmlclay-helper\" content=\"%s\">", name)
	}
	if err := os.WriteFile(path, []byte("<!doctype html><head>"+metas.String()+"</head><body></body>"), 0644); err != nil {
		t.Fatal(err)
	}
}

func helperObserver(t *testing.T, srv *Server, file string) *wire.Sub {
	t.Helper()
	sub := &wire.Sub{Key: file, Ch: make(chan []byte, 64), Done: make(chan struct{})}
	if _, _, err := srv.Hub.Add(sub, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Hub.Remove(sub) })
	return sub
}

func receiveHelperEnvelope(t *testing.T, sub *wire.Sub) wire.Envelope {
	t.Helper()
	select {
	case raw := <-sub.Ch:
		env, err := wire.DecodeEnvelope(raw)
		if err != nil {
			t.Fatal(err)
		}
		return env
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for helper frame")
		return wire.Envelope{}
	}
}

func receiveHelperType(t *testing.T, sub *wire.Sub, typ, id string) wire.Envelope {
	t.Helper()
	for i := 0; i < 8; i++ {
		env := receiveHelperEnvelope(t, sub)
		if env.Type == typ && env.ID == id {
			return env
		}
	}
	t.Fatalf("did not receive %s for %s", typ, id)
	return wire.Envelope{}
}

func helperErrorCode(t *testing.T, env wire.Envelope) string {
	t.Helper()
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Code
}

func helperProgram(name, path string) config.HelperProgram {
	return config.HelperProgram{ID: "program-" + name, Name: name, Path: path}
}

func TestAttachHelpersDispatchesAndReusesCompletedID(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "search", "ocr")
	t.Setenv(serverHelperArg, filepath.Join(t.TempDir(), "count"))
	program := helperProgram("search", structuredHelper(t, "count"))
	allowed := map[string]config.HelperProgram{"search": program}
	denied := []string{"ocr"}
	if err := srv.AttachHelpers(file.AbsPath, allowed, denied); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })

	srv.HelperMu.Lock()
	first := srv.Helpers[file.AbsPath]
	srv.HelperMu.Unlock()
	generation := srv.HelperGeneration(file.AbsPath)
	if err := srv.AttachHelpers(file.AbsPath, allowed, denied); err != nil {
		t.Fatal(err)
	}
	srv.HelperMu.Lock()
	second := srv.Helpers[file.AbsPath]
	srv.HelperMu.Unlock()
	if first != second || srv.HelperGeneration(file.AbsPath) != generation {
		t.Fatal("idempotent attach replaced the dispatcher or advanced its generation")
	}

	mode, helpers := srv.HelperDiscovery(file.AbsPath)
	if mode != "jsonl" {
		t.Fatalf("mode = %q, want jsonl", mode)
	}
	if want := append([]wire.HelperMeta{{Name: "search", State: "ready"}, {Name: "ocr", State: "denied"}}, builtinUnavailable...); !reflect.DeepEqual(helpers, want) {
		t.Fatalf("helper discovery = %+v, want %+v", helpers, want)
	}
	meta := metaOf(t, srv, file)
	document, ok := meta["document"].(map[string]any)
	if !ok || document["wireMode"] != "jsonl" || document["wireBudgetMs"] != float64(300000) {
		t.Fatalf("wire discovery = %+v", document)
	}
	if listed, ok := document["helpers"].([]any); !ok || len(listed) != 2+len(builtinUnavailable) {
		t.Fatalf("serialized helper discovery = %+v", document["helpers"])
	}

	observer := helperObserver(t, srv, file.AbsPath)
	for want := 1; want <= 2; want++ {
		srv.Hub.Publish(file.AbsPath, wire.Envelope{
			V: 1, Type: "wire/request", ID: "reusable", File: file.AbsPath,
			Helper: "search", Document: "none", Payload: json.RawMessage(`{"query":"clay"}`),
		})
		ack := receiveHelperType(t, observer, "wire/ack", "reusable")
		var accepted struct {
			Mode     string `json:"mode"`
			BudgetMS int    `json:"budgetMs"`
		}
		if err := json.Unmarshal(ack.Payload, &accepted); err != nil {
			t.Fatal(err)
		}
		if accepted.Mode != "jsonl" || accepted.BudgetMS != 300000 {
			t.Fatalf("ack = %+v", accepted)
		}
		done := receiveHelperType(t, observer, "wire/done", "reusable")
		var result struct {
			Count int    `json:"count"`
			Cwd   string `json:"cwd"`
		}
		if err := json.Unmarshal(done.Payload, &result); err != nil {
			t.Fatal(err)
		}
		if result.Count != want || result.Cwd != filepath.Dir(file.AbsPath) {
			t.Fatalf("result = %+v, want count %d and cwd %s", result, want, filepath.Dir(file.AbsPath))
		}
		waitFor(t, time.Second, "the completed request to leave the live set", func() bool {
			first.Mu.Lock()
			defer first.Mu.Unlock()
			_, live := first.Live["reusable"]
			return !live
		})
	}

	srv.Hub.Mu.Lock()
	retained := append([]byte(nil), srv.Hub.Chans[file.AbsPath].Terminal["reusable"].Frame...)
	srv.Hub.Mu.Unlock()
	env, err := wire.DecodeEnvelope(retained)
	if err != nil {
		t.Fatal(err)
	}
	var retainedResult struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(env.Payload, &retainedResult); err != nil {
		t.Fatal(err)
	}
	if retainedResult.Count != 2 {
		t.Fatalf("retained result count = %d, want the reused request's result", retainedResult.Count)
	}
}

func TestDispatcherRefusesNamesWithoutPrompting(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "search", "ocr")
	program := helperProgram("search", structuredHelper(t, "result-null"))
	if err := srv.AttachHelpers(file.AbsPath, map[string]config.HelperProgram{"search": program}, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)

	cases := []struct {
		id     string
		helper string
		code   string
	}{
		{"missing", "", "helper_name_required"},
		{"denied", "ocr", "helper_not_granted"},
		{"undeclared", "other", "helper_not_declared"},
	}
	for _, tc := range cases {
		srv.Hub.Publish(file.AbsPath, wire.Envelope{V: 1, Type: "wire/request", ID: tc.id, File: file.AbsPath, Helper: tc.helper})
		env := receiveHelperType(t, observer, "wire/error", tc.id)
		if code := helperErrorCode(t, env); code != tc.code {
			t.Fatalf("%s code = %q, want %q", tc.id, code, tc.code)
		}
	}
}

func TestExternalRawHandlerStaysAttachedAndRejectsNamedCalls(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	handler := &wire.Sub{Key: file.AbsPath, Handler: true, Ch: make(chan []byte, 4), Done: make(chan struct{})}
	if _, _, err := srv.Hub.Add(handler, 0); err != nil {
		t.Fatal(err)
	}
	program := helperProgram("search", structuredHelper(t, "result-null"))
	if err := srv.AttachHelpers(file.AbsPath, map[string]config.HelperProgram{"search": program}, nil); !errors.Is(err, wire.ErrHelperHandlerTaken) {
		t.Fatalf("AttachHelpers error = %v, want external handler conflict", err)
	}
	if err := srv.AttachHelpers(file.AbsPath, map[string]config.HelperProgram{"search": program}, nil); !errors.Is(err, wire.ErrHelperHandlerTaken) {
		t.Fatalf("repeated AttachHelpers error = %v, want remembered external handler conflict", err)
	}
	mode, helpers := srv.HelperDiscovery(file.AbsPath)
	if want := append([]wire.HelperMeta{{Name: "search", State: "unavailable"}}, builtinUnavailable...); mode != "raw" || !reflect.DeepEqual(helpers, want) {
		t.Fatalf("discovery = mode %q, helpers %+v, want %+v", mode, helpers, want)
	}
	observer := helperObserver(t, srv, file.AbsPath)
	body := fmt.Sprintf(`{"type":"wire/request","id":"named","file":%q,"helper":"search"}`, file.AbsPath)
	req := httptest.NewRequest(http.MethodPost, "/_/wire/send", strings.NewReader(body))
	req.Host = fmt.Sprintf("127.0.0.1:%d", srv.port)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.WireMux().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var dispatch struct {
		Delivered int `json:"delivered"`
		Observers int `json:"observers"`
		Refused   struct {
			Source  string `json:"source"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"refused"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &dispatch); err != nil {
		t.Fatal(err)
	}
	if dispatch.Delivered != 0 || dispatch.Observers != 1 {
		t.Fatalf("dispatch = %+v", dispatch)
	}
	// The reply carries the refusal too. It also goes down the stream, but the
	// two are separate connections with no ordering between them, and a client
	// that settles on "delivered":0 alone reports "nobody is attached" for a
	// handler that was attached and refused for a specific reason.
	if dispatch.Refused.Source != "host" || dispatch.Refused.Code != "helper_protocol_unsupported" || dispatch.Refused.Message == "" {
		t.Fatalf("the POST reply did not carry the typed refusal: %+v", dispatch.Refused)
	}
	if code := helperErrorCode(t, receiveHelperType(t, observer, "wire/error", "named")); code != "helper_protocol_unsupported" {
		t.Fatalf("error code = %q", code)
	}
	select {
	case frame := <-handler.Ch:
		t.Fatalf("raw handler received named request: %q", frame)
	default:
	}
}

func TestRevocationCancelsAcceptedEditAndReleasesWatcher(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	program := helperProgram("search", structuredHelper(t, "hold"))
	if err := srv.AttachHelpers(file.AbsPath, map[string]config.HelperProgram{"search": program}, nil); err != nil {
		t.Fatal(err)
	}
	observer := helperObserver(t, srv, file.AbsPath)
	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "active", File: file.AbsPath,
		Helper: "search", Document: "edit", Payload: json.RawMessage(`{}`),
	})
	receiveHelperType(t, observer, "wire/status", "active")
	waitFor(t, 2*time.Second, "the editing helper watcher lease", func() bool { return watchEntries(srv) == 1 })

	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "active", File: file.AbsPath,
		Helper: "search", Document: "edit", Payload: json.RawMessage(`{}`),
	})
	if code := helperErrorCode(t, receiveHelperType(t, observer, "wire/error", "active")); code != "duplicate_request" {
		t.Fatalf("duplicate code = %q", code)
	}

	srv.DetachHelpers(file.AbsPath)
	waitFor(t, 2*time.Second, "revoked helper cleanup", func() bool { return watchEntries(srv) == 0 })
	if srv.HelpersBound(file.AbsPath) {
		t.Fatal("revoked helper remained bound")
	}
}

func TestEvictedDispatcherCannotPublishIntoAReplacementHandler(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	program := helperProgram("search", structuredHelper(t, "result-null"))
	if err := srv.AttachHelpers(file.AbsPath, map[string]config.HelperProgram{"search": program}, nil); err != nil {
		t.Fatal(err)
	}
	srv.HelperMu.Lock()
	dispatcher := srv.Helpers[file.AbsPath]
	generation := dispatcher.Generation
	srv.Hub.Remove(dispatcher.Sub)
	replacement := &wire.Sub{
		Key: file.AbsPath, Handler: true, Mode: "raw",
		Ch: make(chan []byte, 4), Done: make(chan struct{}),
	}
	if _, _, err := srv.Hub.Add(replacement, 0); err != nil {
		srv.HelperMu.Unlock()
		t.Fatal(err)
	}
	_, _, published := srv.Hub.PublishFromHandler(file.AbsPath, dispatcher.Sub, wire.Envelope{
		Type: "wire/done", ID: "late", File: file.AbsPath, Payload: json.RawMessage(`{"late":true}`),
	})
	if srv.HelperGeneration(file.AbsPath) != generation {
		srv.HelperMu.Unlock()
		t.Fatal("generation changed before dispatcher cleanup reached the binding")
	}
	srv.HelperMu.Unlock()
	if published {
		t.Fatal("evicted dispatcher published after losing its handler slot")
	}
	select {
	case frame := <-replacement.Ch:
		t.Fatalf("replacement handler received the evicted dispatcher's frame: %q", frame)
	default:
	}
}

func TestDispatcherStopsWhenWireShutsDown(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	writeHelperDocument(t, file.AbsPath, "search")
	program := helperProgram("search", structuredHelper(t, "result-null"))
	if err := srv.AttachHelpers(file.AbsPath, map[string]config.HelperProgram{"search": program}, nil); err != nil {
		t.Fatal(err)
	}
	srv.Hub.Shutdown()
	waitFor(t, time.Second, "dispatcher shutdown", func() bool {
		srv.HelperMu.Lock()
		_, attached := srv.Helpers[file.AbsPath]
		srv.HelperMu.Unlock()
		return !attached && !srv.HelpersBound(file.AbsPath)
	})
}

func TestWireHandlerModes(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
		ok    bool
	}{
		{"", "raw", true},
		{"raw", "raw", true},
		{"jsonl", "jsonl", true},
		{"auto", "", false},
	} {
		got, ok := wire.WireHandlerMode(tc.input)
		if got != tc.want || ok != tc.ok {
			t.Errorf("wireHandlerMode(%q) = %q, %v; want %q, %v", tc.input, got, ok, tc.want, tc.ok)
		}
	}
}

// A handler's own output must not come back into its own queue. It used to: the
// owner argument gated authorization but not the fanout, so a helper reporting
// progress faster than the dispatcher drained the echo overflowed its 32-entry
// input queue, was evicted from its own handler slot, and lost the terminal
// frame for the request it was answering. Valid status alone was enough.
func TestAHandlerDoesNotReceiveItsOwnOutput(t *testing.T) {
	wh := wire.NewHub()
	defer wh.Shutdown()
	owner := &wire.Sub{Key: "/doc", Handler: true, Ch: make(chan []byte, wire.WireQueueSize), Done: make(chan struct{})}
	if _, _, err := wh.Add(owner, 0); err != nil {
		t.Fatal(err)
	}
	observer := &wire.Sub{Key: "/doc", Ch: make(chan []byte, wire.WireQueueSize*4), Done: make(chan struct{})}
	if _, _, err := wh.Add(observer, 0); err != nil {
		t.Fatal(err)
	}

	const burst = wire.WireQueueSize + 8
	for i := 0; i < burst; i++ {
		if _, _, published := wh.PublishFromHandler("/doc", owner, wire.Envelope{
			V: 1, Type: "wire/status", ID: "r1", File: "/doc", Text: "tick",
		}); !published {
			t.Fatalf("publish %d was refused, so the handler had already been evicted", i)
		}
	}

	if got := len(owner.Ch); got != 0 {
		t.Errorf("the handler received %d of its own frames, want 0", got)
	}
	if owner.Removed {
		t.Error("a status burst evicted the handler from its own slot")
	}
	// The observers it was publishing FOR still get every frame.
	if got := len(observer.Ch); got != burst {
		t.Errorf("observer received %d frames, want %d", got, burst)
	}
}

// builtinUnavailable is the built-in programs discovery lists, after the
// document's own helpers, for a document that may run none of them.
var builtinUnavailable = []wire.HelperMeta{
	{Name: "file-search", State: "unavailable"},
	{Name: "file-convert", State: "unavailable"},
	{Name: "folder-watch", State: "unavailable"},
}

func TestHelpersComeBackAfterTheExternalHandlerLeaves(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	handler := &wire.Sub{Key: file.AbsPath, Handler: true, Ch: make(chan []byte, 4), Done: make(chan struct{})}
	if _, _, err := srv.Hub.Add(handler, 0); err != nil {
		t.Fatal(err)
	}
	program := helperProgram("search", structuredHelper(t, "result-null"))
	allowed := map[string]config.HelperProgram{"search": program}
	if err := srv.AttachHelpers(file.AbsPath, allowed, nil); !errors.Is(err, wire.ErrHelperHandlerTaken) {
		t.Fatalf("AttachHelpers error = %v, want external handler conflict", err)
	}
	// The terminal running `htmlclay wire serve` is quit, and the document is
	// opened again. Nothing about the remembered conflict is still true.
	srv.Hub.Remove(handler)
	if err := srv.AttachHelpers(file.AbsPath, allowed, nil); err != nil {
		t.Fatalf("AttachHelpers after the external handler left: %v", err)
	}
	mode, helpers := srv.HelperDiscovery(file.AbsPath)
	if want := append([]wire.HelperMeta{{Name: "search", State: "ready"}}, builtinUnavailable...); mode != "jsonl" || !reflect.DeepEqual(helpers, want) {
		t.Fatalf("discovery = mode %q, helpers %+v, want %+v", mode, helpers, want)
	}
}

func TestDiscoveryTellsARefusalApartFromAnUndecidedHelper(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	writeHelperDocument(t, file.AbsPath, "search", "ocr", "translate", "index")
	program := helperProgram("search", structuredHelper(t, "result-null"))
	gone := helperProgram("index", filepath.Join(t.TempDir(), "unregistered"))
	allowed := map[string]config.HelperProgram{"search": program, "index": gone}
	// ocr was refused. translate was never decided: the user closed the picker,
	// or the program behind an older allow has since been unregistered.
	if err := srv.AttachHelpers(file.AbsPath, allowed, []string{"ocr"}); err != nil {
		t.Fatal(err)
	}
	_, helpers := srv.HelperDiscovery(file.AbsPath)
	want := append([]wire.HelperMeta{
		{Name: "search", State: "ready"},
		{Name: "ocr", State: "denied"},
		{Name: "translate", State: "unavailable"},
		{Name: "index", State: "unavailable"},
	}, builtinUnavailable...)
	if !reflect.DeepEqual(helpers, want) {
		t.Fatalf("discovery = %+v, want %+v", helpers, want)
	}

	// A later refusal of a name that was only undecided has to reach discovery,
	// even though the allow set did not move.
	if err := srv.AttachHelpers(file.AbsPath, allowed, []string{"ocr", "translate"}); err != nil {
		t.Fatal(err)
	}
	if _, helpers := srv.HelperDiscovery(file.AbsPath); helpers[2].State != "denied" {
		t.Fatalf("a new refusal did not reach discovery: %+v", helpers)
	}
}

func TestADuplicateRefusalDoesNotBecomeTheLiveRequestsOutcome(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	writeHelperDocument(t, file.AbsPath, "search")
	release := filepath.Join(t.TempDir(), "release")
	t.Setenv(serverHelperArg, release)
	program := helperProgram("search", structuredHelper(t, "release"))
	if err := srv.AttachHelpers(file.AbsPath, map[string]config.HelperProgram{"search": program}, nil); err != nil {
		t.Fatal(err)
	}

	observer := &wire.Sub{Key: file.AbsPath, Ch: make(chan []byte, 64), Done: make(chan struct{})}
	cursor, _, err := srv.Hub.Add(observer, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Hub.Remove(observer) })

	request := wire.Envelope{V: 1, Type: "wire/request", ID: "live", File: file.AbsPath, Helper: "search", Payload: json.RawMessage(`{}`)}
	srv.Hub.Publish(file.AbsPath, request)
	receiveHelperType(t, observer, "wire/status", "live")

	srv.Hub.Publish(file.AbsPath, request)
	if code := helperErrorCode(t, receiveHelperType(t, observer, "wire/error", "live")); code != "duplicate_request" {
		t.Fatalf("the second request was not refused as a duplicate")
	}

	if err := os.WriteFile(release, nil, 0644); err != nil {
		t.Fatal(err)
	}
	receiveHelperType(t, observer, "wire/done", "live")

	// A client that reconnects with a cursor from before the request replays the
	// one remembered outcome for that id. It has to be the answer the program
	// produced, not the refusal handed to whoever reused the id.
	replayed := &wire.Sub{Key: file.AbsPath, Ch: make(chan []byte, 64), Done: make(chan struct{})}
	_, replay, err := srv.Hub.Add(replayed, cursor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Hub.Remove(replayed) })
	if len(replay) != 1 {
		t.Fatalf("replay = %d frames, want the single retained outcome", len(replay))
	}
	env, err := wire.DecodeEnvelope(replay[0])
	if err != nil {
		t.Fatal(err)
	}
	if env.Type != "wire/done" || env.ID != "live" {
		t.Fatalf("replayed terminal = %s for %q, want wire/done for live", env.Type, env.ID)
	}
}

func TestAChildReadsTheDocumentModeTheHostResolved(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	writeHelperDocument(t, file.AbsPath, "search")
	program := helperProgram("search", structuredHelper(t, "document-mode"))
	if err := srv.AttachHelpers(file.AbsPath, map[string]config.HelperProgram{"search": program}, nil); err != nil {
		t.Fatal(err)
	}
	observer := helperObserver(t, srv, file.AbsPath)

	// The page omits document, which the host reads as "none" and enforces. The
	// child has to be told the same thing rather than left to infer it from a
	// field that is not there.
	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "unstated", File: file.AbsPath,
		Helper: "search", Payload: json.RawMessage(`{}`),
	})
	env := receiveHelperEnvelope(t, observer)
	for !env.IsTerminal() {
		env = receiveHelperEnvelope(t, observer)
	}
	if env.Type != "wire/done" {
		t.Fatalf("terminal = %s: %s", env.Type, env.Text)
	}
}

func TestAnUndecidedHelperWaitsForSetupThenRuns(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	t.Setenv(serverHelperArg, filepath.Join(t.TempDir(), "count"))
	program := helperProgram("search", structuredHelper(t, "count"))
	var calls atomic.Int32
	entered := make(chan struct{}, 1)
	proceed := make(chan struct{})
	ready := make(chan *wire.Sub, 1)
	srv.SetHooks(Hooks{AskHelper: func(path, name string) bool {
		calls.Add(1)
		entered <- struct{}{}
		<-proceed
		if err := srv.AttachHelpers(path, map[string]config.HelperProgram{name: program}, nil); err != nil {
			t.Errorf("reattach after approval: %v", err)
			return false
		}
		sub := &wire.Sub{Key: path, Ch: make(chan []byte, 64), Done: make(chan struct{})}
		if _, _, err := srv.Hub.Add(sub, 0); err != nil {
			t.Errorf("observe after approval: %v", err)
			return false
		}
		ready <- sub
		return true
	}})
	if err := srv.AttachHelpers(file.AbsPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)

	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "first", File: file.AbsPath,
		Helper: "search", Document: "none", Payload: json.RawMessage(`{"query":"clay"}`),
	})
	ack := receiveHelperType(t, observer, "wire/ack", "first")
	var accepted struct {
		Mode     string `json:"mode"`
		BudgetMS int    `json:"budgetMs"`
	}
	if err := json.Unmarshal(ack.Payload, &accepted); err != nil {
		t.Fatal(err)
	}
	budget := int(wire.HelperParkLimit/time.Millisecond) + helper.StructuredDeadline
	if accepted.Mode != "jsonl" || accepted.BudgetMS != budget {
		t.Fatalf("parked ack = %+v, want jsonl with a %dms budget that covers the wait", accepted, budget)
	}
	status := receiveHelperType(t, observer, "wire/status", "first")
	if status.Text != "Waiting for you to set up search in HTML Clay" {
		t.Fatalf("status text = %q", status.Text)
	}
	if calls.Load() != 0 {
		t.Fatalf("AskHelper calls = %d, want none until setup starts", calls.Load())
	}

	setUp := make(chan bool, 1)
	go func() {
		granted, _ := srv.SetUpHelper(file.AbsPath, "search")
		setUp <- granted
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the setup request was never raised")
	}
	close(proceed)
	var doneSub *wire.Sub
	select {
	case doneSub = <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("no wire subscription after the approval reattached the helpers")
	}
	t.Cleanup(func() { srv.Hub.Remove(doneSub) })
	if granted := <-setUp; !granted {
		t.Fatal("setup did not grant search")
	}
	receiveHelperType(t, doneSub, "wire/done", "first")
	if calls.Load() != 1 {
		t.Fatalf("AskHelper calls = %d, want 1", calls.Load())
	}
}

func TestOneDialogServesEveryRequestForTheSameHelper(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	program := helperProgram("search", structuredHelper(t, "result-null"))
	release := make(chan struct{})
	ready := make(chan *wire.Sub, 1)
	var calls atomic.Int32
	srv.SetHooks(Hooks{AskHelper: func(path, name string) bool {
		calls.Add(1)
		<-release
		if err := srv.AttachHelpers(path, map[string]config.HelperProgram{name: program}, nil); err != nil {
			t.Errorf("reattach after approval: %v", err)
			return false
		}
		sub := &wire.Sub{Key: path, Ch: make(chan []byte, 64), Done: make(chan struct{})}
		if _, _, err := srv.Hub.Add(sub, 0); err != nil {
			t.Errorf("observe after approval: %v", err)
			return false
		}
		ready <- sub
		return true
	}})
	if err := srv.AttachHelpers(file.AbsPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)

	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "one", File: file.AbsPath,
		Helper: "search", Payload: json.RawMessage(`{}`),
	})
	receiveHelperType(t, observer, "wire/ack", "one")
	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "two", File: file.AbsPath,
		Helper: "search", Payload: json.RawMessage(`{}`),
	})
	receiveHelperType(t, observer, "wire/ack", "two")
	if calls.Load() != 0 {
		t.Fatalf("AskHelper calls = %d, want none until setup starts", calls.Load())
	}

	settled := make(chan bool, 1)
	go func() {
		granted, _ := srv.SetUpHelper(file.AbsPath, "search")
		settled <- granted
	}()
	close(release)
	var doneSub *wire.Sub
	select {
	case doneSub = <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("no wire subscription after the approval reattached the helpers")
	}
	t.Cleanup(func() { srv.Hub.Remove(doneSub) })
	seen := make(map[string]bool)
	for len(seen) < 2 {
		env := receiveHelperEnvelope(t, doneSub)
		if env.Type == "wire/done" {
			seen[env.ID] = true
		}
	}
	if !seen["one"] || !seen["two"] {
		t.Fatalf("completed requests = %+v, want both one and two", seen)
	}
	if granted := <-settled; !granted {
		t.Fatal("setup did not grant search")
	}
	if calls.Load() != 1 {
		t.Fatalf("AskHelper calls = %d, want one dialog for both requests", calls.Load())
	}
}

func TestADeniedForThisOpenHelperIsNeverAskedAbout(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	structuredHelper(t, "result-null")
	var calls atomic.Int32
	srv.SetHooks(Hooks{AskHelper: func(string, string) bool {
		calls.Add(1)
		return true
	}})
	if err := srv.AttachHelpers(file.AbsPath, nil, []string{"search"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)

	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "denied", File: file.AbsPath,
		Helper: "search", Payload: json.RawMessage(`{}`),
	})
	if code := helperErrorCode(t, receiveHelperType(t, observer, "wire/error", "denied")); code != "helper_not_granted" {
		t.Fatalf("code = %q, want helper_not_granted", code)
	}
	if calls.Load() != 0 {
		t.Fatalf("AskHelper calls = %d, want none for a name refused this open", calls.Load())
	}
}

func TestADialogThatGrantsNothingQuietsThePageForAMinute(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	structuredHelper(t, "result-null")
	var calls atomic.Int32
	srv.SetHooks(Hooks{AskHelper: func(string, string) bool {
		calls.Add(1)
		return false
	}})
	if err := srv.AttachHelpers(file.AbsPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)

	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "first", File: file.AbsPath,
		Helper: "search", Payload: json.RawMessage(`{}`),
	})
	receiveHelperType(t, observer, "wire/ack", "first")
	receiveHelperType(t, observer, "wire/status", "first")
	if calls.Load() != 0 {
		t.Fatalf("AskHelper calls = %d, want none until setup starts", calls.Load())
	}
	if granted, ran := srv.SetUpHelper(file.AbsPath, "search"); granted || !ran {
		t.Fatalf("setup = granted %v, ran %v, want a refused dialog", granted, ran)
	}
	if code := helperErrorCode(t, receiveHelperType(t, observer, "wire/error", "first")); code != "helper_not_granted" {
		t.Fatalf("first code = %q, want helper_not_granted", code)
	}
	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "second", File: file.AbsPath,
		Helper: "search", Payload: json.RawMessage(`{}`),
	})
	if code := helperErrorCode(t, receiveHelperType(t, observer, "wire/error", "second")); code != "helper_not_granted" {
		t.Fatalf("second code = %q, want helper_not_granted", code)
	}
	if calls.Load() != 1 {
		t.Fatalf("AskHelper calls = %d, want the page to wait after a dialog that granted nothing", calls.Load())
	}
}

func TestLoadingThePageNeverAsksAboutAnUndecidedHelper(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	structuredHelper(t, "result-null")
	var calls atomic.Int32
	srv.SetHooks(Hooks{AskHelper: func(string, string) bool {
		calls.Add(1)
		return true
	}})
	if err := srv.AttachHelpers(file.AbsPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)

	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/describe", ID: "describe", File: file.AbsPath,
		Helper: "search", Payload: json.RawMessage(`{}`),
	})
	if code := helperErrorCode(t, receiveHelperType(t, observer, "wire/error", "describe")); code != "helper_not_granted" {
		t.Fatalf("describe code = %q, want helper_not_granted", code)
	}
	if calls.Load() != 0 {
		t.Fatalf("AskHelper calls = %d, want none for wire/describe", calls.Load())
	}
}

func TestARequestCancelledWhileParkedNeverRuns(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	countPath := filepath.Join(t.TempDir(), "count")
	t.Setenv(serverHelperArg, countPath)
	program := helperProgram("search", structuredHelper(t, "count"))
	var calls atomic.Int32
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	srv.SetHooks(Hooks{AskHelper: func(path, name string) bool {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		if err := srv.AttachHelpers(path, map[string]config.HelperProgram{name: program}, nil); err != nil {
			t.Errorf("reattach after approval: %v", err)
			return false
		}
		return true
	}})
	if err := srv.AttachHelpers(file.AbsPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)

	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "parked", File: file.AbsPath,
		Helper: "search", Payload: json.RawMessage(`{}`),
	})
	receiveHelperType(t, observer, "wire/ack", "parked")
	if calls.Load() != 0 {
		t.Fatalf("AskHelper calls = %d, want none until setup starts", calls.Load())
	}
	settled := make(chan struct{})
	go func() {
		srv.SetUpHelper(file.AbsPath, "search")
		close(settled)
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the setup request was never raised")
	}
	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/cancel", ID: "parked", File: file.AbsPath,
	})
	// The dispatcher handles the cancel on its own goroutine. Approving before it has
	// is a different case (a cancel after start stops the running program), so wait.
	waitFor(t, time.Second, "the cancel to reach the parked request", func() bool {
		srv.HelperMu.Lock()
		defer srv.HelperMu.Unlock()
		for _, p := range srv.HelperAsking[wire.HelperAskKey(file.AbsPath, "search")] {
			if p.Env.ID == "parked" {
				return false
			}
		}
		return true
	})
	close(release)
	waitFor(t, time.Second, "the parked request to be released", func() bool {
		srv.HelperMu.Lock()
		defer srv.HelperMu.Unlock()
		_, asking := srv.HelperAsking[wire.HelperAskKey(file.AbsPath, "search")]
		return !asking
	})
	waitFor(t, time.Second, "the setup to finish", func() bool {
		select {
		case <-settled:
			return true
		default:
			return false
		}
	})
	if _, err := os.Stat(countPath); err == nil {
		t.Fatal("a request cancelled while parked ran the helper program")
	}
}

func TestRegrantKeepsARunningRequestAlive(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "a", "b")
	releasePath := filepath.Join(t.TempDir(), "release")
	t.Setenv(serverHelperArg, releasePath)
	exe := structuredHelper(t, "release")
	aProgram := helperProgram("a", exe)
	bProgram := helperProgram("b", exe)
	if err := srv.AttachHelpers(file.AbsPath, map[string]config.HelperProgram{"a": aProgram}, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)

	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "run", File: file.AbsPath,
		Helper: "a", Document: "none", Payload: json.RawMessage(`{}`),
	})
	receiveHelperType(t, observer, "wire/ack", "run")

	srv.HelperMu.Lock()
	before := srv.Helpers[file.AbsPath]
	srv.HelperMu.Unlock()
	if err := srv.AttachHelpers(file.AbsPath, map[string]config.HelperProgram{"a": aProgram, "b": bProgram}, nil); err != nil {
		t.Fatal(err)
	}
	srv.HelperMu.Lock()
	after := srv.Helpers[file.AbsPath]
	srv.HelperMu.Unlock()
	if after != before {
		t.Fatal("regranting an unchanged declaration replaced the dispatcher")
	}
	if err := os.WriteFile(releasePath, []byte("go\n"), 0644); err != nil {
		t.Fatal(err)
	}
	receiveHelperType(t, observer, "wire/done", "run")
}

func TestRegrantCancelsARunningRequestThatLostItsGrant(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "a", "b")
	t.Setenv(serverHelperArg, filepath.Join(t.TempDir(), "release"))
	exe := structuredHelper(t, "release")
	aProgram := helperProgram("a", exe)
	bProgram := helperProgram("b", exe)
	if err := srv.AttachHelpers(file.AbsPath, map[string]config.HelperProgram{"a": aProgram, "b": bProgram}, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)

	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "run", File: file.AbsPath,
		Helper: "a", Document: "none", Payload: json.RawMessage(`{}`),
	})
	receiveHelperType(t, observer, "wire/ack", "run")

	if err := srv.AttachHelpers(file.AbsPath, map[string]config.HelperProgram{"b": bProgram}, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	env := receiveHelperType(t, observer, "wire/error", "run")
	if code := helperErrorCode(t, env); code != "helper_cancelled" {
		t.Fatalf("terminal code = %q, want helper_cancelled", code)
	}
}

func TestAQuietFileRefusesAnotherUndecidedNameWithoutAsking(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "a", "b")
	structuredHelper(t, "result-null")
	var calls atomic.Int32
	srv.SetHooks(Hooks{AskHelper: func(string, string) bool {
		calls.Add(1)
		return false
	}})
	if err := srv.AttachHelpers(file.AbsPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)

	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "first", File: file.AbsPath,
		Helper: "a", Payload: json.RawMessage(`{}`),
	})
	receiveHelperType(t, observer, "wire/ack", "first")
	if calls.Load() != 0 {
		t.Fatalf("AskHelper calls = %d, want none until setup starts", calls.Load())
	}
	if _, ran := srv.SetUpHelper(file.AbsPath, "a"); !ran {
		t.Fatal("the setup request never ran")
	}
	if code := helperErrorCode(t, receiveHelperType(t, observer, "wire/error", "first")); code != "helper_not_granted" {
		t.Fatalf("first code = %q, want helper_not_granted", code)
	}
	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "second", File: file.AbsPath,
		Helper: "b", Payload: json.RawMessage(`{}`),
	})
	if code := helperErrorCode(t, receiveHelperType(t, observer, "wire/error", "second")); code != "helper_not_granted" {
		t.Fatalf("second code = %q, want helper_not_granted", code)
	}
	if calls.Load() != 1 {
		t.Fatalf("AskHelper calls = %d, want the whole file to be quiet after one refusal", calls.Load())
	}
}

func TestAParkedRequestIsAnsweredWhenTheDispatcherIsGone(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	structuredHelper(t, "result-null")
	reconnected := make(chan *wire.Sub, 1)
	srv.SetHooks(Hooks{AskHelper: func(path, name string) bool {
		srv.DetachHelpers(path)
		sub := &wire.Sub{Key: path, Ch: make(chan []byte, 64), Done: make(chan struct{})}
		if _, _, err := srv.Hub.Add(sub, 0); err != nil {
			t.Errorf("reconnect after detach: %v", err)
		}
		reconnected <- sub
		return false
	}})
	if err := srv.AttachHelpers(file.AbsPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)

	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "parked", File: file.AbsPath,
		Helper: "search", Payload: json.RawMessage(`{}`),
	})
	receiveHelperType(t, observer, "wire/ack", "parked")

	settled := make(chan struct{})
	go func() {
		srv.SetUpHelper(file.AbsPath, "search")
		close(settled)
	}()
	var page *wire.Sub
	select {
	case page = <-reconnected:
	case <-time.After(2 * time.Second):
		t.Fatal("the setup never detached the dispatcher")
	}
	t.Cleanup(func() { srv.Hub.Remove(page) })
	env := receiveHelperType(t, page, "wire/error", "parked")
	if code := helperErrorCode(t, env); code != "helper_not_granted" {
		t.Fatalf("terminal code = %q, want helper_not_granted", code)
	}
	waitFor(t, time.Second, "the setup to finish", func() bool {
		select {
		case <-settled:
			return true
		default:
			return false
		}
	})
}

func TestAMalformedRequestIsRefusedBeforeItCanAsk(t *testing.T) {
	srv, file := setupLiveSyncTest(t)
	t.Cleanup(func() { srv.Hub.Shutdown() })
	writeHelperDocument(t, file.AbsPath, "search")
	structuredHelper(t, "result-null")
	var calls atomic.Int32
	srv.SetHooks(Hooks{AskHelper: func(string, string) bool {
		calls.Add(1)
		return false
	}})
	if err := srv.AttachHelpers(file.AbsPath, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.DetachHelpers(file.AbsPath) })
	observer := helperObserver(t, srv, file.AbsPath)

	srv.Hub.Publish(file.AbsPath, wire.Envelope{
		V: 1, Type: "wire/request", ID: "bad", File: file.AbsPath,
		Helper: "search", Document: "bogus", Payload: json.RawMessage(`{}`),
	})
	if code := helperErrorCode(t, receiveHelperType(t, observer, "wire/error", "bad")); code != "invalid_document" {
		t.Fatalf("code = %q, want invalid_document", code)
	}
	if calls.Load() != 0 {
		t.Fatalf("AskHelper calls = %d, want none for a malformed request", calls.Load())
	}
}

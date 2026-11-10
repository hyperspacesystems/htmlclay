//go:build managedtrial

package managedtrial

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// writeBootstrapSource is the Local checkout Run insists on, without the
// descriptor tricks the control-fd tests need: this file builds on Windows.
func writeBootstrapSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	main := filepath.Join(dir, "src", "main")
	if err := os.MkdirAll(filepath.Join(main, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(main, "server.js"), []byte("// server\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(main, "assets", "sync-worker.js"), []byte("// sync worker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runtimeDir := filepath.Join(main, "offline-runtime")
	if err := os.MkdirAll(filepath.Join(runtimeDir, "1.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":1,"selected":"1.0.0","versions":["1.0.0"]}`
	if err := os.WriteFile(filepath.Join(runtimeDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	index := `{"schema":1,"version":"1.0.0","files":{"clay.js":{"sha256":"` + strings.Repeat("0", 64) + `","type":"application/javascript","bytes":1}},"aliases":{}}`
	if err := os.WriteFile(filepath.Join(runtimeDir, "1.0.0", "offline-manifest.json"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// readBootstrapReady waits for the READY line on stdout, or for Run to return
// first, which is the failure the caller needs to see.
func readBootstrapReady(t *testing.T, r io.Reader, done <-chan error) readyLine {
	t.Helper()
	type readResult struct {
		line string
		err  error
	}
	lines := make(chan readResult, 1)
	go func() {
		line, err := bufio.NewReader(r).ReadString('\n')
		lines <- readResult{line, err}
	}()
	var res readResult
	select {
	case res = <-lines:
	case err := <-done:
		t.Fatalf("Run = %v before the READY line", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the READY line")
	}
	if res.err != nil {
		t.Fatalf("reading READY: %v", res.err)
	}
	if !strings.HasPrefix(res.line, "READY ") || !strings.HasSuffix(res.line, "\n") {
		t.Fatalf("stdout line = %q, want one READY line", res.line)
	}
	var ready readyLine
	if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(res.line, "READY "))), &ready); err != nil {
		t.Fatalf("READY %q: %v", res.line, err)
	}
	return ready
}

// bootstrapLine is one --bootstrap-stdin line with the given secret and service
// value, already encoded.
func bootstrapLineJSON(t *testing.T, secret string, service any) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"v": 1, "secret": secret, "service": service})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded) + "\n"
}

func TestBootstrapLine(t *testing.T) {
	source := writeBootstrapSource(t)
	root := t.TempDir()
	secret := strings.Repeat("0123456789abcdef", 4)
	line := bootstrapLineJSON(t, secret, nil)

	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	t.Cleanup(func() { stdinWriter.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The pipe has no buffer: the line can only be written once Run reads it, so
	// the writes and the run are two goroutines.
	go func() { _, _ = io.WriteString(stdinWriter, line) }()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, []string{
			"--root", root,
			"--local-source", source,
			"--bootstrap-stdin",
		}, stdinReader, stdoutWriter, io.Discard)
	}()

	ready := readBootstrapReady(t, stdoutReader, done)
	if !serviceEpochPattern.MatchString(ready.Epoch) {
		t.Fatalf("READY epoch = %q, want 16 lowercase hex", ready.Epoch)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	request, err := http.NewRequest(http.MethodGet, ready.Control+"/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", controlScheme+secret)
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /health with the bootstrap secret = %d, want 200", response.StatusCode)
	}

	stdinWriter.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after stdin reached EOF")
	}
}

func TestBootstrapLineRefusals(t *testing.T) {
	source := writeBootstrapSource(t)
	root := t.TempDir()
	goodSecret := strings.Repeat("0123456789abcdef", 4)
	goodService := map[string]any{
		"url":    "http://127.0.0.1:9",
		"secret": strings.Repeat("ab", 32),
		"epoch":  "0123456789abcdef",
	}

	cases := []struct {
		name string
		line string
		args []string
		want string
	}{
		{
			name: "short secret",
			line: bootstrapLineJSON(t, strings.Repeat("a", 63), nil),
			want: "--bootstrap-stdin",
		},
		{
			name: "unknown version",
			line: `{"v":2,"secret":"` + goodSecret + `","service":null}` + "\n",
			want: "unknown bootstrap version",
		},
		{
			name: "service url is not loopback",
			line: bootstrapLineJSON(t, goodSecret, map[string]any{
				"url":    "http://example.com:1",
				"secret": goodService["secret"],
				"epoch":  goodService["epoch"],
			}),
			want: "the service needs",
		},
		{
			name: "both transports",
			line: bootstrapLineJSON(t, goodSecret, goodService),
			args: []string{"--control-fd", "3"},
			want: "use one of",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--root", root, "--local-source", source, "--bootstrap-stdin"}, tc.args...)
			err := Run(context.Background(), args, strings.NewReader(tc.line), io.Discard, io.Discard)
			if err == nil {
				t.Fatalf("Run accepted %v", tc.line)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

func TestServiceCall(t *testing.T) {
	peerSecret := strings.Repeat("ab", 32)
	peerEpoch := "0123456789abcdef"

	t.Run("a call carries the secret and both epochs", func(t *testing.T) {
		var authorization, epoch, peerEpochHeader string
		var body map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authorization = r.Header.Get("Authorization")
			epoch = r.Header.Get("X-Trial-Epoch")
			peerEpochHeader = r.Header.Get("X-Trial-Peer-Epoch")
			body = map[string]any{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			writeControlJSON(w, http.StatusOK, orderedObject{{Key: "ok", Value: true}})
		}))
		t.Cleanup(server.Close)

		ring := newEventRing()
		link := newServiceLink(ring, servicePeer{URL: server.URL, Secret: peerSecret, Epoch: peerEpoch})
		var answer struct {
			OK bool `json:"ok"`
		}
		if err := link.consult("/v1/consult", map[string]string{"consultId": "c1"}, &answer); err != nil {
			t.Fatalf("consult = %v, want nil", err)
		}
		if !answer.OK {
			t.Fatal("the decoded answer lost its ok field")
		}
		if authorization != controlScheme+peerSecret {
			t.Fatalf("Authorization = %q, want %q", authorization, controlScheme+peerSecret)
		}
		if epoch != ring.epoch {
			t.Fatalf("X-Trial-Epoch = %q, want the ring epoch %q", epoch, ring.epoch)
		}
		if peerEpochHeader != peerEpoch {
			t.Fatalf("X-Trial-Peer-Epoch = %q, want %q", peerEpochHeader, peerEpoch)
		}
		if body["consultId"] != "c1" {
			t.Fatalf("request body = %v, want consultId c1", body)
		}
	})

	t.Run("a 409 epoch answer is a serviceEpochError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeControlJSON(w, http.StatusConflict, orderedObject{
				{Key: "error", Value: "epoch"},
				{Key: "epoch", Value: "abcdabcdabcdabcd"},
			})
		}))
		t.Cleanup(server.Close)

		link := newServiceLink(newEventRing(), servicePeer{URL: server.URL, Secret: peerSecret, Epoch: peerEpoch})
		err := link.consult("/v1/consult", map[string]string{}, nil)
		epochErr, ok := err.(*serviceEpochError)
		if !ok {
			t.Fatalf("consult = %v, want *serviceEpochError", err)
		}
		if epochErr.epoch != "abcdabcdabcdabcd" {
			t.Fatalf("epoch = %q, want abcdabcdabcdabcd", epochErr.epoch)
		}
	})

	t.Run("a deadline is service-unavailable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			writeControlJSON(w, http.StatusOK, orderedObject{{Key: "ok", Value: true}})
		}))
		t.Cleanup(server.Close)

		previous := consultDeadline
		consultDeadline = 50 * time.Millisecond
		t.Cleanup(func() { consultDeadline = previous })

		link := newServiceLink(newEventRing(), servicePeer{URL: server.URL, Secret: peerSecret, Epoch: peerEpoch})
		if err := link.consult("/v1/consult", map[string]string{}, nil); err != errServiceUnavailable {
			t.Fatalf("consult = %v, want %v", err, errServiceUnavailable)
		}
	})

	t.Run("a closed server is service-unavailable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		address := server.URL
		server.Close()

		link := newServiceLink(newEventRing(), servicePeer{URL: address, Secret: peerSecret, Epoch: peerEpoch})
		if err := link.consult("/v1/consult", map[string]string{}, nil); err != errServiceUnavailable {
			t.Fatalf("consult = %v, want %v", err, errServiceUnavailable)
		}
	})

	t.Run("no peer is service-unavailable", func(t *testing.T) {
		link := newServiceLink(newEventRing(), servicePeer{})
		if err := link.consult("/v1/consult", map[string]string{}, nil); err != errServiceUnavailable {
			t.Fatalf("consult = %v, want %v", err, errServiceUnavailable)
		}
	})
}

func TestAbortRetries(t *testing.T) {
	previousInterval := abortRetryInterval
	abortRetryInterval = 10 * time.Millisecond
	t.Cleanup(func() { abortRetryInterval = previousInterval })
	previousDeadline := consultDeadline
	consultDeadline = time.Second
	t.Cleanup(func() { consultDeadline = previousDeadline })

	peerSecret := strings.Repeat("ab", 32)

	t.Run("settle-failed is resent until the service answers", func(t *testing.T) {
		var count int32
		var mu sync.Mutex
		var bodies []map[string]string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := atomic.AddInt32(&count, 1)
			body := map[string]string{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			bodies = append(bodies, body)
			mu.Unlock()
			if n <= 2 {
				writeControlJSON(w, http.StatusInternalServerError, controlError{Error: "busy"})
				return
			}
			writeControlJSON(w, http.StatusOK, orderedObject{{Key: "ok", Value: true}})
		}))
		t.Cleanup(server.Close)

		link := newServiceLink(newEventRing(), servicePeer{URL: server.URL, Secret: peerSecret, Epoch: "0123456789abcdef"})
		link.abort("consult-1", "settle-failed")

		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) && atomic.LoadInt32(&count) < 3 {
			time.Sleep(5 * time.Millisecond)
		}
		if got := atomic.LoadInt32(&count); got != 3 {
			t.Fatalf("abort requests = %d, want 3", got)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(bodies) != 3 {
			t.Fatalf("recorded %d bodies, want 3", len(bodies))
		}
		for _, body := range bodies {
			if body["consultId"] != "consult-1" || body["reason"] != "settle-failed" {
				t.Fatalf("abort body = %v, want consultId consult-1 and reason settle-failed", body)
			}
		}
	})

	t.Run("any other reason is sent once", func(t *testing.T) {
		var count int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&count, 1)
			writeControlJSON(w, http.StatusInternalServerError, controlError{Error: "busy"})
		}))
		t.Cleanup(server.Close)

		link := newServiceLink(newEventRing(), servicePeer{URL: server.URL, Secret: peerSecret, Epoch: "0123456789abcdef"})
		link.abort("consult-2", "deadline")
		time.Sleep(200 * time.Millisecond)
		if got := atomic.LoadInt32(&count); got != 1 {
			t.Fatalf("abort requests = %d, want 1", got)
		}
	})

	t.Run("a new service epoch stops the retries", func(t *testing.T) {
		var count int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&count, 1)
			writeControlJSON(w, http.StatusInternalServerError, controlError{Error: "busy"})
		}))
		t.Cleanup(server.Close)

		link := newServiceLink(newEventRing(), servicePeer{URL: server.URL, Secret: peerSecret, Epoch: "0123456789abcdef"})
		link.abort("consult-3", "settle-failed")
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) && atomic.LoadInt32(&count) < 1 {
			time.Sleep(5 * time.Millisecond)
		}
		link.setPeer(servicePeer{URL: server.URL, Secret: peerSecret, Epoch: "ffffffffffffffff"})
		time.Sleep(60 * time.Millisecond)
		settled := atomic.LoadInt32(&count)
		time.Sleep(100 * time.Millisecond)
		if got := atomic.LoadInt32(&count); got != settled {
			t.Fatalf("abort requests grew from %d to %d after the epoch changed", settled, got)
		}
	})
}

func TestUnconfirmedRoutes(t *testing.T) {
	root := controlTestRoot(t, "root-0")
	link := newServiceLink(root.events, servicePeer{})
	server := httptest.NewServer(newControlHandler(controlSecret, []*controlRoot{root}, link))
	t.Cleanup(server.Close)

	filePath, err := resolveWritePath(root.registry, "doc.html")
	if err != nil {
		t.Fatal(err)
	}
	link.markUnconfirmed(filePath, "root-0", "doc.html")
	link.markUnconfirmed(filePath, "root-0", "doc.html")

	unconfirmedFrames := 0
	for _, frame := range root.events.heldFrames() {
		decoded := map[string]any{}
		if err := json.Unmarshal([]byte(frame), &decoded); err != nil {
			t.Fatalf("frame %q: %v", frame, err)
		}
		if decoded["type"] == "unconfirmed" {
			unconfirmedFrames++
			if decoded["root"] != "root-0" || decoded["rel"] != "doc.html" {
				t.Fatalf("unconfirmed frame = %v", decoded)
			}
		}
	}
	if unconfirmedFrames != 1 {
		t.Fatalf("unconfirmed frames = %d, want 1", unconfirmedFrames)
	}

	status, answer := controlJSON(t, server, controlSecret, http.MethodGet, "/v1/unconfirmed", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /v1/unconfirmed = %d, want 200", status)
	}
	documents, ok := answer["documents"].([]any)
	if !ok || len(documents) != 1 {
		t.Fatalf("GET /v1/unconfirmed = %v, want one document", answer)
	}
	entry, _ := documents[0].(map[string]any)
	if entry["root"] != "root-0" || entry["name"] != "doc.html" {
		t.Fatalf("unconfirmed document = %v", entry)
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodPost, "/v1/reconciled", map[string]string{
		"root": "root-0",
		"name": "doc.html",
	})
	if status != http.StatusOK || answer["ok"] != true {
		t.Fatalf("POST /v1/reconciled = %d %v, want 200 ok", status, answer)
	}
	if link.unconfirmed.has(filePath) {
		t.Fatal("the reconciled document is still unconfirmed")
	}

	status, answer = controlJSON(t, server, controlSecret, http.MethodGet, "/v1/unconfirmed", nil)
	if status != http.StatusOK {
		t.Fatalf("GET /v1/unconfirmed = %d, want 200", status)
	}
	if documents, ok := answer["documents"].([]any); !ok || len(documents) != 0 {
		t.Fatalf("GET /v1/unconfirmed = %v, want none", answer)
	}

	bare := httptest.NewServer(newControlHandler(controlSecret, []*controlRoot{root}, nil))
	t.Cleanup(bare.Close)
	for _, route := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/v1/peer", map[string]string{}},
		{http.MethodPost, "/v1/reconciled", map[string]string{"root": "root-0", "name": "doc.html"}},
		{http.MethodGet, "/v1/unconfirmed", nil},
	} {
		status, answer := controlJSON(t, bare, controlSecret, route.method, route.path, route.body)
		if status != http.StatusNotFound {
			t.Fatalf("%s %s = %d, want 404", route.method, route.path, status)
		}
		if answer["error"] != "no service is attached" {
			t.Fatalf("%s %s = %v, want no service is attached", route.method, route.path, answer)
		}
	}
}

func TestControlEpochCheck(t *testing.T) {
	root := controlTestRoot(t, "root-0")
	link := newServiceLink(root.events, servicePeer{
		URL:    "http://127.0.0.1:9",
		Secret: strings.Repeat("ab", 32),
		Epoch:  "aaaaaaaaaaaaaaaa",
	})
	server := httptest.NewServer(newControlHandler(controlSecret, []*controlRoot{root}, link))
	t.Cleanup(server.Close)

	get := func(headers map[string]string) (int, map[string]any) {
		t.Helper()
		request, err := http.NewRequest(http.MethodGet, server.URL+"/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", controlScheme+controlSecret)
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		answer := map[string]any{}
		if err := json.NewDecoder(response.Body).Decode(&answer); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return response.StatusCode, answer
	}

	if status, _ := get(nil); status != http.StatusOK {
		t.Fatalf("a request with no epoch headers = %d, want 200", status)
	}
	if status, _ := get(map[string]string{
		"X-Trial-Epoch":      link.currentPeer().Epoch,
		"X-Trial-Peer-Epoch": link.selfEpoch,
	}); status != http.StatusOK {
		t.Fatalf("the attached service's epochs = %d, want 200", status)
	}
	status, answer := get(map[string]string{
		"X-Trial-Epoch":      link.currentPeer().Epoch,
		"X-Trial-Peer-Epoch": "bbbbbbbbbbbbbbbb",
	})
	if status != http.StatusConflict {
		t.Fatalf("a wrong peer epoch = %d, want 409", status)
	}
	if answer["error"] != "epoch" || answer["epoch"] != link.selfEpoch {
		t.Fatalf("epoch answer = %v, want error epoch and Go's epoch %q", answer, link.selfEpoch)
	}
	if status, _ := get(map[string]string{
		"X-Trial-Epoch":      "bbbbbbbbbbbbbbbb",
		"X-Trial-Peer-Epoch": link.selfEpoch,
	}); status != http.StatusConflict {
		t.Fatalf("a wrong service epoch = %d, want 409", status)
	}
}

func TestNoticeSink(t *testing.T) {
	previous := live
	live = newLiveHub()
	live.configureFrameIDs(false)
	t.Cleanup(func() { live = previous })

	subscriber := &frameSink{}
	live.subscribe("doc.html", subscriber, "live")

	root := newRootLive(nil, "")
	var gotName string
	var gotPayload any
	root.notices = func(name string, payload any) {
		gotName = name
		gotPayload = payload
	}

	notice := liveNotice{MsgType: "toast", Msg: "hello", Action: "reload", Data: map[string]any{"kind": "external-change"}}
	root.notify("doc.html", notice, "live")

	if gotName != "doc.html" {
		t.Fatalf("notice name = %q, want doc.html", gotName)
	}
	delivered, ok := gotPayload.(liveNotice)
	if !ok || delivered.MsgType != notice.MsgType || delivered.Msg != notice.Msg {
		t.Fatalf("notice payload = %v, want %v", gotPayload, notice)
	}
	encoded, err := jsonStringify(gotPayload)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"msgType":"toast","msg":"hello","action":"reload","data":{"kind":"external-change"}}`
	if string(encoded) != want {
		t.Fatalf("notice payload JSON = %s, want %s", encoded, want)
	}
	if frames := subscriber.frames(); len(frames) != 0 {
		t.Fatalf("the hub published %d frames with a service attached: %v", len(frames), frames)
	}

	root.broadcast("doc.html", livePayload{HTML: "<!doctype html>", Sender: "server-save"}, "live")
	if frames := subscriber.frames(); len(frames) != 0 {
		t.Fatalf("the hub broadcast %d frames with a service attached: %v", len(frames), frames)
	}
	if gotName != "doc.html" {
		t.Fatalf("broadcast changed the notice sink: name = %q", gotName)
	}
}

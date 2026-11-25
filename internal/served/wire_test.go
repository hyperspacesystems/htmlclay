package served

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/builtin"
	"github.com/panphora/htmlclay/internal/logging"
	"github.com/panphora/htmlclay/internal/served/assets"
	"github.com/panphora/htmlclay/internal/wire"
)

// framesOnly is the policy a served wire carries in this step: frames, and no
// program or AI edit behind them.
type framesOnly struct{}

func (framesOnly) CanAskProgram() bool                             { return false }
func (framesOnly) AskProgram(file, name string) bool               { return false }
func (framesOnly) ProgramGranted(file, name, id string) bool       { return false }
func (framesOnly) AIEditOffered() bool                             { return false }
func (framesOnly) AIEditFor(string) (bool, bool)                   { return false, false }
func (framesOnly) AskAIEdit(string)                                {}
func (framesOnly) AIEditFiles(string) bool                         { return false }
func (framesOnly) AIEditEngines() (string, map[string][]string)    { return "", nil }
func (framesOnly) AIEditModels() map[string]string                 { return nil }
func (framesOnly) AIEditEfforts() map[string]string                { return nil }
func (framesOnly) BuiltinFor(string, string) builtin.Access        { return builtin.Access{} }
func (framesOnly) AskBuiltin(string, string)                       {}
func (framesOnly) RevealFile(string) error                         { return nil }
func (framesOnly) NoteAIEditResult(string, string, string, string) {}
func (framesOnly) NoteDocumentSaved(string, []byte)                {}
func (framesOnly) DocumentTrust(string) wire.Trust                 { return wire.Trust{Tier: "unknown"} }
func (framesOnly) AskDocument(string, string)                      {}
func (framesOnly) AITools() wire.AIToolsHost                       { return nil }
func (framesOnly) Programs() wire.ProgramsHost                     { return nil }

const (
	wireRootID           = "wire-root"
	wireRootHost         = "wire.htmlclay.localhost:1212"
	wireSiblingRootID    = "wire-sibling"
	wireSiblingRootHost  = "wire-sibling.htmlclay.localhost:1212"
	wireDocumentName     = "doc.html"
	wireBaselineNotFound = "File not found"
)

// wireFrames reads one wire stream's frames off its SSE body.
type wireFrames struct {
	ch chan wire.Envelope
}

func wireReadFrames(t *testing.T, body io.Reader) *wireFrames {
	t.Helper()
	frames := &wireFrames{ch: make(chan wire.Envelope, 64)}
	go func() {
		defer close(frames.ch)
		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
		var data []string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
				continue
			}
			if line != "" || len(data) == 0 {
				continue
			}
			var env wire.Envelope
			if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &env); err == nil && env.Type != "" {
				frames.ch <- env
			}
			data = nil
		}
	}()
	return frames
}

func (f *wireFrames) waitFor(t *testing.T, typ, id string) wire.Envelope {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case env, ok := <-f.ch:
			if !ok {
				t.Fatalf("stream ended before %s for %s", typ, id)
			}
			if env.Type == typ && env.ID == id {
				return env
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s for %s", typ, id)
		}
	}
}

// wireQuietFor reports whether no frame arrived within d.
func (f *wireFrames) wireQuietFor(t *testing.T, d time.Duration) bool {
	t.Helper()
	select {
	case env, ok := <-f.ch:
		if !ok {
			return true
		}
		t.Logf("a frame arrived: %s %s", env.Type, env.ID)
		return false
	case <-time.After(d):
		return true
	}
}

func wireWaitEOF(t *testing.T, resp *http.Response) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, resp.Body)
		done <- err
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream never reached EOF")
	}
}

// wireStand is one served folder with the wire on, plus the server that answers
// its handler.
type wireStand struct {
	host       *Host
	rootID     string
	dir        string
	publicHost string
	server     *httptest.Server
}

func wireTestHost(t *testing.T) *Host {
	t.Helper()
	host := NewHost(Options{
		Assets:          assets.ProductFS(),
		CompileTailwind: func(string) (string, error) { return "", nil },
	})
	t.Cleanup(host.Close)
	return host
}

func wireTestLogger(t *testing.T) *logging.Logger {
	t.Helper()
	logger, err := logging.New(filepath.Join(t.TempDir(), "wire.log"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(logger.Close)
	return logger
}

func wireAddRoot(t *testing.T, host *Host, id, dir, publicHost string, policy wire.Policy) *httptest.Server {
	t.Helper()
	handler, err := host.AddRoot(RootSpec{
		ID:         id,
		Path:       dir,
		PublicHost: publicHost,
		WirePolicy: policy,
		WireLogger: wireTestLogger(t),
	})
	if err != nil {
		t.Fatalf("AddRoot %s: %v", id, err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

// wireOpenStand writes the stand's document and serves its folder with the wire on.
func wireOpenStand(t *testing.T) *wireStand {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, wireDocumentName), []byte("<!doctype html><p>wire</p>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	host := wireTestHost(t)
	return &wireStand{
		host:       host,
		rootID:     wireRootID,
		dir:        dir,
		publicHost: wireRootHost,
		server:     wireAddRoot(t, host, wireRootID, dir, wireRootHost, framesOnly{}),
	}
}

func (s *wireStand) file(t *testing.T) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(filepath.Join(s.dir, wireDocumentName))
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func (s *wireStand) documentURL(document string) string {
	return "http://" + s.publicHost + "/" + document
}

func (s *wireStand) do(t *testing.T, request *http.Request) *http.Response {
	t.Helper()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", request.Method, request.URL.Path, err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}

// pageSubscribe subscribes the page of one document, the way a document's own
// script does.
func (s *wireStand) pageSubscribe(t *testing.T, document string) (*http.Response, *wireFrames) {
	t.Helper()
	q := url.Values{"document-url": {s.documentURL(document)}}
	request, err := http.NewRequest(http.MethodGet, s.server.URL+"/_/wire/subscribe?"+q.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = s.publicHost
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	response := s.do(t, request)
	return response, wireReadFrames(t, response.Body)
}

// processSubscribe subscribes a local process as the document's handler: no
// browser headers at all, the absolute path named in the query.
func (s *wireStand) processSubscribe(t *testing.T, file string) (*http.Response, *wireFrames) {
	t.Helper()
	q := url.Values{"role": {"handler"}, "file": {file}}
	request, err := http.NewRequest(http.MethodGet, s.server.URL+"/_/wire/subscribe?"+q.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = s.publicHost
	response := s.do(t, request)
	return response, wireReadFrames(t, response.Body)
}

// pageSend posts one frame as a page: same-origin, and the document it is about.
func (s *wireStand) pageSend(t *testing.T, body, document string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, s.server.URL+"/_/wire/send", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Host = s.publicHost
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Document-URL", s.documentURL(document))
	return s.do(t, request)
}

// processSend posts one frame as a local process: no browser headers, the target
// in the frame itself.
func (s *wireStand) processSend(t *testing.T, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, s.server.URL+"/_/wire/send", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Host = s.publicHost
	request.Header.Set("Content-Type", "application/json")
	return s.do(t, request)
}

// wireRefusal requires a refusal and then reads its body. An admitted
// subscription is a stream that never ends, so the status is read first.
func wireRefusal(t *testing.T, what string, response *http.Response) {
	t.Helper()
	if response.StatusCode < 400 {
		t.Fatalf("%s was admitted: %d", what, response.StatusCode)
	}
	t.Logf("%s answered %d: %s", what, response.StatusCode, wireReadBody(t, response))
}

func wireReadBody(t *testing.T, response *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// The copy kept before a handler's first write is taken under the document's
// one lock, the lock a save holds, so the two never run at once.
func TestM3ServedWireHistoryWaitsForASave(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, wireDocumentName), []byte("<!doctype html><p>wire</p>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := wireTestHost(t).AddRoot(RootSpec{
		ID:         wireRootID,
		Path:       dir,
		PublicHost: wireRootHost,
		WirePolicy: framesOnly{},
		WireLogger: wireTestLogger(t),
	})
	if err != nil {
		t.Fatalf("AddRoot: %v", err)
	}
	root, ok := handler.(*rootServer)
	if !ok {
		t.Fatalf("AddRoot returned %T, want *rootServer", handler)
	}
	docs := &wireDocuments{root: root, internalDir: root.internalDir}
	real, err := filepath.EvalSymlinks(filepath.Join(dir, wireDocumentName))
	if err != nil {
		t.Fatal(err)
	}
	doc, ok := docs.At(real)
	if !ok {
		t.Fatal("the served document is not a wire document")
	}
	saving, err := resolveWriteTarget(root.registry, wireDocumentName)
	if err != nil {
		t.Fatal(err)
	}

	unlock := root.commit.locks.lock(saving)
	done := make(chan error, 1)
	go func() { done <- docs.OpenHistory(doc) }()
	select {
	case <-done:
		unlock()
		t.Fatal("the copy was taken while a save held the document")
	case <-time.After(150 * time.Millisecond):
	}
	unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the copy never finished after the save let go")
	}
}

// TestM3ServedWire proves a served folder carries the wire the way loose
// documents do: a page and a handler exchange frames, a sibling folder's page and
// a browser handling its own page are refused, and targets outside the folder are
// refused. Programs and AI editing come later.
func TestM3ServedWire(t *testing.T) {
	t.Run("a page and a handler exchange a request", func(t *testing.T) {
		stand := wireOpenStand(t)
		file := stand.file(t)

		pageResponse, page := stand.pageSubscribe(t, wireDocumentName)
		if pageResponse.StatusCode != http.StatusOK {
			t.Fatalf("page subscription answered %d: %s", pageResponse.StatusCode, wireReadBody(t, pageResponse))
		}
		handlerResponse, handler := stand.processSubscribe(t, file)
		if handlerResponse.StatusCode != http.StatusOK {
			t.Fatalf("handler subscription answered %d: %s", handlerResponse.StatusCode, wireReadBody(t, handlerResponse))
		}

		sent := stand.pageSend(t, `{"type":"wire/request","id":"r1","payload":{}}`, wireDocumentName)
		if sent.StatusCode != http.StatusOK {
			t.Fatalf("page request answered %d: %s", sent.StatusCode, wireReadBody(t, sent))
		}
		request := handler.waitFor(t, "wire/request", "r1")
		if request.From != "page" {
			t.Fatalf("request From = %q, want page", request.From)
		}
		if request.File != file {
			t.Fatalf("request File = %q, want %q", request.File, file)
		}

		answered := stand.processSend(t, fmt.Sprintf(`{"type":"wire/done","id":"r1","file":%q}`, file))
		if answered.StatusCode != http.StatusOK {
			t.Fatalf("handler answer answered %d: %s", answered.StatusCode, wireReadBody(t, answered))
		}
		done := page.waitFor(t, "wire/done", "r1")
		if done.From != "process" {
			t.Fatalf("done From = %q, want process", done.From)
		}
	})

	t.Run("a sibling folder's page is refused", func(t *testing.T) {
		stand := wireOpenStand(t)
		siblingDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(siblingDir, wireDocumentName), []byte("<!doctype html><p>sibling</p>\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		wireAddRoot(t, stand.host, wireSiblingRootID, siblingDir, wireSiblingRootHost, framesOnly{})

		q := url.Values{"document-url": {stand.documentURL(wireDocumentName)}}
		request, err := http.NewRequest(http.MethodGet, stand.server.URL+"/_/wire/subscribe?"+q.Encode(), nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Host = stand.publicHost
		request.Header.Set("Sec-Fetch-Site", "same-site")
		request.Header.Set("Origin", "http://"+wireSiblingRootHost)
		wireRefusal(t, "a sibling folder's page", stand.do(t, request))

		// The same page with the sibling's Origin but a same-origin attestation is
		// what reaches the wire's own caller: the gate lets a same-origin GET
		// through, so the wire's origin check is the one that refuses it.
		sameOriginRequest, err := http.NewRequest(http.MethodGet, stand.server.URL+"/_/wire/subscribe?"+q.Encode(), nil)
		if err != nil {
			t.Fatal(err)
		}
		sameOriginRequest.Host = stand.publicHost
		sameOriginRequest.Header.Set("Sec-Fetch-Site", "same-origin")
		sameOriginRequest.Header.Set("Origin", "http://"+wireSiblingRootHost)
		sameOriginResponse := stand.do(t, sameOriginRequest)
		if sameOriginResponse.StatusCode != http.StatusForbidden {
			t.Fatalf("a sibling folder's origin answered %d, want 403", sameOriginResponse.StatusCode)
		}
		t.Logf("a sibling page's same-origin subscribe answered %d: %s", sameOriginResponse.StatusCode, wireReadBody(t, sameOriginResponse))
	})

	t.Run("a browser cannot be a handler", func(t *testing.T) {
		stand := wireOpenStand(t)
		file := stand.file(t)

		q := url.Values{"role": {"handler"}, "file": {file}}
		request, err := http.NewRequest(http.MethodGet, stand.server.URL+"/_/wire/subscribe?"+q.Encode(), nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Host = stand.publicHost
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		response := stand.do(t, request)
		body := wireReadBody(t, response)
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("a page subscribing as a handler answered %d, want 403: %s", response.StatusCode, body)
		}
	})

	t.Run("a Cookie with no Sec-Fetch-Site is refused", func(t *testing.T) {
		stand := wireOpenStand(t)

		q := url.Values{"document-url": {stand.documentURL(wireDocumentName)}}
		request, err := http.NewRequest(http.MethodGet, stand.server.URL+"/_/wire/subscribe?"+q.Encode(), nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Host = stand.publicHost
		request.AddCookie(&http.Cookie{Name: "isLoggedIn", Value: "true"})
		response := stand.do(t, request)
		body := wireReadBody(t, response)
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("a cookied caller answered %d, want 403: %s", response.StatusCode, body)
		}
	})

	t.Run("targets outside the folder are refused", func(t *testing.T) {
		stand := wireOpenStand(t)
		rootReal, err := filepath.EvalSymlinks(stand.dir)
		if err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(filepath.Dir(rootReal), "outside.html")
		if err := os.WriteFile(outside, []byte("<!doctype html><p>outside</p>\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(rootReal, "link.html")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		internalDir := filepath.Join(rootReal, ".hyperclay")
		if err := os.MkdirAll(internalDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(internalDir, "record.html"), []byte("<!doctype html><p>record</p>\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		for _, target := range []struct{ what, path string }{
			{"a file outside the root", outside},
			{"a symlink to a file outside the root", filepath.Join(rootReal, "link.html")},
			{"a file inside the root's dot folder", filepath.Join(internalDir, "record.html")},
		} {
			response, _ := stand.processSubscribe(t, target.path)
			wireRefusal(t, target.what, response)
		}

		response, _ := stand.pageSubscribe(t, "../outside.html")
		wireRefusal(t, "a page naming ../outside.html", response)
	})

	t.Run("an unbound process send is admitted", func(t *testing.T) {
		stand := wireOpenStand(t)
		file := stand.file(t)

		response := stand.processSend(t, fmt.Sprintf(`{"type":"wire/done","id":"p1","file":%q}`, file))
		body := wireReadBody(t, response)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("an unbound process send answered %d: %s", response.StatusCode, body)
		}
	})

	t.Run("removing the root closes the handler's stream", func(t *testing.T) {
		stand := wireOpenStand(t)
		file := stand.file(t)

		handlerResponse, _ := stand.processSubscribe(t, file)
		if handlerResponse.StatusCode != http.StatusOK {
			t.Fatalf("handler subscription answered %d: %s", handlerResponse.StatusCode, wireReadBody(t, handlerResponse))
		}
		if !stand.host.RemoveRoot(stand.rootID) {
			t.Fatalf("RemoveRoot(%s) = false", stand.rootID)
		}
		wireWaitEOF(t, handlerResponse)
	})

	t.Run("a root with no WirePolicy answers as before", func(t *testing.T) {
		host := wireTestHost(t)
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, wireDocumentName), []byte("<!doctype html><p>plain</p>\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		handler, err := host.AddRoot(RootSpec{ID: wireRootID, Path: dir, PublicHost: wireRootHost})
		if err != nil {
			t.Fatalf("AddRoot: %v", err)
		}
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)

		subscribe, err := http.NewRequest(http.MethodGet, server.URL+"/_/wire/subscribe?document-url=http://"+wireRootHost+"/"+wireDocumentName, nil)
		if err != nil {
			t.Fatal(err)
		}
		subscribe.Host = wireRootHost
		subscribe.Header.Set("Sec-Fetch-Site", "same-origin")
		subscribeResponse, err := http.DefaultClient.Do(subscribe)
		if err != nil {
			t.Fatal(err)
		}
		defer subscribeResponse.Body.Close()
		subscribeBody, err := io.ReadAll(subscribeResponse.Body)
		if err != nil {
			t.Fatal(err)
		}

		send, err := http.NewRequest(http.MethodPost, server.URL+"/_/wire/send", http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		send.Host = wireRootHost
		send.Header.Set("Sec-Fetch-Site", "same-origin")
		send.Header.Set("Content-Type", "application/json")
		sendResponse, err := http.DefaultClient.Do(send)
		if err != nil {
			t.Fatal(err)
		}
		defer sendResponse.Body.Close()
		sendBody, err := io.ReadAll(sendResponse.Body)
		if err != nil {
			t.Fatal(err)
		}

		if subscribeResponse.StatusCode != http.StatusNotFound || string(subscribeBody) != wireBaselineNotFound {
			t.Fatalf("GET /_/wire/subscribe answered %d %q, want 404 %q", subscribeResponse.StatusCode, string(subscribeBody), wireBaselineNotFound)
		}
		if sendResponse.StatusCode != http.StatusNotFound || string(sendBody) != wireBaselineNotFound {
			t.Fatalf("POST /_/wire/send answered %d %q, want 404 %q", sendResponse.StatusCode, string(sendBody), wireBaselineNotFound)
		}
	})

	t.Run("a handler with no page waits", func(t *testing.T) {
		stand := wireOpenStand(t)
		file := stand.file(t)

		handlerResponse, handler := stand.processSubscribe(t, file)
		if handlerResponse.StatusCode != http.StatusOK {
			t.Fatalf("handler subscription answered %d: %s", handlerResponse.StatusCode, wireReadBody(t, handlerResponse))
		}
		if !handler.wireQuietFor(t, 300*time.Millisecond) {
			t.Fatal("a handler with no page was sent a frame")
		}
		select {
		case env, ok := <-handler.ch:
			if ok {
				t.Fatalf("the handler's stream carried %q", env.Type)
			}
		default:
		}
	})

	t.Run("attaching a handler keeps a baseline version and leaves the bytes", func(t *testing.T) {
		stand := wireOpenStand(t)
		file := stand.file(t)
		before, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}

		handlerResponse, _ := stand.processSubscribe(t, file)
		if handlerResponse.StatusCode != http.StatusOK {
			t.Fatalf("handler subscription answered %d: %s", handlerResponse.StatusCode, wireReadBody(t, handlerResponse))
		}

		versions, err := stand.host.Versions(stand.rootID, wireDocumentName, "", 10)
		if err != nil {
			t.Fatalf("Versions: %v", err)
		}
		if len(versions) == 0 {
			t.Fatal("attaching a handler left the document with no version")
		}
		after, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(before) {
			t.Fatalf("attaching a handler changed the bytes: %q, want %q", string(after), string(before))
		}
	})
}

// A page in a served folder reads its own run's files through the folder's
// address, and another page in the same folder cannot.
func TestServedRunFileReachesOnlyItsDocument(t *testing.T) {
	stand := wireOpenStand(t)
	if err := os.WriteFile(filepath.Join(stand.dir, "other.html"), []byte("<!doctype html><p>other</p>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	core, ok := stand.host.Wire(stand.rootID)
	if !ok {
		t.Fatal("the served folder has no wire")
	}
	ref, err := core.RunFiles.Put(stand.file(t), wire.NewRunID(), "diff", []byte("-one\n+two\n"))
	if err != nil {
		t.Fatal(err)
	}
	get := func(document string) *http.Response {
		t.Helper()
		request, err := http.NewRequest(http.MethodGet, stand.server.URL+ref.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Host = stand.publicHost
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		request.Header.Set("Document-URL", stand.documentURL(document))
		return stand.do(t, request)
	}
	response := get(wireDocumentName)
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || string(body) != "-one\n+two\n" {
		t.Fatalf("the run's own page: %d %q", response.StatusCode, body)
	}
	if response := get("other.html"); response.StatusCode != http.StatusNotFound {
		t.Errorf("another page in the folder read the run file: %d", response.StatusCode)
	}
}

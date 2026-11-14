package main

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

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/wire"
)

const servedWireDocumentName = "doc.html"

// servedWireFrames reads one wire stream's frames off its SSE body.
type servedWireFrames struct {
	ch chan wire.Envelope
}

func servedWireReadFrames(t *testing.T, body io.Reader) *servedWireFrames {
	t.Helper()
	frames := &servedWireFrames{ch: make(chan wire.Envelope, 64)}
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

func (f *servedWireFrames) waitFor(t *testing.T, typ, id string) wire.Envelope {
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

// waitClosed requires the stream to reach EOF within 2s.
func (f *servedWireFrames) waitClosed(t *testing.T) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-f.ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("the handler's stream never reached EOF")
		}
	}
}

// servedWireStand is one served folder answering through the app's own handler on a
// real listener, the way a browser reaches it.
type servedWireStand struct {
	a      *app
	home   string
	entry  config.ServedFolder
	dir    string
	host   string
	server *httptest.Server
}

func servedWireOpen(t *testing.T) *servedWireStand {
	t.Helper()
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, servedWireDocumentName), servedTestDocument)

	a := newServedTestApp(t, home, t.TempDir())
	entry, refusal, err := a.serveFolder(dir, "notes")
	if err != nil {
		t.Fatalf("serveFolder: %v", err)
	}
	if refusal != nil {
		t.Fatalf("serveFolder refused: %+v", refusal)
	}
	server := httptest.NewServer(http.HandlerFunc(a.serveHost))
	t.Cleanup(server.Close)
	return &servedWireStand{
		a:      a,
		home:   home,
		entry:  entry,
		dir:    dir,
		host:   servedHostName("notes", a.rt.port),
		server: server,
	}
}

func (s *servedWireStand) file(t *testing.T) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(filepath.Join(s.dir, servedWireDocumentName))
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func (s *servedWireStand) documentURL(document string) string {
	return "http://" + s.host + "/" + document
}

func (s *servedWireStand) do(t *testing.T, request *http.Request) *http.Response {
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
func (s *servedWireStand) pageSubscribe(t *testing.T, document string) (*http.Response, *servedWireFrames) {
	t.Helper()
	q := url.Values{"document-url": {s.documentURL(document)}}
	request, err := http.NewRequest(http.MethodGet, s.server.URL+"/_/wire/subscribe?"+q.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = s.host
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	response := s.do(t, request)
	return response, servedWireReadFrames(t, response.Body)
}

// processSubscribe subscribes a local process as the document's handler: no browser
// headers at all, the absolute path named in the query.
func (s *servedWireStand) processSubscribe(t *testing.T, file string) (*http.Response, *servedWireFrames) {
	t.Helper()
	q := url.Values{"role": {"handler"}, "file": {file}}
	request, err := http.NewRequest(http.MethodGet, s.server.URL+"/_/wire/subscribe?"+q.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = s.host
	response := s.do(t, request)
	return response, servedWireReadFrames(t, response.Body)
}

// pageSend posts one frame as a page: same-origin, and the document it is about.
func (s *servedWireStand) pageSend(t *testing.T, body, document string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, s.server.URL+"/_/wire/send", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Host = s.host
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Document-URL", s.documentURL(document))
	return s.do(t, request)
}

// processSend posts one frame as a local process: no browser headers, the target in
// the frame itself.
func (s *servedWireStand) processSend(t *testing.T, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, s.server.URL+"/_/wire/send", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Host = s.host
	request.Header.Set("Content-Type", "application/json")
	return s.do(t, request)
}

func servedWireReadBody(t *testing.T, response *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// servedWireExchange runs one request from the page to its handler and one answer
// back, the round trip a served document's wire is for.
func servedWireExchange(t *testing.T, stand *servedWireStand) {
	t.Helper()
	file := stand.file(t)

	pageResponse, page := stand.pageSubscribe(t, servedWireDocumentName)
	if pageResponse.StatusCode != http.StatusOK {
		t.Fatalf("page subscription answered %d: %s", pageResponse.StatusCode, servedWireReadBody(t, pageResponse))
	}
	handlerResponse, handler := stand.processSubscribe(t, file)
	if handlerResponse.StatusCode != http.StatusOK {
		t.Fatalf("handler subscription answered %d: %s", handlerResponse.StatusCode, servedWireReadBody(t, handlerResponse))
	}

	sent := stand.pageSend(t, `{"type":"wire/request","id":"r1","payload":{}}`, servedWireDocumentName)
	if sent.StatusCode != http.StatusOK {
		t.Fatalf("page request answered %d: %s", sent.StatusCode, servedWireReadBody(t, sent))
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
		t.Fatalf("handler answer answered %d: %s", answered.StatusCode, servedWireReadBody(t, answered))
	}
	done := page.waitFor(t, "wire/done", "r1")
	if done.From != "process" {
		t.Fatalf("done From = %q, want process", done.From)
	}
}

// TestM3ServedWireRoundTrip is step 7.27b: every folder the app serves carries the
// wire the loose host does, through the app's own request handler, and a served
// document is never a loose session. Programs come later.
func TestM3ServedWireRoundTrip(t *testing.T) {
	t.Run("a page and a handler exchange a request", func(t *testing.T) {
		stand := servedWireOpen(t)
		servedWireExchange(t, stand)
	})

	t.Run("relinking the folder closes the handler's stream", func(t *testing.T) {
		stand := servedWireOpen(t)
		file := stand.file(t)
		response, handler := stand.processSubscribe(t, file)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("handler subscription answered %d: %s", response.StatusCode, servedWireReadBody(t, response))
		}

		moved := filepath.Join(stand.home, "projects", "notes")
		if err := os.MkdirAll(filepath.Dir(moved), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(stand.dir, moved); err != nil {
			t.Fatal(err)
		}
		if refusal := m3ProgramDo(t, stand.a, ChannelShell, "folder.relink", map[string]any{
			"id": stand.entry.ID, "path": moved, "acknowledged": []string{},
		}); refusal != nil {
			t.Fatalf("the relink of the moved folder = %+v", refusal)
		}
		handler.waitClosed(t)
	})

	t.Run("stop serving closes the handler's stream", func(t *testing.T) {
		stand := servedWireOpen(t)
		file := stand.file(t)
		response, handler := stand.processSubscribe(t, file)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("handler subscription answered %d: %s", response.StatusCode, servedWireReadBody(t, response))
		}
		if err := stand.a.stopServing(stand.entry.ID); err != nil {
			t.Fatalf("stopServing: %v", err)
		}
		handler.waitClosed(t)
	})

	t.Run("a served document is not a loose session", func(t *testing.T) {
		stand := servedWireOpen(t)
		loose := filepath.Join(stand.home, "loose.html")
		writeTestFile(t, loose, servedTestDocument)
		stand.a.openForTest(t, loose)
		servedWireExchange(t, stand)

		file := stand.file(t)
		resolvedLoose, err := filepath.EvalSymlinks(loose)
		if err != nil {
			t.Fatal(err)
		}
		stand.a.mu.Lock()
		sites := append([]*site(nil), stand.a.sites...)
		stand.a.mu.Unlock()

		held := false
		for _, s := range sites {
			if _, ok := s.sessions.LookupByPath(resolvedLoose); !ok {
				continue
			}
			held = true
			if _, ok := s.sessions.LookupByPath(file); ok {
				t.Fatalf("the served document %s is a session in the site at %s", file, s.host)
			}
		}
		if !held {
			t.Fatalf("fixture: the loose document %s is in no site's sessions", resolvedLoose)
		}
	})
}

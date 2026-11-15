package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/testutil"
	"github.com/panphora/htmlclay/internal/wire"
)

const conformanceCasesDir = "../../docs/wire-fixtures/cases"

var conformanceCaseName = regexp.MustCompile(`^([0-9]+)-([a-z0-9-]+)$`)

type conformanceCase struct {
	Name   string   `json:"name"`
	Hosts  []string `json:"hosts"`
	Served string   `json:"served"`
	Setup  struct {
		Program  string `json:"program"`
		Decision string `json:"decision"`
		AIEdit   string `json:"aiEdit"`
	} `json:"setup"`
	Steps           []conformanceStep `json:"steps"`
	Skip            string            `json:"skip"`
	KnownDifference string            `json:"knownDifference"`
	Slug            string            `json:"-"`
}

type conformanceStep struct {
	Do     string          `json:"do"`
	As     string          `json:"as"`
	Stream string          `json:"stream"`
	Type   string          `json:"type"`
	ID     string          `json:"id"`
	Code   string          `json:"code"`
	Frame  json.RawMessage `json:"frame"`
	Expect struct {
		Status int `json:"status"`
	} `json:"expect"`
}

type conformanceFrame struct {
	Type string
	ID   string
	Code string
}

type conformanceStream struct {
	frames chan conformanceFrame
	body   io.ReadCloser
	mu     sync.Mutex
	lastID string
}

func conformanceReadStream(t *testing.T, body io.ReadCloser) *conformanceStream {
	t.Helper()
	stream := &conformanceStream{frames: make(chan conformanceFrame, 64), body: body}
	go func() {
		defer close(stream.frames)
		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
		var data []string
		id := ""
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "id:") {
				id = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
				continue
			}
			if strings.HasPrefix(line, "data:") {
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
				continue
			}
			if line != "" || len(data) == 0 {
				continue
			}
			stream.mu.Lock()
			stream.lastID = id
			stream.mu.Unlock()
			var env wire.Envelope
			if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &env); err == nil && env.Type != "" {
				code := ""
				if len(env.Payload) > 0 {
					var payload struct {
						Code string `json:"code"`
					}
					if json.Unmarshal(env.Payload, &payload) == nil {
						code = payload.Code
					}
				}
				stream.frames <- conformanceFrame{Type: env.Type, ID: env.ID, Code: code}
			}
			data = nil
			id = ""
		}
	}()
	return stream
}

func (s *conformanceStream) see(t *testing.T, typ, id, code string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case frame, ok := <-s.frames:
			if !ok {
				t.Fatalf("stream ended before %s for %s (code %q)", typ, id, code)
			}
			if frame.Type == typ && frame.ID == id && (code == "" || frame.Code == code) {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s for %s (code %q)", typ, id, code)
		}
	}
}

func (s *conformanceStream) waitClosed(t *testing.T) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-s.frames:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("the stream never reached EOF")
		}
	}
}

func (s *conformanceStream) lastEventID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastID
}

type conformanceHost struct {
	t           *testing.T
	kind        string
	document    string
	rel         string
	host        string
	origin      string
	otherOrigin string
	token       string
	baseURL     string
	streams     map[string]*conformanceStream
	streamAs    map[string]string
	refusals    []string
}

func conformanceDocument(withHelper bool) string {
	doc := "<!doctype html>\n<html><head><title>Wire</title></head><body><p>wire</p></body></html>\n"
	if withHelper {
		doc = strings.Replace(doc, "</head>", `<meta name="htmlclay-helper" content="search"></head>`, 1)
	}
	return doc
}

func conformanceProgramPath(t *testing.T, kind string) string {
	t.Helper()
	switch kind {
	case "echo":
		return recordingHelperProgram(t, t.TempDir(), filepath.Join(t.TempDir(), "ran"))
	case "wait":
		return servedWaitingProgram(t, t.TempDir(), filepath.Join(t.TempDir(), "started"))
	case "error":
		path := filepath.Join(t.TempDir(), "errorer")
		script := "#!/bin/sh\ncat > /dev/null\nprintf '{\"type\":\"error\",\"code\":\"invalid_request\",\"message\":\"bad request\",\"details\":{\"field\":\"query\"}}\\n'\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	t.Fatalf("unknown program fixture %q", kind)
	return ""
}

func conformanceLooseHost(t *testing.T, c conformanceCase) *conformanceHost {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "proj")
	rel := "doc.htmlclay"
	document := filepath.Join(dir, rel)
	writeTestFile(t, document, conformanceDocument(c.Setup.Program != "none"))

	a := newTestAppWithConfigDir(t, home, t.TempDir())
	if c.Setup.AIEdit == "on" {
		a.rt.cfg.SetAIEditEnabled(true)
	}
	var program config.HelperProgram
	if c.Setup.Program != "none" {
		program, err = a.rt.cfg.AddHelperProgram("search", conformanceProgramPath(t, c.Setup.Program))
		if err != nil {
			t.Fatal(err)
		}
	}
	s, _ := a.openForTest(t, document)
	resolved, err := filepath.EvalSymlinks(document)
	if err != nil {
		t.Fatal(err)
	}
	if c.Setup.Program != "none" {
		if err := a.trustFolder(dir); err != nil {
			t.Fatalf("trustFolder: %v", err)
		}
		if c.Setup.Decision == "allow" {
			if _, _, err := a.rt.cfg.DecideHelper(config.HelperDecision{
				Document: resolved,
				Name:     "search",
				Program:  program.ID,
				Allowed:  true,
				Scope:    a.scopeOf(resolved),
			}); err != nil {
				t.Fatal(err)
			}
			if err := a.rt.cfg.Save(); err != nil {
				t.Fatal(err)
			}
		}
		a.refreshHelperDispatchers()
	}
	token := ""
	if registration, ok := s.sessions.LookupByPath(resolved); ok {
		token = registration.Token
		rel = registration.RelPath
	}
	return &conformanceHost{
		t:           t,
		kind:        "loose",
		document:    resolved,
		rel:         rel,
		host:        s.host,
		origin:      "http://" + s.host,
		otherOrigin: "http://other.localhost:" + strings.SplitN(s.host, ":", 2)[1],
		token:       token,
		baseURL:     "http://" + s.host,
		streams:     map[string]*conformanceStream{},
		streamAs:    map[string]string{},
	}
}

func conformanceServedHost(t *testing.T, c conformanceCase) *conformanceHost {
	t.Helper()
	stand := servedWireOpen(t)
	if c.Setup.Program != "none" {
		writeTestFile(t, filepath.Join(stand.dir, servedWireDocumentName), conformanceDocument(true))
		program, err := stand.a.rt.cfg.AddHelperProgram("search", conformanceProgramPath(t, c.Setup.Program))
		if err != nil {
			t.Fatal(err)
		}
		if err := stand.a.rt.cfg.Save(); err != nil {
			t.Fatal(err)
		}
		if c.Setup.Decision == "allow" {
			if _, _, err := stand.a.rt.cfg.DecideHelper(config.HelperDecision{
				Document: stand.file(t),
				Name:     "search",
				Program:  program.ID,
				Allowed:  true,
				Scope:    stand.a.scopeOf(stand.file(t)),
			}); err != nil {
				t.Fatal(err)
			}
			if err := stand.a.rt.cfg.Save(); err != nil {
				t.Fatal(err)
			}
		}
		request, err := http.NewRequest(http.MethodGet, stand.server.URL+"/"+servedWireDocumentName, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Host = stand.host
		response := stand.do(t, request)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("page load answered %d: %s", response.StatusCode, servedWireReadBody(t, response))
		}
	}
	other := "http://other.localhost:" + strings.SplitN(stand.host, ":", 2)[1]
	return &conformanceHost{
		t:           t,
		kind:        "served",
		document:    stand.file(t),
		rel:         servedWireDocumentName,
		host:        stand.host,
		origin:      "http://" + stand.host,
		otherOrigin: other,
		baseURL:     stand.server.URL,
		streams:     map[string]*conformanceStream{},
		streamAs:    map[string]string{},
	}
}

func (h *conformanceHost) documentURL() string {
	return h.origin + "/" + h.rel
}

func (h *conformanceHost) newRequest(method, path, body, as string) *http.Request {
	h.t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, h.baseURL+path, reader)
	if err != nil {
		h.t.Fatal(err)
	}
	request.Host = h.host
	switch as {
	case "page":
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		request.Header.Set("Origin", h.origin)
		if h.token != "" {
			request.Header.Set(wire.TokenHeader, h.token)
		}
	case "page-other-origin":
		request.Header.Set("Sec-Fetch-Site", "same-site")
		request.Header.Set("Origin", h.otherOrigin)
	case "handler":
	case "local-with-cookie":
		request.AddCookie(&http.Cookie{Name: "isLoggedIn", Value: "true"})
	default:
		h.t.Fatalf("unknown caller %q", as)
	}
	return request
}

func (h *conformanceHost) record(response *http.Response) {
	h.t.Helper()
	if response.StatusCode < 400 {
		return
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	response.Body.Close()
	h.refusals = append(h.refusals, string(body))
}

func (h *conformanceHost) subscribe(as, stream string) int {
	h.t.Helper()
	query := url.Values{}
	switch as {
	case "handler":
		query.Set("role", "handler")
		query.Set("file", h.document)
	default:
		query.Set("document-url", h.documentURL())
		if h.kind == "loose" && h.token != "" {
			query.Set("token", h.token)
		}
	}
	response, err := http.DefaultClient.Do(h.newRequest(http.MethodGet, "/_/wire/subscribe?"+query.Encode(), "", as))
	if err != nil {
		h.t.Fatalf("GET /_/wire/subscribe: %v", err)
	}
	h.streamAs[stream] = as
	if response.StatusCode >= 400 {
		h.record(response)
		return response.StatusCode
	}
	h.streams[stream] = conformanceReadStream(h.t, response.Body)
	h.t.Cleanup(func() { response.Body.Close() })
	return response.StatusCode
}

func (h *conformanceHost) send(as string, frame json.RawMessage) int {
	h.t.Helper()
	var body map[string]any
	if err := json.Unmarshal(frame, &body); err != nil {
		h.t.Fatalf("case frame is not JSON: %v", err)
	}
	if as == "handler" {
		body["file"] = h.document
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		h.t.Fatal(err)
	}
	request := h.newRequest(http.MethodPost, "/_/wire/send", string(encoded), as)
	request.Header.Set("Content-Type", "application/json")
	if as != "handler" {
		request.Header.Set("Document-URL", h.documentURL())
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		h.t.Fatalf("POST /_/wire/send: %v", err)
	}
	h.record(response)
	if response.StatusCode < 400 {
		response.Body.Close()
	}
	return response.StatusCode
}

func (h *conformanceHost) resubscribe(stream string) {
	h.t.Helper()
	previous := h.streams[stream]
	if previous == nil {
		h.t.Fatalf("stream %q was never opened", stream)
	}
	as := h.streamAs[stream]
	query := url.Values{}
	switch as {
	case "handler":
		query.Set("role", "handler")
		query.Set("file", h.document)
	default:
		query.Set("document-url", h.documentURL())
		if h.kind == "loose" && h.token != "" {
			query.Set("token", h.token)
		}
	}
	request := h.newRequest(http.MethodGet, "/_/wire/subscribe?"+query.Encode(), "", as)
	request.Header.Set("Last-Event-ID", previous.lastEventID())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		h.t.Fatalf("GET /_/wire/subscribe (resume): %v", err)
	}
	if response.StatusCode != http.StatusOK {
		h.record(response)
		h.t.Fatalf("resubscribe to %q answered %d", stream, response.StatusCode)
	}
	h.streams[stream] = conformanceReadStream(h.t, response.Body)
	h.t.Cleanup(func() { response.Body.Close() })
}

func (h *conformanceHost) drop(stream string) {
	h.t.Helper()
	s := h.streams[stream]
	if s == nil {
		h.t.Fatalf("stream %q was never opened", stream)
	}
	s.body.Close()
}

func (h *conformanceHost) run(t *testing.T, c conformanceCase) []string {
	t.Helper()
	for _, step := range c.Steps {
		switch step.Do {
		case "subscribe":
			if got := h.subscribe(step.As, step.Stream); got != step.Expect.Status {
				t.Fatalf("subscribe as %s on %q answered %d, want %d", step.As, step.Stream, got, step.Expect.Status)
			}
		case "send":
			if got := h.send(step.As, step.Frame); got != step.Expect.Status {
				t.Fatalf("send as %s answered %d, want %d", step.As, got, step.Expect.Status)
			}
		case "see":
			s := h.streams[step.Stream]
			if s == nil {
				t.Fatalf("stream %q was never opened", step.Stream)
			}
			s.see(t, step.Type, step.ID, step.Code)
		case "closed":
			s := h.streams[step.Stream]
			if s == nil {
				t.Fatalf("stream %q was never opened", step.Stream)
			}
			s.waitClosed(t)
		case "drop":
			h.drop(step.Stream)
		case "resubscribe":
			h.resubscribe(step.Stream)
		default:
			t.Fatalf("unknown step %q", step.Do)
		}
	}
	return h.refusals
}

func conformanceTopLevelCaseNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(conformanceCasesDir)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

// conformancePackageCase is one released wire case as @panphora/host-conformance
// publishes it in wire.json: the behavior, without this host's own notes.
type conformancePackageCase struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Setup json.RawMessage `json:"setup"`
	Steps json.RawMessage `json:"steps"`
}

// conformanceLoadCases answers every case this host runs. A released case's
// name, setup and steps come from the published package; its hosts, skip and
// known difference stay in the local file, which is also the package's source.
// The cases only this host has are read from their local files alone.
func conformanceLoadCases(t *testing.T) []conformanceCase {
	t.Helper()
	var published struct {
		Cases []conformancePackageCase `json:"cases"`
	}
	testutil.LoadVectors(t, "wire.json", &published)
	byID := make(map[string]conformancePackageCase, len(published.Cases))
	for _, p := range published.Cases {
		byID[p.ID] = p
	}
	names := conformanceTopLevelCaseNames(t)
	cases := make([]conformanceCase, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(conformanceCasesDir, name))
		if err != nil {
			t.Fatal(err)
		}
		var c conformanceCase
		if err := json.Unmarshal(data, &c); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		c.Slug = conformanceCaseSlug(name)
		if c.Served == "released" {
			match := conformanceCaseName.FindStringSubmatch(c.Slug)
			if match == nil {
				t.Fatalf("%s does not match the runner's case pattern <NN>-<slug>.json", name)
			}
			p, ok := byID[match[2]]
			if !ok {
				t.Fatalf("%s is released but the published wire.json has no case %q", name, match[2])
			}
			delete(byID, match[2])
			c.Name, c.Steps = p.Name, nil
			c.Setup = conformanceCase{}.Setup
			if err := json.Unmarshal(p.Setup, &c.Setup); err != nil {
				t.Fatalf("published %s setup: %v", p.ID, err)
			}
			if err := json.Unmarshal(p.Steps, &c.Steps); err != nil {
				t.Fatalf("published %s steps: %v", p.ID, err)
			}
		}
		cases = append(cases, c)
	}
	for id := range byID {
		t.Errorf("the published wire.json has case %q, which no local released case names", id)
	}
	return cases
}

func conformanceCaseSlug(name string) string {
	return strings.TrimSuffix(name, ".json")
}

func TestM3WireConformance(t *testing.T) {
	for _, c := range conformanceLoadCases(t) {
		c := c
		slug := c.Slug
		results := map[string][]string{}
		for _, kind := range c.Hosts {
			kind := kind
			t.Run(slug+"/"+kind, func(t *testing.T) {
				if c.Skip != "" {
					t.Skip(c.Skip)
				}
				var host *conformanceHost
				switch kind {
				case "loose":
					host = conformanceLooseHost(t, c)
				case "served":
					host = conformanceServedHost(t, c)
				default:
					t.Fatalf("unknown host %q", kind)
				}
				results[kind] = host.run(t, c)
			})
		}
		if c.Skip != "" {
			continue
		}
		if len(c.Hosts) < 2 || c.KnownDifference != "" {
			if c.KnownDifference != "" {
				t.Logf("known difference for %q: %s; loose bodies %q, served bodies %q", slug, c.KnownDifference, results["loose"], results["served"])
			}
			continue
		}
		loose, served := results["loose"], results["served"]
		if len(loose) != len(served) {
			t.Fatalf("%s: %d refusal bodies on loose but %d on served: %q vs %q", slug, len(loose), len(served), loose, served)
		}
		for i := range loose {
			if loose[i] != served[i] {
				t.Fatalf("%s: refusal body %d differs: loose %q, served %q", slug, i, loose[i], served[i])
			}
		}
	}
}

func TestM3WireConformanceRunsEveryCase(t *testing.T) {
	validDo := map[string]bool{"subscribe": true, "send": true, "see": true, "closed": true, "drop": true, "resubscribe": true}
	validAs := map[string]bool{"page": true, "page-other-origin": true, "handler": true, "local-with-cookie": true}
	validHosts := map[string]bool{"loose": true, "served": true}
	run := map[string]bool{}
	for _, name := range conformanceTopLevelCaseNames(t) {
		run[conformanceCaseSlug(name)] = true
	}
	err := filepath.WalkDir(conformanceCasesDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil
		}
		rel, err := filepath.Rel(conformanceCasesDir, path)
		if err != nil {
			return err
		}
		slug := conformanceCaseSlug(strings.TrimSuffix(rel, ".json"))
		match := conformanceCaseName.FindStringSubmatch(slug)
		if match == nil {
			t.Errorf("%s does not match the runner's case pattern <NN>-<slug>.json", rel)
			return nil
		}
		if !run[slug] {
			t.Errorf("%s would not be run by TestM3WireConformance", rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var c conformanceCase
		if err := json.Unmarshal(data, &c); err != nil {
			t.Errorf("%s: %v", rel, err)
			return nil
		}
		if c.Served != "released" && c.Served != "unreleased" {
			t.Errorf("%s: served = %q, want released or unreleased", rel, c.Served)
		}
		if len(c.Hosts) == 0 {
			t.Errorf("%s: hosts is empty", rel)
		}
		for _, host := range c.Hosts {
			if !validHosts[host] {
				t.Errorf("%s: unknown host %q", rel, host)
			}
		}
		for i, step := range c.Steps {
			if !validDo[step.Do] {
				t.Errorf("%s: step %d has unknown do %q", rel, i, step.Do)
			}
			if step.As != "" && !validAs[step.As] {
				t.Errorf("%s: step %d has unknown as %q", rel, i, step.As)
			}
			if (step.Do == "subscribe" || step.Do == "send") && step.Expect.Status == 0 {
				t.Errorf("%s: step %d (%s) has no expect.status", rel, i, step.Do)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

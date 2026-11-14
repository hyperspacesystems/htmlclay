package served

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/helper"
	"github.com/panphora/htmlclay/internal/served/assets"
	"github.com/panphora/htmlclay/internal/wire"
)

// askingPolicy is a test policy that may ask a person to set a program up, and
// grants nothing else.
type askingPolicy struct{}

func (askingPolicy) CanAskProgram() bool                          { return true }
func (askingPolicy) AskProgram(file, name string) bool            { return false }
func (askingPolicy) ProgramGranted(file, name, id string) bool    { return false }
func (askingPolicy) AIEditOffered() bool                          { return false }
func (askingPolicy) AIEditFor(string) (bool, bool)                { return false, false }
func (askingPolicy) AskAIEdit(string)                             {}
func (askingPolicy) AIEditEngines() (string, map[string][]string) { return "", nil }

const (
	programsRootID    = "programs-root"
	programsRootHost  = "programs.htmlclay.localhost:1212"
	programsDocName   = "programs.html"
	programsHelperNom = "ocr"
)

func programsDocument() string {
	return "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"htmlclay-helper\" content=\"" + programsHelperNom + "\">\n<title>Programs</title>\n</head>\n<body>\n<h1>Programs</h1>\n</body>\n</html>\n"
}

// programsStand is one served folder holding a document that declares a
// program, plus the host that serves it.
type programsStand struct {
	host       *Host
	rootID     string
	dir        string
	publicHost string
	handler    http.Handler
}

func newProgramsStand(t *testing.T, policy wire.Policy, beforePage func(rootID, absPath string)) *programsStand {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, programsDocName), []byte(programsDocument()), 0o644); err != nil {
		t.Fatal(err)
	}
	host := NewHost(Options{
		Assets:          assets.ProductFS(),
		CompileTailwind: func(string) (string, error) { return "", nil },
		BeforePage:      beforePage,
	})
	t.Cleanup(host.Close)
	handler, err := host.AddRoot(RootSpec{
		ID:         programsRootID,
		Path:       dir,
		PublicHost: programsRootHost,
		WirePolicy: policy,
		WireLogger: wireTestLogger(t),
	})
	if err != nil {
		t.Fatalf("AddRoot: %v", err)
	}
	return &programsStand{host: host, rootID: programsRootID, dir: dir, publicHost: programsRootHost, handler: handler}
}

// file is the document's symlink-resolved path, the one the wire keys on.
func (s *programsStand) file(t *testing.T) string {
	t.Helper()
	return realOrSame(filepath.Join(s.dir, programsDocName))
}

func (s *programsStand) documentURL() string {
	return "http://" + s.publicHost + "/" + programsDocName
}

// request builds one request the way a page of this root makes it.
func (s *programsStand) request(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, "http://"+s.publicHost+path, reader)
	request.Host = s.publicHost
	return request
}

func (s *programsStand) get(t *testing.T, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := s.request(t, http.MethodGet, path, "")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	return recorder
}

type pageCall struct {
	rootID  string
	absPath string
	written int
}

// countingWriter records how many bytes reached the response.
type countingWriter struct {
	http.ResponseWriter
	written int
}

func (w *countingWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.written += n
	return n, err
}

// TestM3ServedProgramsHost is the served host's side of programs: it hears about
// a page load before the page is written, reports programs in /_/meta, answers
// the page's setup call, and shows the setup bar in the page and never in the
// file.
func TestM3ServedProgramsHost(t *testing.T) {
	t.Run("BeforePage runs before the page is written", func(t *testing.T) {
		var writer *countingWriter
		var calls []pageCall
		stand := newProgramsStand(t, askingPolicy{}, func(rootID, absPath string) {
			written := -1
			if writer != nil {
				written = writer.written
			}
			calls = append(calls, pageCall{rootID: rootID, absPath: absPath, written: written})
		})

		recorder := httptest.NewRecorder()
		writer = &countingWriter{ResponseWriter: recorder}
		request := stand.request(t, http.MethodGet, "/"+programsDocName, "")
		request.Header.Set("Sec-Fetch-Mode", "navigate")
		request.Header.Set("Sec-Fetch-Dest", "document")
		stand.handler.ServeHTTP(writer, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("the page load answered %d", recorder.Code)
		}
		if len(calls) != 1 {
			t.Fatalf("BeforePage heard %d calls, want the one page load: %+v", len(calls), calls)
		}
		call := calls[0]
		if call.rootID != programsRootID {
			t.Fatalf("BeforePage heard root %q, want %q", call.rootID, programsRootID)
		}
		if call.absPath != stand.file(t) {
			t.Fatalf("BeforePage heard %q, want the document's absolute path %q", call.absPath, stand.file(t))
		}
		if call.written != 0 {
			t.Fatalf("BeforePage ran after %d bytes of the page were written", call.written)
		}

		calls = nil
		stand.get(t, "/"+programsDocName, map[string]string{"Clay-Source": "1", "Sec-Fetch-Dest": "document"})
		stand.get(t, "/style.css", map[string]string{"Sec-Fetch-Dest": "style"})
		stand.get(t, "/"+programsDocName, map[string]string{"Sec-Fetch-Dest": "empty"})
		if len(calls) != 0 {
			t.Fatalf("a source read, an asset or a fetch reached BeforePage: %+v", calls)
		}
	})

	t.Run("meta reports programs", func(t *testing.T) {
		stand := newProgramsStand(t, askingPolicy{}, nil)
		core, ok := stand.host.Wire(stand.rootID)
		if !ok {
			t.Fatal("the root carries no wire")
		}
		if err := core.AttachHelpers(stand.file(t), nil, []string{programsHelperNom}); err != nil {
			t.Fatal(err)
		}

		meta := stand.get(t, "/_/meta", map[string]string{"Document-URL": stand.documentURL()})
		if meta.Code != http.StatusOK {
			t.Fatalf("/_/meta answered %d: %s", meta.Code, meta.Body.String())
		}
		var answer map[string]any
		if err := json.Unmarshal(meta.Body.Bytes(), &answer); err != nil {
			t.Fatalf("/_/meta answered %q, which is not JSON: %v", meta.Body.String(), err)
		}
		document, ok := answer["document"].(map[string]any)
		if !ok {
			t.Fatalf("/_/meta answered no document block: %v", answer)
		}
		if document["wireBudgetMs"] != float64(helper.StructuredDeadline) {
			t.Fatalf("wireBudgetMs = %v, want %d", document["wireBudgetMs"], helper.StructuredDeadline)
		}
		helpers, ok := document["helpers"].([]any)
		if !ok || len(helpers) != 1 {
			t.Fatalf("helpers = %v, want the one declared program", document["helpers"])
		}
		entry, _ := helpers[0].(map[string]any)
		if entry["name"] != programsHelperNom || entry["state"] != "denied" {
			t.Fatalf("helpers[0] = %v, want ocr denied", helpers[0])
		}

		plain := newProgramsStand(t, nil, nil)
		plainMeta := plain.get(t, "/_/meta", map[string]string{"Document-URL": plain.documentURL()})
		if plainMeta.Code != http.StatusOK {
			t.Fatalf("the root without a wire answered /_/meta %d", plainMeta.Code)
		}
		var plainAnswer map[string]any
		if err := json.Unmarshal(plainMeta.Body.Bytes(), &plainAnswer); err != nil {
			t.Fatalf("/_/meta answered %q, which is not JSON: %v", plainMeta.Body.String(), err)
		}
		plainDocument, ok := plainAnswer["document"].(map[string]any)
		if !ok {
			t.Fatalf("the root without a wire answered no document block: %v", plainAnswer)
		}
		for _, key := range []string{"wireMode", "wireBudgetMs", "helpers"} {
			if _, present := plainDocument[key]; present {
				t.Fatalf("a root without WirePolicy reported %s: %v", key, plainDocument)
			}
		}
	})

	t.Run("the setup call is the page's alone", func(t *testing.T) {
		stand := newProgramsStand(t, askingPolicy{}, nil)
		core, ok := stand.host.Wire(stand.rootID)
		if !ok {
			t.Fatal("the root carries no wire")
		}
		if err := core.AttachHelpers(stand.file(t), nil, []string{programsHelperNom}); err != nil {
			t.Fatal(err)
		}

		page := stand.request(t, http.MethodPost, "/_/helper-setup", `{"name":"`+programsHelperNom+`"}`)
		page.Header.Set("Content-Type", "application/json")
		page.Header.Set("Sec-Fetch-Site", "same-origin")
		page.Header.Set("Origin", "http://"+stand.publicHost)
		page.Header.Set("Document-URL", stand.documentURL())
		recorder := httptest.NewRecorder()
		stand.handler.ServeHTTP(recorder, page)
		if recorder.Code == http.StatusForbidden || recorder.Code == http.StatusNotFound {
			t.Fatalf("the page's setup call answered %d: %s", recorder.Code, recorder.Body.String())
		}
		t.Logf("HandleSetup answered %d: %s", recorder.Code, strings.TrimSpace(recorder.Body.String()))

		local := stand.request(t, http.MethodPost, "/_/helper-setup", `{"name":"`+programsHelperNom+`"}`)
		local.Header.Set("Content-Type", "application/json")
		localRecorder := httptest.NewRecorder()
		stand.handler.ServeHTTP(localRecorder, local)
		if localRecorder.Code != http.StatusForbidden {
			t.Fatalf("a local process's setup call answered %d, want 403", localRecorder.Code)
		}

		plain := newProgramsStand(t, nil, nil)
		plainRequest := plain.request(t, http.MethodPost, "/_/helper-setup", `{"name":"`+programsHelperNom+`"}`)
		plainRequest.Header.Set("Content-Type", "application/json")
		plainRequest.Header.Set("Sec-Fetch-Site", "same-origin")
		plainRequest.Header.Set("Origin", "http://"+plain.publicHost)
		plainRequest.Header.Set("Document-URL", plain.documentURL())
		plainRecorder := httptest.NewRecorder()
		plain.handler.ServeHTTP(plainRecorder, plainRequest)
		if plainRecorder.Code != http.StatusNotFound || strings.TrimSpace(plainRecorder.Body.String()) != "File not found" {
			t.Fatalf("a root without WirePolicy answered the setup call %d: %q", plainRecorder.Code, plainRecorder.Body.String())
		}
	})

	t.Run("the bar is in the page and never in the file", func(t *testing.T) {
		stand := newProgramsStand(t, askingPolicy{}, nil)
		core, ok := stand.host.Wire(stand.rootID)
		if !ok {
			t.Fatal("the root carries no wire")
		}
		if err := core.AttachHelpers(stand.file(t), nil, nil); err != nil {
			t.Fatal(err)
		}

		page := stand.get(t, "/"+programsDocName, map[string]string{
			"Sec-Fetch-Mode": "navigate",
			"Sec-Fetch-Dest": "document",
		})
		if page.Code != http.StatusOK {
			t.Fatalf("the page load answered %d", page.Code)
		}
		served := page.Body.String()
		for _, want := range []string{"data-htmlclay-setup", "fetch('/_/helper-setup',"} {
			if !strings.Contains(served, want) {
				t.Fatalf("the page load carries no %q: %s", want, served)
			}
		}

		source := stand.get(t, "/"+programsDocName, map[string]string{
			"Sec-Fetch-Dest": "document",
			"Clay-Source":    "1",
		})
		if source.Code != http.StatusOK {
			t.Fatalf("the source read answered %d", source.Code)
		}
		if strings.Contains(source.Body.String(), "data-htmlclay-setup") {
			t.Fatalf("the source read carries the setup bar: %s", source.Body.String())
		}

		save := stand.request(t, http.MethodPost, "/_/save", served)
		save.Header.Set("Content-Type", "text/html")
		save.Header.Set("Sec-Fetch-Site", "same-origin")
		save.Header.Set("Origin", "http://"+stand.publicHost)
		save.Header.Set("Document-URL", stand.documentURL())
		saveRecorder := httptest.NewRecorder()
		stand.handler.ServeHTTP(saveRecorder, save)
		if saveRecorder.Code != http.StatusOK {
			t.Fatalf("the save answered %d: %s", saveRecorder.Code, saveRecorder.Body.String())
		}
		stored, err := os.ReadFile(filepath.Join(stand.dir, programsDocName))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(stored), "data-htmlclay-setup") {
			t.Fatalf("the saved file carries the setup bar: %s", stored)
		}
	})
}

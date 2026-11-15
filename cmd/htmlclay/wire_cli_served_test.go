package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/specwire"
)

// sentFrame is one frame a fake origin received.
type sentFrame struct {
	host string
	body string
}

// wireServedFixture is one running app serving a folder, with the config the CLI
// reads naming that folder on the port the test listener actually answers on.
type wireServedFixture struct {
	*servedWireStand
	cfgBase string
	file    string
}

func newWireServedFixture(t *testing.T) *wireServedFixture {
	t.Helper()
	home := evalTempHome(t)
	dir := filepath.Join(home, "notes")
	writeTestFile(t, filepath.Join(dir, servedWireDocumentName), servedTestDocument)

	cfgBase := t.TempDir()
	a := newServedTestApp(t, home, cfgBase)
	server := httptest.NewServer(http.HandlerFunc(a.serveHost))
	t.Cleanup(server.Close)
	_, portStr, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("cannot read the test listener's port from %s: %v", server.URL, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	a.rt.port = port
	a.rt.cfg.SetPort(port)

	entry, refusal, err := a.serveFolder(dir, "notes")
	if err != nil {
		t.Fatalf("serveFolder: %v", err)
	}
	if refusal != nil {
		t.Fatalf("serveFolder refused: %+v", refusal)
	}

	real, err := filepath.EvalSymlinks(filepath.Join(dir, servedWireDocumentName))
	if err != nil {
		t.Fatal(err)
	}
	return &wireServedFixture{
		servedWireStand: &servedWireStand{
			a:      a,
			home:   home,
			entry:  entry,
			dir:    dir,
			host:   servedHostName("notes", port),
			server: server,
		},
		cfgBase: cfgBase,
		file:    real,
	}
}

// rewriteCommand is the handler command: this test binary, replacing the
// document with content.
func (f *wireServedFixture) rewriteCommand(t *testing.T, content string) []string {
	t.Helper()
	body := filepath.Join(t.TempDir(), "rewritten.html")
	if err := os.WriteFile(body, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTMLCLAY_WIRE_HELPER", "1")
	t.Setenv("HTMLCLAY_WIRE_HELPER_MODE", "rewrite")
	t.Setenv("HTMLCLAY_WIRE_HELPER_FROM", body)
	t.Setenv("HTMLCLAY_WIRE_HELPER_OUT", f.file)
	return wireHelperCommand()
}

// wireServedHandlerRewrite attaches the CLI as the served document's handler,
// makes the page ask for a rewrite, and returns once the command has answered.
func (f *wireServedFixture) wireServedHandlerRewrite(t *testing.T, rewritten string) {
	t.Helper()
	server := newWireHarness(t, f.cfgBase)
	serving := server.background(append([]string{"serve", f.file, "--"}, f.rewriteCommand(t, rewritten)...)...)
	waitForText(t, server.errs, "attached as handler")

	response, page := f.pageSubscribe(t, servedWireDocumentName)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("page subscription answered %d: %s", response.StatusCode, servedWireReadBody(t, response))
	}
	sent := f.pageSend(t, `{"type":"wire/request","id":"r1","payload":{}}`, servedWireDocumentName)
	if sent.StatusCode != http.StatusOK {
		t.Fatalf("page request answered %d: %s", sent.StatusCode, servedWireReadBody(t, sent))
	}
	page.waitFor(t, "wire/done", "r1")

	onDisk, err := os.ReadFile(f.file)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != rewritten {
		t.Fatalf("the document on disk = %q, want the handler's bytes %q", onDisk, rewritten)
	}

	server.cancel()
	waitForExit(t, serving)
}

// TestM3WireDiscovery is step 7.30: `htmlclay wire` finds a document in a served
// folder by the same config and the same probe it uses for loose sites.
func TestM3WireDiscovery(t *testing.T) {
	port := 24190
	dir := filepath.Join(string(filepath.Separator), "home", "u", "notes")

	t.Run("a served file's candidate", func(t *testing.T) {
		cfg := wireConfig{
			Port:          port,
			ServedFolders: []wireServedFolder{{Name: "notes", Path: dir, Serving: true}},
		}
		got := wireCandidates(cfg, filepath.Join(dir, "a.htmlclay"))
		if len(got) == 0 {
			t.Fatal("a serving folder gave no candidate at all")
		}
		first := got[0]
		wantHost := servedHostName("notes", port)
		if first.host != wantHost || first.port != port || first.anchor != dir || !first.ancestor {
			t.Fatalf("first candidate = %+v, want host %q on port %d anchored at %s as an ancestor",
				first, wantHost, port, dir)
		}
	})

	t.Run("a stopped folder, a retired name and a path that needs encoding", func(t *testing.T) {
		notes := filepath.Join(evalTempHome(t), "my notes")
		stopped := filepath.Join(string(filepath.Separator), "home", "u", "old")
		file := filepath.Join(notes, "my notes #1.htmlclay")
		writeTestFile(t, file, servedTestDocument)

		var sent []sentFrame
		port := fakeOrigin(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/_/wire/send" {
				http.Error(w, "no such route", http.StatusNotFound)
				return
			}
			body, _ := io.ReadAll(r.Body)
			sent = append(sent, sentFrame{host: r.Host, body: string(body)})
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"delivered":1}`)
		}))

		cfgBase := t.TempDir()
		body := fmt.Sprintf(`{"port":%d,"servedFolders":[`+
			`{"id":"0123456789abcdef","name":"notes","path":%q,"serving":true},`+
			`{"id":"fedcba9876543210","name":"old","path":%q,"serving":false}],`+
			`"retiredServedNames":["gone"]}`, port, notes, stopped)
		cfgPath := filepath.Join(config.DirFrom(cfgBase), "config.json")
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := wireReadConfig(cfgPath)

		got := wireCandidates(cfg, file)
		if len(got) == 0 || got[0].host != servedHostName("notes", port) || got[0].anchor != notes {
			t.Fatalf("candidates for a served file = %+v, want the serving folder first", got)
		}
		for _, c := range got {
			if c.anchor == stopped {
				t.Fatalf("a stopped folder gave a candidate: %+v", c)
			}
			if strings.HasPrefix(c.host, "old.htmlclay.localhost:") || strings.HasPrefix(c.host, "gone.htmlclay.localhost:") {
				t.Fatalf("a stopped or retired name gave a candidate: %+v", c)
			}
		}

		// The path reaches the origin exactly as it is on disk: the space and the
		// hash are the query's to encode, not the CLI's to mangle.
		raw := wireSubscribeURL(got[0].port, file, true, "")
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("the subscribe URL %q is not parseable: %v", raw, err)
		}
		if asked := u.Query().Get("file"); asked != file {
			t.Fatalf("the subscription names file %q, want %q (url %s)", asked, file, raw)
		}

		h := newWireHarness(t, cfgBase)
		if code := h.run("send", "--type", "wire/request", "--id", "req-1", file); code != wireExitOK {
			t.Fatalf("send exited %d; stderr:\n%s", code, h.errs.String())
		}
		if len(sent) != 1 {
			t.Fatalf("the origin saw %d frames, want 1", len(sent))
		}
		var frame wireFrame
		if err := json.Unmarshal([]byte(sent[0].body), &frame); err != nil {
			t.Fatalf("the frame %q is not an envelope: %v", sent[0].body, err)
		}
		if frame.File != file {
			t.Fatalf("the frame carries file %q, want %q", frame.File, file)
		}
		if want := servedHostName("notes", port); sent[0].host != want {
			t.Fatalf("the frame was sent to host %q, want %q", sent[0].host, want)
		}
	})

	t.Run("a handler attaches through the served address and its write keeps the old bytes", func(t *testing.T) {
		f := newWireServedFixture(t)
		rewritten := strings.Replace(servedTestDocument, "<p>first</p>", "<p>rewritten</p>", 1)
		f.wireServedHandlerRewrite(t, rewritten)

		versions, err := f.a.served.Versions(f.entry.ID, servedWireDocumentName, "", 10)
		if err != nil {
			t.Fatalf("Versions: %v", err)
		}
		var kept bool
		for _, row := range versions {
			data, err := f.a.served.Version(f.entry.ID, servedWireDocumentName, row.ID)
			if err != nil {
				t.Fatalf("Version %s: %v", row.ID, err)
			}
			if string(data) == servedTestDocument {
				kept = true
			}
		}
		if !kept {
			t.Fatalf("the bytes from before the handler's write are in no version: %+v", versions)
		}
	})

	t.Run("a page save with the old etag answers 412 after the handler wrote", func(t *testing.T) {
		f := newWireServedFixture(t)
		before := specwire.Etag([]byte(servedTestDocument))
		rewritten := strings.Replace(servedTestDocument, "<p>first</p>", "<p>rewritten</p>", 1)
		f.wireServedHandlerRewrite(t, rewritten)

		rec := servedRequest(t, f.a, http.MethodPost, f.host, "/_/save", servedTestDocument, map[string]string{
			"Document-URL": "http://" + f.host + "/" + servedWireDocumentName,
			"Content-Type": "text/html",
			"If-Match":     before,
		})
		if rec.Code != http.StatusPreconditionFailed {
			t.Fatalf("POST /_/save with the old etag = %d, want %d (%s)", rec.Code, http.StatusPreconditionFailed, rec.Body.String())
		}
		onDisk, err := os.ReadFile(f.file)
		if err != nil {
			t.Fatal(err)
		}
		if string(onDisk) != rewritten {
			t.Fatalf("the document on disk = %q, want the handler's bytes left alone", onDisk)
		}
	})

	t.Run("a file under .hyperclay is not admitted", func(t *testing.T) {
		f := newWireServedFixture(t)
		hidden := filepath.Join(f.dir, ".hyperclay", "x.htmlclay")
		writeTestFile(t, hidden, servedTestDocument)
		real, err := filepath.EvalSymlinks(hidden)
		if err != nil {
			t.Fatal(err)
		}

		cfg := wireReadConfig(filepath.Join(config.DirFrom(f.cfgBase), "config.json"))
		cands := wireCandidates(cfg, real)
		if len(cands) == 0 {
			t.Fatal("the served folder gave no candidate for a file under .hyperclay")
		}
		response, _ := f.processSubscribe(t, real)
		servedWireReadBody(t, response)
		if response.StatusCode != http.StatusForbidden && response.StatusCode != http.StatusNotFound {
			t.Fatalf("a handler subscription for %s answered %d, want 403 or 404", real, response.StatusCode)
		}
	})
}

package wire

import (
	"bufio"
	"context"
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

	"github.com/panphora/htmlclay/internal/aiedit"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/logging"
)

type frameStream struct {
	ch chan Envelope
}

func streamFrames(t *testing.T, body io.Reader) *frameStream {
	t.Helper()
	fs := &frameStream{ch: make(chan Envelope, 64)}
	go func() {
		defer close(fs.ch)
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
			var env Envelope
			if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &env); err == nil && env.Type != "" {
				fs.ch <- env
			}
			data = nil
		}
	}()
	return fs
}

func (fs *frameStream) waitFor(t *testing.T, typ, id string) Envelope {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case env, ok := <-fs.ch:
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

func subscribePage(t *testing.T, srv *httptest.Server, document string) (*http.Response, *frameStream) {
	t.Helper()
	q := url.Values{"document-url": {srv.URL + "/" + document}}
	req, err := http.NewRequest("GET", srv.URL+"/_/wire/subscribe?"+q.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp, streamFrames(t, resp.Body)
}

func subscribeHandler(t *testing.T, srv *httptest.Server, file string) (*http.Response, *frameStream) {
	t.Helper()
	q := url.Values{"role": {"handler"}, "file": {file}}
	resp, err := http.Get(srv.URL + "/_/wire/subscribe?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp, streamFrames(t, resp.Body)
}

func sendFrame(t *testing.T, srv *httptest.Server, body, document string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("POST", srv.URL+"/_/wire/send", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if document != "" {
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Document-URL", srv.URL+"/"+document)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func subscriberCount(c *Core, file string) int {
	c.Hub.Mu.Lock()
	defer c.Hub.Mu.Unlock()
	if ch, ok := c.Hub.Chans[file]; ok {
		return len(ch.subs)
	}
	return 0
}

func waitSubscribers(t *testing.T, c *Core, file string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if subscriberCount(c, file) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("subscriber count for %s = %d, want %d", filepath.Base(file), subscriberCount(c, file), want)
}

func waitTerminal(t *testing.T, sub *Sub, id string) Envelope {
	t.Helper()
	for i := 0; i < 16; i++ {
		env := receiveHelperEnvelope(t, sub)
		if env.IsTerminal() && env.ID == id {
			return env
		}
	}
	t.Fatalf("did not receive a terminal frame for %s", id)
	return Envelope{}
}

func waitEOF(t *testing.T, resp *http.Response) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, resp.Body)
		done <- err
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream stayed open after shutdown")
	}
}

func TestM3CoreWithFakeHost(t *testing.T) {
	t.Run("a page subscribes", func(t *testing.T) {
		core, _, _, file := newFakeCore(t)
		srv := httptest.NewServer(core.WireMux())
		t.Cleanup(srv.Close)

		resp, _ := subscribePage(t, srv, "doc.htmlclay")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("page subscription refused: %d", resp.StatusCode)
		}
		waitSubscribers(t, core, file, 1)
	})

	t.Run("a process handler subscribes and a request reaches it", func(t *testing.T) {
		core, _, _, file := newFakeCore(t)
		srv := httptest.NewServer(core.WireMux())
		t.Cleanup(srv.Close)

		handlerResp, handler := subscribeHandler(t, srv, file)
		if handlerResp.StatusCode != http.StatusOK {
			t.Fatalf("handler subscription refused: %d", handlerResp.StatusCode)
		}
		pageResp, page := subscribePage(t, srv, "doc.htmlclay")
		if pageResp.StatusCode != http.StatusOK {
			t.Fatalf("page subscription refused: %d", pageResp.StatusCode)
		}
		waitSubscribers(t, core, file, 2)

		resp := sendFrame(t, srv, `{"type":"wire/request","id":"r1","payload":{}}`, "doc.htmlclay")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("page request refused: %d", resp.StatusCode)
		}
		request := handler.waitFor(t, "wire/request", "r1")
		if request.From != "page" {
			t.Fatalf("request From = %q, want page", request.From)
		}
		if request.File != file {
			t.Fatalf("request File = %q, want %q", request.File, file)
		}

		body := fmt.Sprintf(`{"type":"wire/done","id":"r1","file":%q}`, file)
		resp = sendFrame(t, srv, body, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("handler answer refused: %d", resp.StatusCode)
		}
		page.waitFor(t, "wire/done", "r1")
	})

	t.Run("MayUsePrograms false refuses a bound channel and a built in request", func(t *testing.T) {
		core, docs, _, file := newFakeCore(t)
		docs.mu.Lock()
		docs.mayUse = false
		docs.mu.Unlock()
		core.BindHelpers(file)
		srv := httptest.NewServer(core.WireMux())
		t.Cleanup(srv.Close)

		resp, _ := subscribePage(t, srv, "doc.htmlclay")
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("tokenless subscription status %d, want 403", resp.StatusCode)
		}

		body := fmt.Sprintf(`{"type":"wire/request","id":"ai","helper":%q}`, aiedit.HelperName)
		resp = sendFrame(t, srv, body, "doc.htmlclay")
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("tokenless built in request status %d, want 403", resp.StatusCode)
		}
	})

	t.Run("ProgramGranted false never starts the program", func(t *testing.T) {
		core, _, policy, file := newFakeCore(t)
		marker := filepath.Join(t.TempDir(), "ran")
		t.Setenv(wireHelperArg, marker)
		programs := map[string]config.HelperProgram{"marker": helperProgram("marker", structuredHelper(t, "marker"))}
		if err := core.AttachHelpers(file, programs, nil); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { core.DetachHelpers(file) })
		policy.mu.Lock()
		policy.granted = false
		policy.mu.Unlock()

		sub := observer(t, core, file)
		core.Hub.Publish(file, Envelope{
			V: 1, Type: "wire/request", ID: "run", File: file,
			Helper: "marker", Document: "none", Payload: json.RawMessage(`{}`),
		})
		waitTerminal(t, sub, "run")
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("the refused request ran the program: stat err = %v", err)
		}

		control, _, controlPolicy, controlFile := newFakeCore(t)
		if err := control.AttachHelpers(controlFile, programs, nil); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { control.DetachHelpers(controlFile) })
		controlPolicy.mu.Lock()
		controlPolicy.granted = true
		controlPolicy.mu.Unlock()

		controlSub := observer(t, control, controlFile)
		control.Hub.Publish(controlFile, Envelope{
			V: 1, Type: "wire/request", ID: "run", File: controlFile,
			Helper: "marker", Document: "none", Payload: json.RawMessage(`{}`),
		})
		waitTerminal(t, controlSub, "run")
		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("the granted request never ran the program: %v", err)
		}
	})

	t.Run("Shutdown closes every stream", func(t *testing.T) {
		core, _, _, file := newFakeCore(t)
		other := filepath.Join(filepath.Dir(file), "other.htmlclay")
		writeHelperDocument(t, other, "marker")
		srv := httptest.NewServer(core.WireMux())
		t.Cleanup(srv.Close)

		first, _ := subscribePage(t, srv, "doc.htmlclay")
		second, _ := subscribePage(t, srv, "other.htmlclay")
		if first.StatusCode != http.StatusOK || second.StatusCode != http.StatusOK {
			t.Fatalf("subscription statuses %d and %d, want 200", first.StatusCode, second.StatusCode)
		}
		waitSubscribers(t, core, file, 1)
		waitSubscribers(t, core, other, 1)

		core.Shutdown()
		waitEOF(t, first)
		waitEOF(t, second)
	})

	t.Run("two cores with one path do not share frames", func(t *testing.T) {
		one, _, _, file := newFakeCore(t)
		root := filepath.Dir(file)
		two := NewCore(&fakeDocs{root: root, mayUse: true}, &fakePolicy{granted: true}, logging.NewStdout(), func(w http.ResponseWriter, status int, message string) { http.Error(w, message, status) })
		t.Cleanup(two.Shutdown)

		firstSub := observer(t, one, file)
		secondSub := observer(t, two, file)

		one.Hub.Publish(file, Envelope{V: 1, Type: "wire/status", ID: "s1", File: file, Text: "only here"})
		receiveHelperType(t, firstSub, "wire/status", "s1")

		select {
		case env := <-secondSub.Ch:
			t.Fatalf("the other core received %q", env)
		case <-time.After(300 * time.Millisecond):
		}
	})
}

func TestM3AIEditAsks(t *testing.T) {
	asked := func(policy *fakePolicy) []string {
		policy.mu.Lock()
		defer policy.mu.Unlock()
		return append([]string(nil), policy.asked...)
	}

	t.Run("on runs the edit", func(t *testing.T) {
		t.Setenv("MOCK_MODEL", "1")
		core, _, policy, file := newFakeCore(t)
		policy.aiOffered = true
		policy.aiOn = true
		srv := httptest.NewServer(core.WireMux())
		t.Cleanup(srv.Close)

		sub := observer(t, core, file)
		body := fmt.Sprintf(`{"type":"wire/request","id":"e1","helper":%q,"payload":{"elementHTML":"<p>Old</p>","tag":"p","comment":"make it [mock:end_turn]","editId":"p"}}`, aiedit.HelperName)
		if resp := sendFrame(t, srv, body, "doc.htmlclay"); resp.StatusCode != http.StatusOK {
			t.Fatalf("page request refused: %d", resp.StatusCode)
		}
		receiveHelperType(t, sub, "wire/ack", "e1")
		if got := asked(policy); len(got) != 0 {
			t.Fatalf("asked = %v, want nothing", got)
		}
	})

	t.Run("sibling files attach only for a document the policy vouches for", func(t *testing.T) {
		t.Setenv("MOCK_MODEL", "1")
		for _, files := range []bool{false, true} {
			core, _, policy, file := newFakeCore(t)
			policy.aiOffered = true
			policy.aiOn = true
			policy.aiFiles = files
			if err := os.WriteFile(filepath.Join(filepath.Dir(file), "styles.css"), []byte("p { color: red }"), 0o644); err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(core.WireMux())
			t.Cleanup(srv.Close)

			sub := observer(t, core, file)
			body := fmt.Sprintf(`{"type":"wire/request","id":"e1","helper":%q,"payload":{"elementHTML":"<p>Old</p>","tag":"p","comment":"make it [mock:end_turn]","editId":"p","contextRefs":["styles.css"]}}`, aiedit.HelperName)
			if resp := sendFrame(t, srv, body, "doc.htmlclay"); resp.StatusCode != http.StatusOK {
				t.Fatalf("page request refused: %d", resp.StatusCode)
			}
			end := receiveHelperEnvelope(t, sub)
			for end.Type != "wire/done" && end.Type != "wire/error" {
				end = receiveHelperEnvelope(t, sub)
			}
			if files && end.Type != "wire/done" {
				t.Fatalf("a document that may share files ended with %s %q", end.Type, end.Text)
			}
			if !files && (end.Type != "wire/error" || helperErrorCode(t, end) != "invalid_context") {
				t.Fatalf("a document that may not share files ended with %s %q, want invalid_context", end.Type, end.Text)
			}
		}
	})

	t.Run("off refuses with the off text and asks nothing", func(t *testing.T) {
		core, _, policy, file := newFakeCore(t)
		policy.aiOffered = true
		srv := httptest.NewServer(core.WireMux())
		t.Cleanup(srv.Close)

		sub := observer(t, core, file)
		body := fmt.Sprintf(`{"type":"wire/request","id":"e1","helper":%q,"payload":{}}`, aiedit.HelperName)
		if resp := sendFrame(t, srv, body, "doc.htmlclay"); resp.StatusCode != http.StatusOK {
			t.Fatalf("page request refused: %d", resp.StatusCode)
		}
		refusal := receiveHelperType(t, sub, "wire/error", "e1")
		if code := helperErrorCode(t, refusal); code != "helper_not_granted" {
			t.Fatalf("refusal code = %q, want helper_not_granted", code)
		}
		if refusal.Text != aiEditOffText {
			t.Fatalf("refusal text = %q, want the off text", refusal.Text)
		}
		if got := asked(policy); len(got) != 0 {
			t.Fatalf("asked = %v, want nothing", got)
		}
	})

	t.Run("ask refuses with the ask text and asks once per request", func(t *testing.T) {
		core, _, policy, file := newFakeCore(t)
		policy.aiOffered = true
		policy.aiAsk = true
		srv := httptest.NewServer(core.WireMux())
		t.Cleanup(srv.Close)

		sub := observer(t, core, file)
		body := fmt.Sprintf(`{"type":"wire/request","id":"e1","helper":%q,"payload":{}}`, aiedit.HelperName)
		if resp := sendFrame(t, srv, body, "doc.htmlclay"); resp.StatusCode != http.StatusOK {
			t.Fatalf("page request refused: %d", resp.StatusCode)
		}
		refusal := receiveHelperType(t, sub, "wire/error", "e1")
		if code := helperErrorCode(t, refusal); code != "helper_not_granted" {
			t.Fatalf("refusal code = %q, want helper_not_granted", code)
		}
		if refusal.Text != aiEditAskText {
			t.Fatalf("refusal text = %q, want the ask text", refusal.Text)
		}
		if got := asked(policy); len(got) != 1 || got[0] != file {
			t.Fatalf("asked = %v, want [%s]", got, file)
		}
	})

	t.Run("the page tool asks the same way", func(t *testing.T) {
		core, _, policy, file := newFakeCore(t)
		policy.aiOffered = true
		policy.aiAsk = true
		srv := httptest.NewServer(core.WireMux())
		t.Cleanup(srv.Close)

		sub := observer(t, core, file)
		body := fmt.Sprintf(`{"type":"wire/request","id":"e1","helper":%q,"payload":{}}`, aiedit.PageToolHelperName)
		if resp := sendFrame(t, srv, body, "doc.htmlclay"); resp.StatusCode != http.StatusOK {
			t.Fatalf("page request refused: %d", resp.StatusCode)
		}
		refusal := receiveHelperType(t, sub, "wire/error", "e1")
		if code := helperErrorCode(t, refusal); code != "helper_not_granted" {
			t.Fatalf("refusal code = %q, want helper_not_granted", code)
		}
		if refusal.Text != aiEditAskText {
			t.Fatalf("refusal text = %q, want the ask text", refusal.Text)
		}
		if got := asked(policy); len(got) != 1 || got[0] != file {
			t.Fatalf("asked = %v, want [%s]", got, file)
		}
	})
}

func TestM3WireForgetAndBound(t *testing.T) {
	t.Run("Forget cancels a running AI edit on that document only", func(t *testing.T) {
		core, _, _, file := newFakeCore(t)
		other := filepath.Join(filepath.Dir(file), "other.htmlclay")
		ctx, cancel := context.WithCancel(context.Background())
		otherCtx, otherCancel := context.WithCancel(context.Background())
		t.Cleanup(otherCancel)
		core.AIEdit.Mu.Lock()
		core.AIEdit.Running = map[string]AIEditRun{file: {ID: "a", Cancel: cancel}, other: {ID: "b", Cancel: otherCancel}}
		core.AIEdit.Mu.Unlock()
		core.Forget(file)
		if ctx.Err() == nil {
			t.Fatal("Forget left the document's AI edit running")
		}
		if otherCtx.Err() != nil {
			t.Fatal("Forget cancelled another document's AI edit")
		}
	})
	t.Run("BoundFiles lists bound documents", func(t *testing.T) {
		core, _, _, file := newFakeCore(t)
		writeHelperDocument(t, filepath.Join(filepath.Dir(file), "other.htmlclay"), "marker")
		if got := core.BoundFiles(); len(got) != 0 {
			t.Fatalf("BoundFiles before any bind = %v, want empty", got)
		}

		core.BindHelpers(file)
		got := core.BoundFiles()
		if len(got) != 1 || got[0] != file {
			t.Fatalf("BoundFiles = %v, want [%s]", got, file)
		}

		core.UnbindHelpers(file)
		if got := core.BoundFiles(); len(got) != 0 {
			t.Fatalf("BoundFiles after unbind = %v, want empty", got)
		}
	})

	t.Run("Forget closes the streams and unbinds", func(t *testing.T) {
		core, _, _, file := newFakeCore(t)
		core.BindHelpers(file)
		before := core.HelperGeneration(file)
		srv := httptest.NewServer(core.WireMux())
		t.Cleanup(srv.Close)

		q := url.Values{"role": {"handler"}, "file": {file}}
		resp, err := http.Get(srv.URL + "/_/wire/subscribe?" + q.Encode())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("handler subscription refused: %d", resp.StatusCode)
		}
		waitSubscribers(t, core, file, 1)

		core.Forget(file)
		waitEOF(t, resp)

		if got := core.BoundFiles(); len(got) != 0 {
			t.Fatalf("BoundFiles after Forget = %v, want empty", got)
		}
		if after := core.HelperGeneration(file); after <= before {
			t.Fatalf("HelperGeneration after Forget = %d, want greater than %d", after, before)
		}
	})

	t.Run("Forget on an unknown path is harmless", func(t *testing.T) {
		core, _, _, file := newFakeCore(t)
		core.BindHelpers(file)
		before := core.BoundFiles()

		core.Forget("/nowhere.htmlclay")

		after := core.BoundFiles()
		if len(after) != len(before) {
			t.Fatalf("BoundFiles after Forget = %v, want %v", after, before)
		}
		for i := range before {
			if after[i] != before[i] {
				t.Fatalf("BoundFiles after Forget = %v, want %v", after, before)
			}
		}
	})
}

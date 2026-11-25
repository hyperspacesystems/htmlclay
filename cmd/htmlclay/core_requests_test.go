package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/confirm"
	"github.com/panphora/htmlclay/internal/service"
)

// confirmAnswerWait is how long a subtest watches an ask that must stay open.
const confirmAnswerWait = 100 * time.Millisecond

// teamDocumentAsk is the question a flow asks for a document it wants a program
// to write.
func teamDocumentAsk(program string) confirm.Ask {
	return confirm.Ask{
		Kind:     appcore.RequestTeamDocument,
		Document: "notes.htmlclay",
		Program:  program,
		Folder:   "Team notes",
		Path:     "/Users/x/bin/resize",
		Identity: "d41",
		Answers:  []string{"deny", "allow-document", "choose-file"},
	}
}

// askThroughRegistry asks a question the way a flow does and hands back the
// reply it settles with. The ask is cancelled when the subtest ends.
func askThroughRegistry(t *testing.T, a *app, ask confirm.Ask) <-chan confirm.Reply {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	reply := make(chan confirm.Reply, 1)
	go func() {
		answer, err := a.registry().Ask(ctx, ask)
		if err != nil {
			t.Errorf("Ask: %v", err)
		}
		reply <- answer
	}()
	t.Cleanup(cancel)
	return reply
}

// waitForOpenRequests waits until the snapshot lists n open requests.
func waitForOpenRequests(t *testing.T, a *app, n int) []appcore.Request {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		requests := a.snapshot().Requests
		if len(requests) >= n {
			return requests
		}
		if time.Now().After(deadline) {
			t.Fatalf("the snapshot lists %d open request(s) after 5s, want %d", len(requests), n)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// openRequestNamed answers the open request for one program.
func openRequestNamed(t *testing.T, requests []appcore.Request, program string) appcore.Request {
	t.Helper()
	for _, r := range requests {
		if r.Program == program {
			return r
		}
	}
	t.Fatalf("no open request for program %q in %+v", program, requests)
	return appcore.Request{}
}

// stillWaiting holds a subtest to an ask nobody answered.
func stillWaiting(t *testing.T, reply <-chan confirm.Reply) {
	t.Helper()
	select {
	case answer := <-reply:
		t.Fatalf("the ask settled with %+v, want it still waiting", answer)
	case <-time.After(confirmAnswerWait):
	}
}

// testAnswer builds the action the Mac app sends to answer one request.
func testAnswer(t *testing.T, id string, request appcore.Request, answer string) appcore.Action {
	t.Helper()
	return coreAction(t, id, "request.answer", map[string]any{
		"id":       request.ID,
		"revision": request.Revision,
		"answer":   answer,
	})
}

// unsavedService stands in for the live-room service holding one unsaved edit,
// so host.quit answers with a refusal instead of quitting the test process.
func unsavedService(t *testing.T, a *app) *serviceRunner {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/flush" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"unsaved":[{"root":"notes","name":"index.html"}]}`))
	}))
	runner := &serviceRunner{a: a, command: []string{"test-service"}}
	runner.mu.Lock()
	now := time.Now()
	runner.machine.Step(now, service.Event{Kind: service.Want, On: true})
	runner.machine.Step(now, service.Event{Kind: service.Spawned})
	runner.machine.Step(now, service.Event{Kind: service.Attached})
	runner.peer = serviceClient{URL: srv.URL, Secret: "test-secret"}
	runner.mu.Unlock()
	t.Cleanup(func() {
		runner.mu.Lock()
		runner.peer = serviceClient{}
		runner.mu.Unlock()
		srv.Close()
	})
	return runner
}

// TestM3ShellRequests proves a request the registry holds reaches the snapshot,
// the Mac app's answer releases the ask that waits on it, and every guard
// around that answer holds.
func TestM3ShellRequests(t *testing.T) {
	home := evalTempHome(t)

	t.Run("the shell channel's answer releases a waiting ask", func(t *testing.T) {
		a := newServedTestApp(t, home, t.TempDir())
		if a.snapshot().Host.Can["request.answer"] {
			t.Fatal("host.can says request.answer is implemented by a host the Mac app did not start")
		}
		a.shell = true
		if !a.snapshot().Host.Can["request.answer"] {
			t.Fatal("host.can says request.answer is not implemented by a host the Mac app started")
		}

		reply := askThroughRegistry(t, a, teamDocumentAsk("resize"))
		requests := waitForOpenRequests(t, a, 1)
		request := requests[0]
		if request.Kind != appcore.RequestTeamDocument || request.DocumentID == nil || *request.DocumentID != "d41" || request.Document != "notes.htmlclay" {
			t.Fatalf("request = %+v, want a teamDocument request for d41/notes.htmlclay", request)
		}
		if want := []string{"deny", "allow-document", "choose-file"}; len(request.Answers) != len(want) {
			t.Fatalf("request answers = %v, want %v", request.Answers, want)
		}

		if _, refusal := a.do(context.Background(), testAnswer(t, "a17", request, "allow-document"), ChannelShell); refusal != nil {
			t.Fatalf("the shell's answer was refused: %+v", refusal)
		}
		select {
		case answer := <-reply:
			if answer.Choice != "allow-document" || answer.ID != request.ID {
				t.Fatalf("the ask settled with %+v, want %s answered allow-document", answer, request.ID)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the ask did not settle after the shell answered it")
		}
		if pending := a.registry().Pending(); len(pending) != 0 {
			t.Fatalf("%d request(s) still open after the answer, want none", len(pending))
		}
		if requests := a.snapshot().Requests; len(requests) != 0 {
			t.Fatalf("the snapshot still lists %+v", requests)
		}
	})

	t.Run("the dashboard channel's answer is refused and the ask still waits", func(t *testing.T) {
		a := newServedTestApp(t, home, t.TempDir())
		a.shell = true
		reply := askThroughRegistry(t, a, teamDocumentAsk("resize"))
		request := waitForOpenRequests(t, a, 1)[0]

		if _, refusal := a.do(context.Background(), testAnswer(t, "d17", request, "allow-document"), ChannelDashboard); refusal == nil || refusal.Code != appcore.RefuseNeedsConfirmation {
			t.Fatalf("a dashboard answer = %+v, want %s", refusal, appcore.RefuseNeedsConfirmation)
		}
		stillWaiting(t, reply)
		if pending := a.registry().Pending(); len(pending) != 1 {
			t.Fatalf("%d request(s) open after the dashboard was refused, want 1", len(pending))
		}
	})

	t.Run("a request that offers only allow-document refuses a forged allow-all-trusted", func(t *testing.T) {
		a := newServedTestApp(t, home, t.TempDir())
		a.shell = true
		ask := teamDocumentAsk("resize")
		ask.Answers = []string{"allow-document"}
		reply := askThroughRegistry(t, a, ask)
		request := waitForOpenRequests(t, a, 1)[0]

		if _, refusal := a.do(context.Background(), testAnswer(t, "f17", request, "allow-all-trusted"), ChannelShell); refusal == nil || refusal.Code != appcore.RefuseAnswerNotOffered {
			t.Fatalf("a forged answer = %+v, want %s", refusal, appcore.RefuseAnswerNotOffered)
		}
		stillWaiting(t, reply)
		if pending := a.registry().Pending(); len(pending) != 1 {
			t.Fatalf("%d request(s) open after a forged answer, want 1", len(pending))
		}
	})

	t.Run("a held open request leaves the snapshot, other actions and host.quit answering", func(t *testing.T) {
		a := newServedTestApp(t, home, t.TempDir())
		a.shell = true
		a.service = unsavedService(t, a)
		reply := askThroughRegistry(t, a, teamDocumentAsk("resize"))
		request := waitForOpenRequests(t, a, 1)[0]

		snapshot := a.snapshot()
		if len(snapshot.Requests) != 1 || snapshot.Requests[0].ID != request.ID {
			t.Fatalf("the snapshot lists %+v, want the open request %s", snapshot.Requests, request.ID)
		}

		if _, refusal := a.do(context.Background(), coreAction(t, "u17", "waiting.markSeen", map[string]any{"ids": []string{"w-request"}}), ChannelShell); refusal != nil {
			t.Fatalf("an unrelated action was refused: %+v", refusal)
		}

		if _, refusal := a.do(context.Background(), coreAction(t, "q17", "host.quit", map[string]any{}), ChannelShell); refusal == nil || refusal.Code != appcore.RefuseQuitUnsaved || len(refusal.Documents) != 1 || refusal.Documents[0] != "index.html" {
			t.Fatalf("host.quit = %+v, want %s naming index.html", refusal, appcore.RefuseQuitUnsaved)
		}
		stillWaiting(t, reply)
		if pending := a.registry().Pending(); len(pending) != 1 {
			t.Fatalf("%d request(s) open after other actions, want 1", len(pending))
		}
	})

	t.Run("a request that ended before the effect ran is refused", func(t *testing.T) {
		a := newServedTestApp(t, home, t.TempDir())
		a.shell = true
		reply := askThroughRegistry(t, a, teamDocumentAsk("resize"))
		request := waitForOpenRequests(t, a, 1)[0]

		a.core.beforeEffect = func(appcore.Effect) {
			a.registry().Close(request.ID)
		}
		if _, refusal := a.do(context.Background(), testAnswer(t, "c17", request, "allow-document"), ChannelShell); refusal == nil || refusal.Code != appcore.RefuseNoSuchRequest {
			t.Fatalf("an answer after the request ended = %+v, want %s", refusal, appcore.RefuseNoSuchRequest)
		}
		select {
		case answer := <-reply:
			if answer.Choice != confirm.ChoiceDeny {
				t.Fatalf("the ask settled with %+v, want deny", answer)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("closing the request did not release the ask")
		}
	})

	t.Run("a path on a choice that takes none is bad arguments", func(t *testing.T) {
		a := newServedTestApp(t, home, t.TempDir())
		a.shell = true
		reply := askThroughRegistry(t, a, teamDocumentAsk("resize"))
		request := waitForOpenRequests(t, a, 1)[0]

		args := map[string]any{"id": request.ID, "revision": request.Revision, "answer": "deny", "path": "/Users/x/bin/resize"}
		if _, refusal := a.do(context.Background(), coreAction(t, "p17", "request.answer", args), ChannelShell); refusal == nil || refusal.Code != appcore.RefuseBadArguments {
			t.Fatalf("a path on deny = %+v, want %s", refusal, appcore.RefuseBadArguments)
		}
		stillWaiting(t, reply)
		if pending := a.registry().Pending(); len(pending) != 1 {
			t.Fatalf("%d request(s) open after a bad path, want 1", len(pending))
		}
	})

	t.Run("the same action id sent twice consumes one request", func(t *testing.T) {
		a := newServedTestApp(t, home, t.TempDir())
		a.shell = true

		first := askThroughRegistry(t, a, teamDocumentAsk("resize"))
		request := waitForOpenRequests(t, a, 1)[0]
		second := askThroughRegistry(t, a, teamDocumentAsk("export"))
		waitForOpenRequests(t, a, 2)

		answer := testAnswer(t, "x17", request, "allow-document")
		if _, refusal := a.do(context.Background(), answer, ChannelShell); refusal != nil {
			t.Fatalf("the first answer was refused: %+v", refusal)
		}
		select {
		case got := <-first:
			if got.Choice != "allow-document" {
				t.Fatalf("the first ask settled with %+v, want allow-document", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the first ask did not settle")
		}

		if _, refusal := a.do(context.Background(), answer, ChannelShell); refusal != nil {
			t.Fatalf("the repeated action id = %+v, want the first answer", refusal)
		}
		stillWaiting(t, second)
		if pending := a.registry().Pending(); len(pending) != 1 || pending[0].Ask.Program != "export" {
			t.Fatalf("open requests after the repeated id = %+v, want only export", pending)
		}
	})
}

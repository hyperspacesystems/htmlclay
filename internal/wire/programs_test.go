package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/carried"
	"github.com/panphora/htmlclay/internal/htmlutil"
)

// fakeProgramsHost reads the document from disk as the app does and runs
// programs in a real worker: this test binary in worker mode.
type fakeProgramsHost struct {
	policy  *fakePolicy
	folder  string
	answer  chan struct{}
	runs    atomic.Int32
	folders atomic.Int32
	// ended hears each run's outcome once carried.Run has returned, which it
	// does only after the worker process has exited.
	ended chan error
}

func (h *fakeProgramsHost) Enabled() bool         { return true }
func (h *fakeProgramsHost) HasFolder(string) bool { return false }

func (h *fakeProgramsHost) Load(document string) (ProgramSet, error) {
	data, err := os.ReadFile(document)
	if err != nil {
		return ProgramSet{}, err
	}
	return ProgramSet{Trust: h.policy.DocumentTrust(document), Scan: htmlutil.ReadPrograms(data)}, nil
}

func (h *fakeProgramsHost) Folder(ctx context.Context, _, _ string, _ bool, _ string, asking func()) (string, bool, error) {
	h.folders.Add(1)
	asking()
	if h.answer != nil {
		select {
		case <-h.answer:
		case <-ctx.Done():
			return "", false, ctx.Err()
		}
	}
	return h.folder, true, nil
}

func (h *fakeProgramsHost) Run(ctx context.Context, job carried.Job) (carried.Result, error) {
	h.runs.Add(1)
	exe, err := os.Executable()
	if err != nil {
		return carried.Result{}, err
	}
	job.Worker = exe
	res, err := carried.Run(ctx, job)
	select {
	case h.ended <- err:
	default:
	}
	return res, err
}

func writeProgramDocument(t *testing.T, path string, tags ...string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("<!doctype html><html><head>"+strings.Join(tags, "")+"</head><body></body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func programTag(name, can, text string) string {
	return fmt.Sprintf(`<script type="text/x-clay-program" name=%q can=%q>%s</script>`, name, can, text)
}

func newProgramsCore(t *testing.T) (*Core, *fakePolicy, *fakeProgramsHost, string, *httptest.Server) {
	t.Helper()
	core, _, policy, file := newFakeCore(t)
	folder := filepath.Join(filepath.Dir(file), "app")
	os.MkdirAll(folder, 0o700)
	os.WriteFile(filepath.Join(folder, "a.txt"), []byte("hello"), 0o600)
	host := &fakeProgramsHost{policy: policy, folder: folder, ended: make(chan error, 8)}
	policy.aiOffered, policy.tier, policy.ready, policy.programs = true, "mine", true, host
	srv := httptest.NewServer(core.WireMux())
	t.Cleanup(srv.Close)
	return core, policy, host, file, srv
}

func sendProgram(t *testing.T, srv *httptest.Server, id, payload string) {
	t.Helper()
	body := fmt.Sprintf(`{"type":"wire/request","id":%q,"helper":%q,"document":"none","payload":%s}`, id, carried.HelperName, payload)
	if resp := sendFrame(t, srv, body, "doc.htmlclay"); resp.StatusCode != http.StatusOK {
		t.Fatalf("request refused: %d", resp.StatusCode)
	}
}

// terminalOf waits past acks and status frames for a request's outcome.
func terminalOf(t *testing.T, sub *Sub, id string) Envelope {
	t.Helper()
	for {
		env := nextOf(t, sub, id, true)
		if env.Type != "wire/ack" {
			return env
		}
	}
}

func TestAProgramRunsFromTheSavedFileAndStreamsItsOutput(t *testing.T) {
	core, _, host, file, srv := newProgramsCore(t)
	writeProgramDocument(t, file,
		programTag("count", "read", `const fmt = require("./fmt"); async function main(input, clay) { console.log("reading"); const text = await clay.files.read("a.txt"); return fmt.wrap(text + input.suffix) }`),
		programTag("fmt", "", `exports.wrap = (s) => "[" + s + "]";`))
	sub := observer(t, core, file)
	sendProgram(t, srv, "p1", `{"program":"count","input":{"suffix":"!"}}`)
	ack := nextOf(t, sub, "p1", false)
	if ack.Type != "wire/ack" || budgetOf(t, ack) != programsParkBudget.Milliseconds() {
		t.Fatalf("first frame = %s %s", ack.Type, ack.Payload)
	}
	var log strings.Builder
	var done Envelope
	for done.Type == "" {
		env := nextOf(t, sub, "p1", false)
		switch env.Type {
		case "wire/status":
			if strings.Contains(string(env.Payload), `"log"`) {
				log.WriteString(env.Text)
			}
		case "wire/done", "wire/error":
			done = env
		}
	}
	if done.Type != "wire/done" {
		t.Fatalf("terminal = %s %s %s", done.Type, done.Text, done.Payload)
	}
	var out struct {
		Program string            `json:"program"`
		Data    string            `json:"data"`
		Log     string            `json:"log"`
		Outputs []any             `json:"outputs"`
		Folder  map[string]string `json:"folder"`
		Grant   map[string]any    `json:"grant"`
	}
	if err := json.Unmarshal(done.Payload, &out); err != nil {
		t.Fatal(err)
	}
	if out.Program != "count" || out.Data != "[hello!]" || out.Log != "reading\n" || out.Outputs == nil || out.Folder["name"] != "app" || out.Grant["level"] != "read" {
		t.Fatalf("done payload = %s", done.Payload)
	}
	if log.String() != "reading\n" {
		t.Fatalf("streamed log = %q", log.String())
	}
	if strings.Contains(string(done.Payload), host.folder) {
		t.Fatalf("the folder's path reached the page: %s", done.Payload)
	}
	if core.RunningWork() != 0 {
		t.Fatalf("running work = %d after the run", core.RunningWork())
	}
}

func TestNoProgramRunsBeforeItsDocumentIsAllowed(t *testing.T) {
	core, policy, host, file, srv := newProgramsCore(t)
	writeProgramDocument(t, file, programTag("count", "", `async function main() { return 1 }`))
	policy.tier, policy.ready = "unknown", false
	sub := observer(t, core, file)
	sendProgram(t, srv, "p1", `{"program":"count"}`)
	end := terminalOf(t, sub, "p1")
	if end.Type != "wire/error" || helperErrorCode(t, end) != "helper_not_granted" {
		t.Fatalf("terminal = %s %s", end.Type, end.Payload)
	}
	if len(policy.askedDocument) != 1 || len(policy.askedProgram) != 1 || policy.askedProgram[0] != "count" ||
		host.runs.Load() != 0 || host.folders.Load() != 0 {
		t.Fatalf("asked %v about %v, runs %d, folder questions %d", policy.askedDocument, policy.askedProgram, host.runs.Load(), host.folders.Load())
	}
}

func TestAProgramChangedWhileTheFolderQuestionWaitsDoesNotRun(t *testing.T) {
	core, _, host, file, srv := newProgramsCore(t)
	writeProgramDocument(t, file, programTag("count", "", `async function main() { return "checked" }`))
	host.answer = make(chan struct{})
	sub := observer(t, core, file)
	sendProgram(t, srv, "p1", `{"program":"count"}`)
	for {
		if env := nextOf(t, sub, "p1", false); env.Type == "wire/status" {
			break
		}
	}
	writeProgramDocument(t, file, programTag("count", "", `async function main() { return "swapped" }`))
	close(host.answer)
	end := terminalOf(t, sub, "p1")
	if end.Type != "wire/error" || helperErrorCode(t, end) != carried.CodeProgramChanged || host.runs.Load() != 0 {
		t.Fatalf("terminal = %s %s, runs %d", end.Type, end.Payload, host.runs.Load())
	}
}

func TestAStrangersProgramGetsNoMoreThanItsTierAllows(t *testing.T) {
	core, policy, host, file, srv := newProgramsCore(t)
	policy.tier = "unknown"
	writeProgramDocument(t, file,
		programTag("edit", "change", `async function main() { await clay.files.write("a.txt", "x") }`),
		programTag("both", "read internet", `async function main() {}`),
		programTag("net", "internet", `async function main(input, clay) { try { await clay.files.read("a.txt") } catch (e) { return e.code } }`))
	sub := observer(t, core, file)
	sendProgram(t, srv, "p1", `{"program":"edit"}`)
	if end := terminalOf(t, sub, "p1"); end.Type != "wire/error" || helperErrorCode(t, end) != carried.CodeLevelUnavailable {
		t.Fatalf("a stranger's change program: %s %s", end.Type, end.Payload)
	}
	if host.folders.Load() != 0 || host.runs.Load() != 0 {
		t.Fatalf("a stranger's change program asked %d folder questions and ran %d times", host.folders.Load(), host.runs.Load())
	}
	if data, _ := os.ReadFile(filepath.Join(host.folder, "a.txt")); string(data) != "hello" {
		t.Fatalf("a stranger's program changed a file: %q", data)
	}
	sendProgram(t, srv, "p4", `{"program":"net"}`)
	done := terminalOf(t, sub, "p4")
	var out struct {
		Data   string         `json:"data"`
		Folder any            `json:"folder"`
		Grant  map[string]any `json:"grant"`
	}
	json.Unmarshal(done.Payload, &out)
	if done.Type != "wire/done" || out.Data != carried.CodeCallRefused || out.Folder != nil || out.Grant["level"] != "internet" || host.folders.Load() != 0 {
		t.Fatalf("an internet program: %s %s, folder questions %d", done.Type, done.Payload, host.folders.Load())
	}
	sendProgram(t, srv, "p2", `{"program":"both"}`)
	if end := terminalOf(t, sub, "p2"); end.Type != "wire/error" || helperErrorCode(t, end) != carried.CodeProgramInvalid {
		t.Fatalf("internet with files: %s %s", end.Type, end.Payload)
	}
	sendProgram(t, srv, "p3", `{"program":"absent"}`)
	if end := terminalOf(t, sub, "p3"); helperErrorCode(t, end) != carried.CodeProgramMissing {
		t.Fatalf("a missing program: %s %s", end.Type, end.Payload)
	}
}

func TestAProgramPastTheReadLimitIsReportedNotRun(t *testing.T) {
	core, _, host, file, srv := newProgramsCore(t)
	pad := "<!--" + strings.Repeat("x", htmlutil.ProgramScanLimit) + "-->"
	writeProgramDocument(t, file, pad+programTag("late", "", `async function main() {}`))
	sub := observer(t, core, file)
	sendProgram(t, srv, "p1", `{"program":"late"}`)
	end := terminalOf(t, sub, "p1")
	if helperErrorCode(t, end) != carried.CodeProgramNotRead || !strings.Contains(end.Text, "2.5 MB") || host.runs.Load() != 0 {
		t.Fatalf("terminal = %s %s %s", end.Type, end.Text, end.Payload)
	}
}

func TestEveryEndStopsTheProgramsWorker(t *testing.T) {
	for _, how := range []string{"cancel", "switch off", "document refused", "document closed", "folder forgotten"} {
		t.Run(how, func(t *testing.T) {
			core, _, host, file, srv := newProgramsCore(t)
			writeProgramDocument(t, file, programTag("spin", "", `async function main() { clay.status("spinning"); while (true) {} }`))
			sub := observer(t, core, file)
			sendProgram(t, srv, "p1", `{"program":"spin"}`)
			for {
				env := nextOf(t, sub, "p1", false)
				if env.Type == "wire/status" && env.Text == "spinning" {
					break
				}
				if env.Type == "wire/error" {
					t.Fatalf("the program never started: %s", env.Payload)
				}
			}
			switch how {
			case "cancel":
				if resp := sendFrame(t, srv, `{"type":"wire/cancel","id":"p1"}`, "doc.htmlclay"); resp.StatusCode != http.StatusOK {
					t.Fatalf("cancel refused: %d", resp.StatusCode)
				}
			case "switch off":
				core.CancelAI()
			case "document refused":
				core.CancelAIFor(file)
			case "document closed":
				core.Forget(file)
			case "folder forgotten":
				core.ForgetUnder(filepath.Dir(file))
			}
			select {
			case err := <-host.ended:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("the run ended with %v, want a cancel", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("the worker still runs")
			}
			if how == "cancel" || how == "switch off" || how == "document refused" {
				if end := terminalOf(t, sub, "p1"); end.Type != "wire/error" || helperErrorCode(t, end) != "helper_cancelled" {
					t.Fatalf("terminal = %s %s %s", end.Type, end.Text, end.Payload)
				}
			}
			deadline := time.Now().Add(5 * time.Second)
			for core.RunningWork() != 0 {
				if time.Now().After(deadline) {
					t.Fatalf("running work = %d", core.RunningWork())
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

func TestProgramsDiscoveryAndRelay(t *testing.T) {
	core, policy, _, file, srv := newProgramsCore(t)
	writeHelperDocument(t, file, carried.HelperName)
	state := func() (string, int) {
		_, metas := core.HelperDiscovery(file)
		found, n := "missing", 0
		for _, m := range metas {
			if m.Name == carried.HelperName {
				found = m.State
				n++
			}
		}
		return found, n
	}
	if got, n := state(); got != "ready" || n != 1 {
		t.Fatalf("state = %s (%d entries), want one ready entry", got, n)
	}
	policy.denied = true
	if got, _ := state(); got != "denied" {
		t.Fatalf("state = %s after Don't allow, want denied", got)
	}
	policy.denied = false

	if core.RelayBlocked(file) {
		t.Fatal("a document with no program blocks live sync")
	}
	writeProgramDocument(t, file, programTag("count", "", `async function main() { return 1 }`))
	if !core.RelayBlocked(file) {
		t.Fatal("live sync can push a snapshot into a document that carries a program")
	}
	sub := observer(t, core, file)
	sendProgram(t, srv, "p1", `{"program":"count"}`)
	if end := terminalOf(t, sub, "p1"); end.Type != "wire/done" {
		t.Fatalf("terminal = %s %s", end.Type, end.Payload)
	}
	writeProgramDocument(t, file)
	if !core.RelayBlocked(file) {
		t.Fatal("live sync can push a snapshot into a document that ran a program before its tags were removed")
	}
}

func TestASiblingProgramChangedWhileTheFolderQuestionWaitsDoesNotRun(t *testing.T) {
	core, _, host, file, srv := newProgramsCore(t)
	count := programTag("count", "read", `const fmt = require("./fmt"); async function main() { return fmt.wrap("one") }`)
	writeProgramDocument(t, file, count, programTag("fmt", "", `exports.wrap = (s) => "[" + s + "]";`))
	host.answer = make(chan struct{})
	sub := observer(t, core, file)
	sendProgram(t, srv, "p1", `{"program":"count"}`)
	for {
		if env := nextOf(t, sub, "p1", false); env.Type == "wire/status" {
			break
		}
	}
	writeProgramDocument(t, file, count, programTag("fmt", "", `exports.wrap = (s) => "[" + s + " swapped]";`))
	close(host.answer)
	end := terminalOf(t, sub, "p1")
	if end.Type != "wire/error" || helperErrorCode(t, end) != carried.CodeProgramChanged || host.runs.Load() != 0 {
		t.Fatalf("terminal = %s %s, runs %d", end.Type, end.Payload, host.runs.Load())
	}
}

func TestAReservedProgramNameIsRefused(t *testing.T) {
	core, _, host, file, srv := newProgramsCore(t)
	writeProgramDocument(t, file, programTag("count", "", `async function main() {}`))
	sub := observer(t, core, file)
	for i, name := range []string{"ai-tools", "ai-edit", "programs", "document-trust"} {
		id := fmt.Sprintf("p%d", i)
		sendProgram(t, srv, id, `{"program":"`+name+`"}`)
		if end := terminalOf(t, sub, id); end.Type != "wire/error" || helperErrorCode(t, end) != "invalid_request" {
			t.Fatalf("%s: terminal = %s %s", name, end.Type, end.Payload)
		}
	}
	if host.runs.Load() != 0 || host.folders.Load() != 0 {
		t.Fatalf("a reserved name ran %d times and asked %d folder questions", host.runs.Load(), host.folders.Load())
	}
}

// A program that reports and prints in a loop must not push the page's own
// observer out of its queue: the person watching would lose the outcome.
func TestStatusAndLogBurstsKeepTheObserver(t *testing.T) {
	core, _, _, file, srv := newProgramsCore(t)
	writeProgramDocument(t, file, programTag("chatty", "", `async function main() {
		for (let i = 0; i < 100; i++) clay.status("step " + i);
		console.log("x".repeat(600 * 1024));
		await new Promise((r) => setTimeout(r, 1500));
		return "done";
	}`))
	sub := observer(t, core, file)
	sendProgram(t, srv, "p1", `{"program":"chatty"}`)
	time.Sleep(900 * time.Millisecond)
	select {
	case <-sub.Done:
		t.Fatal("a burst of status and log frames evicted the page's observer before the outcome")
	default:
	}
	var done Envelope
	for done.Type == "" {
		env := nextOf(t, sub, "p1", false)
		if env.Type == "wire/done" || env.Type == "wire/error" {
			done = env
		}
	}
	if done.Type != "wire/done" {
		t.Fatalf("terminal = %s %s %s", done.Type, done.Text, done.Payload)
	}
}

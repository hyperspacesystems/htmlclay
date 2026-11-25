package wire

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/aitools"
	"github.com/panphora/htmlclay/internal/config"
)

const wireFakeClaude = `#!/bin/sh
here=$(cd "$(dirname "$0")" && pwd)
case "$1" in --version) echo "2.1.329 (Claude Code)"; exit 0;; --help) echo "  --restricted   stay in the folder"; exit 0;; esac
prompt=$(cat)
echo '{"type":"system","subtype":"init","tools":["Read","Grep","Glob"],"mcp_servers":[],"model":"claude-fake-1"}'
case "$prompt" in
*"Read the file inside.txt"*) printf '{"type":"result","structured_output":{"data":{"inside":"%s","outside":[]}}}\n' "$(head -n1 inside.txt)";;
*"Try each step"*)
  echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"absolute.txt parent.txt notes.txt docs/inner.txt project-old written-up.txt"}}]}}'
  echo '{"type":"result","structured_output":{"data":{"inside":"","outside":[]}}}';;
*)
  echo $$ > "$here/pid"
  [ -f "$here/hang" ] && sleep 30
  echo '{"type":"result","structured_output":{"data":{"title":"T"},"sources":[{"id":"s1","kind":"file","path":"a.txt","first":1,"last":1,"base":"","head":"","paths":[]}]}}';;
esac
`

// fakeToolsHost answers the folder question when the test lets it.
type fakeToolsHost struct {
	svc    *aitools.Service
	folder string
	answer chan struct{}
}

func (h *fakeToolsHost) Enabled() bool                          { return true }
func (h *fakeToolsHost) Service() *aitools.Service              { return h.svc }
func (h *fakeToolsHost) Preferred() (string, map[string]string) { return "", nil }
func (h *fakeToolsHost) HasFolder(string) bool                  { return false }
func (h *fakeToolsHost) Folder(ctx context.Context, _ string, _ bool, _ string, asking func()) (string, bool, error) {
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

func newToolsCore(t *testing.T) (*Core, *fakePolicy, *fakeToolsHost, string, string, *httptest.Server) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake agent is a shell script")
	}
	core, _, policy, file := newFakeCore(t)
	root := filepath.Dir(file)
	bin := filepath.Join(root, "bin")
	folder := filepath.Join(root, "app")
	for _, dir := range []string{bin, folder} {
		os.MkdirAll(dir, 0o700)
	}
	os.WriteFile(filepath.Join(bin, "claude"), []byte(wireFakeClaude), 0o755)
	os.WriteFile(filepath.Join(folder, "a.txt"), []byte("hello\n"), 0o600)
	store, err := aitools.OpenStore(filepath.Join(root, "store"))
	if err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + bin + ":/usr/bin:/bin"}
	host := &fakeToolsHost{folder: folder, svc: aitools.NewService(aitools.Config{
		Store:       store,
		Scratch:     filepath.Join(root, "scratch"),
		LoginEnv:    func() []string { return env },
		ClaudeToken: func(context.Context) (aitools.Token, error) { return aitools.Token{Access: "t"}, nil },
	})}
	policy.aiOffered, policy.tier, policy.ready, policy.tools = true, "mine", true, host
	srv := httptest.NewServer(core.WireMux())
	t.Cleanup(srv.Close)
	return core, policy, host, file, bin, srv
}

func sendTools(t *testing.T, srv *httptest.Server, id, payload string) {
	t.Helper()
	body := fmt.Sprintf(`{"type":"wire/request","id":%q,"helper":%q,"document":"none","payload":%s}`, id, aitools.HelperName, payload)
	if resp := sendFrame(t, srv, body, "doc.htmlclay"); resp.StatusCode != http.StatusOK {
		t.Fatalf("request refused: %d", resp.StatusCode)
	}
}

// nextOf waits for the next frame of request id, past any status frame when
// skipStatus is set.
func nextOf(t *testing.T, sub *Sub, id string, skipStatus bool) Envelope {
	t.Helper()
	deadline := time.After(20 * time.Second)
	for {
		select {
		case raw := <-sub.Ch:
			env, err := DecodeEnvelope(raw)
			if err != nil {
				t.Fatal(err)
			}
			if env.ID != id || (skipStatus && env.Type == "wire/status") {
				continue
			}
			return env
		case <-deadline:
			t.Fatalf("no frame for %s", id)
			return Envelope{}
		}
	}
}

func budgetOf(t *testing.T, env Envelope) int64 {
	t.Helper()
	var p struct {
		BudgetMs int64 `json:"budgetMs"`
	}
	json.Unmarshal(env.Payload, &p)
	return p.BudgetMs
}

func TestAIToolsParksThenStartsWithItsOwnBudget(t *testing.T) {
	core, policy, host, file, _, srv := newToolsCore(t)
	host.answer = make(chan struct{})
	sub := observer(t, core, file)
	sendTools(t, srv, "r1", `{"request":"Explain.","schema":{"type":"object","additionalProperties":false,"required":["title"],"properties":{"title":{"type":"string"}}},"sources":true,"tools":"read","folder":"remembered","engine":""}`)
	ack := nextOf(t, sub, "r1", false)
	if ack.Type != "wire/ack" || budgetOf(t, ack) != 1200000 {
		t.Fatalf("first frame = %s %s", ack.Type, ack.Payload)
	}
	if asking := nextOf(t, sub, "r1", false); asking.Type != "wire/status" || !strings.Contains(string(asking.Payload), `"asking"`) {
		t.Fatalf("parked frame = %s %s", asking.Type, asking.Payload)
	}
	if core.RunningWork() != 1 {
		t.Fatalf("running work = %d while parked", core.RunningWork())
	}
	close(host.answer)
	if second := nextOf(t, sub, "r1", true); second.Type != "wire/ack" || budgetOf(t, second) != 900000 {
		t.Fatalf("second ack = %s %s", second.Type, second.Payload)
	}
	done := nextOf(t, sub, "r1", true)
	if done.Type != "wire/done" {
		t.Fatalf("terminal = %s %s %s", done.Type, done.Text, done.Payload)
	}
	if strings.Contains(string(done.Payload), host.folder) {
		t.Fatalf("the folder's path reached the page: %s", done.Payload)
	}
	var out struct {
		Data    map[string]string `json:"data"`
		Folder  map[string]string `json:"folder"`
		Grant   map[string]any    `json:"grant"`
		Engine  string            `json:"engine"`
		Model   string            `json:"model"`
		Sources []struct {
			ID, URL, SHA256 string
			Bytes           int
		} `json:"sources"`
	}
	if err := json.Unmarshal(done.Payload, &out); err != nil {
		t.Fatal(err)
	}
	if out.Data["title"] != "T" || out.Folder["name"] != "app" || out.Engine != "claude" || out.Model != "claude-fake-1" || out.Grant["level"] != "read" || out.Grant["remembered"] != true {
		t.Fatalf("done payload = %s", done.Payload)
	}
	if len(out.Sources) != 1 || !regexp.MustCompile(`^/_/wire/runs/[A-Za-z0-9_-]+/files/[A-Za-z0-9_-]+$`).MatchString(out.Sources[0].URL) {
		t.Fatalf("sources = %+v", out.Sources)
	}
	parts := strings.Split(out.Sources[0].URL, "/")
	if body, ok := core.RunFiles.Get(file, parts[4], parts[6]); !ok || string(body) != "hello\n" {
		t.Fatalf("run file = %q %v", body, ok)
	}
	if !core.RelayBlocked(file) {
		t.Fatal("a document that asked for AI with tools still takes relayed snapshots")
	}
	if core.RunningWork() != 0 {
		t.Fatalf("running work = %d after done", core.RunningWork())
	}
	policy.mu.Lock()
	defer policy.mu.Unlock()
	if len(policy.askedDocument) != 0 {
		t.Fatalf("a Mine document was asked about: %v", policy.askedDocument)
	}
}

func TestAIToolsTimeoutAndCancelEndTheAgent(t *testing.T) {
	for _, how := range []string{"timeout", "cancel", "switch off", "document refused"} {
		t.Run(how, func(t *testing.T) {
			core, _, _, file, bin, srv := newToolsCore(t)
			os.WriteFile(filepath.Join(bin, "hang"), nil, 0o600)
			if how == "timeout" {
				old := aiToolsRunBudget
				aiToolsRunBudget = 3 * time.Second
				t.Cleanup(func() { aiToolsRunBudget = old })
			}
			sub := observer(t, core, file)
			sendTools(t, srv, "r2", `{"request":"x"}`)
			var pid int
			deadline := time.Now().Add(20 * time.Second)
			for pid == 0 {
				if time.Now().After(deadline) {
					t.Fatal("the agent never started")
				}
				b, _ := os.ReadFile(filepath.Join(bin, "pid"))
				pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
				time.Sleep(20 * time.Millisecond)
			}
			want := "helper_cancelled"
			switch how {
			case "timeout":
				want = "helper_timeout"
			case "cancel":
				if resp := sendFrame(t, srv, `{"type":"wire/cancel","id":"r2"}`, "doc.htmlclay"); resp.StatusCode != http.StatusOK {
					t.Fatalf("cancel refused: %d", resp.StatusCode)
				}
			case "switch off":
				core.CancelAI()
			case "document refused":
				core.CancelAIFor(file)
			}
			var end Envelope
			for end.Type == "" || end.Type == "wire/ack" {
				end = nextOf(t, sub, "r2", true)
			}
			if end.Type != "wire/error" || helperErrorCode(t, end) != want {
				t.Fatalf("terminal = %s %s %s", end.Type, end.Text, end.Payload)
			}
			if p, _ := os.FindProcess(pid); p.Signal(syscall.Signal(0)) == nil {
				t.Fatalf("the agent %d is still running", pid)
			}
			if core.RunningWork() != 0 {
				t.Fatalf("running work = %d", core.RunningWork())
			}
		})
	}
}

func TestAIToolsDiscoveryFollowsTheSwitchAndADontAllow(t *testing.T) {
	core, policy, _, file, _, _ := newToolsCore(t)
	state := func() string {
		_, metas := core.HelperDiscovery(file)
		for _, m := range metas {
			if m.Name == aitools.HelperName {
				return m.State
			}
		}
		return "missing"
	}
	if got := state(); got != "ready" {
		t.Fatalf("state = %s, want ready", got)
	}
	policy.mu.Lock()
	policy.ready, policy.denied = false, true
	policy.mu.Unlock()
	if got := state(); got != "denied" {
		t.Fatalf("state after Don't allow = %s, want denied", got)
	}

	// A document that declares a helper named ai-tools cannot shadow the built-in.
	policy.mu.Lock()
	policy.ready, policy.denied = true, false
	policy.mu.Unlock()
	writeHelperDocument(t, file, aitools.HelperName)
	if err := core.AttachHelpers(file, map[string]config.HelperProgram{aitools.HelperName: helperProgram(aitools.HelperName, "/nowhere")}, nil); err != nil {
		t.Fatal(err)
	}
	count := 0
	_, metas := core.HelperDiscovery(file)
	for _, m := range metas {
		if m.Name != aitools.HelperName {
			continue
		}
		count++
		if m.State != "ready" {
			t.Fatalf("state = %s, want ready from the built-in", m.State)
		}
	}
	if count != 1 {
		t.Fatalf("ai-tools listed %d times, want once", count)
	}
}

func TestAIToolsRefusals(t *testing.T) {
	cases := []struct {
		name, payload, tier string
		ready               bool
		code                string
	}{
		{"a folder path", `{"request":"x","folder":"/Users/me/app"}`, "mine", true, "invalid_request"},
		{"an untrusted document", `{"request":"x"}`, "unknown", false, "helper_not_granted"},
		{"the internet", `{"request":"x","tools":"internet"}`, "mine", true, "level_unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			core, policy, _, file, _, srv := newToolsCore(t)
			policy.tier, policy.ready = tc.tier, tc.ready
			sub := observer(t, core, file)
			sendTools(t, srv, "r3", tc.payload)
			end := nextOf(t, sub, "r3", true)
			if end.Type != "wire/error" || helperErrorCode(t, end) != tc.code {
				t.Fatalf("frame = %s %s %s", end.Type, end.Text, end.Payload)
			}
			policy.mu.Lock()
			asked := len(policy.askedDocument)
			policy.mu.Unlock()
			if (tc.code == "helper_not_granted") != (asked == 1) {
				t.Fatalf("asked the document %d times", asked)
			}
			if tc.code == "helper_not_granted" && end.Text != aiEditAskText {
				t.Fatalf("refusal = %q, want AI editing's", end.Text)
			}
		})
	}
	t.Run("change from an allowed stranger runs read-only", func(t *testing.T) {
		core, policy, _, file, _, srv := newToolsCore(t)
		policy.tier = "unknown"
		sub := observer(t, core, file)
		sendTools(t, srv, "r4", `{"request":"x","tools":"change"}`)
		var end Envelope
		for end.Type == "" || end.Type == "wire/ack" {
			end = nextOf(t, sub, "r4", true)
		}
		if end.Type != "wire/done" || !strings.Contains(string(end.Payload), `"level":"read"`) {
			t.Fatalf("frame = %s %s %s", end.Type, end.Text, end.Payload)
		}
	})
}

func TestForgetUnderCancelsAIToolsRuns(t *testing.T) {
	core, _, _, file, bin, srv := newToolsCore(t)
	os.WriteFile(filepath.Join(bin, "hang"), nil, 0o600)
	sendTools(t, srv, "r-forget", `{"request":"x"}`)
	var pid int
	deadline := time.Now().Add(20 * time.Second)
	for pid == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the agent never started")
		}
		b, _ := os.ReadFile(filepath.Join(bin, "pid"))
		pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		time.Sleep(20 * time.Millisecond)
	}
	core.ForgetUnder(filepath.Dir(file))
	deadline = time.Now().Add(5 * time.Second)
	for {
		if p, _ := os.FindProcess(pid); p.Signal(syscall.Signal(0)) != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the agent %d still runs after its folder was forgotten", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

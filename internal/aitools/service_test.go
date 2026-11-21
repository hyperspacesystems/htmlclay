package aitools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/testutil"
)

func request(t *testing.T, payload string) Request {
	t.Helper()
	r, perr := Parse([]byte(payload))
	if perr != nil {
		t.Fatalf("Parse: %v", perr)
	}
	return r
}

const titleSchema = `{"type":"object","additionalProperties":false,"required":["title"],"properties":{"title":{"type":"string"}}}`

func codeOf(err error) string {
	var coded *Error
	if errors.As(err, &coded) {
		return coded.Code
	}
	return ""
}

func TestClaudeRunsRestrictedWithTheTokenOnlyInItsEnvironment(t *testing.T) {
	f := newFakes(t)
	repo := testutil.PlantGitRepo(t)
	folder, err := filepath.EvalSymlinks(repo.Dir)
	if err != nil {
		t.Fatal(err)
	}
	f.set(t, "claude.answer", `{"data":{"title":"T"},"sources":[{"id":"diff","kind":"git","base":"main","head":"feature","paths":[],"path":"","first":0,"last":0},{"id":"s1","kind":"file","path":"a.txt","first":2,"last":3,"base":"","head":"","paths":[]}]}`)
	s := f.service(t)
	files := &runFiles{}
	var phases []string
	started := 0
	res, err := s.Run(context.Background(), Job{
		Document: "/doc.htmlclay",
		Folder:   folder,
		Level:    LevelRead,
		Request:  request(t, `{"request":"Explain the branch.","schema":`+titleSchema+`,"sources":true,"engine":"claude"}`),
		Status:   func(phase, text string) { phases = append(phases, phase) },
		Started:  func() { started++ },
		Put:      files.put,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(res.Data) != `{"title":"T"}` || res.Engine != AgentClaude || res.Model != "claude-fake-1" {
		t.Fatalf("result = %s %s %s", res.Data, res.Engine, res.Model)
	}
	if len(res.Sources) != 2 || res.Sources[0].Kind != "git" || len(res.Sources[0].Base) != 40 || len(res.Sources[0].Head) != 40 {
		t.Fatalf("sources = %+v", res.Sources)
	}
	if got := res.Sources[0].Skipped; !slices.Contains(got, "package-lock.json") {
		t.Fatalf("skipped = %v", got)
	}
	if !strings.Contains(string(files.files["diff"]), "+BETA") || strings.Contains(string(files.files["diff"]), `"v": 2`) {
		t.Fatalf("diff run file = %q", files.files["diff"])
	}
	if string(files.files["s1"]) != "BETA\ngamma\n" || res.Sources[1].Lines[0] != 2 || res.Sources[1].Lines[1] != 3 {
		t.Fatalf("file source = %q %v", files.files["s1"], res.Sources[1].Lines)
	}
	if ran := repo.Ran(); ran != "" {
		t.Fatalf("a program the repository names ran: %s", ran)
	}
	prompts := f.calls(t, ".prompt")
	var run string
	for _, p := range prompts {
		if b, _ := os.ReadFile(p); strings.Contains(string(b), "Explain the branch.") {
			run = strings.TrimSuffix(p, ".prompt")
		}
	}
	if run == "" {
		t.Fatalf("no real run among %v", prompts)
	}
	prompt, _ := os.ReadFile(run + ".prompt")
	if !strings.Contains(string(prompt), "feature since it left main") || !strings.Contains(string(prompt), "+BETA") {
		t.Fatalf("prompt has no git context:\n%s", prompt)
	}
	argv, _ := os.ReadFile(run + ".argv")
	args := strings.Split(string(argv), "\n")
	for _, want := range [][]string{{"--restricted"}, {"--tools", "Read,Grep,Glob"}, {"--strict-mcp-config"}, {"--permission-mode", "dontAsk"}, {"--setting-sources", ""}} {
		if !hasArgs(args, want...) {
			t.Errorf("argv lacks %q:\n%s", want, argv)
		}
	}
	if strings.Contains(string(argv), "tok-1") {
		t.Fatal("the token is on the command line")
	}
	env, _ := os.ReadFile(run + ".env")
	for _, want := range []string{"CLAUDE_CODE_OAUTH_TOKEN=tok-1\n", "CLAUDE_CODE_TMPDIR="} {
		if !strings.Contains(string(env), want) {
			t.Errorf("env lacks %q", want)
		}
	}
	scratch := filepath.Join(filepath.Dir(f.bin), "scratch") + string(filepath.Separator)
	for _, line := range strings.Split(string(env), "\n") {
		if home, ok := strings.CutPrefix(line, "HOME="); ok && !strings.HasPrefix(home, scratch) {
			t.Fatalf("Claude ran with HOME %s, not a scratch folder", home)
		}
	}
	pwd, _ := os.ReadFile(run + ".pwd")
	if strings.TrimSpace(string(pwd)) != folder {
		t.Fatalf("ran in %q, want %q", pwd, folder)
	}
	if started == 0 || !strings.Contains(strings.Join(phases, ","), "starting,starting,working,collecting") {
		t.Fatalf("started=%d phases=%v", started, phases)
	}
	if rec, ok := s.Store().Check(AgentClaude, "2.1.329 (Claude Code)", LevelRead); !ok || !rec.Passed {
		t.Fatalf("self-test record = %+v %v", rec, ok)
	}
}

func TestClaudeWithoutRestrictedIsRefused(t *testing.T) {
	f := newFakes(t)
	f.set(t, "claude.no-restricted", "")
	_, err := f.service(t).Run(context.Background(), Job{Folder: realTemp(t), Level: LevelRead, Request: request(t, `{"request":"x","engine":"claude"}`), Put: (&runFiles{}).put})
	if codeOf(err) != CodeEngineUnsupported || !strings.Contains(err.Error(), "--restricted") {
		t.Fatalf("err = %v", err)
	}
	if len(f.calls(t, ".prompt")) != 0 {
		t.Fatal("Claude ran without --restricted")
	}
}

func TestClaudeStartingWithOtherToolsIsStopped(t *testing.T) {
	f := newFakes(t)
	f.set(t, "claude.tools", `"Read","Grep","Glob","StructuredOutput","Bash"`)
	_, err := f.service(t).Run(context.Background(), Job{Folder: realTemp(t), Level: LevelRead, Request: request(t, `{"request":"x","engine":"claude"}`), Put: (&runFiles{}).put})
	if codeOf(err) != CodeEngineUnsupported || !strings.Contains(err.Error(), "the tool Bash") || !strings.Contains(err.Error(), "HTML Clay needs an update") {
		t.Fatalf("the refusal must name the tool and say HTML Clay needs an update: %v", err)
	}
}

func TestClaudeRenewsARefusedSignInOnceThenRetries(t *testing.T) {
	f := newFakes(t)
	s := f.service(t)
	login := []string{"PATH=" + f.bin + ":/usr/bin:/bin", "HOME=" + f.home, "CLAUDE_CODE_OAUTH_TOKEN=leak", "SSH_AUTH_SOCK=/tmp/agent.sock"}
	s.cfg.LoginEnv = func() []string { return login }
	job := Job{Folder: realTemp(t), Level: LevelRead, Request: request(t, `{"request":"x","engine":"claude"}`), Put: (&runFiles{}).put}
	if _, err := s.Run(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	f.set(t, "claude.valid", "tok-2")
	f.set(t, "claude.renewed", "tok-2")
	var phases []string
	job.Status = func(phase, text string) { phases = append(phases, text) }
	if _, err := s.Run(context.Background(), job); err != nil {
		t.Fatalf("after renewal: %v", err)
	}
	if strings.TrimSpace(f.read(t, "claude.token")) != "tok-2" || !strings.Contains(strings.Join(phases, "|"), "renew its sign-in") {
		t.Fatalf("no renewal: token %q, status %v", f.read(t, "claude.token"), phases)
	}
	renewals := 0
	for _, p := range f.calls(t, ".argv") {
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), "Reply with the word OK.") {
			renewals++
			env, _ := os.ReadFile(strings.TrimSuffix(p, ".argv") + ".env")
			if !strings.Contains(string(env), "HOME="+f.home+"\n") || strings.Contains(string(env), "CLAUDE_CODE_OAUTH_TOKEN") || strings.Contains(string(env), "SSH_AUTH_SOCK") {
				t.Fatalf("renewal must run as the person, with no token or agent: %s", env)
			}
			if argv, _ := os.ReadFile(p); !strings.Contains(string(argv), "--strict-mcp-config") {
				t.Fatalf("renewal argv lacks --strict-mcp-config: %s", argv)
			}
		}
	}
	if renewals != 1 {
		t.Fatalf("renewals = %d, want 1", renewals)
	}
	f.set(t, "claude.valid", "tok-3")
	if _, err := s.Run(context.Background(), job); codeOf(err) != CodeEngineFailed || !strings.Contains(err.Error(), "sign in again") {
		t.Fatalf("a token renewal cannot fix: %v", err)
	}

	// An expired token is renewed before the run; when the run is refused too,
	// the request renews that one time and then reports the expired sign-in.
	f = newFakes(t)
	f.set(t, "claude.renewed", "tok-renewed")
	s = f.service(t)
	s.cfg.ClaudeToken = func(context.Context) (Token, error) {
		b, err := os.ReadFile(filepath.Join(f.bin, "claude.token"))
		if err != nil {
			return Token{}, ErrNoToken
		}
		token := Token{Access: strings.TrimSpace(string(b)), Expires: time.Now().Add(-time.Minute)}
		if token.Access == "tok-renewed" {
			token.Expires = time.Now().Add(time.Hour)
		}
		return token, nil
	}
	job = Job{Folder: realTemp(t), Level: LevelRead, Request: request(t, `{"request":"x","engine":"claude"}`), Put: (&runFiles{}).put}
	if _, err := s.Run(context.Background(), job); codeOf(err) != CodeEngineFailed || !strings.Contains(err.Error(), "sign in again") {
		t.Fatalf("an expired token the renewal cannot fix: %v", err)
	}
	renewals = 0
	for _, p := range f.calls(t, ".argv") {
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), "Reply with the word OK.") {
			renewals++
		}
	}
	if renewals != 1 {
		t.Fatalf("renewals = %d, want exactly 1 for one request", renewals)
	}

	// A renewal that leaves the token as close to expiring as before is not
	// tried a second time in the same request.
	f = newFakes(t)
	s = f.service(t)
	s.cfg.ClaudeToken = func(context.Context) (Token, error) {
		b, err := os.ReadFile(filepath.Join(f.bin, "claude.token"))
		if err != nil {
			return Token{}, ErrNoToken
		}
		return Token{Access: strings.TrimSpace(string(b)), Expires: time.Now().Add(time.Minute)}, nil
	}
	f.set(t, "claude.valid", "tok-other")
	job = Job{Folder: realTemp(t), Level: LevelRead, Request: request(t, `{"request":"x","engine":"claude"}`), Put: (&runFiles{}).put}
	if _, err := s.Run(context.Background(), job); codeOf(err) != CodeEngineFailed || !strings.Contains(err.Error(), "sign in again") {
		t.Fatalf("a renewal that changed nothing: %v", err)
	}
	renewals = 0
	for _, p := range f.calls(t, ".argv") {
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), "Reply with the word OK.") {
			renewals++
		}
	}
	if renewals != 1 {
		t.Fatalf("renewals = %d for a renewal that changed nothing, want 1", renewals)
	}
}

func TestSelfTestRefusesAnAgentThatLeaksAndKeepsTheResultPerVersion(t *testing.T) {
	f := newFakes(t)
	f.set(t, "claude.leak", "")
	s := f.service(t)
	job := Job{Folder: realTemp(t), Level: LevelRead, Request: request(t, `{"request":"x","engine":"claude"}`), Put: (&runFiles{}).put}
	_, err := s.Run(context.Background(), job)
	if codeOf(err) != CodeEngineUnsupported || !strings.Contains(err.Error(), CanaryFolderLink) {
		t.Fatalf("err = %v", err)
	}
	if len(f.calls(t, ".prompt")) != 2 {
		t.Fatalf("calls = %d, want the inside and outside test only", len(f.calls(t, ".prompt")))
	}
	os.Remove(filepath.Join(f.bin, "claude.leak"))
	if _, err := s.Run(context.Background(), job); codeOf(err) != CodeEngineUnsupported {
		t.Fatalf("the kept failure must refuse without a new test: %v", err)
	}
	if len(f.calls(t, ".prompt")) != 2 {
		t.Fatal("the self-test ran again for the same version")
	}
	f.set(t, "claude.version", "2.1.330 (Claude Code)")
	if err := os.WriteFile(filepath.Join(f.bin, "claude"), []byte(fakeClaude+"# 2.1.330\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Run(context.Background(), job); err != nil {
		t.Fatalf("a new version gets its own test: %v", err)
	}
}

func TestSelfTestPassesOnlyAnAgentThatTriedEveryWayOut(t *testing.T) {
	outsideRuns := func(f fakes) int {
		n := 0
		for _, p := range f.calls(t, ".prompt") {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), "Try each step") {
				n++
			}
		}
		return n
	}
	t.Run("claude that never tries", func(t *testing.T) {
		f := newFakes(t)
		f.set(t, "claude.lazy", "")
		s := f.service(t)
		job := Job{Folder: realTemp(t), Level: LevelRead, Request: request(t, `{"request":"x","engine":"claude"}`), Put: (&runFiles{}).put}
		_, err := s.Run(context.Background(), job)
		if codeOf(err) != CodeEngineFailed || !strings.Contains(err.Error(), "did not try") || !strings.Contains(err.Error(), CanaryFolderLink) {
			t.Fatalf("an agent that never tried must not pass: %v", err)
		}
		if n := outsideRuns(f); n != 2 {
			t.Fatalf("outside runs = %d, want the test and one retry", n)
		}
		if rec, ok := s.Store().Check(AgentClaude, "2.1.329 (Claude Code)", LevelRead); ok {
			t.Fatalf("an untried test must record nothing: %+v", rec)
		}
		os.Remove(filepath.Join(f.bin, "claude.lazy"))
		if _, err := s.Run(context.Background(), job); err != nil {
			t.Fatalf("the next request checks again and passes: %v", err)
		}
	})
	t.Run("claude that tries only the first way out", func(t *testing.T) {
		f := newFakes(t)
		f.set(t, "claude.partial", "")
		s := f.service(t)
		job := Job{Folder: realTemp(t), Level: LevelRead, Request: request(t, `{"request":"x","engine":"claude"}`), Put: (&runFiles{}).put}
		_, err := s.Run(context.Background(), job)
		if codeOf(err) != CodeEngineFailed || !strings.Contains(err.Error(), CanarySibling) || strings.Contains(err.Error(), CanaryAbsolute) {
			t.Fatalf("each way out needs its own try: %v", err)
		}
	})
	t.Run("claude that tries on the retry", func(t *testing.T) {
		f := newFakes(t)
		f.set(t, "claude.lazy-once", "")
		s := f.service(t)
		job := Job{Folder: realTemp(t), Level: LevelRead, Request: request(t, `{"request":"x","engine":"claude"}`), Put: (&runFiles{}).put}
		if _, err := s.Run(context.Background(), job); err != nil {
			t.Fatal(err)
		}
		if n := outsideRuns(f); n != 2 {
			t.Fatalf("outside runs = %d, want the test and one retry", n)
		}
		if rec, ok := s.Store().Check(AgentClaude, "2.1.329 (Claude Code)", LevelRead); !ok || !rec.Passed {
			t.Fatalf("record = %+v", rec)
		}
	})
	t.Run("agy that answers without trying", func(t *testing.T) {
		f := newFakes(t)
		f.set(t, "agy.lazy", "")
		s := f.service(t)
		job := Job{Folder: realTemp(t), Level: LevelRead, Request: request(t, `{"request":"x","engine":"agy"}`), Put: (&runFiles{}).put}
		_, err := s.Run(context.Background(), job)
		if codeOf(err) != CodeEngineFailed || !strings.Contains(err.Error(), "did not try") {
			t.Fatalf("an agy run that was never refused must not pass: %v", err)
		}
		if argvs, _ := filepath.Glob(filepath.Join(f.bin, "agy.argv.*")); len(argvs) != 3 {
			t.Fatalf("agy calls = %d, want the inside test, the first canary and its retry", len(argvs))
		}
		if _, ok := s.Store().Check(AgentAgy, "1.5.13", LevelRead); ok {
			t.Fatal("an untried test must record nothing")
		}
	})
}

func TestChangeLevelIsClaudeOnlyAndItsSelfTestChecksWrites(t *testing.T) {
	f := newFakes(t)
	s := f.service(t)
	job := Job{Folder: realTemp(t), Level: LevelChange, Request: request(t, `{"request":"x","tools":"change","engine":"codex"}`), Put: (&runFiles{}).put}
	if _, err := s.Run(context.Background(), job); codeOf(err) != CodeLevelUnavailable {
		t.Fatalf("codex at change: %v", err)
	}
	f.set(t, "claude.tools", `"Read","Grep","Glob","Edit","Write"`)
	f.set(t, "claude.write", "")
	job.Request = request(t, `{"request":"x","tools":"change","engine":"claude"}`)
	_, err := s.Run(context.Background(), job)
	if codeOf(err) != CodeEngineUnsupported || !strings.Contains(err.Error(), CanaryWrite) {
		t.Fatalf("a write outside the folder must fail the change test: %v", err)
	}
	for _, p := range f.calls(t, ".argv") {
		b, _ := os.ReadFile(p)
		args := strings.Split(string(b), "\n")
		if hasArgs(args, "-p") && !hasArgs(args, "--tools", "Read,Grep,Glob,Edit,Write") && !strings.Contains(string(b), "Reply with") {
			t.Fatalf("change run without Edit and Write: %s", b)
		}
		if hasArgs(args, "-p") && !hasArgs(args, "--restricted") {
			t.Fatalf("change run without --restricted: %s", b)
		}
	}
	if rec, ok := s.Store().Check(AgentClaude, "2.1.329 (Claude Code)", LevelRead); ok {
		t.Fatalf("the change test must not record the read level: %+v", rec)
	}
}

func TestCodexRunsTheFolderReadProfileAndKeepsItsSignIn(t *testing.T) {
	f := newFakes(t)
	folder := realTemp(t)
	res, err := f.service(t).Run(context.Background(), Job{Folder: folder, Level: LevelRead, Request: request(t, `{"request":"x","schema":`+titleSchema+`,"engine":"codex"}`), Put: (&runFiles{}).put})
	if err != nil {
		t.Fatal(err)
	}
	if res.Engine != AgentCodex || string(res.Data) != `{"title":"T"}` {
		t.Fatalf("result = %+v", res)
	}
	const folderRead = "default_permissions = \"folder-read\"\n\n[permissions.folder-read.filesystem]\n\":minimal\" = \"read\"\n\n[permissions.folder-read.filesystem.\":workspace_roots\"]\n\".\" = \"read\"\n\n[permissions.folder-read.network]\nenabled = false\n"
	if f.read(t, "codex.config") != folderRead {
		t.Fatalf("config.toml = %q", f.read(t, "codex.config"))
	}
	args := lines(f.read(t, "codex.argv"))
	for _, want := range [][]string{{"exec"}, {"--ephemeral"}, {"--cd", folder}, {"--disable", "plugins"}} {
		if !hasArgs(args, want...) {
			t.Errorf("argv lacks %q", want)
		}
	}
	for _, bad := range []string{"--yolo", "--dangerously-bypass-approvals-and-sandbox", "danger-full-access"} {
		if hasArgs(args, bad) {
			t.Fatalf("argv has %s", bad)
		}
	}
	env := f.read(t, "codex.env")
	if strings.Contains(env, "CODEX_HOME="+filepath.Join(f.home, ".codex")) || !strings.Contains(env, "CODEX_HOME=") {
		t.Fatalf("CODEX_HOME must be scratch: %s", env)
	}
	auth, _ := os.ReadFile(filepath.Join(f.home, ".codex", "auth.json"))
	if strings.TrimSpace(string(auth)) != `{"tokens":"refreshed"}` {
		t.Fatalf("a refreshed sign-in must be written back: %s", auth)
	}
}

func TestAgyRunsWithTheReadOnlySettingsAndExplainsARefusedRead(t *testing.T) {
	f := newFakes(t)
	folder := realTemp(t)
	s := f.service(t)
	job := Job{Folder: folder, Level: LevelRead, Request: request(t, `{"request":"x","schema":`+titleSchema+`,"engine":"agy"}`), Put: (&runFiles{}).put}
	if _, err := s.Run(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	argvs, _ := filepath.Glob(filepath.Join(f.bin, "agy.argv.*"))
	if len(argvs) != 1+len((&canaries{Attempts: map[string]string{CanaryAbsolute: "", CanaryParent: "", CanaryFileLink: "", CanaryFolderLink: "", CanarySibling: ""}}).names())+1 {
		t.Fatalf("agy calls = %d: one inside test, one per outside canary, one run", len(argvs))
	}
	for _, p := range argvs {
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), "--dangerously-skip-permissions") {
			t.Fatal("agy ran with --dangerously-skip-permissions")
		}
	}
	last, _ := os.ReadFile(filepath.Join(f.bin, "agy.argv.6"))
	args := strings.Split(string(last), "\n")
	var settings struct {
		AllowNonWorkspaceAccess bool `json:"allowNonWorkspaceAccess"`
		Permissions             struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
		} `json:"permissions"`
	}
	for i, a := range args {
		if a == "--settings" && i+1 < len(args) {
			json.Unmarshal([]byte(args[i+1]), &settings)
		}
	}
	if settings.AllowNonWorkspaceAccess || len(settings.Permissions.Allow) != 1 || settings.Permissions.Allow[0] != "read_file("+folder+"/**)" {
		t.Fatalf("settings = %+v", settings)
	}
	for _, want := range []string{"command(*)", "write_file(*)", "edit_file(*)"} {
		if !strings.Contains(strings.Join(settings.Permissions.Deny, " "), want) {
			t.Fatalf("deny lacks %s", want)
		}
	}
	f.set(t, "agy.refuse", "")
	_, err := s.Run(context.Background(), job)
	if codeOf(err) != CodeEngineFailed || !strings.Contains(err.Error(), "tried to read a file outside") || !strings.Contains(err.Error(), "Try again") {
		t.Fatalf("err = %v", err)
	}
}

func TestABusyAgentParksTheThirdRequest(t *testing.T) {
	f := newFakes(t)
	s := f.service(t)
	job := Job{Folder: realTemp(t), Level: LevelRead, Request: request(t, `{"request":"x","engine":"claude"}`), Put: (&runFiles{}).put}
	if _, err := s.Run(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	f.set(t, "claude.wait", "")
	var wg sync.WaitGroup
	var mu sync.Mutex
	parked := 0
	errs := make(chan error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		j := job
		j.Status = func(phase, text string) {
			if strings.Contains(text, "busy") {
				mu.Lock()
				parked++
				mu.Unlock()
			}
		}
		go func() { defer wg.Done(); _, err := s.Run(context.Background(), j); errs <- err }()
	}
	deadline := time.Now().Add(20 * time.Second)
	for len(f.calls(t, ".waiting")) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("two runs never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	if parked != 1 || len(f.calls(t, ".waiting")) != 2 {
		t.Fatalf("parked = %d, running = %d", parked, len(f.calls(t, ".waiting")))
	}
	mu.Unlock()
	f.set(t, "claude.go", "")
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestAnAnswerThatDoesNotMatchTheSchemaIsRefused(t *testing.T) {
	f := newFakes(t)
	f.set(t, "claude.answer", `{"data":{"title":7}}`)
	_, err := f.service(t).Run(context.Background(), Job{Folder: realTemp(t), Level: LevelRead, Request: request(t, `{"request":"x","schema":`+titleSchema+`,"engine":"claude"}`), Put: (&runFiles{}).put})
	if codeOf(err) != CodeResultInvalid || !strings.Contains(err.Error(), "data.title") {
		t.Fatalf("err = %v", err)
	}
}

func TestAMovedFolderIsFolderMissing(t *testing.T) {
	f := newFakes(t)
	_, err := f.service(t).Run(context.Background(), Job{Folder: filepath.Join(realTemp(t), "gone"), Level: LevelRead, Request: request(t, `{"request":"x","engine":"claude"}`), Put: (&runFiles{}).put})
	if codeOf(err) != CodeFolderMissing || strings.Contains(err.Error(), string(filepath.Separator)) {
		t.Fatalf("err = %v (must name the folder only, never its path)", err)
	}
}

func TestAgyGetsADiffThatFitsItsLimit(t *testing.T) {
	f := newFakes(t)
	folder := realTemp(t)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = folder
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	line := strings.Repeat("x", 90) + "\n"
	big := filepath.Join(folder, "big.txt")
	if err := os.WriteFile(big, []byte(strings.Repeat(line, 2000)), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "big.txt")
	git("commit", "-qm", "first")
	if err := os.WriteFile(big, []byte(strings.Repeat(line, 4000)), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "big.txt")
	git("commit", "-qm", "second")

	// The limit is lowered so a diff of a few hundred kilobytes is over it, the
	// way a real diff is over the limit on Linux and Windows.
	limit := agyMaxPrompt
	agyMaxPrompt = 64 << 10
	defer func() { agyMaxPrompt = limit }()

	s := f.service(t)
	job := Job{Folder: folder, Level: LevelRead, Request: request(t, `{"request":"x","schema":`+titleSchema+`,"engine":"agy"}`), Put: (&runFiles{}).put}
	if _, err := s.Run(context.Background(), job); err != nil {
		t.Fatalf("agy refused the run: %v", err)
	}
	argvs, _ := filepath.Glob(filepath.Join(f.bin, "agy.argv.*"))
	prompt := ""
	for _, p := range argvs {
		b, _ := os.ReadFile(p)
		// The prompt is the last argument, so it runs to the end of the file.
		if i := strings.LastIndex(string(b), "\n-p="); i >= 0 && strings.Contains(string(b), "### Diff") {
			prompt = strings.TrimSuffix(string(b)[i+len("\n-p="):], "\n")
		}
	}
	if prompt == "" {
		t.Fatal("the run's prompt never reached agy")
	}
	if len(prompt) > agyMaxPrompt {
		t.Fatalf("the prompt is %d bytes, over the %d byte limit", len(prompt), agyMaxPrompt)
	}
	if !strings.Contains(prompt, "Only the first") {
		t.Fatal("the prompt does not say the diff was cut to fit")
	}
}

const fakeSleepingClaude = `#!/bin/sh
here=$(cd "$(dirname "$0")" && pwd)
case "$1" in
--version) echo "2.1.329 (Claude Code)"; exit 0;;
--help) echo "  --restricted    Keep Claude Code inside the working folder"; echo "  -p, --print"; exit 0;;
esac
echo $$ > "$here/sleep.pid"
sleep 30
`

func waitForSelfTestProcess(t *testing.T, f fakes) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		b, err := os.ReadFile(filepath.Join(f.bin, "sleep.pid"))
		if err == nil {
			pid, perr := strconv.Atoi(strings.TrimSpace(string(b)))
			if perr == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("the self-test never started its agent")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if p, err := os.FindProcess(pid); err != nil || p.Signal(syscall.Signal(0)) != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the self-test's agent (pid %d) still runs after its waiter left", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestASelfTestStopsWhenItsOnlyWaiterLeaves(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake agents are shell scripts")
	}
	f := newFakes(t)
	f.set(t, "claude", fakeSleepingClaude)
	s := f.service(t)
	ctx, cancel := context.WithCancel(context.Background())
	job := Job{Folder: realTemp(t), Level: LevelRead, Request: request(t, `{"request":"x","engine":"claude"}`), Put: (&runFiles{}).put}
	done := make(chan error, 1)
	go func() {
		_, err := s.Run(ctx, job)
		done <- err
	}()
	pid := waitForSelfTestProcess(t, f)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled request returned no error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the cancelled request never returned")
	}
	waitForProcessGone(t, pid)
}

func TestCloseStopsASelfTest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake agents are shell scripts")
	}
	f := newFakes(t)
	f.set(t, "claude", fakeSleepingClaude)
	s := f.service(t)
	job := Job{Folder: realTemp(t), Level: LevelRead, Request: request(t, `{"request":"x","engine":"claude"}`), Put: (&runFiles{}).put}
	done := make(chan error, 1)
	go func() {
		_, err := s.Run(context.Background(), job)
		done <- err
	}()
	pid := waitForSelfTestProcess(t, f)
	s.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a closed service returned no error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the closed request never returned")
	}
	waitForProcessGone(t, pid)
}

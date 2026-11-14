package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// tokenLaneFixture is an agent fixture whose lane holds a token a local program
// would read, made the way a start makes one.
func tokenLaneFixture(t *testing.T) (*agentLaneFixture, string) {
	t.Helper()
	fx := newAgentLaneFixture(t)
	token, file, err := loadAgentToken(t.TempDir())
	if err != nil {
		t.Fatalf("loadAgentToken: %v", err)
	}
	fx.app.rt.agentToken, fx.app.rt.agentTokenFile = token, file
	return fx, token
}

func TestM3AgentToken(t *testing.T) {
	t.Run("the good request is answered", func(t *testing.T) {
		fx, token := tokenLaneFixture(t)
		rec := agentLaneRequest(t, fx.app, http.MethodGet, "/whoami",
			map[string]string{"Authorization": "Bearer " + token})
		if rec.Code != http.StatusOK {
			t.Fatalf("GET whoami with the token = %d %s, want 200", rec.Code, rec.Body.String())
		}
	})

	t.Run("no token answers 401 naming the token file", func(t *testing.T) {
		fx, _ := tokenLaneFixture(t)
		rec := servedRequest(t, fx.app, http.MethodGet, agentLaneHost, agentLanePrefix+"/whoami", "", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("GET whoami with no token = %d %s, want 401", rec.Code, rec.Body.String())
		}
		var refusal agentLaneRefusal
		if err := json.Unmarshal(rec.Body.Bytes(), &refusal); err != nil {
			t.Fatalf("refusal body %s: %v", rec.Body.String(), err)
		}
		if refusal.Error.Code != "unauthorized" {
			t.Errorf("code = %q, want unauthorized", refusal.Error.Code)
		}
		if !strings.Contains(refusal.Error.Message, fx.app.rt.agentTokenFile) {
			t.Errorf("message = %q, want it to name %q", refusal.Error.Message, fx.app.rt.agentTokenFile)
		}
		if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
			t.Errorf("WWW-Authenticate = %q, want Bearer", got)
		}
	})

	t.Run("a wrong token answers 401", func(t *testing.T) {
		fx, token := tokenLaneFixture(t)
		wrong := strings.Repeat("c", 64)
		for name, header := range map[string]string{
			"a wrong bearer": "Bearer " + wrong,
			"a basic header": "Basic " + token,
		} {
			rec := agentLaneRequest(t, fx.app, http.MethodGet, "/whoami", map[string]string{"Authorization": header})
			var refusal agentLaneRefusal
			if rec.Code != http.StatusUnauthorized || json.Unmarshal(rec.Body.Bytes(), &refusal) != nil || refusal.Error.Code != "unauthorized" {
				t.Fatalf("%s = %d %s, want 401 unauthorized", name, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("an empty token admits nothing", func(t *testing.T) {
		fx := newAgentLaneFixture(t)
		fx.app.rt.agentToken, fx.app.rt.agentTokenFile = "", testAgentTokenFile
		rec := agentLaneRequest(t, fx.app, http.MethodGet, "/whoami", map[string]string{"Authorization": "Bearer "})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("GET whoami on a lane with no token = %d %s, want 401", rec.Code, rec.Body.String())
		}
		var refusal agentLaneRefusal
		if err := json.Unmarshal(rec.Body.Bytes(), &refusal); err != nil || refusal.Error.Code != "unauthorized" {
			t.Fatalf("refusal = %s, want unauthorized", rec.Body.String())
		}
	})

	t.Run("the file is 0600 and holds 64 hex characters", func(t *testing.T) {
		dir := t.TempDir()
		token, file, err := loadAgentToken(dir)
		if err != nil {
			t.Fatalf("loadAgentToken: %v", err)
		}
		if file != filepath.Join(dir, agentTokenName) {
			t.Fatalf("token file = %q, want %q", file, filepath.Join(dir, agentTokenName))
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(file)
			if err != nil {
				t.Fatalf("stat %s: %v", file, err)
			}
			if mode := info.Mode().Perm(); mode != 0o600 {
				t.Errorf("mode = %#o, want 0600", mode)
			}
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		held := strings.TrimSpace(string(data))
		if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(held) {
			t.Errorf("file holds %q, want 64 lowercase hex characters", held)
		}
		if held != token {
			t.Errorf("file holds %q, but loadAgentToken answered %q", held, token)
		}
		left, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
		if err != nil {
			t.Fatalf("glob: %v", err)
		}
		if len(left) != 0 {
			t.Errorf("temporary files left behind: %v", left)
		}
	})

	t.Run("a second start keeps the same token", func(t *testing.T) {
		dir := t.TempDir()
		first, _, err := loadAgentToken(dir)
		if err != nil {
			t.Fatalf("first loadAgentToken: %v", err)
		}
		second, _, err := loadAgentToken(dir)
		if err != nil {
			t.Fatalf("second loadAgentToken: %v", err)
		}
		if first != second {
			t.Fatalf("second start answered %q, want the first token %q", second, first)
		}
		if err := os.WriteFile(filepath.Join(dir, agentTokenName), []byte("not-a-token\n"), 0o600); err != nil {
			t.Fatalf("write a bad token file: %v", err)
		}
		fresh, _, err := loadAgentToken(dir)
		if err != nil {
			t.Fatalf("loadAgentToken over a bad file: %v", err)
		}
		if fresh == "not-a-token" || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(fresh) {
			t.Fatalf("a bad token file answered %q, want a fresh token", fresh)
		}
	})

	t.Run("the token appears in no snapshot and no log line", func(t *testing.T) {
		// The config folder is short on purpose: the single-instance socket lives
		// in it, and a test name is longer than a socket path may be.
		base, err := os.MkdirTemp("", "cfg")
		if err != nil {
			t.Fatalf("temp config base: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(base) })
		t.Setenv("HTMLCLAY_CONFIG_BASE", base)

		read, write, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}
		saved := os.Stderr
		os.Stderr = write
		starter := &app{rt: &appRuntime{}}
		starter.initConfig()
		os.Stderr = saved
		write.Close()
		var logged bytes.Buffer
		if _, err := io.Copy(&logged, read); err != nil {
			t.Fatalf("read stderr: %v", err)
		}
		read.Close()

		token := starter.rt.agentToken
		if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(token) {
			t.Fatalf("initConfig answered the token %q, want 64 lowercase hex characters", token)
		}
		if strings.Contains(logged.String(), token) {
			t.Errorf("the token appears in the start's log lines: %s", logged.String())
		}

		home := evalTempHome(t)
		a := newServedTestApp(t, home, t.TempDir())
		a.rt.agentToken, a.rt.agentTokenFile = token, starter.rt.agentTokenFile
		snapshot, err := json.Marshal(a.snapshot())
		if err != nil {
			t.Fatalf("encode the snapshot: %v", err)
		}
		if strings.Contains(string(snapshot), token) {
			t.Errorf("the token appears in the snapshot: %s", snapshot)
		}
	})
}

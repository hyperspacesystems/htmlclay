package aitools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

const fakeClaude = `#!/bin/sh
here=$(cd "$(dirname "$0")" && pwd)
case "$1" in
--version) cat "$here/claude.version" 2>/dev/null || echo "2.1.329 (Claude Code)"; exit 0;;
--help) [ -f "$here/claude.no-restricted" ] || echo "  --restricted    Keep Claude Code inside the working folder"; echo "  -p, --print"; exit 0;;
esac
call="$here/claude.call.$$"
for a in "$@"; do printf '%s\n' "$a"; done > "$call.argv"
env > "$call.env"
pwd > "$call.pwd"
last=""; for a in "$@"; do last="$a"; done
if [ "$last" = "Reply with the word OK." ]; then
  cp "$here/claude.renewed" "$here/claude.token" 2>/dev/null
  echo '{"type":"result","is_error":false,"result":"OK"}'
  exit 0
fi
prompt=$(cat)
printf '%s' "$prompt" > "$call.prompt"
tools=$(cat "$here/claude.tools" 2>/dev/null || echo '"Read","Grep","Glob","StructuredOutput"')
echo '{"type":"system","subtype":"init","tools":['"$tools"'],"mcp_servers":[],"model":"claude-fake-1"}'
if [ "$CLAUDE_CODE_OAUTH_TOKEN" != "$(cat "$here/claude.valid" 2>/dev/null)" ]; then
  echo '{"type":"result","is_error":true,"api_error_status":401,"result":"Invalid bearer token"}'
  exit 1
fi
case "$prompt" in
*"Read the file inside.txt"*)
  printf '{"type":"result","is_error":false,"structured_output":{"data":{"inside":"%s","outside":[]}}}\n' "$(head -n1 inside.txt)";;
*"Try each step"*)
  touch "$here/claude.outside-tried"
  lazy=""
  [ -f "$here/claude.lazy" ] && lazy=1
  [ -f "$here/claude.lazy-once" ] && { lazy=1; rm "$here/claude.lazy-once"; }
  if [ -z "$lazy" ]; then
    printf '%s\n' "$prompt" | grep -E '^[0-9]+\. ' | while IFS= read -r step; do
      [ -f "$here/claude.partial" ] && [ -n "$tried" ] && continue
      tried=1
      tool=Read; case "$step" in *create*) tool=Write;; esac
      printf '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"%s","input":{"file_path":"%s"}}]}}\n' "$tool" "$(printf '%s' "$step" | tr -d '"\\')"
    done
  fi
  echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"StructuredOutput","input":{"outside":["absolute.txt elsewhere parent.txt notes.txt docs project-old written-up.txt written.txt were refused"]}}]}}'
  [ -f "$here/claude.leak" ] && printf '{"type":"user","tool_result":"%s"}\n' "$(cat docs/inner.txt)"
  [ -f "$here/claude.write" ] && echo HELLO > ../written-up.txt
  echo '{"type":"result","is_error":false,"structured_output":{"data":{"inside":"","outside":["../parent.txt was refused"]}}}';;
*)
  if [ -f "$here/claude.wait" ]; then
    touch "$call.waiting"
    while [ ! -f "$here/claude.go" ]; do sleep 0.05; done
  fi
  printf '{"type":"result","is_error":false,"structured_output":%s}\n' "$(cat "$here/claude.answer")";;
esac
`

const fakeCodex = `#!/bin/sh
here=$(cd "$(dirname "$0")" && pwd)
case "$1" in --version) echo "codex-cli 0.166.0"; exit 0;; esac
env > "$here/codex.env"
for a in "$@"; do printf '%s\n' "$a"; done > "$here/codex.argv"
cp "$CODEX_HOME/config.toml" "$here/codex.config"
out=""; prev=""
for a in "$@"; do [ "$prev" = "-o" ] && out="$a"; prev="$a"; done
prompt=$(cat)
case "$prompt" in
*"Read the file inside.txt"*) printf '{"data":{"inside":"%s","outside":[]}}' "$(head -n1 inside.txt)" > "$out";;
*"Try each step"*) printf '{"data":{"inside":"","outside":[]}}' > "$out";;
*) cat "$here/codex.answer" > "$out"; echo '{"tokens":"refreshed"}' > "$CODEX_HOME/auth.json";;
esac
`

const fakeAgy = `#!/bin/sh
here=$(cd "$(dirname "$0")" && pwd)
case "$1" in --version) echo "1.5.13"; exit 0;; esac
n=$(ls "$here" | grep -c '^agy.argv')
for a in "$@"; do printf '%s\n' "$a"; done > "$here/agy.argv.$n"
env > "$here/agy.env"
prompt=""
for a in "$@"; do case "$a" in -p=*) prompt="${a#-p=}";; esac; done
refuse() { echo 'jetski: no output produced, a tool required the "read_file" permission that headless mode cannot prompt for' >&2; exit 1; }
case "$prompt" in
*"Read the file inside.txt"*) printf '{"status":"SUCCESS","response":"","structured_output":{"data":{"inside":"%s","outside":[]}}}' "$(head -n1 inside.txt)";;
*"Try each step"*) [ -f "$here/agy.lazy" ] && { printf '{"status":"SUCCESS","response":"","structured_output":{"data":{"inside":"","outside":[]}}}'; exit 0; }; refuse;;
*) [ -f "$here/agy.refuse" ] && refuse; printf '{"status":"SUCCESS","response":"","structured_output":%s}' "$(cat "$here/agy.answer")";;
esac
`

// fakes is a folder of fake agents and the files that steer them.
type fakes struct {
	bin  string
	home string
}

func newFakes(t *testing.T) fakes {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake agents are shell scripts")
	}
	root := realTemp(t)
	f := fakes{bin: filepath.Join(root, "bin"), home: filepath.Join(root, "home")}
	for _, dir := range []string{f.bin, filepath.Join(f.home, ".codex")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for name, script := range map[string]string{"claude": fakeClaude, "codex": fakeCodex, "agy": fakeAgy} {
		if err := os.WriteFile(filepath.Join(f.bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.set(t, "claude.token", "tok-1")
	f.set(t, "claude.valid", "tok-1")
	f.set(t, "claude.answer", `{"data":{"title":"T"}}`)
	f.set(t, "codex.answer", `{"data":{"title":"T"}}`)
	f.set(t, "agy.answer", `{"data":{"title":"T"}}`)
	if err := os.WriteFile(filepath.Join(f.home, ".codex", "auth.json"), []byte(`{"tokens":"first"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f fakes) set(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.bin, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f fakes) read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.bin, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// calls lists the fake Claude's calls that read stdin, by the file suffix.
func (f fakes) calls(t *testing.T, suffix string) []string {
	t.Helper()
	matches, _ := filepath.Glob(filepath.Join(f.bin, "claude.call.*"+suffix))
	return matches
}

func (f fakes) service(t *testing.T) *Service {
	t.Helper()
	store, err := OpenStore(filepath.Join(filepath.Dir(f.bin), "store"))
	if err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + f.bin + ":/usr/bin:/bin", "HOME=" + f.home}
	return NewService(Config{
		Store:    store,
		Scratch:  filepath.Join(filepath.Dir(f.bin), "scratch"),
		Home:     f.home,
		LoginEnv: func() []string { return env },
		ClaudeToken: func(context.Context) (Token, error) {
			b, err := os.ReadFile(filepath.Join(f.bin, "claude.token"))
			if err != nil {
				return Token{}, ErrNoToken
			}
			return Token{Access: strings.TrimSpace(string(b))}, nil
		},
	})
}

func realTemp(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// runFiles is a Put that keeps what it stores.
type runFiles struct {
	mu    sync.Mutex
	files map[string][]byte
}

func (r *runFiles) put(id string, body []byte) (string, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.files == nil {
		r.files = map[string][]byte{}
	}
	r.files[id] = append([]byte(nil), body...)
	sum := sha256.Sum256(body)
	return "/_/wire/runs/0123456789abcdef0123456789abcdef/files/" + id, hex.EncodeToString(sum[:]), nil
}

func lines(s string) []string { return strings.Split(strings.TrimSpace(s), "\n") }

func hasArgs(argv []string, want ...string) bool {
	for i := range argv {
		if i+len(want) <= len(argv) && slicesEqual(argv[i:i+len(want)], want) {
			return true
		}
	}
	return false
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

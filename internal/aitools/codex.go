package aitools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/runner"
)

// CodexConfig is the whole config.toml in Codex's scratch home: the
// folder-read permission profile, which round 3 proved with codex sandbox on
// Codex 0.166.0 (every outside read refused, reads inside allowed, writes and
// network refused). Codex reads no other config: its home is scratch.
const CodexConfig = `default_permissions = "folder-read"

[permissions.folder-read.filesystem]
":minimal" = "read"

[permissions.folder-read.filesystem.":workspace_roots"]
"." = "read"

[permissions.folder-read.network]
enabled = false
`

// codexDisabled are Codex features that reach past the folder or the
// computer. Its shell stays: Codex reads files through it, inside the
// folder-read sandbox.
var codexDisabled = []string{"apps", "browser_use", "computer_use", "plugins", "image_generation", "multi_agent"}

func codexArgs(folder, schemaFile, outFile, model string) []string {
	argv := []string{"exec", "--ephemeral", "--skip-git-repo-check", "--color", "never", "--cd", folder, "--output-schema", schemaFile, "-o", outFile}
	for _, f := range codexDisabled {
		argv = append(argv, "--disable", f)
	}
	if model != "" {
		argv = append(argv, "-m", model)
	}
	return append(argv, "-")
}

// codexAuth is the person's own Codex sign-in, copied into the scratch home
// and written back when Codex refreshed it during the run.
func codexAuth(home string) string { return filepath.Join(home, ".codex", "auth.json") }

func runCodex(ctx context.Context, program string, loginEnv []string, home string, c call) (reply, error) {
	codexHome := filepath.Join(c.Scratch, "codex-home")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		return reply{}, err
	}
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(CodexConfig), 0o600); err != nil {
		return reply{}, err
	}
	auth, err := os.ReadFile(codexAuth(home))
	if err != nil {
		return reply{}, fail(CodeEngineFailed, "Codex is not signed in on this computer. Run codex in Terminal once to sign in, then try again.")
	}
	scratchAuth := filepath.Join(codexHome, "auth.json")
	if err := os.WriteFile(scratchAuth, auth, 0o600); err != nil {
		return reply{}, err
	}
	schemaFile := filepath.Join(c.Scratch, "schema.json")
	if err := os.WriteFile(schemaFile, c.Schema, 0o600); err != nil {
		return reply{}, err
	}
	outFile := filepath.Join(c.Scratch, "answer.txt")
	res := run(ctx, proc{
		Argv:    append([]string{program}, codexArgs(c.Folder, schemaFile, outFile, c.Model)...),
		Dir:     c.Folder,
		Env:     runner.CleanEnv(c.Scratch, agentPath(program, loginEnv), "CODEX_HOME="+codexHome),
		Stdin:   c.Prompt,
		Started: c.Started,
	})
	writeBackCodexAuth(home, auth, scratchAuth)
	out := append(append([]byte(nil), res.Stdout...), res.Stderr...)
	if err := ctx.Err(); err != nil {
		return reply{Output: out}, err
	}
	text, readErr := os.ReadFile(outFile)
	if res.Err != nil || readErr != nil {
		message := "Codex stopped without an answer."
		if t := tail(res.Stderr, 300); t != "" {
			message += " It said: " + t
		}
		return reply{Output: out}, fail(CodeEngineFailed, message)
	}
	out = append(out, text...)
	answer, ok := jsonObject(string(text))
	if !ok {
		return reply{Output: out, Model: c.Model}, fail(CodeResultInvalid, "Codex's answer was not the JSON the page asked for.")
	}
	return reply{Answer: answer, Model: c.Model, Output: out}, nil
}

// writeBackCodexAuth copies a sign-in Codex refreshed during the run back to
// the person's own record, unless that record changed meanwhile.
func writeBackCodexAuth(home string, before []byte, scratchAuth string) {
	after, err := os.ReadFile(scratchAuth)
	if err != nil || bytes.Equal(after, before) {
		return
	}
	var check map[string]any
	if json.Unmarshal(after, &check) != nil {
		return
	}
	path := codexAuth(home)
	if now, err := os.ReadFile(path); err != nil || !bytes.Equal(now, before) {
		return
	}
	tmp := fmt.Sprintf("%s.htmlclay-%d", path, os.Getpid())
	if err := os.WriteFile(tmp, after, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
	}
}

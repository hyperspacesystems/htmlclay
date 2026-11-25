//go:build managedtrial

package served

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// tailwind-compile.cjs is the compiler this host runs: tailwind-hyperclay's own
// compileTailwind, loaded from the Local checkout named by --local-source, so
// the stylesheet is byte-identical to the one Local writes. HTML Clay's own
// `internal/tailwind` runs a different compiler with different plugins, which is
// why this host does not use it.
//
//go:embed tailwind-compile.cjs
var tailwindScript []byte

// tailwindScriptPath is the embedded script written once per process for node to
// run.
var (
	tailwindScriptOnce sync.Once
	tailwindScriptFile string
	tailwindScriptErr  error
)

// tailwindCompiler is the pinned compiler, replaceable by a test.
var tailwindCompiler = func(localSource, html string) (string, error) {
	return compileTailwindWithNode(localSource, html)
}

// compileTailwindWithNode is tailwind-hyperclay's compileTailwind over one
// document: the script on disk, the document on stdin, the CSS on stdout.
func compileTailwindWithNode(localSource, html string) (string, error) {
	script, err := tailwindScriptPath()
	if err != nil {
		return "", err
	}
	cmd := exec.Command("node", script, "--local-source", localSource)
	cmd.Stdin = bytes.NewBufferString(html)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := stderr.String()
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("%s", message)
	}
	return stdout.String(), nil
}

func tailwindScriptPath() (string, error) {
	tailwindScriptOnce.Do(func() {
		dir := filepath.Join(os.TempDir(), fmt.Sprintf("htmlclay-managed-trial-%d", os.Getpid()))
		if err := os.MkdirAll(dir, 0o777); err != nil {
			tailwindScriptErr = err
			return
		}
		path := filepath.Join(dir, "tailwind-compile.cjs")
		if err := os.WriteFile(path, tailwindScript, 0o666); err != nil {
			tailwindScriptErr = err
			return
		}
		tailwindScriptFile = path
	})
	return tailwindScriptFile, tailwindScriptErr
}

func init() {
	defaultTailwindCompile = func(localSource string) func(string) (string, error) {
		return func(html string) (string, error) {
			return tailwindCompiler(localSource, html)
		}
	}
}

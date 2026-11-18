package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A key that arrives through a pipe or a file is read as it is: there is no
// terminal whose echo could be turned off, and setEcho says so instead of
// changing anything. This runs on every system, so the Windows console code
// is at least compiled and called by GitHub's Windows run.
func TestTheKeyPromptLeavesAFileThatIsNoTerminalAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key.txt")
	if err := os.WriteFile(path, []byte(connectKeyOne+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := setEcho(file, false); err == nil {
		t.Fatal("setEcho changed the echo of a file that is no terminal")
	}
	var prompt bytes.Buffer
	key, err := readSyncKey(file, &prompt)
	if err != nil || key != connectKeyOne {
		t.Fatalf("readSyncKey = %q, %v", key, err)
	}
	if strings.TrimSpace(prompt.String()) != "" {
		t.Fatalf("a key from a file was prompted for: %q", prompt.String())
	}
}

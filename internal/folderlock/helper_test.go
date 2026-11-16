package folderlock_test

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panphora/htmlclay/internal/folderlock"
)

func TestHelperSharesLocksWithNode(t *testing.T) {
	tmp := t.TempDir()
	lockDir := filepath.Join(tmp, "htmlclay-folder-locks")
	realFolder := filepath.Join(tmp, "RealDocs")
	linkFolder := filepath.Join(tmp, "LinkDocs")
	if err := os.MkdirAll(realFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realFolder, linkFolder); err != nil {
		t.Fatal(err)
	}

	// 1. Prove CGO_ENABLED=0 cross-compilation for darwin/arm64, linux/amd64, and windows/amd64.
	for _, target := range [][2]string{
		{"darwin", "arm64"},
		{"linux", "amd64"},
		{"windows", "amd64"},
	} {
		out := filepath.Join(tmp, "helper-"+target[0]+"-"+target[1])
		if target[0] == "windows" {
			out += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", out, "../../cmd/folderlock-helper")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target[0], "GOARCH="+target[1])
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cross-build %v failed: %v\n%s", target, err, output)
		}
	}

	// Build native helper for this machine.
	helperBin := filepath.Join(tmp, "folderlock-helper")
	buildCmd := exec.Command("go", "build", "-o", helperBin, "../../cmd/folderlock-helper")
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("native build failed: %v\n%s", err, output)
	}

	clientPath, err := filepath.Abs("../../cmd/folderlock-helper/client.cjs")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Go acquires lock via symlink; Node attempts acquire on realFolder and gets held=true.
	goLock, err := folderlock.Acquire(lockDir, linkFolder)
	if err != nil {
		t.Fatalf("Go Acquire via symlink: %v", err)
	}

	nodeDriver := filepath.Join(tmp, "driver.cjs")
	driverSrc := `
const readline = require('node:readline');
const { createFolderLockClient } = require(process.argv[2]);
const client = createFolderLockClient(process.argv[3], process.argv[4]);
const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
rl.on('line', async (line) => {
  const cmd = JSON.parse(line);
  if (cmd.op === 'acquire') console.log(JSON.stringify(await client.acquire(cmd.id, cmd.folder)));
  if (cmd.op === 'release') console.log(JSON.stringify(await client.release(cmd.id)));
});
console.log('DRIVER_READY');
`
	if err := os.WriteFile(nodeDriver, []byte(driverSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	nodeCmd := exec.Command("node", nodeDriver, clientPath, helperBin, lockDir)
	stdin, err := nodeCmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := nodeCmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	nodeCmd.Stderr = os.Stderr
	if err := nodeCmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if nodeCmd.Process != nil {
			_ = nodeCmd.Process.Kill()
			_ = nodeCmd.Wait()
		}
	})

	reader := bufio.NewScanner(stdout)
	if !reader.Scan() || reader.Text() != "DRIVER_READY" {
		t.Fatalf("unexpected driver startup: %q", reader.Text())
	}

	_, _ = stdin.Write([]byte(`{"op":"acquire","id":"root-1","folder":` + `"` + realFolder + `"}` + "\n"))
	if !reader.Scan() {
		t.Fatal("no response from node driver")
	}
	if !strings.Contains(reader.Text(), `"ok":false`) || !strings.Contains(reader.Text(), `"held":true`) {
		t.Fatalf("expected Node acquire to see held=true while Go holds symlink lock, got %s", reader.Text())
	}

	// 3. Go releases -> Node acquires -> Go sees folderlock.ErrHeld!
	if err := goLock.Release(); err != nil {
		t.Fatal(err)
	}
	_, _ = stdin.Write([]byte(`{"op":"acquire","id":"root-1","folder":` + `"` + realFolder + `"}` + "\n"))
	if !reader.Scan() || !strings.Contains(reader.Text(), `"ok":true`) {
		t.Fatalf("expected Node acquire to succeed after Go release, got %s", reader.Text())
	}

	if _, err := folderlock.Acquire(lockDir, linkFolder); !errors.Is(err, folderlock.ErrHeld) {
		t.Fatalf("expected Go Acquire to get ErrHeld while Node holds lock, got %v", err)
	}

	// 4. SIGKILL Node process -> stdin closes -> helper exits -> Go re-acquires within 1s!
	if err := nodeCmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = nodeCmd.Wait()

	deadline := time.Now().Add(2 * time.Second)
	var recovered *folderlock.Lock
	for time.Now().Before(deadline) {
		recovered, err = folderlock.Acquire(lockDir, realFolder)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("Go could not re-acquire lock after Node SIGKILL: %v", err)
	}
	_ = recovered.Release()
}

package folderlock

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testFolder(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	folder := filepath.Join(base, "folder")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	return folder, filepath.Join(base, "locks")
}

func TestAcquireTwiceInOneProcess(t *testing.T) {
	folder, dir := testFolder(t)

	first, err := Acquire(dir, folder)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	second, err := Acquire(dir, folder)
	if !errors.Is(err, ErrHeld) {
		t.Fatalf("second Acquire = %v (lock %v), want ErrHeld", err, second)
	}
	if second != nil {
		t.Fatalf("second Acquire returned a lock while the folder was held")
	}
	if err := first.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	third, err := Acquire(dir, folder)
	if err != nil {
		t.Fatalf("Acquire after Release: %v", err)
	}
	if err := third.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestDifferentFoldersDoNotConflict(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "locks")
	one := filepath.Join(base, "one")
	two := filepath.Join(base, "two")
	for _, folder := range []string{one, two} {
		if err := os.Mkdir(folder, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	first, err := Acquire(dir, one)
	if err != nil {
		t.Fatalf("Acquire %s: %v", one, err)
	}
	defer first.Release()
	second, err := Acquire(dir, two)
	if err != nil {
		t.Fatalf("Acquire %s while %s was held: %v", two, one, err)
	}
	defer second.Release()
}

func TestSameFolderThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need a privilege on Windows")
	}
	folder, dir := testFolder(t)
	link := filepath.Join(filepath.Dir(folder), "link")
	if err := os.Symlink(folder, link); err != nil {
		t.Fatal(err)
	}

	held, err := Acquire(dir, folder)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer held.Release()
	through, err := Acquire(dir, link)
	if !errors.Is(err, ErrHeld) {
		t.Fatalf("Acquire through the symlink = %v (lock %v), want ErrHeld", err, through)
	}
}

// TestChild is the helper process TestTwoProcessesRace starts. Without the
// environment marker it does nothing.
func TestChild(t *testing.T) {
	if os.Getenv("FOLDERLOCK_CHILD") != "1" {
		t.Skip("helper process for TestTwoProcessesRace")
	}
	lock, err := Acquire(os.Getenv("FOLDERLOCK_DIR"), os.Getenv("FOLDERLOCK_FOLDER"))
	switch {
	case errors.Is(err, ErrHeld):
		fmt.Fprintln(os.Stdout, "HELD")
	case err != nil:
		fmt.Fprintln(os.Stdout, "ERROR "+err.Error())
	default:
		fmt.Fprintln(os.Stdout, "WON")
	}
	io.Copy(io.Discard, os.Stdin)
	if lock != nil {
		lock.Release()
	}
}

type lockChild struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	out   *bufio.Reader
}

func startLockChild(t *testing.T, dir, folder string) *lockChild {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestChild$")
	cmd.Env = append(os.Environ(),
		"FOLDERLOCK_CHILD=1",
		"FOLDERLOCK_DIR="+dir,
		"FOLDERLOCK_FOLDER="+folder,
	)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	child := &lockChild{cmd: cmd, stdin: stdin, out: bufio.NewReader(stdout)}
	t.Cleanup(func() {
		stdin.Close()
		cmd.Process.Kill()
		cmd.Wait()
	})
	return child
}

func (c *lockChild) answer(t *testing.T) string {
	t.Helper()
	line, err := c.out.ReadString('\n')
	if err != nil {
		t.Fatalf("reading child answer: %v", err)
	}
	return strings.TrimSpace(line)
}

func (c *lockChild) finish(t *testing.T) {
	t.Helper()
	c.stdin.Close()
	c.cmd.Wait()
}

func TestTwoProcessesRace(t *testing.T) {
	folder, dir := testFolder(t)

	for i := 0; i < 20; i++ {
		first := startLockChild(t, dir, folder)
		second := startLockChild(t, dir, folder)
		children := []*lockChild{first, second}
		answers := []string{first.answer(t), second.answer(t)}

		winner := -1
		for j, answer := range answers {
			switch answer {
			case "WON":
				if winner >= 0 {
					t.Fatalf("iteration %d: both children won: %v", i, answers)
				}
				winner = j
			case "HELD":
			default:
				t.Fatalf("iteration %d: child %d answered %q", i, j, answer)
			}
		}
		if winner < 0 {
			t.Fatalf("iteration %d: neither child won: %v", i, answers)
		}

		won := children[winner]
		if err := won.cmd.Process.Kill(); err != nil {
			t.Fatalf("iteration %d: killing the winner: %v", i, err)
		}
		won.cmd.Wait()
		children[1-winner].finish(t)

		third := startLockChild(t, dir, folder)
		if answer := third.answer(t); answer != "WON" {
			t.Fatalf("iteration %d: a child after the winner was killed answered %q, want WON", i, answer)
		}
		third.finish(t)
	}
}

func TestDefaultDirHonorsTheEnvironment(t *testing.T) {
	want := t.TempDir()
	t.Setenv("HTMLCLAY_FOLDER_LOCKS", want)
	got, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}
	if got != want {
		t.Fatalf("DefaultDir = %q, want %q", got, want)
	}
}

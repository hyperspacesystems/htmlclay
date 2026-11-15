//go:build darwin

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const shellParentEnv = "HTMLCLAY_TEST_SHELL_PARENT"

// startShellParentTree starts a parent process that starts a child; the child
// arms exitWithParent (and disarms it when disarm is set) and reports ready.
// It answers the parent and the child's pid.
func startShellParentTree(t *testing.T, disarm bool) (*exec.Cmd, int) {
	t.Helper()
	parent := exec.Command(os.Args[0], "-test.run=^TestShellHostExitsWithItsParent$")
	mode := "parent"
	if disarm {
		mode = "parent-disarm"
	}
	parent.Env = append(os.Environ(), shellParentEnv+"="+mode)
	out, err := parent.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		_ = parent.Process.Kill()
		t.Fatalf("no child pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		_ = parent.Process.Kill()
		t.Fatalf("bad child pid %q", line)
	}
	return parent, pid
}

func alive(pid int) bool {
	return !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

func TestShellHostExitsWithItsParent(t *testing.T) {
	switch mode := os.Getenv(shellParentEnv); mode {
	case "child", "child-disarm":
		exitWithParent()
		if mode == "child-disarm" {
			shellParentWatch.Store(false)
		}
		fmt.Println("armed")
		time.Sleep(5 * time.Second)
		os.Exit(0)
	case "parent", "parent-disarm":
		childMode := "child"
		if mode == "parent-disarm" {
			childMode = "child-disarm"
		}
		child := exec.Command(os.Args[0], "-test.run=^TestShellHostExitsWithItsParent$")
		child.Env = append(os.Environ(), shellParentEnv+"="+childMode)
		out, err := child.StdoutPipe()
		if err != nil {
			os.Exit(2)
		}
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		if _, err := bufio.NewReader(out).ReadString('\n'); err != nil {
			os.Exit(2)
		}
		fmt.Println(child.Process.Pid)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}

	t.Run("armed: the host exits with its parent", func(t *testing.T) {
		parent, pid := startShellParentTree(t, false)
		_ = parent.Process.Kill()
		_ = parent.Wait()
		deadline := time.Now().Add(2 * time.Second)
		for alive(pid) && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if alive(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatal("the host outlived its parent")
		}
	})

	t.Run("disarmed: the host outlives its parent", func(t *testing.T) {
		parent, pid := startShellParentTree(t, true)
		_ = parent.Process.Kill()
		_ = parent.Wait()
		time.Sleep(time.Second)
		if !alive(pid) {
			t.Fatal("a disarmed watch still ended the host")
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
	})
}

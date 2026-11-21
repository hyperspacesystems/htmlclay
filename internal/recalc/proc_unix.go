//go:build !windows

package recalc

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

type browserProc struct {
	cmd         *exec.Cmd
	toBrowser   *os.File
	fromBrowser *os.File
	stderr      *limitedBuffer
	killOnce    sync.Once
}

func startBrowser(bin string, args []string) (*browserProc, error) {
	childRead, hostWrite, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("cdp pipe 3: %w", err)
	}
	hostRead, childWrite, err := os.Pipe()
	if err != nil {
		_ = childRead.Close()
		_ = hostWrite.Close()
		return nil, fmt.Errorf("cdp pipe 4: %w", err)
	}

	stderr := &limitedBuffer{max: 8192}
	cmd := exec.Command(bin, args...)
	cmd.Stderr = stderr
	cmd.WaitDelay = 2 * time.Second
	// fd 3 = browser reads CDP commands; fd 4 = browser writes CDP replies/events.
	cmd.ExtraFiles = []*os.File{childRead, childWrite}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		_ = childRead.Close()
		_ = hostWrite.Close()
		_ = hostRead.Close()
		_ = childWrite.Close()
		return nil, fmt.Errorf("start browser: %w", err)
	}
	_ = childRead.Close()
	_ = childWrite.Close()

	return &browserProc{
		cmd:         cmd,
		toBrowser:   hostWrite,
		fromBrowser: hostRead,
		stderr:      stderr,
	}, nil
}

func (p *browserProc) kill() {
	if p == nil {
		return
	}
	p.killOnce.Do(func() {
		if p.toBrowser != nil {
			_ = p.toBrowser.Close()
		}
		if p.fromBrowser != nil {
			_ = p.fromBrowser.Close()
		}
		if p.cmd != nil && p.cmd.Process != nil {
			if err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				_ = p.cmd.Process.Kill()
			}
			_ = p.cmd.Wait()
		}
	})
}

func (p *browserProc) pid() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *browserProc) stderrText() string {
	if p == nil || p.stderr == nil {
		return ""
	}
	return p.stderr.String()
}

func needsNoSandbox() bool {
	return runtime.GOOS == "linux" && os.Geteuid() == 0
}

func platformArgs() []string {
	if runtime.GOOS == "linux" {
		// Prevent a fresh hidden profile from triggering a desktop GNOME Keyring or KWallet prompt.
		return []string{"--password-store=basic"}
	}
	return nil
}

// isSnapBrowser reports whether bin launches a Snap-confined browser on Linux.
func isSnapBrowser(bin string) bool {
	if runtime.GOOS != "linux" || bin == "" {
		return false
	}
	if strings.HasPrefix(bin, "/snap/") {
		return true
	}
	if resolved, err := filepath.EvalSymlinks(bin); err == nil {
		if strings.HasPrefix(resolved, "/snap/") || resolved == "/usr/bin/snap" {
			return true
		}
	}
	// Ubuntu's /usr/bin/chromium-browser is a small shell wrapper around /snap/bin/chromium.
	if f, err := os.Open(bin); err == nil {
		var head [512]byte
		n, _ := f.Read(head[:])
		_ = f.Close()
		s := string(head[:n])
		if strings.HasPrefix(s, "#!") && strings.Contains(s, "/snap/bin/") {
			return true
		}
	}
	return false
}

// profileParentDir returns the parent directory for a hidden browser's temporary profile.
// Snap-confined browsers mount a private /tmp, so a profile created in host /tmp leaks inside
// /tmp/snap-private-tmp/snap.<name>/tmp; placing it under ~/snap/chromium/common lets both the
// Snap sandbox and host os.RemoveAll see the same directory.
func profileParentDir(bin string) string {
	if !isSnapBrowser(bin) {
		return ""
	}
	home := ""
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		home = u.HomeDir
	} else if h, err := os.UserHomeDir(); err == nil {
		home = h
	}
	if home == "" {
		return ""
	}
	dir := filepath.Join(home, "snap", "chromium", "common")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	return dir
}

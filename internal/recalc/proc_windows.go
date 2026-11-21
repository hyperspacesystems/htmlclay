//go:build windows

package recalc

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type browserProc struct {
	cmd         *exec.Cmd
	toBrowser   *os.File
	fromBrowser *os.File
	job         windows.Handle
	stderr      *limitedBuffer
	killOnce    sync.Once
}

func createKillOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func startBrowser(bin string, args []string) (*browserProc, error) {
	sa := windows.SecurityAttributes{
		Length:        uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		InheritHandle: 1,
	}

	var childRead, hostWrite windows.Handle
	if err := windows.CreatePipe(&childRead, &hostWrite, &sa, 64*1024); err != nil {
		return nil, fmt.Errorf("cdp read pipe: %w", err)
	}
	if err := windows.SetHandleInformation(hostWrite, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		_ = windows.CloseHandle(childRead)
		_ = windows.CloseHandle(hostWrite)
		return nil, fmt.Errorf("cdp write handle inherit: %w", err)
	}

	var hostRead, childWrite windows.Handle
	if err := windows.CreatePipe(&hostRead, &childWrite, &sa, 64*1024); err != nil {
		_ = windows.CloseHandle(childRead)
		_ = windows.CloseHandle(hostWrite)
		return nil, fmt.Errorf("cdp write pipe: %w", err)
	}
	if err := windows.SetHandleInformation(hostRead, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		_ = windows.CloseHandle(childRead)
		_ = windows.CloseHandle(hostWrite)
		_ = windows.CloseHandle(hostRead)
		_ = windows.CloseHandle(childWrite)
		return nil, fmt.Errorf("cdp read handle inherit: %w", err)
	}

	// Pass the two inherited Win32 pipe handles to Chromium via --remote-debugging-io-pipes
	// (inserted before the trailing "about:blank" URL argument).
	pipeFlag := fmt.Sprintf("--remote-debugging-io-pipes=%d,%d", uint32(childRead), uint32(childWrite))
	fullArgs := make([]string, 0, len(args)+1)
	if len(args) > 0 && args[len(args)-1] == "about:blank" {
		fullArgs = append(fullArgs, args[:len(args)-1]...)
		fullArgs = append(fullArgs, pipeFlag, "about:blank")
	} else {
		fullArgs = append(fullArgs, args...)
		fullArgs = append(fullArgs, pipeFlag)
	}

	job, jobErr := createKillOnCloseJob()
	stderr := &limitedBuffer{max: 8192}
	cmd := exec.Command(bin, fullArgs...)
	cmd.Stderr = stderr
	cmd.WaitDelay = 2 * time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW,
		AdditionalInheritedHandles: []syscall.Handle{
			syscall.Handle(childRead),
			syscall.Handle(childWrite),
		},
	}

	if err := cmd.Start(); err != nil {
		_ = windows.CloseHandle(childRead)
		_ = windows.CloseHandle(hostWrite)
		_ = windows.CloseHandle(hostRead)
		_ = windows.CloseHandle(childWrite)
		if jobErr == nil && job != 0 {
			_ = windows.CloseHandle(job)
		}
		return nil, fmt.Errorf("start browser: %w", err)
	}

	// Close the child ends in the parent so browser exit delivers EOF to hostRead.
	_ = windows.CloseHandle(childRead)
	_ = windows.CloseHandle(childWrite)

	if jobErr == nil && job != 0 {
		procH, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
		if err != nil {
			_ = windows.CloseHandle(job)
			job = 0
		} else {
			if aErr := windows.AssignProcessToJobObject(job, procH); aErr != nil {
				_ = windows.CloseHandle(job)
				job = 0
			}
			_ = windows.CloseHandle(procH)
		}
	}

	return &browserProc{
		cmd:         cmd,
		toBrowser:   os.NewFile(uintptr(hostWrite), "cdp-to-browser"),
		fromBrowser: os.NewFile(uintptr(hostRead), "cdp-from-browser"),
		job:         job,
		stderr:      stderr,
	}, nil
}

type jobAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

func waitJobEmpty(job windows.Handle, limit time.Duration) {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		var info jobAccounting
		err := windows.QueryInformationJobObject(
			job,
			windows.JobObjectBasicAccountingInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
			nil,
		)
		if err != nil || info.ActiveProcesses == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (p *browserProc) kill() {
	if p == nil {
		return
	}
	p.killOnce.Do(func() {
		// Terminate the job (or process) before closing synchronous pipe handles so the child
		// breaks the pipe cleanly instead of racing CancelIoEx against a live blocking ReadFile.
		if p.job != 0 {
			_ = windows.TerminateJobObject(p.job, 1)
			waitJobEmpty(p.job, 2*time.Second)
			_ = windows.CloseHandle(p.job)
			p.job = 0
		} else if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		if p.toBrowser != nil {
			_ = p.toBrowser.Close()
		}
		if p.fromBrowser != nil {
			_ = p.fromBrowser.Close()
		}
		if p.cmd != nil && p.cmd.Process != nil {
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

func needsNoSandbox() bool { return false }

func platformArgs() []string { return nil }

func profileParentDir(string) string { return "" }

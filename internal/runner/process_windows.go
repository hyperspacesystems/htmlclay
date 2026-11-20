//go:build windows

package runner

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobProcess is a child inside a job object that kills every process in it
// when the job closes, as internal/service/spawn_windows.go does for the
// service. Programs the child starts join the job too.
type jobProcess struct {
	cmd   *exec.Cmd
	done  chan struct{}
	err   error
	mu    sync.Mutex
	job   windows.Handle
	input *stdinPipe
}

// resumeProcess starts the suspended child, so it runs only once it is in the
// job: a program it started in the meantime would escape the job.
var resumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

func startGroup(ctx context.Context, cmd *exec.Cmd, pipes []*pipe, in *stdinPipe) (Process, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_SUSPENDED}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		closePipes(pipes)
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		closePipes(pipes)
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		windows.CloseHandle(job)
		closePipes(pipes)
		if in != nil {
			in.close()
		}
		return nil, err
	}
	if in != nil {
		in.start()
	}
	copied := startCopies(pipes)
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, proc)
	}
	if err == nil {
		var status uintptr
		if status, _, _ = resumeProcess.Call(uintptr(proc)); status != 0 {
			err = fmt.Errorf("runner: resuming the child: %v", windows.NTStatus(status))
		}
	}
	if proc != 0 {
		windows.CloseHandle(proc)
	}
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		windows.CloseHandle(job)
		if in != nil {
			in.closeWrite()
		}
		finishCopies(copied, pipes)
		return nil, err
	}
	p := &jobProcess{cmd: cmd, done: make(chan struct{}), job: job, input: in}
	go func() {
		p.err = cmd.Wait()
		p.closeJob()
		if in != nil {
			in.closeWrite()
		}
		finishCopies(copied, pipes)
		close(p.done)
	}()
	go func() {
		select {
		case <-ctx.Done():
			p.Kill()
		case <-p.done:
		}
	}()
	return p, nil
}

func (p *jobProcess) Wait() error {
	<-p.done
	return p.err
}

func (p *jobProcess) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job == 0 {
		return nil
	}
	if p.input != nil {
		p.input.closeWrite()
	}
	return windows.TerminateJobObject(p.job, 1)
}

func (p *jobProcess) closeJob() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job != 0 {
		windows.CloseHandle(p.job)
		p.job = 0
	}
}

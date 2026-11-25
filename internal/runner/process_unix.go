//go:build !windows

package runner

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
)

type groupProcess struct {
	cmd   *exec.Cmd
	done  chan struct{}
	err   error
	input *stdinPipe
}

func startGroup(ctx context.Context, cmd *exec.Cmd, pipes []*pipe, in *stdinPipe, _ uint64) (Process, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
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
	g := &groupProcess{cmd: cmd, done: make(chan struct{}), input: in}
	go func() {
		g.err = cmd.Wait()
		killGroup(cmd.Process.Pid)
		if in != nil {
			in.closeWrite()
		}
		finishCopies(copied, pipes)
		close(g.done)
	}()
	go func() {
		select {
		case <-ctx.Done():
			g.Kill()
		case <-g.done:
		}
	}()
	return g, nil
}

func (g *groupProcess) Wait() error {
	<-g.done
	return g.err
}

func (g *groupProcess) Pid() int { return g.cmd.Process.Pid }

func (g *groupProcess) Kill() error {
	select {
	case <-g.done:
		return nil
	default:
	}
	if g.input != nil {
		g.input.closeWrite()
	}
	return killGroup(g.cmd.Process.Pid)
}

// killGroup ends every process in the group. The kernel keeps a group's id
// from being reused while any member lives, so this never reaches a stranger.
func killGroup(pgid int) error {
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

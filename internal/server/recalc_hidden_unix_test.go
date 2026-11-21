//go:build !windows

package server

import (
	"errors"
	"syscall"
)

func processGone(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return errors.Is(err, syscall.ESRCH)
}

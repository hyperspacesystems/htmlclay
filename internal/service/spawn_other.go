//go:build !windows

package service

import "os/exec"

// afterStart does nothing off Windows: the service holds no file descriptors of
// its own and end of stdin ends it when the app goes away.
func afterStart(cmd *exec.Cmd) (func(), error) {
	return nil, nil
}

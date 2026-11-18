//go:build !windows

package main

import (
	"os"
	"os/exec"
)

// setEcho turns the terminal's echo of typed characters on or off, so a sync
// key typed at the prompt does not show on screen.
func setEcho(tty *os.File, on bool) error {
	mode := "-echo"
	if on {
		mode = "echo"
	}
	cmd := exec.Command("stty", mode)
	cmd.Stdin = tty
	return cmd.Run()
}

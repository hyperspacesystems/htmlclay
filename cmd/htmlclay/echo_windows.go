//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// setEcho turns the console's echo of typed characters on or off, so a sync
// key typed at the prompt does not show on screen. Windows has no stty: the
// console's input mode carries the echo flag.
func setEcho(tty *os.File, on bool) error {
	handle := windows.Handle(tty.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return err
	}
	if on {
		mode |= windows.ENABLE_ECHO_INPUT
	} else {
		mode &^= windows.ENABLE_ECHO_INPUT
	}
	return windows.SetConsoleMode(handle, mode)
}

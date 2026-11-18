//go:build !linux && !windows

package main

import (
	"errors"
	"time"
)

func clickTray(pid int, prefix string, id int, wait time.Duration, list bool) error {
	return errors.New("click works on Linux (the tray menu is on D-Bus) and on Windows (the tray window takes the click)")
}

//go:build windows

package main

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

// The tray library gives the app one hidden top-level window of this class, and
// its window procedure runs a menu row when WM_COMMAND arrives with the row's
// id. Ids count from 1 in the order the rows were made.
const (
	trayWindowClass = "SystrayClass"
	wmCommand       = 0x0111
)

var (
	postMessage = windows.NewLazySystemDLL("user32.dll").NewProc("PostMessageW")

	// One callback for the life of the process: Windows keeps a small fixed
	// number of them. wantPid and foundWindow are what it reads and writes.
	wantPid     uint32
	foundWindow windows.HWND
	findTray    = windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var owner uint32
		if _, err := windows.GetWindowThreadProcessId(hwnd, &owner); err != nil || owner != wantPid {
			return 1
		}
		name := make([]uint16, 64)
		n, err := windows.GetClassName(hwnd, &name[0], int32(len(name)))
		if err != nil || windows.UTF16ToString(name[:n]) != trayWindowClass {
			return 1
		}
		foundWindow = hwnd
		return 0
	})
)

// trayWindow finds the tray's hidden window in pid, or 0.
func trayWindow(pid int) windows.HWND {
	wantPid, foundWindow = uint32(pid), 0
	// EnumWindows reports an error when the callback stops it early, so the
	// answer is foundWindow and not its error.
	_ = windows.EnumWindows(findTray, nil)
	return foundWindow
}

// clickTray sends the tray of pid the message its own menu sends when a person
// picks row id. Windows has no way to read another process's tray labels, so a
// row is named by id and never by prefix: 1.x makes "Restart to Update" first,
// which is id 1. The row runs whatever it means at that moment, so the caller
// waits until the app has logged that the update is staged.
func clickTray(pid int, prefix string, id int, wait time.Duration, list bool) error {
	if list {
		return errors.New("the tray menu cannot be listed on Windows; a row is clicked by -id")
	}
	if id <= 0 {
		return errors.New("on Windows a row is clicked by -id, not by -prefix")
	}
	deadline := time.Now().Add(wait)
	for {
		if hwnd := trayWindow(pid); hwnd != 0 {
			if ok, _, err := postMessage.Call(uintptr(hwnd), wmCommand, uintptr(id), 0); ok == 0 {
				return fmt.Errorf("could not post the click to the tray window of pid %d: %w", pid, err)
			}
			fmt.Printf("clicked: row id %d of pid %d\n", id, pid)
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("pid %d has no tray window (class %s); the app must run in the signed-in desktop session", pid, trayWindowClass)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

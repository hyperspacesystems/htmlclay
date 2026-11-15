//go:build darwin

package main

import (
	"os"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

// shellParentWatch is armed while a host the Mac app started is still starting:
// if the app dies then, nobody would ever close its stdin.
var shellParentWatch atomic.Bool

func exitWithParent() {
	parent := os.Getppid()
	kq, err := unix.Kqueue()
	if err != nil {
		return
	}
	shellParentWatch.Store(true)
	change := []unix.Kevent_t{{Ident: uint64(parent), Filter: unix.EVFILT_PROC, Flags: unix.EV_ADD | unix.EV_ONESHOT, Fflags: unix.NOTE_EXIT}}
	if _, err := unix.Kevent(kq, change, nil, nil); err != nil {
		if os.Getppid() != parent {
			os.Exit(3)
		}
		return
	}
	go func() {
		events := make([]unix.Kevent_t, 1)
		for {
			n, err := unix.Kevent(kq, nil, events, nil)
			if n > 0 || (err != nil && err != unix.EINTR) {
				if shellParentWatch.Load() {
					os.Exit(3)
				}
				return
			}
		}
	}()
}

//go:build darwin

package platform

import (
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func downloaded(path string) bool {
	_, err := unix.Getxattr(path, "com.apple.quarantine", nil)
	return err == nil
}

func changeTime(info os.FileInfo) (time.Time, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(st.Ctimespec.Unix()), true
}

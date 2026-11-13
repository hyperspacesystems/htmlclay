//go:build windows

package platform

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// downloaded reads the Zone.Identifier stream browsers attach to downloads.
// Zone 3 is the internet and 4 is restricted sites; 0 to 2 are this machine,
// the local intranet and trusted sites.
func downloaded(path string) bool {
	f, err := os.Open(path + ":Zone.Identifier")
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		value, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "ZoneId=")
		if !ok {
			continue
		}
		zone, err := strconv.Atoi(strings.TrimSpace(value))
		return err != nil || zone >= 3
	}
	return false
}

func changeTime(info os.FileInfo) (time.Time, bool) {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(0, data.CreationTime.Nanoseconds()), true
}

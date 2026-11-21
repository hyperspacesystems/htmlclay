//go:build linux

package carried

import (
	"bytes"
	"os"
	"strconv"
)

const memoryWatched = true

var pageSize = uint64(os.Getpagesize())

// memoryOf is the process's resident size, from /proc/<pid>/statm.
func memoryOf(pid int) (uint64, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/statm")
	if err != nil {
		return 0, false
	}
	fields := bytes.Fields(data)
	if len(fields) < 2 {
		return 0, false
	}
	pages, err := strconv.ParseUint(string(fields[1]), 10, 64)
	if err != nil {
		return 0, false
	}
	return pages * pageSize, true
}

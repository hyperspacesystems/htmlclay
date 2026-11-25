//go:build windows

package carried

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const memoryWatched = true

type processMemoryCounters struct {
	cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

var getProcessMemoryInfo = windows.NewLazySystemDLL("kernel32.dll").NewProc("K32GetProcessMemoryInfo")

// memoryOf is the process's committed memory. The job object's limit holds
// the cap in the kernel; this watch reports it as the memory cap.
func memoryOf(pid int) (uint64, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0, false
	}
	defer windows.CloseHandle(h)
	var c processMemoryCounters
	c.cb = uint32(unsafe.Sizeof(c))
	if ok, _, _ := getProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&c)), uintptr(c.cb)); ok == 0 {
		return 0, false
	}
	return uint64(c.PagefileUsage), true
}

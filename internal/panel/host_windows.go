//go:build windows

package panel

import (
	"os/exec"
	"syscall"
	"unsafe"

	"github.com/wailsapp/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
)

var (
	pAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
	pGetCursorPos             = user32.NewProc("GetCursorPos")
	pSetThreadDpiContext      = user32.NewProc("SetThreadDpiAwarenessContext")
)

// RuntimeInstalled reports whether this PC has the WebView2 runtime, with the
// binding's own registry lookup, which answers "" and no error when it has not.
func RuntimeInstalled() bool {
	version, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString("")
	return err == nil && version != ""
}

// ChildCommand starts exe as the panel's child, with no console window and
// with its WebView2 profile in dataDir.
func ChildCommand(exe, dataDir string) *exec.Cmd {
	cmd := exec.Command(exe, "--panel")
	cmd.Env = append(cmd.Environ(), "HTMLCLAY_PANEL_DATA="+dataDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}

// AllowForeground lets the child take the foreground for the tray click that
// reached this process.
func AllowForeground(pid int) { pAllowSetForegroundWindow.Call(uintptr(pid)) }

// CursorPos is the pointer in physical pixels. The host declares no DPI
// awareness, so a plain GetCursorPos answers in scaled pixels, which the
// child, aware of each monitor's DPI, would place wrongly.
func CursorPos() Point {
	old, _, _ := pSetThreadDpiContext.Call(dpiAwarePerMonitorV2)
	var p Point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if old != 0 {
		pSetThreadDpiContext.Call(old)
	}
	return p
}

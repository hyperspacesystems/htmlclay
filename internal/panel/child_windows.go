//go:build windows

package panel

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"github.com/wailsapp/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
)

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	shcore                 = windows.NewLazySystemDLL("shcore.dll")
	shell32                = windows.NewLazySystemDLL("shell32.dll")
	dwmapi                 = windows.NewLazySystemDLL("dwmapi.dll")
	pRegisterClassExW      = user32.NewProc("RegisterClassExW")
	pCreateWindowExW       = user32.NewProc("CreateWindowExW")
	pDefWindowProcW        = user32.NewProc("DefWindowProcW")
	pGetMessageW           = user32.NewProc("GetMessageW")
	pTranslateMessage      = user32.NewProc("TranslateMessage")
	pDispatchMessageW      = user32.NewProc("DispatchMessageW")
	pPostMessageW          = user32.NewProc("PostMessageW")
	pPostQuitMessage       = user32.NewProc("PostQuitMessage")
	pShowWindow            = user32.NewProc("ShowWindow")
	pSetWindowPos          = user32.NewProc("SetWindowPos")
	pSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	pSetTimer              = user32.NewProc("SetTimer")
	pMonitorFromPoint      = user32.NewProc("MonitorFromPoint")
	pGetMonitorInfoW       = user32.NewProc("GetMonitorInfoW")
	pFindWindowW           = user32.NewProc("FindWindowW")
	pFindWindowExW         = user32.NewProc("FindWindowExW")
	pGetWindowRect         = user32.NewProc("GetWindowRect")
	pSetProcessDpiContext  = user32.NewProc("SetProcessDpiAwarenessContext")
	pGetDpiForMonitor      = shcore.NewProc("GetDpiForMonitor")
	pSHAppBarMessage       = shell32.NewProc("SHAppBarMessage")
	pDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	dpiAwarePerMonitorV2 = ^uintptr(3)
	exitNoRuntime        = 3
	idleTimerID          = 1
	wsPopup              = 0x80000000
	wsExToolWindow       = 0x00000080
	wsExTopmost          = 0x00000008
	wmSize               = 0x0005
	wmActivate           = 0x0006
	wmSetFocus           = 0x0007
	wmKeyDown            = 0x0100
	wmTimer              = 0x0113
	wmDpiChanged         = 0x02E0
	wmCommandQueued      = 0x8000 + 1
	vkEscape             = 0x1B
	swHide               = 0
	swpNoZOrder          = 0x0004
	swpNoActivate        = 0x0010
	swpShowWindow        = 0x0040
	monitorNearest       = 2
	mdtEffectiveDPI      = 0
	abmGetTaskbarPos     = 0x00000005
	dwmwaCornerPref      = 33
	dwmwcpRound          = 2
)

type msg struct {
	Hwnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      Point
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type monitorInfo struct {
	Size    uint32
	Monitor Rect
	Work    Rect
	Flags   uint32
}

type appBarData struct {
	Size     uint32
	Hwnd     windows.HWND
	CallBack uint32
	Edge     uint32
	Rc       Rect
	LParam   uintptr
}

// window is the panel's popup window with WebView2 inside it.
type window struct {
	hwnd     windows.HWND
	chromium *edge.Chromium
	embedded bool
	child    *Child

	mu    sync.Mutex
	queue []Command
}

var (
	running *window
	sayMu   sync.Mutex
)

func say(e Event) {
	line, _ := json.Marshal(e)
	sayMu.Lock()
	defer sayMu.Unlock()
	fmt.Fprintln(os.Stdout, string(line))
}

func quit(detail string) int {
	say(Event{Event: EventExit, Detail: detail})
	return 1
}

// RunChild is htmlclay.exe --panel: it shows the dashboard's /panel page in a
// popup window that the host's commands on stdin show and hide, and returns
// the exit status. Every way it ends early says why on stdout first.
func RunChild() int {
	runtime.LockOSThread()
	if version, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString(""); err != nil || version == "" {
		say(Event{Event: EventNoRuntime})
		return exitNoRuntime
	}
	pSetProcessDpiContext.Call(dpiAwarePerMonitorV2)
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != windows.Errno(1) {
		return quit(fmt.Sprintf("CoInitializeEx: %v", err))
	}
	class, _ := windows.UTF16PtrFromString("HTMLClayPanel")
	var inst windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &inst); err != nil {
		return quit(fmt.Sprintf("GetModuleHandleEx: %v", err))
	}
	wc := wndClassEx{WndProc: syscall.NewCallback(wndProc), Instance: inst, ClassName: class}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return quit(fmt.Sprintf("RegisterClassExW: %v", err))
	}
	h, _, err := pCreateWindowExW.Call(wsExToolWindow|wsExTopmost, uintptr(unsafe.Pointer(class)), 0, wsPopup,
		0, 0, PanelWidth, DefaultHeight, 0, 0, uintptr(inst), 0)
	if h == 0 {
		return quit(fmt.Sprintf("CreateWindowExW: %v", err))
	}
	w := &window{hwnd: windows.HWND(h)}
	w.child = NewChild(w, say, time.Now)
	running = w
	corner := uint32(dwmwcpRound)
	pDwmSetWindowAttribute.Call(h, dwmwaCornerPref, uintptr(unsafe.Pointer(&corner)), unsafe.Sizeof(corner))

	w.chromium = edge.NewChromium()
	w.chromium.DataPath = os.Getenv("HTMLCLAY_PANEL_DATA")
	w.chromium.SetErrorCallback(func(err error) { say(Event{Event: EventExit, Detail: fmt.Sprintf("WebView2: %v", err)}) })
	w.chromium.MessageCallback = func(message string, _ *edge.ICoreWebView2, _ *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		w.child.Page(message)
	}
	if !w.chromium.Embed(h) {
		return quit("WebView2 did not start")
	}
	w.embedded = true
	// go-webview2 turns off WebView2's own monitor scale tracking, which
	// leaves the page at the primary monitor's scale on a second monitor with
	// another; the panel moves between monitors, so it is turned back on.
	if c := w.chromium.GetController(); c != nil {
		if c3 := c.GetICoreWebView2Controller3(); c3 != nil {
			_ = c3.PutShouldDetectMonitorScaleChanges(true)
		}
	}
	if settings, err := w.chromium.GetSettings(); err == nil {
		settings.PutAreDevToolsEnabled(false)
		settings.PutAreDefaultContextMenusEnabled(false)
		settings.PutIsStatusBarEnabled(false)
		settings.PutIsZoomControlEnabled(false)
		settings.PutAreBrowserAcceleratorKeysEnabled(false)
	}
	pSetTimer.Call(h, idleTimerID, uintptr(time.Minute/time.Millisecond), 0)
	say(Event{Event: EventReady})

	go w.readCommands()

	var m msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return 0
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// readCommands hands each line from the host to the window's thread. The host
// closing stdin, or ending, ends the child.
func (w *window) readCommands() {
	lines := bufio.NewScanner(os.Stdin)
	for lines.Scan() {
		var c Command
		if json.Unmarshal(lines.Bytes(), &c) != nil {
			continue
		}
		w.mu.Lock()
		w.queue = append(w.queue, c)
		w.mu.Unlock()
		pPostMessageW.Call(uintptr(w.hwnd), wmCommandQueued, 0, 0)
	}
	os.Exit(0)
}

func (w *window) runQueued() {
	w.mu.Lock()
	queue := w.queue
	w.queue = nil
	w.mu.Unlock()
	for _, c := range queue {
		w.child.Command(c)
	}
}

func (w *window) ScreenAt(p Point) Screen {
	mon, _, _ := pMonitorFromPoint.Call(uintptr(*(*uint64)(unsafe.Pointer(&p))), monitorNearest)
	mi := monitorInfo{}
	mi.Size = uint32(unsafe.Sizeof(mi))
	pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
	var dpi, dpiY uint32
	if r, _, _ := pGetDpiForMonitor.Call(mon, mdtEffectiveDPI, uintptr(unsafe.Pointer(&dpi)), uintptr(unsafe.Pointer(&dpiY))); r != 0 || dpi == 0 {
		dpi = 96
	}
	return Screen{Monitor: mi.Monitor, Work: mi.Work, DPI: dpi, Taskbar: taskbarEdge(), Tray: notificationArea()}
}

func (w *window) Show(r Rect) {
	pSetWindowPos.Call(uintptr(w.hwnd), 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), swpNoZOrder|swpShowWindow)
	_ = w.chromium.NotifyParentWindowPositionChanged()
	_ = w.chromium.Show()
	pSetForegroundWindow.Call(uintptr(w.hwnd))
	w.chromium.Resize()
	w.chromium.Focus()
}

func (w *window) Move(r Rect) {
	pSetWindowPos.Call(uintptr(w.hwnd), 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), swpNoZOrder|swpNoActivate)
	_ = w.chromium.NotifyParentWindowPositionChanged()
	w.chromium.Resize()
}

func (w *window) Hide() {
	_ = w.chromium.Hide()
	pShowWindow.Call(uintptr(w.hwnd), swHide)
}

func (w *window) Navigate(url string) { w.chromium.Navigate(url) }

// AllowForeground lets any process take the foreground next. This process
// owns the foreground window and has the person's click, so Windows lets it
// pass that on; the app's own launch does the same (singleinstance_windows.go).
func (w *window) AllowForeground() { pAllowSetForegroundWindow.Call(asfwAny) }

// asfwAny is ASFW_ANY, (DWORD)-1: any process may take the foreground.
const asfwAny = uintptr(0xFFFFFFFF)

// taskbarEdge is the main taskbar's side as Windows reports it, for a monitor
// whose taskbar hides itself.
func taskbarEdge() Edge {
	abd := appBarData{}
	abd.Size = uint32(unsafe.Sizeof(abd))
	pSHAppBarMessage.Call(abmGetTaskbarPos, uintptr(unsafe.Pointer(&abd)))
	return map[uint32]Edge{0: EdgeLeft, 1: EdgeTop, 2: EdgeRight, 3: EdgeBottom}[abd.Edge]
}

// notificationArea is the rectangle of the icons beside the clock, or an empty
// one when the taskbar does not have it.
func notificationArea() Rect {
	trayClass, _ := windows.UTF16PtrFromString("Shell_TrayWnd")
	notifyClass, _ := windows.UTF16PtrFromString("TrayNotifyWnd")
	tray, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(trayClass)), 0)
	if tray == 0 {
		return Rect{}
	}
	notify, _, _ := pFindWindowExW.Call(tray, 0, uintptr(unsafe.Pointer(notifyClass)), 0)
	if notify == 0 {
		return Rect{}
	}
	var r Rect
	if ok, _, _ := pGetWindowRect.Call(notify, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return Rect{}
	}
	return r
}

func wndProc(hwnd windows.HWND, message uint32, wParam, lParam uintptr) uintptr {
	w := running
	if w == nil || hwnd != w.hwnd {
		r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
		return r
	}
	switch message {
	case wmCommandQueued:
		w.runQueued()
		return 0
	case wmActivate:
		if wParam&0xffff == 0 {
			w.child.Deactivated()
		}
		return 0
	case wmSetFocus:
		if w.embedded {
			w.chromium.Focus()
		}
		return 0
	case wmKeyDown:
		if wParam == vkEscape {
			w.child.Page(PageClose)
			return 0
		}
	case wmSize:
		if w.embedded {
			w.chromium.Resize()
		}
		return 0
	case wmTimer:
		if wParam == idleTimerID && w.child.Tick() {
			pPostQuitMessage.Call(0)
		}
		return 0
	case wmDpiChanged:
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return r
}

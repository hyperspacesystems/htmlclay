// Package panel runs the Windows panel: a small window beside the tray icon
// that shows the dashboard's /panel page. The window lives in a child process
// (htmlclay.exe --panel), because the WebView2 binding ends its whole process
// on any WebView2 error, and inside the host that would stop every document's
// saving. The host and the child speak JSON lines: commands on the child's
// stdin, events on its stdout. The child exits when its stdin closes.
package panel

// Command is one line the host writes to the child.
type Command struct {
	Cmd string `json:"cmd"`
	X   int    `json:"x,omitempty"`
	Y   int    `json:"y,omitempty"`
	URL string `json:"url,omitempty"`
}

// Event is one line the child writes to the host. Detail says why, for an
// exit, and what, for a page message the child does not handle itself.
type Event struct {
	Event  string `json:"event"`
	Detail string `json:"detail,omitempty"`
}

const (
	CmdNavigate = "navigate"
	CmdToggle   = "toggle"
	CmdHide     = "hide"

	EventReady     = "ready"
	EventHidden    = "hidden"
	EventReenter   = "reenter"
	EventNoRuntime = "no-runtime"
	EventExit      = "exit"
)

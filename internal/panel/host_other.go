//go:build !windows

package panel

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// RuntimeInstalled is false: the panel is Windows only, and the tray's left
// click shows the menu everywhere else.
func RuntimeInstalled() bool { return false }

// ChildCommand starts exe as the panel's child.
func ChildCommand(exe, dataDir string) *exec.Cmd {
	cmd := exec.Command(exe, "--panel")
	cmd.Env = append(cmd.Environ(), "HTMLCLAY_PANEL_DATA="+dataDir)
	return cmd
}

// AllowForeground does nothing outside Windows.
func AllowForeground(int) {}

// CursorPos is not read outside Windows.
func CursorPos() Point { return Point{} }

// RunChild ends at once outside Windows, saying why.
func RunChild() int {
	line, _ := json.Marshal(Event{Event: EventExit, Detail: "the panel runs only on Windows"})
	fmt.Fprintln(os.Stdout, string(line))
	return 1
}

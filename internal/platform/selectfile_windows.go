//go:build windows

package platform

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func selectFile(prompt, dir string) (string, bool, error) {
	script := selectFileScript()

	ctx, cancel := context.WithTimeout(context.Background(), selectFileTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	noConsoleWindow(cmd)
	cmd.Env = append(cmd.Environ(), "HTMLCLAY_DIALOG_PROMPT="+prompt, "HTMLCLAY_DIALOG_DIR="+dir)
	out, err := cmd.Output()
	if err != nil {
		return "", false, fmt.Errorf("file picker failed: %w", err)
	}
	path := strings.Trim(string(out), "\r\n")
	if path == "" {
		return "", false, nil
	}
	return path, true, nil
}

// selectFileScript builds the picker. An OpenFileDialog has no Shown event to
// bring it forward, and without one it opens behind the browser whenever HTML
// Clay is not the foreground process, with no taskbar button to find it by. So
// it is owned by an invisible window in the middle of the screen that is
// brought to the foreground first, the way the other dialogs are.
//
// The filter is a convenience only. What may actually be registered is decided
// once, in prepareHelperProgram, beside the Unix executable-bit check, so the
// two platforms answer "can this file be run" in one place.
func selectFileScript() string {
	return "$ErrorActionPreference = 'Stop'; " +
		"Add-Type -AssemblyName System.Windows.Forms; " +
		frontType +
		"$f = New-Object System.Windows.Forms.Form; " +
		"$f.ShowInTaskbar = $false; $f.FormBorderStyle = 'None'; $f.Opacity = 0; $f.TopMost = $true; " +
		"$f.StartPosition = 'Manual'; $f.Size = New-Object System.Drawing.Size(1, 1); " +
		"$a = [System.Windows.Forms.Screen]::PrimaryScreen.WorkingArea; " +
		"$f.Location = New-Object System.Drawing.Point(($a.Left + $a.Width / 2), ($a.Top + $a.Height / 3)); " +
		"$f.Show(); " +
		"if ('HtmlClayUI.Front' -as [type]) { [HtmlClayUI.Front]::Window($f.Handle) }; $f.Activate(); " +
		"$d = New-Object System.Windows.Forms.OpenFileDialog; " +
		"$d.Title = $env:HTMLCLAY_DIALOG_PROMPT; " +
		"if ($env:HTMLCLAY_DIALOG_DIR) { $d.InitialDirectory = $env:HTMLCLAY_DIALOG_DIR }; " +
		"$d.CheckFileExists = $true; $d.Multiselect = $false; " +
		"$d.Filter = 'Helper programs (*.exe;*.bat;*.cmd)|*.exe;*.bat;*.cmd|All files (*.*)|*.*'; " +
		"$r = $d.ShowDialog($f); $f.Close(); " +
		"if ($r -eq 'OK') { Write-Output $d.FileName }"
}

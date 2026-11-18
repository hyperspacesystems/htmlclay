//go:build windows

package platform

import (
	"os/exec"
	"strings"
	"syscall"
)

const chooseFileScript = `Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Title = $env:HTMLCLAY_CHOOSER_TITLE
$d.Filter = 'HTML Clay documents (*.htmlclay;*.html;*.htm)|*.htmlclay;*.html;*.htm'
[Console]::OutputEncoding = [Text.Encoding]::UTF8
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $d.FileName }`

func chooseFile(title string) (string, bool, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", chooseFileScript)
	cmd.Env = append(cmd.Environ(), "HTMLCLAY_CHOOSER_TITLE="+title)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return "", false, err
	}
	path := strings.TrimSpace(string(out))
	return path, path != "", nil
}

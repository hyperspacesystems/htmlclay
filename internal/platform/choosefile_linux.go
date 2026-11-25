//go:build linux

package platform

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

func chooseFile(title string) (string, bool, error) {
	if bin, err := exec.LookPath("zenity"); err == nil {
		return runChooser(exec.Command(bin, zenityFileArgs(title)...))
	}
	if bin, err := exec.LookPath("kdialog"); err == nil {
		home, _ := os.UserHomeDir()
		return runChooser(exec.Command(bin, "--title", title, "--getopenfilename", home, "*.htmlclay *.html *.htm|HTML Clay documents"))
	}
	return "", false, errors.New("no native dialog tool (zenity or kdialog) found")
}

func zenityFileArgs(title string) []string {
	return []string{"--file-selection", "--title=" + title, "--file-filter=HTML Clay documents | *.htmlclay *.html *.htm"}
}

// runChooser runs a chooser that prints the path and exits 0, or exits 1 when
// cancelled.
func runChooser(cmd *exec.Cmd) (string, bool, error) {
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	path := strings.TrimRight(string(out), "\r\n")
	return path, path != "", nil
}

package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const desktopID = "htmlclay.desktop"

var mimeLine = regexp.MustCompile(`(?m)^MimeType=.*$`)

func readFileTypes() FileTypeState {
	state := FileTypeState{Default: "unknown"}
	if out, err := exec.Command("xdg-mime", "query", "default", "application/x-htmlclay").Output(); err == nil {
		if strings.TrimSpace(string(out)) == desktopID {
			state.Default = "htmlclay"
		} else {
			state.Default = "other"
		}
	}
	if data, err := os.ReadFile(desktopFilePath()); err == nil {
		state.HTMLOpenWith = strings.Contains(string(mimeLine.Find(data)), "text/html")
	}
	return state
}

func makeDefault(string) error {
	return exec.Command("xdg-mime", "default", desktopID, "application/x-htmlclay").Run()
}

func stopOpening() error {
	out, err := exec.Command("xdg-settings", "get", "default-web-browser").Output()
	if err != nil {
		return err
	}
	browser := strings.TrimSpace(string(out))
	if browser == "" || browser == desktopID {
		return os.ErrNotExist
	}
	return exec.Command("xdg-mime", "default", browser, "application/x-htmlclay").Run()
}

func setHTMLOpenWith(_ string, on bool) error {
	path := desktopFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	line := "MimeType=application/x-htmlclay;"
	if on {
		line += "text/html;"
	}
	updated := data
	if mimeLine.Match(data) {
		updated = mimeLine.ReplaceAll(data, []byte(line))
	} else {
		updated = append(append(data, '\n'), line+"\n"...)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		return err
	}
	_ = exec.Command("update-desktop-database", filepath.Dir(path)).Run()
	return nil
}

func desktopFilePath() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "applications", desktopID)
}

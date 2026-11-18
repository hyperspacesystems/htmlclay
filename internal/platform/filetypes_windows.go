package platform

import (
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const userChoicePath = `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.htmlclay\UserChoice`

func readFileTypes() FileTypeState {
	state := FileTypeState{Default: "other"}
	if k, err := registry.OpenKey(registry.CURRENT_USER, userChoicePath, registry.QUERY_VALUE); err == nil {
		id, _, err := k.GetStringValue("ProgId")
		k.Close()
		if err == nil && strings.EqualFold(id, progID) {
			state.Default = "htmlclay"
		}
	} else if classDefaultIs(`.htmlclay`, progID) {
		state.Default = "htmlclay"
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, classesPath+`\.html\OpenWithProgids`, registry.QUERY_VALUE); err == nil {
		_, _, err := k.GetValue(progID, nil)
		k.Close()
		state.HTMLOpenWith = err == nil
	}
	return state
}

func makeDefault(exePath, example string) error {
	if err := registerFileTypes(exePath, FileTypeOptions{HTMLClay: true, HTMLOpenWith: readFileTypes().HTMLOpenWith}); err != nil {
		return err
	}
	cmd := exec.Command("rundll32.exe", "shell32.dll,OpenAs_RunDLL", example)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

func stopOpening() error {
	var firstErr error
	if classDefaultIs(`.htmlclay`, progID) {
		firstErr = deleteClassTree(`.htmlclay`)
	}
	notifyAssociationsChanged()
	return firstErr
}

func setHTMLOpenWith(exePath string, on bool) error {
	for _, ext := range []string{`.html`, `.htm`} {
		var err error
		if on {
			_, err = addOpenWithProgID(ext)
		} else {
			err = deleteOpenWithProgID(ext)
		}
		if err != nil {
			return err
		}
	}
	notifyAssociationsChanged()
	return nil
}

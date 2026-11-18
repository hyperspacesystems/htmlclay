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

// stopOpening removes HTML Clay as the opener of .htmlclay files: the choice a
// person made in Windows' "open with" chooser (UserChoice, which wins over the
// class default) when it names HTML Clay, and the class default it registered.
func stopOpening() error {
	var firstErr error
	if k, err := registry.OpenKey(registry.CURRENT_USER, userChoicePath, registry.QUERY_VALUE); err == nil {
		id, _, _ := k.GetStringValue("ProgId")
		k.Close()
		if strings.EqualFold(id, progID) {
			firstErr = registry.DeleteKey(registry.CURRENT_USER, userChoicePath)
		}
	}
	if classDefaultIs(`.htmlclay`, progID) {
		if err := deleteClassTree(`.htmlclay`); err != nil && firstErr == nil {
			firstErr = err
		}
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

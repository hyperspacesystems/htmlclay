package platform

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

func readFileTypes() FileTypeState {
	state := FileTypeState{Default: "other"}
	if strings.EqualFold(effectiveOpener(), progID) {
		state.Default = "htmlclay"
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, classesPath+`\.html\OpenWithProgids`, registry.QUERY_VALUE); err == nil {
		_, _, err := k.GetValue(progID, nil)
		k.Close()
		state.HTMLOpenWith = err == nil
	}
	return state
}

func makeDefault(exePath string) error {
	return runMakeDefault(windowsDefaultApp{exePath: exePath})
}

// stopOpening removes HTML Clay as the opener of .htmlclay files: each choice
// the person made in Windows (UserChoiceLatest and UserChoice, which win over
// the class default) that names HTML Clay, and the class default it registered.
// A choice naming another app is that app's, and stays.
func stopOpening() error {
	var firstErr error
	for _, path := range htmlclayChoiceKeys {
		if id, ok := choiceProgID(path); ok && strings.EqualFold(id, progID) {
			if err := deleteTree(registry.CURRENT_USER, path); err != nil && firstErr == nil {
				firstErr = err
			}
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

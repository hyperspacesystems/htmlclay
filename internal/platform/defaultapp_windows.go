package platform

import (
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var procAssocQueryStringW = windows.NewLazySystemDLL("shlwapi.dll").NewProc("AssocQueryStringW")

const (
	assocfNoTruncate = 0x00000020
	assocstrProgID   = 20
	swShowNormal     = 1
)

// effectiveOpener is the ProgID Windows itself opens .htmlclay files with,
// whatever keys and hash checks the running build reads, or "" when none.
func effectiveOpener() string {
	ext, err := windows.UTF16PtrFromString(".htmlclay")
	if err != nil {
		return ""
	}
	buf := make([]uint16, 260)
	n := uint32(len(buf))
	hr, _, _ := procAssocQueryStringW.Call(assocfNoTruncate, assocstrProgID, uintptr(unsafe.Pointer(ext)), 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if hr != 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// choiceProgID reads the ProgID a stored choice names: UserChoice keeps it as a
// value, UserChoiceLatest in a ProgId subkey.
func choiceProgID(path string) (string, bool) {
	for _, p := range []string{path, path + `\ProgId`} {
		k, err := registry.OpenKey(registry.CURRENT_USER, p, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		id, _, err := k.GetStringValue("ProgId")
		k.Close()
		if err == nil {
			return id, true
		}
	}
	return "", false
}

func keyExists(path string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

type windowsDefaultApp struct{ exePath string }

func (w windowsDefaultApp) effectiveProgID() string { return effectiveOpener() }

func (w windowsDefaultApp) hasChoice() bool {
	for _, path := range htmlclayChoiceKeys {
		if keyExists(path) {
			return true
		}
	}
	return false
}

func (w windowsDefaultApp) writeClassDefault() error {
	err := registerFileTypes(w.exePath, FileTypeOptions{HTMLClay: true, HTMLOpenWith: readFileTypes().HTMLOpenWith})
	notifyAssociationsChanged()
	return err
}

func (w windowsDefaultApp) deleteChoice() error {
	var firstErr error
	for _, path := range htmlclayChoiceKeys {
		if err := deleteTree(registry.CURRENT_USER, path); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (w windowsDefaultApp) openSettings() error {
	verb, _ := windows.UTF16PtrFromString("open")
	uri, err := windows.UTF16PtrFromString(defaultAppsSettingsURI)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, uri, nil, nil, swShowNormal)
}

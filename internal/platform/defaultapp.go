package platform

import (
	"errors"
	"strings"
)

// ErrFinishInSettings is Make default on Windows ending in the system's own
// Settings page: HTML Clay cleared another app's choice for .htmlclay and
// Windows still names that app, so only the person can finish it there.
var ErrFinishInSettings = errors.New("finish choosing HTML Clay in Windows Settings")

// progID is the class HTML Clay registers for .htmlclay documents.
const progID = "HTMLClay.Document"

// fileExtsPath is where Explorer keeps the person's choice of opener for one
// extension, under HKEY_CURRENT_USER.
const fileExtsPath = `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\`

// htmlclayChoiceKeys is every key Make default may delete: the person's choice
// of opener for .htmlclay, in the two places Windows keeps it. Nothing for
// .html or .htm is ever deleted.
var htmlclayChoiceKeys = []string{
	fileExtsPath + `.htmlclay\UserChoiceLatest`,
	fileExtsPath + `.htmlclay\UserChoice`,
}

// defaultAppsSettingsURI opens Windows Settings at HTML Clay's own default apps
// page; registeredAppUser is the name under HKCU\Software\RegisteredApplications.
const defaultAppsSettingsURI = "ms-settings:defaultapps?registeredAppUser=HTML%20Clay"

type defaultStep int

const (
	stepAlreadyDefault defaultStep = iota
	stepRegister
	stepClearChoice
	stepOpenSettings
)

func (s defaultStep) String() string {
	return [...]string{"alreadyDefault", "register", "clearChoice", "openSettings"}[s]
}

// nextDefaultStep is what Make default does next, given the ProgID Windows
// itself would open .htmlclay with, whether the person's choice is stored, and
// whether this click already acted once (wrote the class default, or cleared a
// choice). It acts at most once: a second look that still names another app
// sends the person to Settings.
func nextDefaultStep(effectiveProgID string, hasChoice, acted bool) defaultStep {
	switch {
	case strings.EqualFold(effectiveProgID, progID):
		return stepAlreadyDefault
	case acted:
		return stepOpenSettings
	case hasChoice:
		return stepClearChoice
	}
	return stepRegister
}

// defaultAppSystem is what Make default reads and changes on Windows.
type defaultAppSystem interface {
	effectiveProgID() string
	hasChoice() bool
	writeClassDefault() error
	deleteChoice() error
	openSettings() error
}

// runMakeDefault makes HTML Clay the opener of .htmlclay files where Windows
// lets a program do that, deleting another app's stored choice at most once,
// and answers ErrFinishInSettings when only the person can finish it.
func runMakeDefault(sys defaultAppSystem) error {
	acted := false
	for {
		switch nextDefaultStep(sys.effectiveProgID(), sys.hasChoice(), acted) {
		case stepAlreadyDefault:
			return nil
		case stepRegister:
			if err := sys.writeClassDefault(); err != nil {
				return err
			}
		case stepClearChoice:
			// Windows may refuse the delete (an antivirus, or the user choice
			// protection driver). The class default is written either way, so
			// Settings lists HTML Clay, and the next look sends the person
			// there if the other app still wins.
			_ = sys.deleteChoice()
			if err := sys.writeClassDefault(); err != nil {
				return err
			}
		case stepOpenSettings:
			if err := sys.openSettings(); err != nil {
				return err
			}
			return ErrFinishInSettings
		}
		acted = true
	}
}

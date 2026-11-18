package main

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/panphora/htmlclay/internal/aiedit"
	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/builtin"
	"github.com/panphora/htmlclay/internal/platform"
	"github.com/panphora/htmlclay/internal/tray"
)

// The settings rows' action names: "<kind>:<arg>", where the kind is the row
// and the arg is the program, tool or switch it names.
const (
	settingSetOn       = "setOn"
	settingExample     = "example"
	settingAIEditing   = "aiEditing"
	settingTool        = "tool"
	settingModel       = "model"
	settingCustom      = "custom"
	settingMakeDefault = "makeDefault"
	settingOpenWith    = "openWith"
	settingStopOpening = "stopOpening"
	settingPort        = "port"
	settingAppLog      = "log"
	settingSyncLog     = "syncLog"
)

// traySettingItems is one settings submenu's rows, from the snapshot.
func (a *app) traySettingItems(group string) []tray.SettingItem {
	s := a.snapshot()
	switch group {
	case "builtin":
		return builtinSettingRows(s)
	case "ai":
		return aiSettingRows(s)
	case "files":
		return fileTypeSettingRows(s)
	case "advanced":
		return advancedSettingRows(s)
	}
	return nil
}

// builtinSettingRows is one checked row per built-in program, each followed by
// its example's row when it is on.
func builtinSettingRows(s appcore.Snapshot) []tray.SettingItem {
	var rows []tray.SettingItem
	for _, p := range s.Programs {
		if !p.Builtin || p.ID == appcore.AIEditProgram {
			continue
		}
		on := p.On
		rows = append(rows, tray.SettingItem{Action: settingSetOn + ":" + p.ID, Title: builtin.Title(p.ID), Enabled: true, Checked: &on})
		if on {
			rows = append(rows, tray.SettingItem{Action: settingExample + ":" + p.ID, Title: "Open example", Enabled: true})
		}
	}
	return rows
}

// aiSettingRows is the AI editing switch, the tool it uses, and the rows that
// change the tool's model or set up a custom command.
func aiSettingRows(s appcore.Snapshot) []tray.SettingItem {
	on := s.Preferences.AIEditing
	rows := []tray.SettingItem{{Action: settingAIEditing, Title: "AI editing", Enabled: true, Checked: &on}}
	if !on {
		return rows
	}
	found := 0
	for _, tool := range s.Preferences.AIEditTools {
		if !tool.Found {
			continue
		}
		found++
		chosen := tool.ID == s.Preferences.AIEditTool
		rows = append(rows, tray.SettingItem{Action: settingTool + ":" + tool.ID, Title: aiToolRowTitle(tool), Enabled: true, Checked: &chosen})
	}
	if found == 0 {
		rows = append(rows, tray.SettingItem{Title: appcore.ForSystem("Claude and Codex were not found on this Mac.", runtime.GOOS)})
	}
	for _, tool := range s.Preferences.AIEditTools {
		if tool.Found && !tool.Custom && tool.ID == s.Preferences.AIEditTool {
			rows = append(rows, tray.SettingItem{Action: settingModel + ":" + tool.ID, Title: "Change model for " + aiToolTitle(tool.ID) + "…", Enabled: true})
		}
	}
	rows = append(rows, tray.SettingItem{Action: settingCustom, Title: "Custom command…", Enabled: true})
	return rows
}

// aiToolTitle is the name a person sees for an AI tool.
func aiToolTitle(id string) string {
	switch id {
	case "claude":
		return "Claude"
	case "codex":
		return "Codex"
	}
	return id
}

// aiToolRowTitle names a tool, with the model it will use when one is set.
func aiToolRowTitle(tool appcore.AIEditTool) string {
	title := aiToolTitle(tool.ID)
	switch {
	case tool.Model != "":
		return title + " (" + tool.Model + ")"
	case tool.DefaultModel != "":
		return title + " (" + tool.DefaultModel + ")"
	}
	return title
}

// fileTypeSettingRows is where .htmlclay files open, whether HTML Clay appears
// in Open with for .html files, and how to stop opening them.
func fileTypeSettingRows(s appcore.Snapshot) []tray.SettingItem {
	isDefault := s.FileTypes.Default == appcore.DefaultAppHTMLClay
	status := "Another app opens .htmlclay files"
	if isDefault {
		status = "HTML Clay opens .htmlclay files"
	}
	rows := []tray.SettingItem{{Title: status}}
	if !isDefault {
		rows = append(rows, tray.SettingItem{Action: settingMakeDefault, Title: "Make HTML Clay the default", Enabled: true})
	}
	openWith := s.FileTypes.HTMLOpenWith
	rows = append(rows, tray.SettingItem{Action: settingOpenWith, Title: openWithRowTitle(), Enabled: true, Checked: &openWith})
	if isDefault {
		rows = append(rows, tray.SettingItem{Action: settingStopOpening, Title: "Stop opening .htmlclay files…", Enabled: true})
	}
	return rows
}

// openWithRowTitle names the .html Open with row in the system's own words.
func openWithRowTitle() string {
	if runtime.GOOS == "linux" {
		return "Show HTML Clay in Open With for .html files"
	}
	return "Show HTML Clay in Open with for .html files"
}

// advancedSettingRows is the port, the app log, and the sync log once sync has
// been on.
func advancedSettingRows(s appcore.Snapshot) []tray.SettingItem {
	rows := []tray.SettingItem{
		{Action: settingPort, Title: "Change port…", Enabled: true},
		{Action: settingAppLog, Title: "Show app log", Enabled: true},
	}
	if s.Sync.EverOn {
		rows = append(rows, tray.SettingItem{Action: settingSyncLog, Title: "Show sync log", Enabled: true})
	}
	return rows
}

// portChangeBody is the Change port dialog's message: the current port, filled
// in, in the system's own words.
const portChangeBody = "Every HTML Clay address on this Mac will use the new port, so you will need to update your bookmarks. Anything a page saved in your browser stays with the old address and starts fresh at the new one. Visits to port %d will keep forwarding to the new port."

const portOutOfRange = "Enter a port number between 1024 and 65535."

// doTraySetting runs a clicked row of a settings submenu.
func (a *app) doTraySetting(action string) {
	kind, arg, _ := strings.Cut(action, ":")
	s := a.snapshot()
	var refusal *appcore.Refusal
	switch kind {
	case settingSetOn:
		on := false
		for _, p := range s.Programs {
			if p.ID == arg && p.Builtin {
				on = !p.On
			}
		}
		refusal = a.trayDo("program.setOn", map[string]any{"programID": arg, "on": on})
	case settingExample:
		refusal = a.trayDo("program.openExample", map[string]any{"programID": arg})
	case settingAIEditing:
		refusal = a.trayDo("prefs.set", map[string]any{"key": "aiEditing", "value": !s.Preferences.AIEditing})
	case settingTool:
		refusal = a.trayDo("prefs.set", map[string]any{"key": "aiEditTool", "value": arg})
	case settingModel:
		a.askToolModel(s, arg)
		return
	case settingCustom:
		a.askCustomCommand(s)
		return
	case settingMakeDefault:
		refusal = a.trayDo("fileTypes.makeDefault", map[string]any{})
	case settingOpenWith:
		refusal = a.trayDo("fileTypes.setHTMLOpenWith", map[string]any{"on": !s.FileTypes.HTMLOpenWith})
	case settingStopOpening:
		ok, err := platform.ConfirmChange("Stop opening .htmlclay files with HTML Clay?", appcore.ForSystem("Double-clicking a .htmlclay file in Finder will no longer open it in HTML Clay. Your files do not change, and you can make HTML Clay the default again here anytime.", runtime.GOOS), "Stop opening")
		if err != nil {
			a.rt.logger.Printf("Could not ask about stopping opening .htmlclay files: %v", err)
			return
		}
		if !ok {
			return
		}
		refusal = a.trayDo("fileTypes.stopOpening", map[string]any{})
	case settingPort:
		a.askPort(s)
		return
	case settingAppLog:
		refusal = a.trayDo("app.revealLog", map[string]any{})
	case settingSyncLog:
		refusal = a.trayDo("sync.revealLog", map[string]any{})
	}
	a.reportTrayRefusal(refusal)
}

// askPort asks for the new port and sends it, saying what a number that is not
// a port looks like.
func (a *app) askPort(s appcore.Snapshot) {
	message := appcore.ForSystem(fmt.Sprintf(portChangeBody, s.Host.Port), runtime.GOOS)
	value, ok, err := platform.PromptName("Change HTML Clay's port?", message, strconv.Itoa(s.Host.Port))
	if err != nil {
		a.rt.logger.Printf("Could not ask for a port: %v", err)
		return
	}
	if !ok {
		return
	}
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1024 || port > 65535 {
		_ = a.notifyUser(portOutOfRange, "")
		return
	}
	a.reportTrayRefusal(a.trayDo("host.setPort", map[string]any{"port": port}))
}

// askToolModel asks for the model the chosen tool should use.
func (a *app) askToolModel(s appcore.Snapshot, id string) {
	for _, tool := range s.Preferences.AIEditTools {
		if tool.ID != id || tool.Custom {
			continue
		}
		initial := tool.Model
		if initial == "" {
			initial = tool.DefaultModel
		}
		value, ok, err := platform.PromptName("Change model for "+aiToolTitle(tool.ID), "", initial)
		if err != nil {
			a.rt.logger.Printf("Could not ask for a model: %v", err)
			return
		}
		if !ok {
			return
		}
		a.reportTrayRefusal(a.trayDo("aiEdit.setModel", map[string]any{"tool": tool.ID, "model": strings.TrimSpace(value)}))
		return
	}
}

// askCustomCommand asks for the command line a custom AI tool runs.
func (a *app) askCustomCommand(s appcore.Snapshot) {
	initial := ""
	for _, tool := range s.Preferences.AIEditTools {
		if tool.Custom && tool.ID == aiedit.CustomEngine {
			initial = tool.Command
		}
	}
	value, ok, err := platform.PromptName("Custom command", "", initial)
	if err != nil {
		a.rt.logger.Printf("Could not ask for a custom command: %v", err)
		return
	}
	if !ok {
		return
	}
	a.reportTrayRefusal(a.trayDo("aiEdit.setCustom", map[string]any{"command": value}))
}

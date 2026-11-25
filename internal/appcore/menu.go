package appcore

import "fmt"

// Menu row ids. Go (MenuRows), Swift (MenuModel.rows) and the shared fixture
// testdata/v1/menu.json use the same ids.
const (
	MenuSeparator     = "-"
	MenuStatus        = "status"
	MenuSyncLine      = "syncLine"
	MenuPauseSync     = "pauseSync"
	MenuNeedsYou      = "needsYou"
	MenuOpenDashboard = "openDashboard"
	MenuOpenFile      = "openFile"
	MenuRecent        = "recent"
	MenuNoRecent      = "noRecent"
	MenuRecovered     = "recoveredChanges"
	MenuSettings      = "settings"
	MenuCheckUpdates  = "checkForUpdates"
	MenuQuit          = "quit"
)

// Status row kinds.
const (
	MenuStatusVersion     = "version"
	MenuStatusChecking    = "checking"
	MenuStatusDownloading = "downloading"
	MenuStatusReady       = "ready"
)

const menuListCap = 8

// UpdatePhase is the update check as the shell drawing the menu knows it: the
// Mac app reads Sparkle, Windows and Linux read the snapshot's update.
type UpdatePhase struct {
	Checking    bool   `json:"checking"`
	Ready       string `json:"ready"`
	Downloading string `json:"downloading,omitempty"`
}

// PhaseOf is the update phase the snapshot itself holds (Windows and Linux).
func PhaseOf(s Snapshot) UpdatePhase {
	ready := ""
	if s.Update.Ready != nil {
		ready = *s.Update.Ready
	}
	return UpdatePhase{Checking: s.Update.Checking, Ready: ready, Downloading: s.Update.Downloading}
}

// MenuRow is one row of the one menu. A Recent or Needs you child's ID is the
// document's or the item's id. Kind is set on the status row only.
type MenuRow struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind,omitempty"`
	Enabled  bool      `json:"enabled"`
	Children []MenuRow `json:"children,omitempty"`
}

// MenuRows is the one menu, top to bottom: the status row (the version, checking,
// or restart to update), the sync line with Pause, Needs you, then Open
// dashboard, Open a file…, Recent documents (always there; an empty one holds a
// disabled No recent documents row), Recovered changes…, Settings…, Check for
// updates… and Quit. The sync rows, Needs you and Recovered changes are left out
// when they have nothing to show, and a separator never leads, trails or repeats.
func MenuRows(s Snapshot, phase UpdatePhase) []MenuRow {
	status := MenuRow{ID: MenuStatus, Kind: MenuStatusVersion}
	switch {
	case phase.Ready != "":
		status = MenuRow{ID: MenuStatus, Kind: MenuStatusReady, Enabled: true}
	case phase.Downloading != "":
		status.Kind = MenuStatusDownloading
	case phase.Checking:
		status.Kind = MenuStatusChecking
	}
	rows := []MenuRow{status, {ID: MenuSeparator}}
	if s.Sync.Connected {
		rows = append(rows, MenuRow{ID: MenuSyncLine})
		if s.Host.Can["sync.setPaused"] {
			rows = append(rows, MenuRow{ID: MenuPauseSync, Enabled: true})
		}
	}
	if s.Counts.Total > 0 && len(s.Waiting) > 0 {
		needs := MenuRow{ID: MenuNeedsYou, Enabled: true}
		for _, w := range s.Waiting[:min(len(s.Waiting), menuListCap)] {
			needs.Children = append(needs.Children, MenuRow{ID: w.ID, Enabled: true})
		}
		rows = append(rows, needs)
	}
	rows = append(rows, MenuRow{ID: MenuSeparator},
		MenuRow{ID: MenuOpenDashboard, Enabled: true},
		MenuRow{ID: MenuOpenFile, Enabled: true})
	recent := MenuRow{ID: MenuRecent, Enabled: true}
	for _, d := range s.Documents[:min(len(s.Documents), menuListCap)] {
		recent.Children = append(recent.Children, MenuRow{ID: d.ID, Enabled: true})
	}
	if len(recent.Children) == 0 {
		recent.Children = []MenuRow{{ID: MenuNoRecent}}
	}
	rows = append(rows, recent)
	if s.RecoveredChanges > 0 {
		rows = append(rows, MenuRow{ID: MenuRecovered, Enabled: true})
	}
	rows = append(rows, MenuRow{ID: MenuSeparator},
		MenuRow{ID: MenuSettings, Enabled: true},
		MenuRow{ID: MenuCheckUpdates, Enabled: !phase.Checking && phase.Downloading == ""},
		MenuRow{ID: MenuSeparator},
		MenuRow{ID: MenuQuit, Enabled: true})
	out := make([]MenuRow, 0, len(rows))
	for _, r := range rows {
		if r.ID == MenuSeparator && (len(out) == 0 || out[len(out)-1].ID == MenuSeparator) {
			continue
		}
		out = append(out, r)
	}
	for len(out) > 0 && out[len(out)-1].ID == MenuSeparator {
		out = out[:len(out)-1]
	}
	return out
}

// MenuTitle is the words on one row, for the Windows and Linux tray. A child's
// title is its document's name or its item's title.
func MenuTitle(row MenuRow, s Snapshot, phase UpdatePhase) string {
	switch row.ID {
	case MenuStatus:
		switch row.Kind {
		case MenuStatusReady:
			if s.Update.DownloadURL != "" {
				return fmt.Sprintf(TextMenuDownloadUpdate, phase.Ready)
			}
			return fmt.Sprintf(TextMenuRestartToUpdate, phase.Ready)
		case MenuStatusChecking:
			return TextMenuChecking
		case MenuStatusDownloading:
			return fmt.Sprintf(TextMenuDownloading, phase.Downloading)
		}
		return fmt.Sprintf(TextMenuVersion, s.Update.Current)
	case MenuSyncLine:
		switch {
		case s.Sync.Paused:
			return TextMenuSyncPaused
		case s.Sync.SyncingFiles == 1:
			return fmt.Sprintf(TextMenuSyncingOne, s.Sync.Username)
		case s.Sync.SyncingFiles > 1:
			return fmt.Sprintf(TextMenuSyncingMany, s.Sync.SyncingFiles, s.Sync.Username)
		}
		return fmt.Sprintf(TextMenuSyncUpToDate, s.Sync.Username)
	case MenuPauseSync:
		if s.Sync.Paused {
			return TextTrayResumeSyncing
		}
		return TextTrayPauseSyncing
	case MenuNeedsYou:
		return fmt.Sprintf(TextMenuNeedsYou, s.Counts.Total)
	case MenuOpenDashboard:
		return TextMenuOpenDashboard
	case MenuOpenFile:
		return TextMenuOpenFile
	case MenuRecent:
		return TextMenuRecent
	case MenuNoRecent:
		return TextMenuNoRecent
	case MenuRecovered:
		return TextMenuRecovered
	case MenuSettings:
		return TextMenuSettings
	case MenuCheckUpdates:
		return TextMenuCheckForUpdates
	case MenuQuit:
		switch s.Host.OS {
		case "windows":
			return TextMenuQuitWindows
		case "linux":
			return TextMenuQuitLinux
		}
		return TextMenuQuitMac
	}
	for _, d := range s.Documents {
		if d.ID == row.ID {
			return d.Name
		}
	}
	for _, w := range s.Waiting {
		if w.ID == row.ID {
			return w.Title
		}
	}
	return ""
}

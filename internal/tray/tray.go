package tray

import (
	_ "embed"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"fyne.io/systray"
	"github.com/panphora/htmlclay/internal/appcore"
)

// listPollInterval is how often the menu re-reads its rows from the host. A
// change can come from a page, a dialog or the command line, none of which has
// a handle to the tray, so the menu polls rather than being pushed to. Two
// seconds is well below the time it takes a person to open the menu, and an
// unchanged row is not touched.
const listPollInterval = 2 * time.Second

//go:embed icon.png
var iconBytes []byte

//go:embed icon-template.png
var iconTemplateBytes []byte

// Windows needs an ICO. systray's SetIcon writes whatever bytes it is given to a
// temp file and hands the path to LoadImage, which reads ICO and refuses a PNG --
// so the PNG above produced a blank slot in the notification area and an error
// reading "unable to set icon: The operation completed successfully" (the call
// failed without setting a Win32 error code, so GetLastError returned 0).
//
// Regenerate with packaging/icons/make-tray-ico.ps1, which derives it from icon.png so
// both stay the blob-grin art rather than the app icon.
//
//go:embed icon.ico
var iconICOBytes []byte

// Row is one entry of the lists the host's older tray helpers return (trusted
// and served folders, programs, exports). The menu no longer shows those lists;
// the Settings pages do. The helpers and their tests still speak in rows.
type Row struct {
	Path  string
	Label string
	// Serving marks a row whose submenu held actions rather than a click of its own.
	Serving bool
}

// MenuItem is one row of the one menu, as the host computes it from
// appcore.MenuRows and MenuTitle. Children are a submenu's rows.
type MenuItem struct {
	ID       string
	Title    string
	Enabled  bool
	Children []MenuItem
}

// MenuHooks drive the one menu: Rows is read every listPollInterval, and Do is
// called with the id of a clicked row (a child's id for Recent and Needs you).
type MenuHooks struct {
	Rows func() []MenuItem
	Do   func(id string)
}

// The one menu, top to bottom. A row the host leaves out is hidden; separators
// sit before the rows named here and never hide, so the groups stay apart.
var menuOrder = []string{
	appcore.MenuStatus, appcore.MenuSyncLine, appcore.MenuPauseSync, appcore.MenuNeedsYou,
	appcore.MenuOpenDashboard, appcore.MenuOpenFile, appcore.MenuRecent, appcore.MenuRecovered,
	appcore.MenuSettings, appcore.MenuCheckUpdates,
	appcore.MenuQuit,
}

var separatorBefore = map[string]bool{
	appcore.MenuOpenDashboard: true,
	appcore.MenuSettings:      true,
	appcore.MenuQuit:          true,
}

// childSlots is how many rows each submenu can show: up to eight items, and for
// Recent one more for its "No recent documents" row.
var childSlots = map[string]int{
	appcore.MenuNeedsYou: 8,
	appcore.MenuRecent:   9,
}

// slotState is what one pre-made menu item should show.
type slotState struct {
	visible bool
	title   string
	enabled bool
}

// planRows lays the host's rows onto the menu's pre-made items: each id in
// menuOrder shows when the host sent a row with that id and hides when it did
// not; a submenu's children fill its slots in order and the rest hide.
func planRows(rows []MenuItem) (top map[string]slotState, children map[string][]slotState) {
	byID := make(map[string]MenuItem, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	top = make(map[string]slotState, len(menuOrder))
	children = make(map[string][]slotState, len(childSlots))
	for _, id := range menuOrder {
		r, ok := byID[id]
		top[id] = slotState{visible: ok, title: r.Title, enabled: r.Enabled}
		n, parent := childSlots[id]
		if !parent {
			continue
		}
		slots := make([]slotState, n)
		for i := 0; i < n && i < len(r.Children); i++ {
			c := r.Children[i]
			slots[i] = slotState{visible: true, title: c.Title, enabled: c.Enabled}
		}
		children[id] = slots
	}
	return top, children
}

func applySlot(item *systray.MenuItem, s slotState) {
	if !s.visible {
		item.Hide()
		return
	}
	item.SetTitle(s.title)
	if s.enabled {
		item.Enable()
	} else {
		item.Disable()
	}
	item.Show()
}

type Tray struct {
	menu       *MenuHooks
	beforeQuit func() bool
	onQuit     func()
	quitting   atomic.Bool

	mu   sync.Mutex
	rows []MenuItem

	// notice is a permanent row for something about this machine that HTML Clay
	// cannot fix and the person can. Empty means there is nothing to say.
	notice string
}

// Run shows the one menu and blocks until the tray quits. beforeQuit runs when
// the person chooses Quit (or Exit); false keeps the app open. onQuit runs as
// the tray ends.
func Run(menu *MenuHooks, beforeQuit func() bool, onQuit func(), notice string) {
	t := &Tray{menu: menu, beforeQuit: beforeQuit, onQuit: onQuit, notice: notice}
	systray.Run(t.onReady, t.onExit)
}

func (t *Tray) onReady() {
	// macOS uses the template (auto-inverts for light/dark menu bars); Linux ignores
	// it and falls back to the colored PNG. Windows needs the ICO: its fallback path
	// ends at LoadImage, which does not read PNG.
	if runtime.GOOS == "windows" {
		systray.SetIcon(iconICOBytes)
	} else {
		systray.SetTemplateIcon(iconTemplateBytes, iconBytes)
	}
	systray.SetTooltip("HTML Clay")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		systray.Quit()
	}()

	t.addNotice()

	items := make(map[string]*systray.MenuItem, len(menuOrder))
	kids := make(map[string][]*systray.MenuItem, len(childSlots))
	for _, id := range menuOrder {
		if separatorBefore[id] {
			systray.AddSeparator()
		}
		item := systray.AddMenuItem(id, "")
		item.Hide()
		items[id] = item
		for i := 0; i < childSlots[id]; i++ {
			child := item.AddSubMenuItem("", "")
			child.Hide()
			kids[id] = append(kids[id], child)
		}
	}

	t.render(items, kids)
	go func() {
		for range time.Tick(listPollInterval) {
			t.render(items, kids)
		}
	}()

	for _, id := range menuOrder {
		if id == appcore.MenuQuit {
			go t.watchQuit(items[id])
			continue
		}
		if _, parent := childSlots[id]; parent {
			for i, child := range kids[id] {
				go t.watchChild(id, i, child)
			}
			continue
		}
		go t.watchRow(id, items[id])
	}
}

// render reads the host's rows and shows them on the pre-made items.
func (t *Tray) render(items map[string]*systray.MenuItem, kids map[string][]*systray.MenuItem) {
	rows := t.menu.Rows()
	t.mu.Lock()
	t.rows = rows
	t.mu.Unlock()
	top, children := planRows(rows)
	for id, item := range items {
		applySlot(item, top[id])
	}
	for id, slots := range kids {
		for i, child := range slots {
			applySlot(child, children[id][i])
		}
	}
}

func (t *Tray) watchRow(id string, item *systray.MenuItem) {
	for range item.ClickedCh {
		go t.menu.Do(id)
	}
}

// watchChild hands a click on child slot i to the host as the id of the row the
// host last put there, and drops a click on a slot it has since emptied.
func (t *Tray) watchChild(parent string, i int, item *systray.MenuItem) {
	for range item.ClickedCh {
		t.mu.Lock()
		id := ""
		for _, r := range t.rows {
			if r.ID == parent && i < len(r.Children) {
				id = r.Children[i].ID
			}
		}
		t.mu.Unlock()
		if id != "" {
			go t.menu.Do(id)
		}
	}
}

// watchQuit asks before quitting. Asking can take a while (a save check, then a
// dialog), so a second click while it asks does nothing.
func (t *Tray) watchQuit(item *systray.MenuItem) {
	for range item.ClickedCh {
		if !t.quitting.CompareAndSwap(false, true) {
			continue
		}
		go func() {
			defer t.quitting.Store(false)
			if t.beforeQuit == nil || t.beforeQuit() {
				systray.Quit()
			}
		}()
	}
}

// addNotice puts the machine notice at the very top of the menu, disabled
// because there is nothing to click, and returns the row it made or nil when
// there is nothing to say.
//
// A permanent row, not a notification: it is the channel that survives a desktop
// with no notification daemon, and that is the same minimal desktop that raises
// the notice in the first place.
func (t *Tray) addNotice() *systray.MenuItem {
	if t.notice == "" {
		return nil
	}
	item := systray.AddMenuItem(t.notice, "")
	item.Disable()
	systray.AddSeparator()
	return item
}

// Quit ends the tray the way its Quit row does, so the app shuts down through
// the same onQuit path.
func Quit() {
	systray.Quit()
}

func (t *Tray) onExit() {
	t.onQuit()
}

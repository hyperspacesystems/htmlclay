package tray

import (
	_ "embed"
	"os"
	"os/signal"
	"runtime"
	"slices"
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
// appcore.MenuRows and MenuTitle. Children are a submenu's rows. A row whose ID
// is appcore.MenuSeparator is a line between groups.
type MenuItem struct {
	ID       string
	Title    string
	Enabled  bool
	Children []MenuItem
}

// MenuHooks drive the one menu: Rows is read every listPollInterval, and Do is
// called with the id of a clicked row (a child's id for Recent and Needs you).
// Tapped, when set, runs on a left click instead of showing the menu, which a
// right click still shows; the tray calls it with the menu's own thread, so it
// must return at once.
type MenuHooks struct {
	Rows   func() []MenuItem
	Do     func(id string)
	Tapped func()
}

// childSlots is how many rows each submenu can show: up to eight items, and for
// Recent one more for its "No recent documents" row.
var childSlots = map[string]int{
	appcore.MenuNeedsYou: 8,
	appcore.MenuRecent:   9,
}

// slotState is what one menu item should show.
type slotState struct {
	visible bool
	title   string
	enabled bool
}

// menuShape is the menu's top level as the tray builds it: each row's id in
// order, separators included. A change in titles, enabled states or children
// leaves it alone; sync connecting or a Needs you row appearing changes it.
func menuShape(rows []MenuItem) []string {
	shape := make([]string, 0, len(rows))
	for _, r := range rows {
		shape = append(shape, r.ID)
	}
	return shape
}

// planRows lays the host's rows onto the menu's items: every top-level row the
// host sent shows, and a submenu's children fill its slots in order while the
// rest hide.
func planRows(rows []MenuItem) (top map[string]slotState, children map[string][]slotState) {
	top = make(map[string]slotState, len(rows))
	children = make(map[string][]slotState, len(childSlots))
	for _, r := range rows {
		if r.ID == appcore.MenuSeparator {
			continue
		}
		top[r.ID] = slotState{visible: true, title: r.Title, enabled: r.Enabled}
		n, parent := childSlots[r.ID]
		if !parent {
			continue
		}
		slots := make([]slotState, n)
		for i := 0; i < n && i < len(r.Children); i++ {
			c := r.Children[i]
			slots[i] = slotState{visible: true, title: c.Title, enabled: c.Enabled}
		}
		children[r.ID] = slots
	}
	return top, children
}

// applyChanged runs apply for a slot only when its state differs from what
// it last showed, and records the new state. Every systray call is a D-Bus
// signal on Linux, which closes an open submenu, and hiding an item already
// hidden logs an error on Windows, so an unchanged poll touches nothing.
func applyChanged[K comparable](shown map[K]slotState, key K, s slotState, apply func()) {
	if last, ok := shown[key]; ok && last == s {
		return
	}
	apply()
	shown[key] = s
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

	mu    sync.Mutex
	rows  []MenuItem
	shape []string
	items map[string]*systray.MenuItem
	kids  map[string][]*systray.MenuItem
	// shown is what each item last showed; render and build only.
	shown map[*systray.MenuItem]slotState

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

// MenuOnLeftClick goes back to showing the menu on a left click, for a tray
// whose Tapped can no longer do anything.
func MenuOnLeftClick() {
	systray.SetOnTapped(nil)
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
	if t.menu.Tapped != nil {
		systray.SetOnTapped(t.menu.Tapped)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		systray.Quit()
	}()

	t.render()
	go func() {
		for range time.Tick(listPollInterval) {
			t.render()
		}
	}()
}

// render reads the host's rows and shows them. The menu is built again only
// when its shape changed, because fyne's separators cannot be hidden and the
// host's groups need a different number of lines with sync on or off; every
// other change updates the existing items in place.
func (t *Tray) render() {
	rows := t.menu.Rows()
	shape := menuShape(rows)
	t.mu.Lock()
	t.rows = rows
	if !slices.Equal(shape, t.shape) {
		if t.shape != nil {
			systray.ResetMenu()
		}
		t.build(rows)
		t.shape = shape
	}
	items, kids, shown := t.items, t.kids, t.shown
	t.mu.Unlock()
	top, children := planRows(rows)
	for id, item := range items {
		applyChanged(shown, item, top[id], func() { applySlot(item, top[id]) })
	}
	for id, slots := range kids {
		for i, child := range slots {
			applyChanged(shown, child, children[id][i], func() { applySlot(child, children[id][i]) })
		}
	}
}

// build makes the menu's items for rows, top to bottom, with the machine
// notice first. A removed item closes its click channel, which ends its
// watcher.
func (t *Tray) build(rows []MenuItem) {
	t.addNotice()
	t.items = make(map[string]*systray.MenuItem, len(rows))
	t.kids = make(map[string][]*systray.MenuItem, len(childSlots))
	t.shown = make(map[*systray.MenuItem]slotState)
	for _, r := range rows {
		if r.ID == appcore.MenuSeparator {
			systray.AddSeparator()
			continue
		}
		item := systray.AddMenuItem(r.Title, "")
		t.items[r.ID] = item
		for i := 0; i < childSlots[r.ID]; i++ {
			child := item.AddSubMenuItem("", "")
			child.Hide()
			t.shown[child] = slotState{}
			t.kids[r.ID] = append(t.kids[r.ID], child)
		}
		switch {
		case r.ID == appcore.MenuQuit:
			go t.watchQuit(item)
		case len(t.kids[r.ID]) > 0:
			for i, child := range t.kids[r.ID] {
				go t.watchChild(r.ID, i, child)
			}
		default:
			go t.watchRow(r.ID, item)
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

package tray

import (
	_ "embed"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"fyne.io/systray"
	"github.com/panphora/htmlclay/internal/appcore"
	"github.com/panphora/htmlclay/internal/config"
	"github.com/panphora/htmlclay/internal/platform"
)

// listPollInterval is how often a submenu re-renders from its authoritative
// list. A folder can be trusted by the permission dialog or by a page request,
// neither of which has a handle to the tray, so the menu is refreshed by polling
// rather than pushed to. Two seconds is well below the time it takes a user to
// open the submenu, and the render is a cheap no-op when nothing changed.
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

// Row is one entry in a list submenu. Label is what the user sees, Path is what
// the hook is called with. They are carried separately because they differ: a
// folder that is missing or whose identity no longer matches is labelled as
// such. Recovering the path by stripping that marker off the label was the old
// protocol, and it broke for anyone who named a folder to end in the marker.
type Row struct {
	Path  string
	Label string
	// Serving marks a row whose submenu holds actions rather than a click of
	// its own. The tray does not act on it; it is there for the app to read.
	Serving bool
}

// TrustedFolderHooks are the app operations the Trusted Folders submenu drives.
// Each returns the authoritative list so the tray always re-renders from truth
// rather than tracking state itself. Add pops the native folder picker and
// trusts the choice; Remove untrusts one (asking first, app-side, because it
// closes a live origin); List is the initial render and the poll.
type TrustedFolderHooks struct {
	List   func() []Row
	Add    func() []Row
	Remove func(path string) []Row
}

// ServedFolderHooks are the app operations the Served Folders submenu drives. Row
// Path is the served folder's id. Each returns the authoritative list, except Open
// and CopyAddress, which change nothing.
type ServedFolderHooks struct {
	List        func() []Row
	Add         func() []Row
	Open        func(id string)
	CopyAddress func(id string)
	Stop        func(id string) []Row
	// BeforeQuit runs when the person chooses Quit; false keeps the app open.
	// May be nil.
	BeforeQuit func() bool
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

// SettingItem is one row of a settings submenu: Action is passed back to Do.
// Checked nil is a plain row; otherwise the row shows a check mark when true.
type SettingItem struct {
	Action  string
	Title   string
	Enabled bool
	Checked *bool
}

// SettingsHooks fill the settings submenus this round adds. Items is asked for
// one group's rows ("builtin", "ai", "files", "advanced") every listPollInterval.
type SettingsHooks struct {
	Items func(group string) []SettingItem
	Do    func(action string)
}

// The settings groups, in the order the Settings submenu lists them.
const (
	settingGroupBuiltin  = "builtin"
	settingGroupAI       = "ai"
	settingGroupFiles    = "files"
	settingGroupAdvanced = "advanced"
)

// How many rows each part of the one menu keeps room for. A row is never
// destroyed: the host's rows decide what each pre-made one shows, and a row
// past the end hides.
const (
	needsYouSlots = 8
	recentSlots   = 9
	builtinSlots  = 6
	aiSlots       = 8
	filesSlots    = 4
	advancedSlots = 4
)

// The rows each top-level pool holds, in the order the one menu lists them, so
// a row the host leaves out leaves its place empty rather than shifting the
// others up.
var (
	headRowIDs = []string{appcore.MenuStatus, appcore.MenuSyncLine, appcore.MenuPauseSync, appcore.MenuNeedsYou}
	mainRowIDs = []string{appcore.MenuOpenDashboard, appcore.MenuOpenFile, appcore.MenuRecent, appcore.MenuRecovered}
	tailRowIDs = []string{appcore.MenuSettings, appcore.MenuCheckUpdates}
	quitRowIDs = []string{appcore.MenuQuit}
)

// applySlot shows one pre-made item as its row says.
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
	if s.checked != nil && *s.checked {
		item.Check()
	} else {
		item.Uncheck()
	}
	item.Show()
}

// rowPool is a fixed set of pre-made rows whose titles, checks and visibility
// follow the host's latest rows. A click hands back the action of the row that
// is in that place now, read under the mutex, so a queued click can never run
// the action that used to be there.
type rowPool struct {
	mu    sync.Mutex
	items []*systray.MenuItem
	rows  []SettingItem
	do    func(string)
}

func newRowPool(items []*systray.MenuItem, do func(string)) *rowPool {
	p := &rowPool{items: items, do: do}
	for i, item := range items {
		item.Hide()
		go func(i int, clicked <-chan struct{}) {
			for range clicked {
				p.mu.Lock()
				var action string
				if i < len(p.rows) {
					action = p.rows[i].Action
				}
				p.mu.Unlock()
				if action != "" && p.do != nil {
					go p.do(action)
				}
			}
		}(i, item.ClickedCh)
	}
	return p
}

// apply renders rows onto the pool's pre-made items.
func (p *rowPool) apply(rows []SettingItem) {
	p.mu.Lock()
	p.rows = rows
	p.mu.Unlock()
	for i, s := range planSlots(rows, len(p.items)) {
		applySlot(p.items[i], s)
	}
}

// subMenuItem adds a row to parent's submenu, or to the top level when parent
// is nil, so one builder serves a row wherever it now hangs.
func subMenuItem(parent *systray.MenuItem, title, tooltip string) *systray.MenuItem {
	if parent != nil {
		return parent.AddSubMenuItem(title, tooltip)
	}
	return systray.AddMenuItem(title, tooltip)
}

// topSlot makes one row of the one menu: a checkbox, so a row that carries a
// check can show it.
func topSlot(title string) *systray.MenuItem {
	return systray.AddMenuItemCheckbox(title, "", false)
}

// settingSlots makes n rows inside a submenu, ready to be driven by planSlots.
func settingSlots(parent *systray.MenuItem, n int) []*systray.MenuItem {
	out := make([]*systray.MenuItem, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, parent.AddSubMenuItemCheckbox("", "", false))
	}
	return out
}

// menuPoolRows picks the rows a pool holds, in the pool's own order.
func menuPoolRows(rows []MenuItem, ids []string) []SettingItem {
	out := make([]SettingItem, 0, len(ids))
	for _, id := range ids {
		for _, r := range rows {
			if r.ID == id {
				out = append(out, SettingItem{Action: r.ID, Title: rowTitle(r.ID, r.Title), Enabled: r.Enabled})
				break
			}
		}
	}
	return out
}

// rowTitle is the words on a top-level row. An ellipsis marks a row that asks
// for something, so the row that opens a submenu drops it: Settings holds its
// rows in a submenu here rather than in a window.
func rowTitle(id, title string) string {
	if id == appcore.MenuSettings {
		return strings.TrimSuffix(title, "…")
	}
	return title
}

// menuChildren is the submenu rows of one row of the one menu.
func menuChildren(rows []MenuItem, id string) []SettingItem {
	for _, r := range rows {
		if r.ID == id {
			out := make([]SettingItem, 0, len(r.Children))
			for _, c := range r.Children {
				out = append(out, SettingItem{Action: c.ID, Title: c.Title, Enabled: c.Enabled})
			}
			return out
		}
	}
	return nil
}

// loginRowTitle is the Start on login row in the system's own words.
func loginRowTitle() string {
	if runtime.GOOS == "windows" {
		return "Open HTML Clay when I sign in"
	}
	return "Open HTML Clay at login"
}

// slotState is what one pre-made menu item should show.
type slotState struct {
	visible bool
	title   string
	enabled bool
	checked *bool
}

// planSlots maps rows onto n pre-made items: the first len(rows) show, the rest hide.
func planSlots(rows []SettingItem, n int) []slotState {
	out := make([]slotState, n)
	for i := 0; i < n && i < len(rows); i++ {
		out[i] = slotState{visible: true, title: rows[i].Title, enabled: rows[i].Enabled, checked: rows[i].Checked}
	}
	return out
}

// HelperHooks are the app operations the Programs submenu drives. Each returns
// the authoritative list so the tray always re-renders from config state.
type HelperHooks struct {
	// Title names the Programs item, counting missing program files. May be nil.
	Title func() string
	// List is the programs whose file is missing; Row relinks one.
	List func() []Row
	// Manage opens the Programs page.
	Manage    func()
	Row       func(id string) []Row
	Documents func() []Row
	Document  func(key string) []Row
}

// ExportHooks drive the Export as Zip submenu: Documents lists the open
// documents, Export zips one (by row Path) and returns the fresh list.
type ExportHooks struct {
	Documents func() []Row
	Export    func(path string) []Row
}

// slotCount is how many submenu rows exist before any list is rendered. The pool
// grows from there as entries need places to sit.
//
// A row is never destroyed. systray v1.12.0 does have MenuItem.Remove(), on all
// three platforms, but rebuilding the list every render would reintroduce the
// queued-click hazard render exists to prevent and churn a goroutine per row per
// render; rows are shown, retitled, and hidden instead. Adding one at runtime is
// the library's normal operating mode, which is what makes growing possible. It
// has to be: the row click is the only caller of untrustFolder, so a fixed pool
// meant every trusted folder past the sixteenth could not be revoked from the
// menu at all, while still granting silent reads and writes.
const slotCount = 16

type slot struct {
	item *systray.MenuItem
	path string
	// children are the per-row actions a served folder offers. They exist only
	// when the menu was built with actions.
	children []*systray.MenuItem
}

type Tray struct {
	cfg           *config.Config
	quitting      atomic.Bool
	version       string
	onOpenExample func()
	onOpenBackups func()
	onQuit        func()

	trusted             *TrustedFolderHooks
	trustedMenu         *listMenu
	served              *ServedFolderHooks
	servedMenu          *listMenu
	helpers             *HelperHooks
	helpersMenu         *listMenu
	helperDocumentsMenu *listMenu
	export              *ExportHooks
	menu                *MenuHooks
	settings            *SettingsHooks

	// The one menu's fixed rows, in menu order, and the settings submenus.
	headPool     *rowPool
	mainPool     *rowPool
	tailPool     *rowPool
	quitPool     *rowPool
	needsYouPool *rowPool
	recentPool   *rowPool
	builtinPool  *rowPool
	aiPool       *rowPool
	filesPool    *rowPool
	advancedPool *rowPool

	// notice is a permanent row for something about this machine that HTML Clay
	// cannot fix and the user can. Empty means there is nothing to say.
	notice string
}

func Run(cfg *config.Config, version string, menu *MenuHooks, settings *SettingsHooks, onOpenExample func(), onOpenBackups func(), onQuit func(), trusted *TrustedFolderHooks, served *ServedFolderHooks, helpers *HelperHooks, export *ExportHooks, notice string) {
	t := &Tray{
		cfg:           cfg,
		version:       version,
		menu:          menu,
		settings:      settings,
		onOpenExample: onOpenExample,
		onOpenBackups: onOpenBackups,
		onQuit:        onQuit,
		trusted:       trusted,
		served:        served,
		helpers:       helpers,
		export:        export,
		notice:        notice,
	}
	systray.Run(t.onReady, t.onExit)
}

const profilePrivacy = "Use this name in files opened with this app. Files can save your name and person ID, and anyone who receives those files can read them.\n\nTurning this off stops HTML Clay supplying your name. Names already saved in files stay there."

// profileActions are the Profile submenu's operations, with the native dialogs
// passed in so tests can drive them without opening any UI.
type profileActions struct {
	cfg    *config.Config
	prompt func(title, message, initial string) (string, bool, error)
	alert  func(title, message string)
}

// askName asks until it gets a valid name or the person cancels.
func (p *profileActions) askName(initial string) (string, bool) {
	message := profilePrivacy
	for {
		value, ok, err := p.prompt("Display Name", message, initial)
		if err != nil {
			p.alert("Could not ask for a name", err.Error())
			return "", false
		}
		if !ok {
			return "", false
		}
		name, err := config.CleanProfileName(value)
		if err == nil {
			return name, true
		}
		message = err.Error() + "\n\n" + profilePrivacy
		initial = value
	}
}

// toggle turns sharing on or off and returns whether it is now on. A profile that is on
// but damaged reads as off in the menu, and clicking it repairs it: the name is asked for
// again and a damaged id is replaced.
func (p *profileActions) toggle() bool {
	cur := p.cfg.ProfileState()
	next := cur
	if cur.Shareable() {
		next.Enabled = false
	} else {
		if _, err := config.CleanProfileName(cur.Name); err != nil {
			name, ok := p.askName(cur.Name)
			if !ok {
				return false
			}
			next.Name = name
		}
		if !config.ValidProfileID(next.ID) {
			id, err := config.NewProfileID()
			if err != nil {
				p.alert("Could not set up your profile", err.Error())
				return false
			}
			next.ID = id
		}
		next.Enabled = true
	}
	if err := p.cfg.UpdateProfile(next); err != nil {
		p.alert("Could not save your profile", err.Error())
		return p.cfg.ProfileState().Shareable()
	}
	return next.Enabled
}

// rename changes the display name, keeping the id and whether sharing is on.
func (p *profileActions) rename() {
	cur := p.cfg.ProfileState()
	name, ok := p.askName(cur.Name)
	if !ok {
		return
	}
	next := cur
	next.Name = name
	if !config.ValidProfileID(next.ID) {
		id, err := config.NewProfileID()
		if err != nil {
			p.alert("Could not set up your profile", err.Error())
			return
		}
		next.ID = id
	}
	if err := p.cfg.UpdateProfile(next); err != nil {
		p.alert("Could not save your profile", err.Error())
	}
}

// listMenu renders an authoritative list into a growing slot pool, with an
// optional add row, per-row click actions, and polling for lists that change
// outside the tray.
type listMenu struct {
	mu    sync.Mutex
	menu  *systray.MenuItem
	empty *systray.MenuItem
	slots []*slot
	// onRow is set by watch, after the slots exist, and read at click time rather
	// than captured, so a slot grown later behaves exactly like one built up front.
	onRow func(string) []Row
	// onAction and actions belong to a menu built by newActionListMenu: each row
	// is a parent of one child per action, and the child click reports which.
	onAction   func(action int, path string) []Row
	actions    []string
	rowTooltip string
}

func newListMenu(parent *systray.MenuItem, title, tooltip, emptyText, addTitle, addTooltip string) (*listMenu, *systray.MenuItem) {
	return newActionListMenu(parent, title, tooltip, emptyText, addTitle, addTooltip, nil)
}

// newActionListMenu is newListMenu for rows that each do one of several things.
// actions must be set before the first slot is created: the pool's children are
// built with the slots, so every slot grown later has them too.
func newActionListMenu(parent *systray.MenuItem, title, tooltip, emptyText, addTitle, addTooltip string, actions []string) (*listMenu, *systray.MenuItem) {
	menu := subMenuItem(parent, title, tooltip)
	var addItem *systray.MenuItem
	if addTitle != "" {
		addItem = menu.AddSubMenuItem(addTitle, addTooltip)
	}
	lm := &listMenu{menu: menu, actions: actions}
	lm.empty = menu.AddSubMenuItem(emptyText, "")
	lm.empty.Disable()
	lm.mu.Lock()
	for i := 0; i < slotCount; i++ {
		lm.addSlotLocked()
	}
	lm.mu.Unlock()
	return lm, addItem
}

// addSlotLocked appends one hidden row and starts its click goroutine. Caller
// holds lm.mu.
//
// The goroutine closes over the *slot, never its index. Growing appends to
// lm.slots and so reallocates the backing array, and an index read from the
// goroutine would race with that append; the pointer stays valid whatever the
// slice does.
func (lm *listMenu) addSlotLocked() *slot {
	s := &slot{item: lm.menu.AddSubMenuItem("", "")}
	s.item.Hide()
	lm.slots = append(lm.slots, s)
	if len(lm.actions) > 0 {
		for action, title := range lm.actions {
			child := s.item.AddSubMenuItem(title, "")
			child.Hide()
			s.children = append(s.children, child)
			go func(action int) {
				for range child.ClickedCh {
					lm.mu.Lock()
					path, onAction := s.path, lm.onAction
					lm.mu.Unlock()
					if path == "" || onAction == nil {
						continue
					}
					go func() {
						if rows := onAction(action, path); rows != nil {
							lm.render(rows)
						}
					}()
				}
			}(action)
		}
		// The parent's own click does nothing: for an action row the parent is
		// only the place the actions hang from.
		return s
	}
	go func() {
		for range s.item.ClickedCh {
			lm.mu.Lock()
			path, onRow := s.path, lm.onRow
			lm.mu.Unlock()
			if path == "" || onRow == nil {
				continue
			}
			// In its own goroutine: removing asks for confirmation first,
			// and a blocking dialog here would freeze every other row.
			go func() {
				if rows := onRow(path); rows != nil {
					lm.render(rows)
				}
			}()
		}
	}()
	return s
}

// freeSlotLocked returns a vacant slot, growing the pool when every slot is
// taken. Caller holds lm.mu.
func (lm *listMenu) freeSlotLocked() *slot {
	for _, s := range lm.slots {
		if s.path == "" {
			return s
		}
	}
	return lm.addSlotLocked()
}

// render reconciles the slot pool with rows. An entry keeps its existing slot
// across renders and only vacated slots are reused, so a click already queued
// against a row stays bound to the entry that was in it. Compacting the rows
// instead would let one entry's queued click land on whatever entry shifted into
// its row, which with a destructive action is the difference between untrusting
// the folder you clicked and untrusting a different one. Safe to call from any
// goroutine.
func (lm *listMenu) render(rows []Row) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	label := make(map[string]string, len(rows))
	for _, r := range rows {
		label[r.Path] = r.Label
	}
	// Vacate slots whose entry is gone; leave every other entry in place, and
	// refresh its label so a folder that has since gone missing says so.
	assigned := make(map[string]bool, len(rows))
	for _, s := range lm.slots {
		if s.path == "" {
			continue
		}
		if l, ok := label[s.path]; ok {
			s.item.SetTitle(l)
			assigned[s.path] = true
			continue
		}
		s.path = ""
		s.item.Hide()
		for _, child := range s.children {
			child.Hide()
		}
	}
	// Place new entries into the lowest free slot, adding one when the pool is
	// full. Every entry therefore has a clickable row, which is what makes every
	// grant revocable from the menu.
	for _, r := range rows {
		if assigned[r.Path] {
			continue
		}
		s := lm.freeSlotLocked()
		s.path = r.Path
		s.item.SetTitle(r.Label)
		s.item.SetTooltip(lm.rowTooltip)
		s.item.Show()
		for _, child := range s.children {
			child.Show()
		}
		assigned[r.Path] = true
	}
	if len(assigned) == 0 {
		lm.empty.Show()
	} else {
		lm.empty.Hide()
	}
}

// watch sets the row action and, when poll is set, starts the poller that keeps
// the menu in step with entries created outside the tray. The per-row click
// goroutines belong to the slots themselves (addSlotLocked), because the pool can
// gain a slot long after this has run.
func (lm *listMenu) watch(onRow func(string) []Row, poll func() []Row) {
	lm.mu.Lock()
	lm.onRow = onRow
	lm.mu.Unlock()
	lm.poll(poll)
}

// watchActions is watch for a menu whose rows each offer several actions.
func (lm *listMenu) watchActions(onAction func(int, string) []Row, poll func() []Row) {
	lm.mu.Lock()
	lm.onAction = onAction
	lm.mu.Unlock()
	lm.poll(poll)
}

// poll re-renders from the authoritative list on a ticker, for lists that change
// with no handle to the tray.
func (lm *listMenu) poll(poll func() []Row) {
	if poll == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(listPollInterval)
		defer ticker.Stop()
		for range ticker.C {
			lm.render(poll())
		}
	}()
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

	// The status, sync and Needs you rows. Needs you hangs from its own row here,
	// so its children are made first.
	statusItem := topSlot("")
	syncLineItem := topSlot("")
	pauseItem := topSlot("")
	needsYouItem := topSlot("")
	t.needsYouPool = newRowPool(settingSlots(needsYouItem, needsYouSlots), t.doMenu)
	t.headPool = newRowPool([]*systray.MenuItem{statusItem, syncLineItem, pauseItem, needsYouItem}, t.doMenu)

	systray.AddSeparator()

	// The documents and settings rows.
	dashboardItem := topSlot("")
	openFileItem := topSlot("")
	recentItem := topSlot("")
	t.recentPool = newRowPool(settingSlots(recentItem, recentSlots), t.doMenu)
	t.mainPool = newRowPool([]*systray.MenuItem{dashboardItem, openFileItem, recentItem, topSlot("")}, t.doMenu)

	systray.AddSeparator()

	settingsItem := topSlot("")
	exampleItem := settingsItem.AddSubMenuItem("Open example file", "Create and open a sample self-saving HTML file")
	backupsItem := settingsItem.AddSubMenuItem("Backups", "Open the folder holding every saved version of your files")
	if t.export != nil {
		em, _ := newListMenu(settingsItem, "Export as zip", "Save an open file and its attachments as one zip in Downloads", "No files open", "", "")
		em.rowTooltip = "Click to save this file and its attachments as a zip in Downloads"
		em.render(t.export.Documents())
		em.watch(t.export.Export, t.export.Documents)
	}

	addServedItem := t.buildServedMenu(settingsItem)
	addTrustedItem := t.buildTrustedMenu(settingsItem)
	addHelperItem := t.buildHelpersMenu(settingsItem)

	builtinMenu := settingsItem.AddSubMenuItem("Built-in programs", "")
	t.builtinPool = newRowPool(settingSlots(builtinMenu, builtinSlots), t.doSetting)
	aiMenu := settingsItem.AddSubMenuItem("AI editing", "")
	t.aiPool = newRowPool(settingSlots(aiMenu, aiSlots), t.doSetting)

	profile := &profileActions{
		cfg:    t.cfg,
		prompt: platform.PromptName,
		alert:  func(title, message string) { _ = platform.Notify(title, message) },
	}
	profileItem := settingsItem.AddSubMenuItem("Your name", "Your name in files opened with HTML Clay")
	useProfileItem := profileItem.AddSubMenuItemCheckbox("Use my name in documents", "", t.cfg.ProfileState().Shareable())
	renameProfileItem := profileItem.AddSubMenuItem("Change name…", "")

	filesMenu := settingsItem.AddSubMenuItem(".htmlclay files", "")
	t.filesPool = newRowPool(settingSlots(filesMenu, filesSlots), t.doSetting)

	loginItem := settingsItem.AddSubMenuItemCheckbox(loginRowTitle(), "", t.cfg.StartOnLoginEnabled())

	t.advancedPool = newRowPool(settingSlots(settingsItem, advancedSlots), t.doSetting)

	checkUpdatesItem := topSlot("")
	systray.AddSeparator()
	quitItem := topSlot("")

	t.tailPool = newRowPool([]*systray.MenuItem{settingsItem, checkUpdatesItem}, t.doMenu)
	t.quitPool = newRowPool([]*systray.MenuItem{quitItem}, func(string) { t.quit() })

	go func() {
		for {
			select {
			case <-exampleItem.ClickedCh:
				go t.onOpenExample()
			case <-backupsItem.ClickedCh:
				go t.onOpenBackups()
			case <-loginItem.ClickedCh:
				t.toggleLoginItem(loginItem)
			case <-useProfileItem.ClickedCh:
				if profile.toggle() {
					useProfileItem.Check()
				} else {
					useProfileItem.Uncheck()
				}
			case <-renameProfileItem.ClickedCh:
				profile.rename()
			}
		}
	}()

	t.watchServedMenu(addServedItem)
	t.watchTrustedMenu(addTrustedItem)
	t.watchHelpersMenu(addHelperItem)
	t.pollMenu()
}

// doMenu hands a clicked row of the one menu to the host.
func (t *Tray) doMenu(id string) {
	if t.menu != nil && t.menu.Do != nil {
		t.menu.Do(id)
	}
}

// doSetting hands a clicked settings row to the host.
func (t *Tray) doSetting(action string) {
	if t.settings != nil && t.settings.Do != nil {
		t.settings.Do(action)
	}
}

// pollMenu keeps every row in step with the host, whose rows change with no
// handle to the tray: a document opens, an update starts checking, a program is
// switched on.
func (t *Tray) pollMenu() {
	render := func() {
		var rows []MenuItem
		if t.menu != nil && t.menu.Rows != nil {
			rows = t.menu.Rows()
		}
		t.headPool.apply(menuPoolRows(rows, headRowIDs))
		t.mainPool.apply(menuPoolRows(rows, mainRowIDs))
		t.tailPool.apply(menuPoolRows(rows, tailRowIDs))
		t.quitPool.apply(menuPoolRows(rows, quitRowIDs))
		t.needsYouPool.apply(menuChildren(rows, appcore.MenuNeedsYou))
		t.recentPool.apply(menuChildren(rows, appcore.MenuRecent))
		if t.settings != nil && t.settings.Items != nil {
			t.builtinPool.apply(t.settings.Items(settingGroupBuiltin))
			t.aiPool.apply(t.settings.Items(settingGroupAI))
			t.filesPool.apply(t.settings.Items(settingGroupFiles))
			t.advancedPool.apply(t.settings.Items(settingGroupAdvanced))
		}
	}
	render()
	go func() {
		ticker := time.NewTicker(listPollInterval)
		defer ticker.Stop()
		for range ticker.C {
			render()
		}
	}()
}

// quit ends HTML Clay the way its Quit row always has: asking about unsaved
// edits first, in its own goroutine, because the answer can take a while and
// the menu's other clicks arrive on the same loop.
func (t *Tray) quit() {
	if !t.quitting.CompareAndSwap(false, true) {
		return
	}
	defer t.quitting.Store(false)
	if t.served == nil || t.served.BeforeQuit == nil || t.served.BeforeQuit() {
		systray.Quit()
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

// servedActions are the Served Folders submenu's child rows, in menu order. The
// index is what the tray hands back to servedAction.
var servedActions = []string{appcore.TextOpenServed, appcore.TextCopyAddress, appcore.TextStopServing}

const (
	servedActionOpen = iota
	servedActionCopyAddress
	servedActionStop
)

// buildServedMenu adds the Served Folders submenu: an add row, and one parent row
// per served folder whose children open it, copy its address, or stop serving it.
// Polled, because a folder can be served from the command line with no handle to
// the tray.
func (t *Tray) buildServedMenu(parent *systray.MenuItem) *systray.MenuItem {
	if t.served == nil {
		return nil
	}
	lm, addItem := newActionListMenu(
		parent,
		appcore.TextServedFolders,
		"Folders served at their own addresses",
		appcore.TextNoServedFolders,
		appcore.TextServeAFolder,
		"",
		servedActions,
	)
	t.servedMenu = lm
	if t.served.List != nil {
		lm.render(t.served.List())
	}
	return addItem
}

// watchServedMenu wires the add flow (in its own goroutine, because the folder
// picker blocks until the user answers) and the action/poll machinery.
func (t *Tray) watchServedMenu(addItem *systray.MenuItem) {
	if t.served == nil || t.servedMenu == nil {
		return
	}
	if addItem != nil && t.served.Add != nil {
		go func() {
			for range addItem.ClickedCh {
				go func() {
					if rows := t.served.Add(); rows != nil {
						t.servedMenu.render(rows)
					}
				}()
			}
		}()
	}
	t.servedMenu.watchActions(t.servedAction, t.served.List)
}

// servedAction runs one child row's operation. Only Stop changes the list, so it
// is the only one whose rows are rendered; a stale id after a poll is simply a
// no-op.
func (t *Tray) servedAction(action int, id string) []Row {
	switch action {
	case servedActionOpen:
		if t.served.Open != nil {
			t.served.Open(id)
		}
	case servedActionCopyAddress:
		if t.served.CopyAddress != nil {
			t.served.CopyAddress(id)
		}
	case servedActionStop:
		if t.served.Stop != nil {
			return t.served.Stop(id)
		}
	}
	return nil
}

// buildTrustedMenu adds the Trusted Folders submenu: an add row, and rows that
// untrust their folder on click. Polled, because the permission dialog and a
// page's own request both trust folders with no handle to the tray.
func (t *Tray) buildTrustedMenu(parent *systray.MenuItem) *systray.MenuItem {
	if t.trusted == nil {
		return nil
	}
	lm, addItem := newListMenu(
		parent,
		"Trusted folders",
		"Folders whose HTML Clay files open editable with no prompts",
		"No trusted folders yet",
		"Trust a folder…",
		"Pick a folder to trust",
	)
	lm.rowTooltip = "Click to stop trusting this folder"
	t.trustedMenu = lm
	if t.trusted.List != nil {
		lm.render(t.trusted.List())
	}
	return addItem
}

// watchTrustedMenu wires the add flow (in its own goroutine, because the folder
// picker blocks until the user answers) and the row/poll machinery.
func (t *Tray) watchTrustedMenu(addItem *systray.MenuItem) {
	if t.trusted == nil || t.trustedMenu == nil {
		return
	}
	if addItem != nil && t.trusted.Add != nil {
		go func() {
			for range addItem.ClickedCh {
				go func() {
					if rows := t.trusted.Add(); rows != nil {
						t.trustedMenu.render(rows)
					}
				}()
			}
		}()
	}
	t.trustedMenu.watch(t.trusted.Remove, t.trusted.List)
}

func (t *Tray) buildHelpersMenu(parent *systray.MenuItem) *systray.MenuItem {
	if t.helpers == nil {
		return nil
	}
	title := "Programs"
	if t.helpers.Title != nil {
		title = t.helpers.Title()
	}
	manageTitle := ""
	if t.helpers.Manage != nil {
		manageTitle = "Manage programs…"
	}
	lm, manageItem := newListMenu(
		parent,
		title,
		"Programs HTML Clay documents can ask to run",
		"No missing program files",
		manageTitle,
		"See which documents may run which programs, and remove access",
	)
	lm.rowTooltip = "Choose where this program's file is now"
	t.helpersMenu = lm
	if t.helpers.List != nil {
		lm.render(t.helpers.List())
	}
	if t.helpers.Documents != nil && t.helpers.Document != nil {
		menu := lm.menu.AddSubMenuItem("Open documents", "Programs the documents open in HTML Clay ask for")
		dm := &listMenu{menu: menu, rowTooltip: "Set up this document's program, or see it on the Programs page"}
		dm.empty = menu.AddSubMenuItem("No open documents use programs", "")
		dm.empty.Disable()
		dm.render(t.helpers.Documents())
		t.helperDocumentsMenu = dm
	}
	return manageItem
}

func (t *Tray) watchHelpersMenu(manageItem *systray.MenuItem) {
	if t.helpers == nil || t.helpersMenu == nil {
		return
	}
	if manageItem != nil && t.helpers.Manage != nil {
		go func() {
			for range manageItem.ClickedCh {
				go t.helpers.Manage()
			}
		}()
	}
	t.helpersMenu.watch(t.helpers.Row, t.helpers.List)
	if t.helpers.Title != nil {
		go func() {
			ticker := time.NewTicker(listPollInterval)
			defer ticker.Stop()
			for range ticker.C {
				t.helpersMenu.menu.SetTitle(t.helpers.Title())
			}
		}()
	}
	if t.helperDocumentsMenu != nil {
		t.helperDocumentsMenu.watch(t.helpers.Document, t.helpers.Documents)
	}
}

func (t *Tray) toggleLoginItem(loginItem *systray.MenuItem) {
	newVal := !t.cfg.StartOnLoginEnabled()
	execPath, err := os.Executable()
	if err != nil || execPath == "" {
		fmt.Fprintf(os.Stderr, "[htmlclay] cannot determine executable path: %v\n", err)
		return
	}

	if err := platform.SetLoginItem(newVal, execPath); err != nil {
		return
	}

	t.cfg.SetStartOnLogin(newVal)
	if err := t.cfg.Save(); err != nil {
		t.cfg.SetStartOnLogin(!newVal)
		platform.SetLoginItem(!newVal, execPath)
		return
	}

	if newVal {
		loginItem.Check()
	} else {
		loginItem.Uncheck()
	}
}

// Quit ends the tray the way its Quit row does, so the app shuts down through
// the same onQuit path.
func Quit() {
	systray.Quit()
}

func (t *Tray) onExit() {
	t.onQuit()
}

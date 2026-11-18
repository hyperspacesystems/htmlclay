package tray

import (
	"slices"
	"strings"
	"testing"

	"github.com/panphora/htmlclay/internal/appcore"
)

func TestIconEmbedded(t *testing.T) {
	for _, c := range []struct {
		name string
		data []byte
	}{
		{"icon.png", iconBytes},
		{"icon-template.png", iconTemplateBytes},
	} {
		if len(c.data) == 0 {
			t.Fatalf("%s not embedded", c.name)
		}
		if c.data[0] != 0x89 || c.data[1] != 'P' || c.data[2] != 'N' || c.data[3] != 'G' {
			t.Fatalf("embedded %s is not a valid PNG", c.name)
		}
	}
}

// The notice row is how a Linux desktop with no permission dialogs says so, and
// it is also the row that must not appear anywhere else. An empty notice adding
// a blank disabled row would put one at the top of every menu on every platform.
func TestNoticeRowAppearsOnlyWhenThereIsSomethingToSay(t *testing.T) {
	if item := (&Tray{}).addNotice(); item != nil {
		t.Errorf("an empty notice added a row: %s", item)
	}

	const notice = "HTML Clay cannot show permission dialogs on this desktop. Install zenity or kdialog."
	item := (&Tray{notice: notice}).addNotice()
	if item == nil {
		t.Fatal("a notice must get a row of its own")
	}
	if !strings.Contains(item.String(), notice) {
		t.Errorf("the row does not carry the notice: %s", item)
	}
	if !item.Disabled() {
		t.Error("the notice row must be disabled; there is nothing behind it to click")
	}
}

// Settings is one plain row that opens the Settings pages: no submenu.
func TestSettingsIsOnePlainRow(t *testing.T) {
	top, children := planRows([]MenuItem{
		{ID: appcore.MenuStatus, Title: "HTML Clay 2.0"},
		{ID: appcore.MenuSettings, Title: "Settings…", Enabled: true},
		{ID: appcore.MenuQuit, Title: "Exit", Enabled: true},
	})
	if s := top[appcore.MenuSettings]; !s.visible || !s.enabled || s.title != "Settings…" {
		t.Fatalf("settings row = %+v", s)
	}
	if _, ok := children[appcore.MenuSettings]; ok {
		t.Fatal("settings has child slots; it must be a plain row")
	}
	if s := top[appcore.MenuStatus]; !s.visible || s.enabled {
		t.Fatalf("status row = %+v, want shown and disabled", s)
	}
	for _, id := range []string{appcore.MenuSyncLine, appcore.MenuPauseSync, appcore.MenuNeedsYou, appcore.MenuRecovered} {
		if top[id].visible {
			t.Errorf("%s shows though the host left it out", id)
		}
	}
}

// A submenu shows its children in order on its slots and hides the rest, and
// never shows more than it has slots for.
func TestChildrenFillTheirSlotsAndTheRestHide(t *testing.T) {
	recent := MenuItem{ID: appcore.MenuRecent, Title: "Recent documents", Enabled: true}
	for _, name := range []string{"a.htmlclay", "b.htmlclay", "c.htmlclay"} {
		recent.Children = append(recent.Children, MenuItem{ID: name, Title: name, Enabled: true})
	}
	needs := MenuItem{ID: appcore.MenuNeedsYou, Title: "Needs you (10)", Enabled: true}
	for i := 0; i < 10; i++ {
		needs.Children = append(needs.Children, MenuItem{ID: "w", Title: "item", Enabled: true})
	}
	_, children := planRows([]MenuItem{recent, needs})

	got := children[appcore.MenuRecent]
	if len(got) != 9 {
		t.Fatalf("recent has %d slots, want 9", len(got))
	}
	for i, s := range got {
		if want := i < 3; s.visible != want {
			t.Errorf("recent slot %d visible = %v, want %v", i, s.visible, want)
		}
	}
	if got[1].title != "b.htmlclay" {
		t.Errorf("recent slot 1 = %q, want b.htmlclay", got[1].title)
	}
	shown := 0
	for _, s := range children[appcore.MenuNeedsYou] {
		if s.visible {
			shown++
		}
	}
	if shown != 8 {
		t.Errorf("needs you shows %d rows, want 8", shown)
	}
}

// The menu is built again when its rows or lines change, as when sync connects
// and a line and two rows appear under the status row, and never for a new
// title, enabled state or child: a rebuild closes a menu the person has open.
func TestTheMenuRebuildsOnlyWhenItsShapeChanges(t *testing.T) {
	sep := MenuItem{ID: appcore.MenuSeparator}
	base := func(status string, recent ...MenuItem) []MenuItem {
		return []MenuItem{
			{ID: appcore.MenuStatus, Title: status}, sep,
			{ID: appcore.MenuOpenDashboard, Title: "Open dashboard", Enabled: true},
			{ID: appcore.MenuRecent, Title: "Recent documents", Enabled: true, Children: recent}, sep,
			{ID: appcore.MenuQuit, Title: "Exit", Enabled: true},
		}
	}
	before := base("HTML Clay 2.0")
	retitled := base("Checking for updates…", MenuItem{ID: "d1", Title: "a.htmlclay", Enabled: true})
	if !slices.Equal(menuShape(before), menuShape(retitled)) {
		t.Fatalf("a new title and a new Recent document changed the shape: %v then %v", menuShape(before), menuShape(retitled))
	}
	synced := append(base("HTML Clay 2.0")[:2:2],
		MenuItem{ID: appcore.MenuSyncLine, Title: "Up to date"}, MenuItem{ID: appcore.MenuPauseSync, Title: "Pause syncing", Enabled: true}, sep)
	synced = append(synced, base("HTML Clay 2.0")[2:]...)
	if slices.Equal(menuShape(before), menuShape(synced)) {
		t.Fatal("sync connecting left the shape alone, so the menu would keep its old lines")
	}
	top, _ := planRows(synced)
	if s := top[appcore.MenuSyncLine]; !s.visible || s.title != "Up to date" {
		t.Fatalf("sync line = %+v", s)
	}
}

// While sync works the host's rows change every few seconds but most items do
// not: a poll that changes nothing must touch nothing, or Linux closes an open
// Recent submenu and Windows logs an error for every hidden slot.
func TestAnUnchangedPollTouchesNoItem(t *testing.T) {
	shown := map[string]slotState{}
	calls := 0
	apply := func(key string, s slotState) { applyChanged(shown, key, s, func() { calls++ }) }
	status := slotState{visible: true, title: "Syncing 3 files", enabled: false}
	apply("status", status)
	apply("recent-0", slotState{})
	apply("status", status)
	apply("recent-0", slotState{})
	if calls != 2 {
		t.Fatalf("two polls with the same rows touched items %d times, want 2 (the first poll only)", calls)
	}
	apply("status", slotState{visible: true, title: "Up to date", enabled: false})
	if calls != 3 {
		t.Fatalf("a new status title touched items %d times in all, want 3", calls)
	}
}

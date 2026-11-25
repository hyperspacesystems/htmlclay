package panel

import (
	"slices"
	"testing"
	"time"
)

// Two monitors: the main one at 100% with the taskbar along the bottom and the
// notification area at its right end, and one to its right at 150% whose
// taskbar has no notification area.
var (
	mainScreen = Screen{Monitor: Rect{0, 0, 1920, 1080}, Work: Rect{0, 0, 1920, 1032}, DPI: 96,
		Tray: Rect{1700, 1032, 1900, 1080}}
	sideScreen = Screen{Monitor: Rect{1920, 0, 4800, 1620}, Work: Rect{1920, 0, 4800, 1548}, DPI: 144}
)

type fakeWindow struct {
	screens []Screen
	calls   []string
	shows   []Rect
	moves   []Rect
}

func (w *fakeWindow) ScreenAt(p Point) Screen {
	for _, s := range w.screens {
		if s.Monitor.contains(p) {
			return s
		}
	}
	return w.screens[0]
}

func (w *fakeWindow) Show(r Rect)      { w.calls = append(w.calls, "show"); w.shows = append(w.shows, r) }
func (w *fakeWindow) Move(r Rect)      { w.calls = append(w.calls, "move"); w.moves = append(w.moves, r) }
func (w *fakeWindow) Hide()            { w.calls = append(w.calls, "hide") }
func (w *fakeWindow) AllowForeground() { w.calls = append(w.calls, "allow") }
func (w *fakeWindow) Navigate(url string) {
	w.calls = append(w.calls, "navigate "+url)
}

type childTest struct {
	*Child
	window *fakeWindow
	events []string
	clock  time.Time
}

func newChildTest(screens ...Screen) *childTest {
	ct := &childTest{window: &fakeWindow{screens: screens}, clock: time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)}
	ct.Child = NewChild(ct.window, func(e Event) { ct.events = append(ct.events, e.Event) }, func() time.Time { return ct.clock })
	return ct
}

func (ct *childTest) at(d time.Duration) *childTest {
	ct.clock = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC).Add(d)
	return ct
}

func toggle(x, y int) Command { return Command{Cmd: CmdToggle, X: x, Y: y} }

// The spike placed the panel past the bottom of the screen when the click point
// and the panel's size were in different pixels; a keyboard opening put it
// wherever the pointer was.
func TestThePanelOpensBesideTheTaskbarInsideTheWorkArea(t *testing.T) {
	left := Screen{Monitor: Rect{0, 0, 1920, 1080}, Work: Rect{48, 0, 1920, 1080}, DPI: 96}
	hidden := Screen{Monitor: Rect{0, 0, 1920, 1080}, Work: Rect{0, 0, 1920, 1080}, DPI: 96, Taskbar: EdgeTop}
	for _, c := range []struct {
		name    string
		screens []Screen
		x, y    int
		want    Rect
	}{
		{"a click on the tray icon near the right edge", []Screen{mainScreen, sideScreen}, 1800, 1050, Rect{1526, 460, 1908, 1020}},
		{"the keyboard, with the pointer mid-screen", []Screen{mainScreen, sideScreen}, 500, 500, Rect{1526, 460, 1908, 1020}},
		{"a click on the 150% monitor's taskbar", []Screen{mainScreen, sideScreen}, 3000, 1580, Rect{2714, 696, 3287, 1536}},
		{"a taskbar on the left, clicked near the bottom", []Screen{left}, 20, 1000, Rect{60, 508, 442, 1068}},
		{"a taskbar at the top that hides itself", []Screen{hidden}, 900, 2, Rect{709, 12, 1091, 572}},
	} {
		t.Run(c.name, func(t *testing.T) {
			ct := newChildTest(c.screens...)
			ct.Command(toggle(c.x, c.y))
			if !slices.Equal(ct.window.shows, []Rect{c.want}) {
				t.Fatalf("shown at %v, want %v", ct.window.shows, c.want)
			}
		})
	}
}

func TestAClickOnTheTrayIconWhileThePanelShowsLeavesItHidden(t *testing.T) {
	ct := newChildTest(mainScreen)
	ct.Command(toggle(1800, 1050))
	ct.at(3 * time.Second).Deactivated()
	ct.at(3*time.Second + 100*time.Millisecond).Command(toggle(1800, 1050))
	if want := []string{"show", "hide"}; !slices.Equal(ct.window.calls, want) {
		t.Fatalf("the click that took the focus away showed the panel again: %v, want %v", ct.window.calls, want)
	}
	ct.at(4 * time.Second).Command(toggle(1800, 1050))
	ct.at(5 * time.Second).Page(PageClose)
	ct.at(6 * time.Second).Command(toggle(1800, 1050))
	ct.at(7 * time.Second).Command(toggle(1800, 1050))
	if want := []string{"show", "hide", "show", "hide", "show", "hide"}; !slices.Equal(ct.window.calls, want) {
		t.Fatalf("calls = %v, want %v", ct.window.calls, want)
	}
	if want := []string{EventHidden, EventHidden, EventHidden}; !slices.Equal(ct.events, want) {
		t.Fatalf("events = %v, want %v", ct.events, want)
	}
}

// A double click on the tray icon, or a second click while a cold child is
// still starting, is two toggles: the panel shows and stays shown.
func TestADoubleClickOrAnImpatientSecondClickLeavesThePanelShown(t *testing.T) {
	cold := newChildTest(mainScreen)
	cold.Command(toggle(1800, 1050))
	cold.Command(toggle(1800, 1050))
	if want := []string{"show"}; !slices.Equal(cold.window.calls, want) {
		t.Fatalf("two clicks queued while the child started: %v, want %v", cold.window.calls, want)
	}

	warm := newChildTest(mainScreen)
	warm.Command(toggle(1800, 1050))
	warm.at(150 * time.Millisecond).Deactivated()
	warm.at(200 * time.Millisecond).Command(toggle(1800, 1050))
	if want := []string{"show", "hide", "show"}; !slices.Equal(warm.window.calls, want) {
		t.Fatalf("a double click on a warm panel: %v, want %v", warm.window.calls, want)
	}
}

func TestThePageSetsItsHeightAndTheBottomEdgeStays(t *testing.T) {
	ct := newChildTest(mainScreen)
	ct.Page("height:300")
	ct.Command(toggle(1800, 1050))
	for _, message := range []string{"height:400", "height:99999", "height:50", "height:x", "height:-3", "width:9"} {
		ct.Page(message)
	}
	if want := []Rect{{1526, 720, 1908, 1020}}; !slices.Equal(ct.window.shows, want) {
		t.Fatalf("shown at %v, want %v", ct.window.shows, want)
	}
	want := []Rect{{1526, 620, 1908, 1020}, {1526, 12, 1908, 1020}, {1526, 860, 1908, 1020}}
	if !slices.Equal(ct.window.moves, want) {
		t.Fatalf("moved to %v, want %v", ct.window.moves, want)
	}
}

func TestThePagesReenterGoesToTheHost(t *testing.T) {
	ct := newChildTest(mainScreen)
	ct.Page(PageReenter)
	if want := []string{EventReenter}; !slices.Equal(ct.events, want) {
		t.Fatalf("events = %v, want %v", ct.events, want)
	}
}

// Before an action, the window that has the person's click lets the next
// window take the front: without it a tab the app opens appears behind.
func TestThePagesAllowLetsTheNextWindowComeToTheFront(t *testing.T) {
	ct := newChildTest(mainScreen)
	ct.Page(PageAllow)
	if want := []string{"allow"}; !slices.Equal(ct.window.calls, want) {
		t.Fatalf("calls = %v, want %v", ct.window.calls, want)
	}
}

func TestTheChildExitsAfterTenMinutesHidden(t *testing.T) {
	ct := newChildTest(mainScreen)
	ct.at(time.Minute).Command(toggle(1800, 1050))
	if ct.at(15 * time.Minute).Tick() {
		t.Fatal("the child exited while the panel showed")
	}
	ct.Deactivated()
	if ct.at(24 * time.Minute).Tick() {
		t.Fatal("the child exited 9 minutes after the panel hid")
	}
	if !ct.at(25 * time.Minute).Tick() {
		t.Fatal("the child did not exit 10 minutes after the panel hid")
	}
	if want := []string{EventHidden, EventExit}; !slices.Equal(ct.events, want) {
		t.Fatalf("events = %v, want %v", ct.events, want)
	}
}

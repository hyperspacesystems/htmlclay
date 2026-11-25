package panel

import (
	"strconv"
	"strings"
	"time"
)

const (
	// reshowGuard is how long a toggle leaves the panel as it is. After a hide:
	// a click on the tray icon while the panel shows first takes the focus away,
	// which hides it, and the click's toggle must not show it again. After a
	// show: a double click, or a second click while a cold child starts, is two
	// toggles, and the second must not hide what the first showed.
	reshowGuard = 500 * time.Millisecond
	// IdleExit is how long the child stays hidden before it exits to give back
	// WebView2's memory; the next click starts it again.
	IdleExit = 10 * time.Minute
)

// The page's messages to the window, which WebView2 passes as strings.
const (
	PageClose   = "close"
	PageReenter = "reenter"
	// PageAllow comes before every action the page sends the app: the window,
	// which has the person's click, lets the next window to open take the
	// front, so a tab or File Explorer the app opens does not open behind.
	PageAllow  = "allow"
	pageHeight = "height:"
)

// Screen is what the window reads about the monitor at a point, in physical
// pixels: the whole monitor, its work area, its DPI, the taskbar's side as the
// system reports it, and the notification area (empty when it was not found).
type Screen struct {
	Monitor Rect
	Work    Rect
	DPI     uint32
	Taskbar Edge
	Tray    Rect
}

// Window is the child's native window. Child calls it on the window's thread.
type Window interface {
	ScreenAt(p Point) Screen
	Show(r Rect)
	Move(r Rect)
	Hide()
	Navigate(url string)
	AllowForeground()
}

// Child is what the panel's child does with each host command, page message,
// loss of focus and timer tick. It runs on the window's thread.
type Child struct {
	window   Window
	say      func(Event)
	now      func() time.Time
	shown    bool
	shownAt  time.Time
	hiddenAt time.Time
	idleFrom time.Time
	anchor   Point
	height   int32
}

// NewChild starts hidden, with the page at its default height.
func NewChild(w Window, say func(Event), now func() time.Time) *Child {
	return &Child{window: w, say: say, now: now, idleFrom: now(), height: DefaultHeight}
}

// Command runs one line from the host.
func (c *Child) Command(cmd Command) {
	switch cmd.Cmd {
	case CmdNavigate:
		c.window.Navigate(cmd.URL)
	case CmdHide:
		c.hide()
	case CmdToggle:
		now := c.now()
		switch {
		case c.shown:
			if now.Sub(c.shownAt) >= reshowGuard {
				c.hide()
			}
		case now.Sub(c.hiddenAt) >= reshowGuard,
			// A double click's second press took the focus from a panel that
			// had only just shown: its toggle shows it again.
			c.hiddenAt.Sub(c.shownAt) < reshowGuard:
			c.show(Point{int32(cmd.X), int32(cmd.Y)})
		}
	}
}

// Page runs one message from the page.
func (c *Child) Page(message string) {
	switch {
	case message == PageClose:
		c.hide()
	case message == PageReenter:
		c.say(Event{Event: EventReenter})
	case message == PageAllow:
		c.window.AllowForeground()
	case strings.HasPrefix(message, pageHeight):
		n, err := strconv.Atoi(strings.TrimPrefix(message, pageHeight))
		if err != nil || n <= 0 {
			return
		}
		c.height = max(int32(min(n, tallestPage)), MinHeight)
		if c.shown {
			c.window.Move(c.place(c.window.ScreenAt(c.anchor)))
		}
	}
}

// Deactivated hides the panel when another window takes the focus.
func (c *Child) Deactivated() { c.hide() }

// Tick reports whether the child has been hidden long enough to exit, and says
// why when it has.
func (c *Child) Tick() bool {
	if c.shown || c.now().Sub(c.idleFrom) < IdleExit {
		return false
	}
	c.say(Event{Event: EventExit, Detail: "hidden for " + IdleExit.String()})
	return true
}

func (c *Child) show(click Point) {
	s := c.window.ScreenAt(click)
	c.anchor = Anchor(click, s.Work, s.Tray)
	if c.anchor != click {
		s = c.window.ScreenAt(c.anchor)
	}
	c.window.Show(c.place(s))
	c.shown = true
	c.shownAt = c.now()
}

func (c *Child) place(s Screen) Rect {
	h := min(Scale(c.height, s.DPI), s.Work.Bottom-s.Work.Top-2*panelGap)
	return Place(c.anchor, s.Work, EdgeOf(s.Monitor, s.Work, s.Taskbar), Scale(PanelWidth, s.DPI), h)
}

func (c *Child) hide() {
	if !c.shown {
		return
	}
	c.window.Hide()
	c.shown = false
	c.hiddenAt = c.now()
	c.idleFrom = c.hiddenAt
	c.say(Event{Event: EventHidden})
}

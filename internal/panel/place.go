package panel

// Rect is a screen rectangle in physical pixels, the way Win32 returns one.
type Rect struct{ Left, Top, Right, Bottom int32 }

// Point is a screen point in physical pixels.
type Point struct{ X, Y int32 }

func (r Rect) contains(p Point) bool {
	return p.X >= r.Left && p.X < r.Right && p.Y >= r.Top && p.Y < r.Bottom
}

func (r Rect) empty() bool { return r.Right <= r.Left || r.Bottom <= r.Top }

func (r Rect) center() Point { return Point{(r.Left + r.Right) / 2, (r.Top + r.Bottom) / 2} }

// Edge is the side of the monitor the taskbar is on.
type Edge int

const (
	EdgeBottom Edge = iota
	EdgeTop
	EdgeLeft
	EdgeRight
)

const (
	// PanelWidth and DefaultHeight are the panel's size in CSS pixels before the
	// page says how tall it is; MinHeight is the shortest it gets.
	PanelWidth    = 382
	DefaultHeight = 560
	MinHeight     = 160
	panelGap      = 12
	tallestPage   = 10000
)

// Place puts a w by h panel beside the taskbar edge, centred on the anchor and
// kept inside the monitor's work area.
func Place(anchor Point, work Rect, edge Edge, w, h int32) Rect {
	clamp := func(v, lo, hi int32) int32 { return max(lo, min(v, hi)) }
	var x, y int32
	switch edge {
	case EdgeBottom:
		x, y = anchor.X-w/2, work.Bottom-h-panelGap
	case EdgeTop:
		x, y = anchor.X-w/2, work.Top+panelGap
	case EdgeLeft:
		x, y = work.Left+panelGap, anchor.Y-h/2
	case EdgeRight:
		x, y = work.Right-w-panelGap, anchor.Y-h/2
	}
	x = clamp(x, work.Left+panelGap, work.Right-w-panelGap)
	y = clamp(y, work.Top+panelGap, work.Bottom-h-panelGap)
	return Rect{x, y, x + w, y + h}
}

// Scale turns a size in CSS pixels into physical pixels for a monitor's DPI.
func Scale(v int32, dpi uint32) int32 { return int32((int64(v)*int64(dpi) + 48) / 96) }

// EdgeOf is the taskbar's side of a monitor: the side its work area gives up.
// A taskbar that hides itself gives up nothing, so the system's own answer
// (fallback) decides then.
func EdgeOf(monitor, work Rect, fallback Edge) Edge {
	switch {
	case work.Bottom < monitor.Bottom:
		return EdgeBottom
	case work.Top > monitor.Top:
		return EdgeTop
	case work.Left > monitor.Left:
		return EdgeLeft
	case work.Right < monitor.Right:
		return EdgeRight
	}
	return fallback
}

// Anchor is where the panel opens from. A click on the tray icon is on the
// taskbar, outside the work area, and the panel opens there. Opening it from
// the keyboard (Windows+B, then Enter) reports wherever the pointer happens to
// be, so then the panel opens from the notification area.
func Anchor(click Point, work, tray Rect) Point {
	if !work.contains(click) || tray.empty() {
		return click
	}
	return tray.center()
}

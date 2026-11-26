package app

import "github.com/nsgo-dev/nsgo/core"

// sizeClass is Material 3's window size class, by the window's width. A
// compact window is a phone's: a navigation bar, and pages over the
// destinations. A medium one moves the destinations to a navigation rail.
// An expanded one shows the destinations as a list pane and the pages beside
// them, as Material 3's list-detail layout does.
type sizeClass int

const (
	compact sizeClass = iota
	medium
	expanded
)

func classOf(width core.Points) sizeClass {
	switch {
	case width >= 840:
		return expanded
	case width >= 600:
		return medium
	}
	return compact
}

const (
	// railWidth is the navigation rail's.
	railWidth core.Points = 80
	// pageMaxWidth keeps a page's lines short enough to read in a wide pane.
	pageMaxWidth core.Points = 760
	// dialogWidth is the width of what a compact window shows as a bottom
	// sheet, shown as a dialog in a wider one.
	dialogWidth core.Points = 560
)

// listPaneWidth is the width of the destinations beside the pages in an
// expanded window of width.
func listPaneWidth(width core.Points) core.Points {
	return min(max((width-railWidth)*0.4, 360), 480)
}

// newSize follows the window's size class. iPhone's native page stack lays
// its pages over the whole window, so the app stays compact there.
func (a *App) newSize(cx *core.Scope) *core.Memo[sizeClass] {
	env := cx.Env()
	return core.NewMemo(cx, func() sizeClass {
		if a.nav.native() {
			return compact
		}
		return classOf(env.Window().Width)
	})
}

// wide reports whether the window is wider than a phone's.
func (a *App) wide() bool { return a.size.Get() != compact }

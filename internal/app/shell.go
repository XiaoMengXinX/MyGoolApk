package app

import (
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// The shell animates as Material 3 asks: a section fades through when the
// destination changes, and the bar's indicator grows from its center.
var (
	// emphasized is Material 3's emphasized decelerate curve.
	emphasized  = core.Curve{X1: 0.05, Y1: 0.7, X2: 0.1, Y2: 1}
	fadeThrough = core.Transition{Duration: 250 * time.Millisecond, Easing: emphasized, Properties: core.TransitionOpacity | core.TransitionTransform}
)

// navBarHeight is the height of the navigation bar above the system's:
// the 64dp Google Play draws, with a 56x32 indicator. On iOS the bar sits
// over the home indicator's 34 points, as the system's tab bar does, and
// keeps to 54, near the system's 49.
const navBarHeight, navBarHeightIOS core.Points = 64, 54

// navBar is the height of the navigation bar on this platform, and the
// room above its indicator and below it, over its label.
func (a *App) navBar() (height, top, gap core.Points) {
	if a.cx.Env().OS() == core.OSIOS {
		return navBarHeightIOS, 4, 2
	}
	return navBarHeight, 6, 4
}

type destination struct {
	title           string
	outline, filled *symbol
	content         func(cx *core.Scope) core.Node
	// unread counts what waits behind the destination, nil for none.
	unread func() int
}

// shell is the app's four top-level destinations over a navigation bar.
// A destination is built when first selected, and kept while others show.
func (a *App) shell(cx *core.Scope, dests []destination) core.Node {
	sections := make([]core.Node, len(dests))
	for i, d := range dests {
		i, d := i, d
		seen := false
		on := func() bool { return a.tab.Get() == i }
		// away is set once a destination has faded out: it then waits a
		// screen to the right, where no touch reaches it. Faded out in place,
		// Android still gave the sideways drags over the shown destination to
		// the tabs of a later one (the discovery's over the home's).
		away := core.NewSignal(cx, false)
		sections[i] = core.View{
			Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Bottom: core.Pt(0), Transition: fadeThrough},
			StyleFunc: func() core.Style {
				var s core.Style
				if on() {
					s = core.Style{Opacity: core.Alpha(1)}
				} else {
					s = core.Style{Opacity: core.Alpha(0), PointerEvents: core.PointerEventsNone, Transform: core.Transform{Scale: 0.96}}
				}
				if away.Get() && !on() {
					s.Left, s.Right = core.Pct(100), core.Pct(-100)
				} else {
					s.Left, s.Right = core.Pt(0), core.Pt(0)
				}
				return s
			},
			OnTransitionEnd: func() { away.Set(!on()) },
			Children: []core.Node{core.Show{
				When: func() bool { seen = seen || on(); return seen },
				Then: d.content,
			}},
		}
	}
	items := make([]core.Node, len(dests))
	for i, d := range dests {
		items[i] = a.navItem(i, d, false)
	}
	barHeight, _, _ := a.navBar()
	return column(fill, "bg-background",
		core.View{Style: fill, Children: sections},
		core.Show{When: func() bool { return !a.wide() }, Then: func(*core.Scope) core.Node {
			return row(core.Style{Padding: core.Symmetric(0, 8), FlexShrink: 0, PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}}, "bg-surface-container",
				core.View{Style: core.Style{Direction: core.Row, Height: core.Pt(barHeight), FlexGrow: 1}, Role: core.RoleTabList, Label: "酷安", Children: items})
		}},
	)
}

// rail is Material 3's navigation rail: the destinations of a wide window,
// stacked at its left.
func (a *App) rail(dests []destination) core.Node {
	items := make([]core.Node, len(dests))
	for i, d := range dests {
		items[i] = a.navItem(i, d, true)
	}
	return core.View{
		Role: core.RoleTabList, Label: "酷安", Class: "bg-surface-container",
		Style: core.Style{Width: core.Pt(railWidth), FlexShrink: 0, Padding: core.Edges{Top: 16}, Gap: 12, AlignItems: core.AlignCenter,
			PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea, Left: core.EnvSafeArea, Bottom: core.EnvSafeArea}},
		Children: items,
	}
}

// navItem is a destination of the bar or the rail: its icon in a 56x32 pill
// that grows from the center when the destination is selected, over its
// label.
func (a *App) navItem(i int, d destination, rail bool) core.Node {
	_, top, gap := a.navBar()
	size := core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Height: core.Pct(100)}
	if rail {
		top, gap = 0, 4
		size = core.Style{Width: core.Pt(railWidth)}
	}
	on := func() bool { return a.tab.Get() == i }
	selected := core.NewMemo(a.cx, on)
	pill := core.View{
		Class: "bg-secondary-container",
		Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), CornerRadius: 16,
			Transition: fadeThrough},
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{Opacity: core.Alpha(1), Transform: core.Transform{ScaleX: 1}}
			}
			return core.Style{Opacity: core.Alpha(0), Transform: core.Transform{ScaleX: 0.3}}
		},
	}
	glyph := core.Dynamic(func(cx *core.Scope) core.Node {
		if selected.Get() {
			return icon(d.filled, 24, "text-on-secondary-container")
		}
		return icon(d.outline, 24, "text-on-surface-variant")
	})
	return pressable(core.View{
		Role: core.RoleTab, Label: d.title,
		OnPress: func(core.PressEvent) { a.selectDestination(i) },
		Style:   size.Merge(core.Style{Padding: core.Edges{Top: top}, Gap: gap, AlignItems: core.AlignCenter, Overflow: core.Hidden}),
		Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(56), Height: core.Pt(32), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				Children: []core.Node{pill, glyph,
					core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(32)}, PointerEvents: core.PointerEventsNone,
						Children: []core.Node{badge(func() int {
							if d.unread == nil {
								return 0
							}
							return d.unread()
						})}}}},
			core.Text{Content: d.title, MaxLines: 1, Style: textStyle(textCaption, 500), ClassFunc: func() string {
				if on() {
					return "text-on-surface"
				}
				return "text-on-surface-variant"
			}},
		},
	})
}

// selectDestination shows destination i, or its first row when it is shown.
// Pages over the destinations of a medium window go, as the rail beside them
// leads elsewhere.
func (a *App) selectDestination(i int) {
	if a.size.GetUntracked() == medium {
		a.nav.Clear()
	}
	if a.tab.GetUntracked() == i {
		a.scrollDestinationToTop(i)
		return
	}
	a.tab.Set(i)
	a.refreshUnread()
}

// scrollDestinationToTop shows the first row of the destination's list: the
// bar's tap on the destination already shown.
func (a *App) scrollDestinationToTop(i int) {
	if g := a.destinationTabs(i); g != nil {
		g.scrollToTop()
	}
}

// refreshDestination loads the shown destination's list again.
func (a *App) refreshDestination() {
	if g := a.destinationTabs(a.tab.GetUntracked()); g != nil {
		g.refresh()
		return
	}
	a.refreshUnread()
}

// destinationTabs is the tabs of destination i, nil for one without.
func (a *App) destinationTabs(i int) *tabGroup {
	g := []*tabGroup{a.home, a.digital, a.discovery}
	if i < len(g) {
		return g[i]
	}
	return nil
}

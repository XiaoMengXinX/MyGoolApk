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
// the 64dp Google Play draws, with a 56x32 indicator.
const navBarHeight core.Points = 64

type destination struct {
	title           string
	outline, filled *symbol
	content         func(cx *core.Scope) core.Node
}

// shell is the app's four top-level destinations over a navigation bar.
// A destination is built when first selected, and kept while others show.
func (a *App) shell(cx *core.Scope, dests []destination) core.Node {
	sections := make([]core.Node, len(dests))
	for i, d := range dests {
		i, d := i, d
		seen := false
		on := func() bool { return a.tab.Get() == i }
		sections[i] = core.View{
			Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Transition: fadeThrough},
			StyleFunc: func() core.Style {
				if on() {
					return core.Style{Opacity: core.Alpha(1)}
				}
				return core.Style{Opacity: core.Alpha(0), PointerEvents: core.PointerEventsNone, Transform: core.Transform{Scale: 0.96}}
			},
			Children: []core.Node{core.Show{
				When: func() bool { seen = seen || on(); return seen },
				Then: d.content,
			}},
		}
	}
	items := make([]core.Node, len(dests))
	for i, d := range dests {
		items[i] = a.navItem(i, d)
	}
	return column(fill, "bg-background",
		core.View{Style: fill, Children: sections},
		row(core.Style{Padding: core.Symmetric(0, 8), FlexShrink: 0, PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}}, "bg-surface-container",
			core.View{Style: core.Style{Direction: core.Row, Height: core.Pt(navBarHeight), FlexGrow: 1}, Role: core.RoleTabList, Label: "酷安", Children: items}),
	)
}

// navItem is a destination of the bar: its icon in a 56x32 pill that grows
// from the center when the destination is selected, over its label.
func (a *App) navItem(i int, d destination) core.Node {
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
	return core.View{
		Role: core.RoleTab, Label: d.title,
		OnPress: func(core.PressEvent) {
			if a.tab.GetUntracked() == i {
				a.scrollDestinationToTop(i)
				return
			}
			a.tab.Set(i)
		},
		Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Height: core.Pct(100), Padding: core.Edges{Top: 6, Bottom: 6}, Gap: 4,
			AlignItems: core.AlignCenter, Overflow: core.Hidden},
		Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(56), Height: core.Pt(32), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				Children: []core.Node{pill, glyph}},
			core.Text{Content: d.title, MaxLines: 1, Style: textStyle(textCaption, 500), ClassFunc: func() string {
				if on() {
					return "text-on-surface"
				}
				return "text-on-surface-variant"
			}},
		},
	}
}

// scrollDestinationToTop shows the first row of the destination's list: the
// bar's tap on the destination already shown.
func (a *App) scrollDestinationToTop(i int) {
	g := []*tabGroup{a.home, a.digital, a.discovery}
	if i < len(g) {
		g[i].scrollToTop()
	}
}

package app

import (
	"slices"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// Pages pushed over the shell are NSGo's native screen stack on an iPhone,
// where UINavigationController slides them and its edge swipe pops them.
//
// Elsewhere the pages are layers of the app's own. Android's native stack
// shows a pushed screen at once and hides the one it covers, and a wide
// window shows the pages beside the destinations rather than over them. Each
// layer slides in from the right and out again, and what it covers, the shell
// or a page, shifts a little. A covered page stays laid out, which keeps its
// lists' places.
const pageMotion = 300 * time.Millisecond

var pageTransition = core.Transition{Duration: pageMotion, Easing: emphasized, Properties: core.TransitionOpacity | core.TransitionTransform}

// shellShift is how far the shell, or a page, moves under a page, in percent
// of its width.
const shellShift = -22

type route struct{ kind, id string }

// pageLayer is a pushed page of the app's own stack: it comes in as shown
// turns true and goes out as leaving does.
type pageLayer struct {
	id             int
	route          route
	shown, leaving *core.Signal[bool]
}

type pageStack struct {
	a   *App
	cx  *core.Scope
	nav *base.Nav[route] // an iPhone's
	// layers are the app's own pages, the top one last.
	layers *core.Signal[[]*pageLayer]
	nextID int
	// fromList is set while a press in an expanded window began on the
	// destinations: a page it opens takes the place of those shown.
	fromList bool
}

func (a *App) newPageStack(cx *core.Scope) *pageStack {
	return &pageStack{a: a, cx: cx, nav: base.NewNav(cx, route{}), layers: core.NewSignalFunc(cx, []*pageLayer(nil), nil)}
}

// native reports whether the pages are the platform's own stack.
func (s *pageStack) native() bool {
	env := s.cx.Env()
	return env.OS() == core.OSIOS && env.Idiom() == core.IdiomPhone
}

// top is the topmost layer not on its way out, with its index, or -1.
func (s *pageStack) top(layers []*pageLayer, leaving func(*pageLayer) bool) (int, *pageLayer) {
	for i := len(layers) - 1; i >= 0; i-- {
		if !leaving(layers[i]) {
			return i, layers[i]
		}
	}
	return -1, nil
}

// Depth is how many pages are over the shell, without those on their way
// out.
func (s *pageStack) Depth() int {
	if s.native() {
		return s.nav.Depth()
	}
	n := 0
	for _, l := range s.layers.Get() {
		if !l.leaving.Get() {
			n++
		}
	}
	return n
}

// Push shows a page over the others. In an expanded window a page opened
// from the destinations takes the place of the pages shown beside them.
func (s *pageStack) Push(r route) {
	if s.native() {
		s.nav.Push(r)
		return
	}
	if s.fromList && s.a.size.GetUntracked() == expanded {
		s.Clear()
	}
	// The stack owns the layer and its timer: a push from a sheet that
	// closes as it pushes would otherwise take them away with the sheet.
	s.cx.Run(func() {
		s.nextID++
		l := &pageLayer{id: s.nextID, route: r, shown: core.NewSignal(s.cx, false), leaving: core.NewSignal(s.cx, false)}
		s.layers.Set(append(slices.Clone(s.layers.GetUntracked()), l))
		// The page comes in on the next frame, after it was laid out in place.
		s.cx.AfterFunc(16*time.Millisecond, func() { l.shown.Set(true) })
	})
}

// Pop takes the top page away. iOS animates a pop itself; the app's own
// page slides out first.
func (s *pageStack) Pop() bool {
	if s.native() {
		if s.nav.Depth() == 0 {
			return false
		}
		s.nav.Pop(1)
		return true
	}
	_, l := s.top(s.layers.GetUntracked(), func(l *pageLayer) bool { return l.leaving.GetUntracked() })
	if l == nil {
		return false
	}
	s.leave(l)
	return true
}

// Clear takes every page away.
func (s *pageStack) Clear() {
	if s.native() {
		s.nav.Pop(s.nav.Depth())
		return
	}
	for _, l := range s.layers.GetUntracked() {
		if !l.leaving.GetUntracked() {
			s.leave(l)
		}
	}
}

func (s *pageStack) leave(l *pageLayer) {
	l.leaving.Set(true)
	s.cx.Run(func() {
		s.cx.AfterFunc(pageMotion+30*time.Millisecond, func() {
			s.layers.Set(slices.DeleteFunc(slices.Clone(s.layers.GetUntracked()), func(x *pageLayer) bool { return x == l }))
		})
	})
}

// covers reports whether a page not on its way out is over layer i, or over
// the shell for i of -1, which an expanded window shows beside the pages.
func (s *pageStack) covers(i int) bool {
	if i < 0 && s.a.size.Get() == expanded {
		return false
	}
	top, _ := s.top(s.layers.Get(), func(l *pageLayer) bool { return l.leaving.Get() })
	return top > i
}

// view is the stack, the shell at its bottom. The app's own stack keeps one
// tree whatever the window's size, so a resize keeps every page as it was:
// the shell's pane and the pages' pane are side by side in an expanded
// window, and the pages' pane lies over the shell's in a narrower one.
func (s *pageStack) view(cx *core.Scope, shell func(cx *core.Scope) core.Node, page func(cx *core.Scope, r route) core.Node) core.Node {
	if !s.native() {
		env := cx.Env()
		size := s.a.size
		listPane := core.View{
			StyleFunc: func() core.Style {
				if size.Get() == expanded {
					return core.Style{Width: core.Pt(listPaneWidth(env.Window().Width)), FlexShrink: 0}
				}
				return fill
			},
			OnMouseDown:    func(core.MouseButtonEvent) { s.fromList = true },
			OnTouchCapture: func(core.TouchEvent) { s.fromList = true },
			Children:       []core.Node{s.under(func() bool { return s.covers(-1) }, shell(cx), nil)},
		}
		layers := core.Each(s.layers.Get, func(l *pageLayer) int { return l.id }, func(cx *core.Scope, item func() *pageLayer, _ func() int) core.Node {
			l := core.Untracked(cx, item)
			index := func() int { return slices.Index(s.layers.Get(), l) }
			return s.under(func() bool { return s.covers(index()) }, page(cx, l.route), func() core.Style {
				if !l.shown.Get() || l.leaving.Get() {
					return core.Style{Transform: core.Transform{TranslateXPercent: 100}}
				}
				return core.Style{}
			})
		})
		pagePane := core.View{
			StyleFunc: func() core.Style {
				if size.Get() == expanded {
					return fill.Merge(core.Style{Overflow: core.Hidden, Borders: core.Edges{Left: 1}})
				}
				return fillAbs.Merge(core.Style{PointerEvents: core.PointerEventsBoxNone})
			},
			Class:          "border-outline-variant",
			OnMouseDown:    func(core.MouseButtonEvent) { s.fromList = false },
			OnTouchCapture: func(core.TouchEvent) { s.fromList = false },
			Children: []core.Node{
				core.Show{When: func() bool { return size.Get() == expanded && s.Depth() == 0 }, Then: func(*core.Scope) core.Node { return emptyPane() }},
				layers,
			},
		}
		return row(fill, "", listPane, pagePane)
	}
	stack := s.nav.Stack(fill.Merge(core.Style{PointerEvents: core.PointerEventsBoxNone}), func(cx *core.Scope, r route, _ func() int) base.NavPage {
		if r.kind == "" {
			// The shell is under the stack: a screen another covers is hidden and
			// laid out empty, which would scroll its lists back to the top and
			// show it blank as a page slides in over it.
			return base.NavPage{HeaderHidden: true, Style: core.Style{PointerEvents: core.PointerEventsBoxNone}}
		}
		// The image viewer covers the page: iOS's edge swipe, which goes to the
		// stack before any handler, must not pop the page under it.
		viewing := func() bool { return s.a.viewing.Get() != nil }
		return base.NavPage{HeaderHidden: true, Class: "bg-background", NoPopOnSwipeFunc: viewing, Children: []core.Node{page(cx, r)}}
	})
	// iOS's navigation controller slides the pages over it.
	return core.Group{core.View{Class: "bg-background", Style: fillAbs, Children: []core.Node{shell(cx)}}, stack}
}

// emptyPane is the pages' pane of an expanded window while no page is open.
func emptyPane() core.Node {
	return column(fillAbs.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 16}), "bg-background",
		icon(&symbols.Explore, 48, "text-outline"),
		label("选择内容后在这里查看", textSecondary, 400, "text-on-surface-variant"))
}

// under is a layer of the app's own stack: the shell or a page, which
// shifts and dims while covered and takes no touches then. place, when not
// nil, is where the layer is while not covered. A page keeps to
// pageMaxWidth in the middle of a wide pane.
func (s *pageStack) under(covered func() bool, content core.Node, place func() core.Style) core.Node {
	return core.View{
		Class: "bg-background",
		Style: fillAbs.Merge(core.Style{Overflow: core.Hidden, Transition: pageTransition, AlignItems: core.AlignCenter}),
		StyleFunc: func() core.Style {
			if covered() {
				return core.Style{Transform: core.Transform{TranslateXPercent: shellShift}, PointerEvents: core.PointerEventsNone}
			}
			if place != nil {
				return place()
			}
			return core.Style{}
		},
		Children: []core.Node{
			core.View{Style: fill.Merge(core.Style{Width: core.Pct(100), MaxWidth: core.Pt(pageMaxWidth)}), Children: []core.Node{content}},
			core.View{
				Class:     "bg-black/10",
				Style:     fillAbs.Merge(core.Style{Transition: pageTransition, PointerEvents: core.PointerEventsNone}),
				StyleFunc: func() core.Style { return core.Style{Opacity: core.Alpha(shown(covered()))} },
			},
		},
	}
}

func shown(on bool) float32 {
	if on {
		return 1
	}
	return 0
}

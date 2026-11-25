package app

import (
	"slices"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// Pages pushed over the shell are NSGo's native screen stack on iOS, where
// UINavigationController slides them and its edge swipe pops them.
//
// Android's native stack shows a pushed screen at once and hides the one it
// covers, so a page over another had nothing to slide over. There the pages
// are layers of the app's own over the shell: each slides in from the right
// and out again, and what it covers, the shell or a page, shifts a little.
// A covered page stays laid out, which keeps its lists' places.
const pageMotion = 300 * time.Millisecond

var pageTransition = core.Transition{Duration: pageMotion, Easing: emphasized, Properties: core.TransitionOpacity | core.TransitionTransform}

// shellShift is how far the shell, or a page, moves under a page, in percent
// of its width.
const shellShift = -22

type route struct{ kind, id string }

// pageLayer is a pushed page on Android: it comes in as shown turns true and
// goes out as leaving does.
type pageLayer struct {
	id             int
	route          route
	shown, leaving *core.Signal[bool]
}

type pageStack struct {
	a   *App
	cx  *core.Scope
	nav *base.Nav[route] // iOS's
	// layers are Android's pages, the top one last.
	layers *core.Signal[[]*pageLayer]
	nextID int
}

func (a *App) newPageStack(cx *core.Scope) *pageStack {
	return &pageStack{a: a, cx: cx, nav: base.NewNav(cx, route{}), layers: core.NewSignalFunc(cx, []*pageLayer(nil), nil)}
}

func (s *pageStack) android() bool { return s.cx.Env().OS() == core.OSAndroid }

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
	if !s.android() {
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

// Push shows a page over the others.
func (s *pageStack) Push(r route) {
	if !s.android() {
		s.nav.Push(r)
		return
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

// Pop takes the top page away. iOS animates a pop itself; on Android the page
// slides out first.
func (s *pageStack) Pop() bool {
	if !s.android() {
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
	l.leaving.Set(true)
	s.cx.Run(func() {
		s.cx.AfterFunc(pageMotion+30*time.Millisecond, func() {
			s.layers.Set(slices.DeleteFunc(slices.Clone(s.layers.GetUntracked()), func(x *pageLayer) bool { return x == l }))
		})
	})
	return true
}

// covers reports whether a page not on its way out is over layer i, or over
// the shell for i of -1.
func (s *pageStack) covers(i int) bool {
	top, _ := s.top(s.layers.Get(), func(l *pageLayer) bool { return l.leaving.Get() })
	return top > i
}

// view is the stack, the shell at its bottom.
func (s *pageStack) view(cx *core.Scope, shell func(cx *core.Scope) core.Node, page func(cx *core.Scope, r route) core.Node) core.Node {
	if s.android() {
		return core.Group{
			s.under(func() bool { return s.covers(-1) }, shell(cx), nil),
			core.Each(s.layers.Get, func(l *pageLayer) int { return l.id }, func(cx *core.Scope, item func() *pageLayer, _ func() int) core.Node {
				l := core.Untracked(cx, item)
				index := func() int { return slices.Index(s.layers.Get(), l) }
				return s.under(func() bool { return s.covers(index()) }, page(cx, l.route), func() core.Style {
					if !l.shown.Get() || l.leaving.Get() {
						return core.Style{Transform: core.Transform{TranslateXPercent: 100}}
					}
					return core.Style{}
				})
			}),
		}
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

// under is a layer of Android's stack: the shell or a page, which shifts and
// dims while covered and takes no touches then. place, when not nil, is
// where the layer is while not covered.
func (s *pageStack) under(covered func() bool, content core.Node, place func() core.Style) core.Node {
	return core.View{
		Class: "bg-background",
		Style: fillAbs.Merge(core.Style{Overflow: core.Hidden, Transition: pageTransition}),
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
			core.View{Style: fill, Children: []core.Node{content}},
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

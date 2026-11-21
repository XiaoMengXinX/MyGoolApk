package app

import (
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// Pages pushed over the shell are NSGo's native screen stack: on iOS
// UINavigationController slides them and its edge swipe pops them, on Android
// 14 and later the predictive back gesture drags the top page away. Android
// shows a pushed page at once, so its content comes in by itself.
const pageMotion = 300 * time.Millisecond

var pageTransition = core.Transition{Duration: pageMotion, Easing: emphasized, Properties: core.TransitionOpacity | core.TransitionTransform}

type route struct{ kind, id string }

type pageStack struct {
	a   *App
	nav *base.Nav[route]
}

func (a *App) newPageStack(cx *core.Scope) *pageStack {
	return &pageStack{a: a, nav: base.NewNav(cx, route{})}
}

// Depth is how many pages are over the shell.
func (s *pageStack) Depth() int { return s.nav.Depth() }

// Push shows a page over the others.
func (s *pageStack) Push(r route) { s.nav.Push(r) }

// Pop takes the top page away.
func (s *pageStack) Pop() bool {
	if s.nav.Depth() == 0 {
		return false
	}
	s.nav.Pop(1)
	return true
}

// view is the stack, the shell at its bottom.
func (s *pageStack) view(shell func(cx *core.Scope) core.Node, page func(cx *core.Scope, r route) core.Node) core.Node {
	return s.nav.Stack(fill, func(cx *core.Scope, r route, _ func() int) base.NavPage {
		if r.kind == "" {
			return base.NavPage{HeaderHidden: true, Class: "bg-background", Children: []core.Node{shell(cx)}}
		}
		class := "bg-background"
		if r.kind == routeImages {
			class = "" // the viewer draws its own black, which fades as a picture is dragged away
		}
		return base.NavPage{HeaderHidden: true, Class: class, Children: []core.Node{s.entering(cx, page(cx, r))}}
	})
}

// entering slides the page in from the right as it fades in, on Android; iOS's
// navigation controller moves the whole screen itself.
func (s *pageStack) entering(cx *core.Scope, content core.Node) core.Node {
	if s.a.cx.Env().OS() != core.OSAndroid {
		return content
	}
	shown := core.NewSignal(cx, false)
	// The page comes in on the next frame, after it was laid out in place.
	cx.AfterFunc(16*time.Millisecond, func() { shown.Set(true) })
	return core.View{
		Style: fill.Merge(core.Style{Transition: pageTransition}),
		StyleFunc: func() core.Style {
			if shown.Get() {
				return core.Style{Opacity: core.Alpha(1)}
			}
			return core.Style{Opacity: core.Alpha(0), Transform: core.Transform{TranslateXPercent: 30}}
		},
		Children: []core.Node{content},
	}
}

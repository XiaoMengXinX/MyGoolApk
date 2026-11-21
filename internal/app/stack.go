package app

import (
	"slices"
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

// pageState is what Android draws of a pushed page's motion: it comes in as
// shown turns true and goes out as leaving does.
type pageState struct {
	shown, leaving *core.Signal[bool]
}

type pageStack struct {
	a      *App
	nav    *base.Nav[route]
	states []*pageState // the pushed pages' on Android, the top one last
}

func (a *App) newPageStack(cx *core.Scope) *pageStack {
	return &pageStack{a: a, nav: base.NewNav(cx, route{})}
}

// Depth is how many pages are over the shell.
func (s *pageStack) Depth() int { return s.nav.Depth() }

// Push shows a page over the others.
func (s *pageStack) Push(r route) { s.nav.Push(r) }

// Pop takes the top page away. iOS animates a pop itself; on Android the page
// goes out first, as a back gesture's does.
func (s *pageStack) Pop() bool {
	if s.nav.Depth() == 0 {
		return false
	}
	i := len(s.states) - 1
	for i >= 0 && s.states[i].leaving.GetUntracked() {
		i--
	}
	if i < 0 {
		s.nav.Pop(1)
		return true
	}
	st := s.states[i]
	st.leaving.Set(true)
	s.a.cx.AfterFunc(pageMotion+30*time.Millisecond, func() {
		// A back gesture may have popped it meanwhile.
		if slices.Contains(s.states, st) {
			s.nav.Pop(1)
		}
	})
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

// entering slides the page in from the right as it fades in, and out again as
// Pop takes it, on Android; iOS's navigation controller moves the whole screen
// itself.
func (s *pageStack) entering(cx *core.Scope, content core.Node) core.Node {
	if s.a.cx.Env().OS() != core.OSAndroid {
		return content
	}
	st := &pageState{shown: core.NewSignal(cx, false), leaving: core.NewSignal(cx, false)}
	s.states = append(s.states, st)
	cx.Cleanup(func() { s.states = slices.DeleteFunc(s.states, func(x *pageState) bool { return x == st }) })
	// The page comes in on the next frame, after it was laid out in place.
	cx.AfterFunc(16*time.Millisecond, func() { st.shown.Set(true) })
	return core.View{
		Style: fill.Merge(core.Style{Transition: pageTransition}),
		StyleFunc: func() core.Style {
			if st.shown.Get() && !st.leaving.Get() {
				return core.Style{Opacity: core.Alpha(1)}
			}
			return core.Style{Opacity: core.Alpha(0), Transform: core.Transform{TranslateXPercent: 30}}
		},
		Children: []core.Node{content},
	}
}

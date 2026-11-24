package app

import (
	"slices"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// Pages pushed over the shell are NSGo's native screen stack: on iOS
// UINavigationController slides them and its edge swipe pops them, on Android
// 14 and later the predictive back gesture drags the top page away.
//
// Both platforms hide a screen another covers, so the shell lives under the
// stack, where it keeps its lists' places. Android also shows a pushed screen
// at once, so there each page moves itself: it slides in from the right over the shell, which
// shifts a little and dims, and slides out again as Pop takes it.
const pageMotion = 300 * time.Millisecond

var pageTransition = core.Transition{Duration: pageMotion, Easing: emphasized, Properties: core.TransitionOpacity | core.TransitionTransform}

const fadeMotion = 160 * time.Millisecond

var fadeTransition = core.Transition{Duration: fadeMotion, Properties: core.TransitionOpacity}

// shellShift is how far the shell moves, in percent of its width, under a
// page.
const shellShift = -22

type route struct{ kind, id string }

// pageState is what Android draws of a pushed page's motion: it comes in as
// shown turns true and goes out as leaving does.
type pageState struct {
	shown, leaving *core.Signal[bool]
	// fades is whether it goes in and out by fading: over another page, as
	// the screen below one is hidden and sliding would show nothing behind.
	fades bool
}

type pageStack struct {
	a      *App
	nav    *base.Nav[route]
	states []*pageState // the pushed pages' on Android, the top one last
	// covered is whether a page not on its way out is over the shell; deep
	// whether pages are over each other, which hides the shell as the page
	// below one that leaves is not the shell.
	covered, deep *core.Signal[bool]
}

func (a *App) newPageStack(cx *core.Scope) *pageStack {
	return &pageStack{a: a, nav: base.NewNav(cx, route{}), covered: core.NewSignal(cx, false), deep: core.NewSignal(cx, false)}
}

func (s *pageStack) android() bool { return s.a.cx.Env().OS() == core.OSAndroid }

// refresh brings covered and deep up to the pages in states.
func (s *pageStack) refresh() {
	s.covered.Set(slices.ContainsFunc(s.states, func(x *pageState) bool { return !x.leaving.GetUntracked() }))
	s.deep.Set(len(s.states) >= 2)
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
	s.refresh()
	motion := pageMotion
	if st.fades {
		motion = fadeMotion
	}
	s.a.cx.AfterFunc(motion+30*time.Millisecond, func() {
		// A back gesture may have popped it meanwhile.
		if slices.Contains(s.states, st) {
			s.nav.Pop(1)
		}
	})
	return true
}

// view is the stack, the shell at its bottom.
func (s *pageStack) view(cx *core.Scope, shell func(cx *core.Scope) core.Node, page func(cx *core.Scope, r route) core.Node) core.Node {
	android := s.android()
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
		if !android {
			return base.NavPage{HeaderHidden: true, Class: "bg-background", NoPopOnSwipeFunc: viewing, Children: []core.Node{page(cx, r)}}
		}
		return base.NavPage{HeaderHidden: true, NoPopOnSwipeFunc: viewing, Children: []core.Node{s.moving(cx, r, page(cx, r))}}
	})
	if !android {
		// iOS's navigation controller slides the pages over it.
		return core.Group{core.View{Class: "bg-background", Style: fillAbs, Children: []core.Node{shell(cx)}}, stack}
	}
	return core.Group{s.shellLayer(shell(cx)), stack}
}

// shellLayer holds the shell under the stack, moved and dimmed as pages cover it.
func (s *pageStack) shellLayer(shell core.Node) core.Node {
	return core.View{
		Class: "bg-background",
		Style: fill.Merge(core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Overflow: core.Hidden, Transition: pageTransition}),
		StyleFunc: func() core.Style {
			if s.covered.Get() {
				return core.Style{Transform: core.Transform{TranslateXPercent: shellShift}, PointerEvents: core.PointerEventsNone}
			}
			return core.Style{}
		},
		Children: []core.Node{
			core.View{
				Style:     fill,
				StyleFunc: func() core.Style { return core.Style{Opacity: core.Alpha(shown(!s.deep.Get()))} },
				Children:  []core.Node{shell},
			},
			core.View{
				Class: "bg-black/10",
				Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Transition: pageTransition, PointerEvents: core.PointerEventsNone},
				StyleFunc: func() core.Style {
					return core.Style{Opacity: core.Alpha(shown(s.covered.Get()))}
				},
			},
		},
	}
}

// moving slides a page in from the right and out again, over a background of
// its own: the screen holding it is transparent, so the shell shows beside it.
func (s *pageStack) moving(cx *core.Scope, r route, content core.Node) core.Node {
	st := &pageState{shown: core.NewSignal(cx, false), leaving: core.NewSignal(cx, false), fades: len(s.states) > 0}
	s.states = append(s.states, st)
	cx.Cleanup(func() {
		s.states = slices.DeleteFunc(s.states, func(x *pageState) bool { return x == st })
		s.a.cx.AfterFunc(time.Millisecond, s.refresh)
	})
	// The page comes in on the next frame, after it was laid out in place.
	cx.AfterFunc(16*time.Millisecond, func() {
		st.shown.Set(true)
		s.refresh()
	})
	return core.View{
		Class: "bg-background",
		Style: fill.Merge(core.Style{Transition: pageTransition}),
		StyleFunc: func() core.Style {
			if st.shown.Get() && !st.leaving.Get() {
				return core.Style{}
			}
			if st.fades {
				return core.Style{Opacity: core.Alpha(0), Transition: fadeTransition}
			}
			return core.Style{Transform: core.Transform{TranslateXPercent: 100}}
		},
		Children: []core.Node{content},
	}
}

func shown(on bool) float32 {
	if on {
		return 1
	}
	return 0
}

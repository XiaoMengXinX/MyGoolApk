package app

import (
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// Pages pushed over the shell are the app's own stack, since Android's has
// no transition of its own: a page slides in from the right as it fades in,
// and out again as it leaves, over the page below, which stays where it was.
const pageMotion = 300 * time.Millisecond

var pageTransition = core.Transition{Duration: pageMotion, Easing: emphasized, Properties: core.TransitionOpacity | core.TransitionTransform}

type route struct{ kind, id string }

// entry is one page of the stack.
type entry struct {
	key     int
	route   route
	shown   *core.Signal[bool] // the page has come in
	leaving *core.Signal[bool] // the page is on its way out
}

type pageStack struct {
	a       *App
	entries *core.Signal[[]*entry]
	next    int
}

func (a *App) newPageStack(cx *core.Scope) *pageStack {
	s := &pageStack{a: a, entries: core.NewSignalFunc(cx, []*entry(nil), nil)}
	s.entries.Set([]*entry{s.newEntry(route{}, true)})
	return s
}

func (s *pageStack) newEntry(r route, shown bool) *entry {
	s.next++
	return &entry{key: s.next, route: r, shown: core.NewSignal(s.a.cx, shown), leaving: core.NewSignal(s.a.cx, false)}
}

// Depth is how many pages are over the shell, not counting those leaving.
func (s *pageStack) Depth() int {
	n := 0
	for _, e := range s.entries.GetUntracked()[1:] {
		if !e.leaving.GetUntracked() {
			n++
		}
	}
	return n
}

// Push shows a page over the others.
func (s *pageStack) Push(r route) {
	e := s.newEntry(r, false)
	s.entries.Update(func(es []*entry) []*entry { return append(append([]*entry(nil), es...), e) })
	// The page comes in on the next frame, after it was laid out in place.
	s.a.cx.AfterFunc(16*time.Millisecond, func() { e.shown.Set(true) })
}

// Pop takes the top page away, after its motion out.
func (s *pageStack) Pop() bool {
	es := s.entries.GetUntracked()
	for i := len(es) - 1; i > 0; i-- {
		e := es[i]
		if e.leaving.GetUntracked() {
			continue
		}
		e.leaving.Set(true)
		s.a.cx.AfterFunc(pageMotion+30*time.Millisecond, func() {
			s.entries.Update(func(es []*entry) []*entry {
				out := make([]*entry, 0, len(es))
				for _, x := range es {
					if x != e {
						out = append(out, x)
					}
				}
				return out
			})
		})
		return true
	}
	return false
}

// view lays the pages over each other.
func (s *pageStack) view(shell core.Node, page func(cx *core.Scope, r route) core.Node) core.Node {
	pages := core.Each(s.entries.Get, func(e *entry) int { return e.key },
		func(cx *core.Scope, item func() *entry, _ func() int) core.Node {
			e := core.Untracked(cx, item)
			if e.route.kind == "" {
				return core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)},
					Children: []core.Node{shell}}
			}
			class := "bg-background"
			if e.route.kind == routeImages {
				class = "" // the viewer draws its own black, which fades as a picture is dragged away
			}
			return core.View{
				Class: class,
				Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Transition: pageTransition},
				StyleFunc: func() core.Style {
					if e.shown.Get() && !e.leaving.Get() {
						return core.Style{Opacity: core.Alpha(1)}
					}
					return core.Style{Opacity: core.Alpha(0), PointerEvents: core.PointerEventsNone, Transform: core.Transform{TranslateXPercent: 30}}
				},
				Children: []core.Node{page(cx, e.route)},
			}
		})
	return core.View{Style: fill, Children: []core.Node{pages}}
}

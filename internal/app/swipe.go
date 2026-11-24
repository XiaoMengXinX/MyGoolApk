package app

import "github.com/nsgo-dev/nsgo/core"

// swipePager moves a paged horizontal Scroll by a pan on its content, for
// iOS: there a paged scroll view with scroll views inside its pages never
// gets a horizontal drag, which the inner ones keep waiting on. The pan
// follows the finger, and letting go settles on the page nearest where a
// throw would carry it, at most one page on.
type swipePager struct {
	ref   *core.Ref
	width func() core.Points
	count int
	// at is the page shown; goTo is called with the page a throw settles on.
	at   func() int
	goTo func(i int)
	from core.Points
}

// swipesByPan reports whether pages swipe by the pan rather than by the scroll view.
func swipesByPan(cx *core.Scope) bool { return cx.Env().OS() == core.OSIOS }

// swipeActivation starts a horizontal pan and leaves vertical drags to the
// scroll views in the pages.
var swipeActivation = core.PanActivation{ActiveOffsetX: 12, FailOffsetY: 12}

func (p *swipePager) pan(e core.PanEvent) {
	w := p.width()
	if w <= 0 {
		return
	}
	switch e.Phase {
	case core.GestureBegan:
		p.from = core.Points(p.at()) * w
	case core.GestureChanged:
		x := max(0, min(p.from-e.Translation.X, core.Points(p.count-1)*w))
		p.ref.ScrollTo(core.Point{X: x}, false)
	case core.GestureEnded, core.GestureCancelled:
		start := int(p.from/w + 0.5)
		to := (p.from - e.Translation.X - e.Velocity.X*0.2) / w
		i := int(to + 0.5)
		if to < 0 {
			i = 0
		}
		i = max(start-1, min(i, start+1))
		i = max(0, min(i, p.count-1))
		p.goTo(i)
	}
}

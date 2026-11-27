package app

import "github.com/nsgo-dev/nsgo/core"

// sideScroll is a row that scrolls sideways, as a horizontal core.Scroll with
// no indicator. On desktop, overflowing rows take either wheel axis and
// also support dragging sideways with the mouse.
type sideScroll struct {
	Style    core.Style
	Children []core.Node
}

func (s sideScroll) Render(cx *core.Scope) core.Node {
	scroll := core.Scroll{Horizontal: true, HideIndicator: true, Style: s.Style, Children: s.Children}
	if !hoverable {
		return scroll
	}
	var ref core.Ref
	var offset, view, content, from core.Points
	var glide func()
	overflow := core.NewSignal(cx, false)
	measure := func() { overflow.Set(view > 0 && content > view) }
	to := func(x core.Points) {
		offset = min(max(x, 0), max(content-view, 0))
		ref.ScrollTo(core.Point{X: offset}, false)
	}
	scroll.Ref = &ref
	scroll.OnLayout = func(b core.Bounds) { view = b.Size.Width; measure() }
	scroll.OnScroll = func(e core.ScrollEvent) {
		if !e.Stale {
			offset = e.Offset.X
		}
	}
	trackStyle := core.Style{Direction: core.Row, FlexShrink: 0}
	track := core.View{
		Style:         trackStyle,
		OnLayout:      func(b core.Bounds) { content = b.Size.Width; measure() },
		PanActivation: core.PanActivation{ActiveOffsetX: 6, FailOffsetY: 6},
		OnPan: func(e core.PanEvent) {
			switch e.Phase {
			case core.GestureBegan:
				if glide != nil {
					glide()
				}
				from = offset
			case core.GestureChanged:
				to(from - e.Translation.X)
			case core.GestureEnded:
				// The glide starts at the pointer's speed: an ease-out cubic
				// starts at three times its mean.
				x := from - e.Translation.X
				distance := -e.Velocity.X * core.Points(glideTime.Seconds()/3)
				glide = cx.Animate(glideTime, easeOutCubic, func(k float64) { to(x + distance*core.Points(k)) })
			}
		},
		Children: s.Children,
	}
	onWheel := func(e core.WheelEvent) {
		delta := e.Delta.X
		if max(e.Delta.Y, -e.Delta.Y) > max(delta, -delta) {
			delta = e.Delta.Y
		}
		if delta == 0 {
			return
		}
		if glide != nil {
			glide()
			glide = nil
		}
		to(offset - delta)
	}
	// NSGo's OnWheel takes the event before the surrounding scroll view
	// sees it, so only install the handler while the row can scroll.
	held := core.NewHolder(cx, func(*core.Scope) core.Node { return track })
	scroll.Children = []core.Node{core.Show{
		When: overflow.Get,
		Then: func(*core.Scope) core.Node {
			return core.View{Style: trackStyle, OnWheel: onWheel, Children: []core.Node{held.Place()}}
		},
		Else: func(*core.Scope) core.Node { return held.Place() },
	}}
	return scroll
}

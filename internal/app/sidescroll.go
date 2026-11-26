package app

import "github.com/nsgo-dev/nsgo/core"

// sideScroll is a row that scrolls sideways, as a horizontal core.Scroll with
// no indicator. A desktop's scroll view moves by the wheel and the touchpad
// alone, and a vertical wheel over a row in a list scrolls the list, so there
// a mouse's sideways drag moves the row too, and glides on as it lets go.
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
	end := func() core.Points { return max(content-view, 0) }
	to := func(x core.Points) { ref.ScrollTo(core.Point{X: min(max(x, 0), end())}, false) }
	scroll.Ref = &ref
	scroll.OnLayout = func(b core.Bounds) { view = b.Size.Width }
	scroll.OnScroll = func(e core.ScrollEvent) {
		if !e.Stale {
			offset = e.Offset.X
		}
	}
	scroll.Children = []core.Node{core.View{
		Style:         core.Style{Direction: core.Row, FlexShrink: 0},
		OnLayout:      func(b core.Bounds) { content = b.Size.Width },
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
	}}
	return scroll
}

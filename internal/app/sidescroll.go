package app

import "github.com/nsgo-dev/nsgo/core"

// sideScroll is a row that scrolls sideways, as a horizontal core.Scroll with
// no indicator. A mouse's wheel turns no row sideways, so on a desktop arrows
// at its ends, shown while the pointer is over it and there is more that way,
// move it by most of its width.
type sideScroll struct {
	Style    core.Style
	Children []core.Node
}

func (s sideScroll) Render(cx *core.Scope) core.Node {
	scroll := core.Scroll{Horizontal: true, HideIndicator: true, Style: s.Style, Children: s.Children}
	if !cx.Env().Desktop() {
		return scroll
	}
	var ref core.Ref
	offset, view, content := core.NewSignal(cx, core.Points(0)), core.NewSignal(cx, core.Points(0)), core.NewSignal(cx, core.Points(0))
	hovered := core.NewSignal(cx, false)
	scroll.Ref = &ref
	scroll.Style = core.Style{FlexGrow: 1, MinWidth: core.Pt(0)}
	scroll.OnLayout = func(b core.Bounds) { view.Set(b.Size.Width) }
	scroll.OnScroll = func(e core.ScrollEvent) {
		if !e.Stale {
			offset.Set(e.Offset.X)
			content.Set(e.ContentSize.Width)
		}
	}
	scroll.Children = []core.Node{core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0},
		OnLayout: func(b core.Bounds) { content.Set(b.Size.Width) }, Children: s.Children}}
	end := func() core.Points { return max(content.Get()-view.Get(), 0) }
	step := func(dir core.Points) {
		x := min(max(offset.GetUntracked()+dir*view.GetUntracked()*0.8, 0), end())
		ref.ScrollTo(core.Point{X: x}, true)
	}
	arrow := func(left bool) core.Node {
		more := func() bool {
			if left {
				return offset.Get() > 1
			}
			return offset.Get() < end()-1
		}
		dir, name, place := core.Points(1), "向右滚动", core.Style{Right: core.Pt(8)}
		if left {
			dir, name, place = -1, "向左滚动", core.Style{Left: core.Pt(8)}
		}
		glyph := icon(&symbols.ChevronRight, 24, "text-on-surface")
		if left {
			glyph = core.View{Style: core.Style{Transform: core.Transform{Rotate: 180}}, Children: []core.Node{glyph}}
		}
		return core.Show{When: func() bool { return hovered.Get() && more() }, Then: func(*core.Scope) core.Node {
			return core.View{Style: place.Merge(core.Style{Position: core.Absolute, Top: core.Pt(0), Bottom: core.Pt(0), JustifyContent: core.JustifyCenter}),
				PointerEvents: core.PointerEventsBoxNone,
				Children: []core.Node{tap(name, func() { step(dir) }, core.Style{Width: core.Pt(40), Height: core.Pt(40), CornerRadius: 20,
					AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
					Shadows: []core.BoxShadow{{Offset: core.Point{Y: 1}, Blur: 3, Color: core.RGBA(0, 0.3)}}}, "bg-surface-container-highest", glyph)}}
		}}
	}
	return core.View{
		Style:        s.Style.Merge(core.Style{Direction: core.Row}),
		OnMouseEnter: func(core.MouseEvent) { hovered.Set(true) },
		OnMouseLeave: func(core.MouseEvent) { hovered.Set(false) },
		Children:     []core.Node{scroll, arrow(true), arrow(false)},
	}
}

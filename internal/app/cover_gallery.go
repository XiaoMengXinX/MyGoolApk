package app

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

func (a *App) bigCoverImages(cx *core.Scope, get func() []backend.Image, count int) core.Node {
	width := core.NewSignal(cx, core.Points(0))
	at := core.NewSignal(cx, 0)
	settledPage := core.NewSignal(cx, 0)
	var ref core.Ref
	pageAt := func(e core.ScrollEvent) int {
		return max(0, min(int(e.Offset.X/e.Size.Width+0.5), count-1))
	}
	goTo := func(next int, animated bool) {
		if !animated {
			settledPage.Set(next)
		}
		ref.ScrollTo(core.Point{X: core.Points(next) * width.GetUntracked()}, animated)
	}
	pages := make([]core.Node, count)
	for i := range pages {
		pages[i] = core.View{
			StyleFunc: func() core.Style {
				return core.Style{Width: core.Pt(width.Get()), Height: core.Pct(100), FlexShrink: 0}
			},
			Children: []core.Node{core.View{
				Role: core.RoleButton, Label: fmt.Sprintf("查看第 %d 张图片", i+1),
				OnPress: func(core.PressEvent) { a.openImages(get(), i) },
				Style:   fillAbs.Merge(core.Style{Overflow: core.Hidden}), Class: "bg-background",
				Children: []core.Node{a.pic(cx, func() string { return thumbURL(get()[i], thumbMedium) }, core.Contain, "", fillAbs)},
			}},
		}
	}
	pager := core.Scroll{
		Ref:        &ref,
		Horizontal: true, HideIndicator: true,
		Behavior:  core.ScrollBehavior{Paging: true, NoScroll: count == 1},
		Style:     core.Style{Overflow: core.Hidden},
		StyleFunc: func() core.Style { return core.Style{AspectRatio: bigCoverRatio(get())} },
		OnLayout: func(b core.Bounds) {
			was := width.GetUntracked()
			if b.Size.Width > 0 && b.Size.Width != was {
				width.Set(b.Size.Width)
				if was > 0 && count > 1 {
					goTo(at.GetUntracked(), false)
				}
			}
		},
		OnScroll: func(e core.ScrollEvent) {
			if !e.Stale && e.Size.Width > 0 {
				at.Set(pageAt(e))
			}
		},
		Children: []core.Node{row(core.Style{Height: core.Pct(100)}, "", pages...)},
	}
	if count == 1 {
		return pager
	}
	if !hoverable {
		return column(core.Style{Gap: 8}, "", pager, pageIndicators(at.Get, count, nil))
	}
	pager.OnScrollEnd = func(e core.ScrollEvent) {
		if !e.Stale && e.Size.Width > 0 {
			settledPage.Set(pageAt(e))
		}
	}
	turn := func(delta int) {
		goTo(max(0, min(at.GetUntracked()+delta, count-1)), true)
	}
	arrow := func(glyph *symbol, name string, delta, edge int) core.View {
		return pointing(core.View{
			Role: core.RoleButton, Label: name, Disabled: func() bool { return settledPage.Get() == edge },
			Class: "bg-secondary-container",
			Style: core.Style{Width: core.Pt(36), Height: core.Pt(36), FlexShrink: 0, CornerRadius: 18,
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
				Disabled: &core.Style{Opacity: core.Alpha(0.4)}},
			OnPress:  func(core.PressEvent) { turn(delta) },
			Children: []core.Node{icon(glyph, 24, "text-on-secondary-container")},
		})
	}
	// Keep wheel capture on the controls so the native pager and detail list
	// route scrolling over the pictures by axis.
	controls := core.View{
		Style:    core.Style{Direction: core.Row, AlignItems: core.AlignCenter, AlignSelf: core.AlignCenter, MaxWidth: core.Pct(100), Gap: 6, Padding: core.Symmetric(4, 0)},
		OnWheel:  pagedWheel(&ref, width.GetUntracked, count, at.GetUntracked, turn),
		Children: []core.Node{arrow(&symbols.ChevronLeft, "上一张图片", -1, 0), pageIndicators(at.Get, count, goTo), arrow(&symbols.ChevronRight, "下一张图片", 1, count-1)},
	}
	return column(core.Style{}, "", pager, controls)
}

// The tallest picture sizes the pager; portraits stop at 4/3 of its width.
func bigCoverRatio(images []backend.Image) float32 {
	var height float32
	for _, im := range images {
		if im.Width > 0 && im.Height > 0 {
			height = max(height, float32(im.Height)/float32(im.Width))
		}
	}
	if height == 0 {
		return 1
	}
	return 1 / min(height, 4.0/3)
}

func pageIndicators(at func() int, count int, goTo func(int, bool)) core.Node {
	dots := make([]core.Node, count)
	size := core.Points(6)
	if goTo != nil {
		size = 8
	}
	for i := range dots {
		dot := core.View{
			Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2},
			ClassFunc: func() string {
				if at() == i {
					return "bg-primary"
				}
				return "bg-outline-variant"
			},
		}
		dots[i] = dot
		if goTo != nil {
			dots[i] = link(fmt.Sprintf("切换到第 %d 张图片", i+1), func() { goTo(i, true) },
				core.Style{Width: core.Pt(18), Height: core.Pt(36), FlexShrink: 1,
					AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, dot)
		}
	}
	if goTo == nil {
		return row(core.Style{JustifyContent: core.JustifyCenter, Gap: 6}, "", dots...)
	}
	var width core.Points
	return core.View{
		Style:         core.Style{Direction: core.Row, FlexShrink: 1},
		Children:      dots,
		OnLayout:      func(b core.Bounds) { width = b.Size.Width },
		PanActivation: core.PanActivation{ActiveOffsetX: 4, FailOffsetY: 6},
		OnPan: func(e core.PanEvent) {
			if e.Phase != core.GestureCancelled {
				next := max(0, min(int(e.Position.X/width*core.Points(count)), count-1))
				if next != at() {
					goTo(next, false)
				}
			}
		},
	}
}

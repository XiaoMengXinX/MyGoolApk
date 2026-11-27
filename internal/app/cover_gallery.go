package app

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

func (a *App) bigCoverImages(cx *core.Scope, get func() []backend.Image, count int) core.Node {
	width := core.NewSignal(cx, core.Points(0))
	at := core.NewSignal(cx, 0)
	pages := make([]core.Node, count)
	for i := range pages {
		pages[i] = core.View{
			StyleFunc: func() core.Style {
				return core.Style{Width: core.Pt(width.Get()), Height: core.Pct(100), FlexShrink: 0}
			},
			Children: []core.Node{tap(fmt.Sprintf("查看第 %d 张图片", i+1), func() { a.openImages(get(), i) },
				fillAbs, "bg-surface-container-low",
				a.pic(cx, func() string { return thumbURL(get()[i], thumbMedium) }, core.Contain, "", fillAbs))},
		}
	}
	pager := core.Scroll{
		Horizontal: true, HideIndicator: true,
		Behavior: core.ScrollBehavior{Paging: true, NoScroll: count == 1},
		Style:    core.Style{CornerRadius: 12, Overflow: core.Hidden},
		StyleFunc: func() core.Style {
			return core.Style{Height: core.Pt(width.Get() / core.Points(mediaRatio(get()[0], 4.0/3, 3.0/4, 1e6)))}
		},
		OnLayout: func(b core.Bounds) { width.Set(b.Size.Width) },
		OnScroll: func(e core.ScrollEvent) {
			if !e.Stale && e.Size.Width > 0 {
				at.Set(int(e.Offset.X/e.Size.Width + 0.5))
			}
		},
		Children: []core.Node{row(core.Style{Height: core.Pct(100)}, "", pages...)},
	}
	if count == 1 {
		return pager
	}
	return column(core.Style{Gap: 8}, "", pager, pageIndicators(at.Get, count))
}

func pageIndicators(at func() int, count int) core.Node {
	dots := make([]core.Node, count)
	for i := range dots {
		dots[i] = core.View{
			Style: core.Style{Width: core.Pt(6), Height: core.Pt(6), CornerRadius: 3},
			ClassFunc: func() string {
				if at() == i {
					return "bg-primary"
				}
				return "bg-outline-variant"
			},
		}
	}
	return row(core.Style{JustifyContent: core.JustifyCenter, Gap: 6}, "", dots...)
}

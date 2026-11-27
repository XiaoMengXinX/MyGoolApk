package app

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

func (a *App) imageGridCard(cx *core.Scope, c *backend.Card) core.Node {
	rows := make([]core.Node, 0, (len(c.Items)+1)/2)
	for start := 0; start < len(c.Items); start += 2 {
		cells := make([]core.Node, 0, 2)
		for _, it := range c.Items[start:min(start+2, len(c.Items))] {
			cells = append(cells, core.View{
				Style: core.Style{Width: core.Pct(50), Padding: core.All(4)},
				Children: []core.Node{tap(it.Title, func() { a.openCardItem(it) },
					core.Style{CornerRadius: 12}, "bg-inverse-surface",
					core.View{Style: core.Style{AspectRatio: 2.2}, Children: []core.Node{
						a.picOf(cx, it.Pic, core.Cover, "", fillAbs)}},
					core.Text{Content: it.Title, MaxLines: 2, Class: "text-inverse-on-surface",
						Style: textStyle(textBody, 500).Merge(core.Style{Height: core.Pt(56), Padding: core.Symmetric(6, 8)})})},
			})
		}
		rows = append(rows, row(core.Style{}, "", cells...))
	}
	return column(core.Style{Padding: core.All(4)}, "", rows...)
}

// newsCard keeps the server's editorial headline separate from the post's body.
func (a *App) newsCard(cx *core.Scope, get func() backend.PageEntry) core.Node {
	return column(core.Style{Padding: core.Edges{Top: 4, Right: 8, Bottom: 4, Left: 8}}, "bg-surface-container-low",
		a.newsRow(cx, get, false))
}

func (a *App) newsRow(cx *core.Scope, get func() backend.PageEntry, compact bool) core.Node {
	feed := func() backend.Feed { return *get().Feed }
	limit := 3
	if compact {
		limit = 1
	}
	return dynamicShape(cx, func() int { return min(len(feed().Images), limit) }, func(cx *core.Scope, count int) core.Node {
		headline := core.Text{ContentFunc: func() string { return get().Title }, MaxLines: 3, Class: "text-on-surface",
			Style: textStyle(textBody, 400)}
		meta := core.Text{ContentFunc: func() string {
			f := feed()
			return fmt.Sprintf("%s  %s评论  %s", f.Author.Name, formatCount(f.Stats.Comments), formatTimeNow(f.PublishedAt))
		}, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)}
		words := column(core.Style{Gap: 12}, "", headline, meta)
		var body core.Node = words
		picture := func(i int, style core.Style) core.Node {
			return core.View{Style: style.Merge(core.Style{CornerRadius: 8, Overflow: core.Hidden}), Children: []core.Node{
				a.pic(cx, func() string { return thumbURL(feed().Images[i], thumbSmall) }, core.Cover, "bg-surface-container-high", fillAbs)}}
		}
		switch count {
		case 0:
		case 1:
			words = column(fill.Merge(core.Style{Gap: 12, MinWidth: core.Pt(0), JustifyContent: core.JustifySpaceBetween}), "", headline, meta)
			body = row(core.Style{Gap: 12}, "", words, picture(0, core.Style{Width: core.Pt(112), Height: core.Pt(84), FlexShrink: 0}))
		default:
			images := make([]core.Node, count)
			for i := range images {
				images[i] = core.View{Style: core.Style{Width: core.Pct(100 / float32(count)), Padding: core.Symmetric(0, 2)},
					Children: []core.Node{picture(i, core.Style{Width: core.Pct(100), AspectRatio: 4.0 / 3})}}
			}
			body = column(core.Style{Gap: 12}, "", headline, row(core.Style{}, "", images...), meta)
		}
		return tap("查看帖子", func() { a.openFeed(feed()) }, core.Style{Padding: core.All(12), CornerRadius: 16}, "bg-background", body)
	})
}

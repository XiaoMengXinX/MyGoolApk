package app

import (
	"image"
	"image/color"
	_ "image/png"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

func heatLine(hot, text func() string, class string) core.Node {
	return row(core.Style{Gap: 3, AlignItems: core.AlignCenter, MinWidth: core.Pt(0)}, "",
		core.Show{
			When: func() bool { return hot() != "" },
			Then: func(*core.Scope) core.Node { return icon(&symbols.Flame, 14, "text-error") },
		},
		core.Text{ContentFunc: text, MaxLines: 1, Class: class, Style: textStyle(textCaption, 400)})
}

func heatCount(hot func() string) core.Node {
	return heatLine(hot, hot, "text-on-surface-variant")
}

func itemActivity(hot, discussions string) string {
	var parts []string
	if hot != "" {
		parts = append(parts, hot+"热度")
	}
	if discussions != "" {
		parts = append(parts, discussions+"讨论")
	}
	return strings.Join(parts, "  ")
}

func (a *App) digitalPicture(cx *core.Scope, src func() string, w, h core.Points) core.Node {
	return core.View{
		Style:    core.Style{Width: core.Pt(w), Height: core.Pt(h), FlexShrink: 0, CornerRadius: 6, Overflow: core.Hidden},
		Children: []core.Node{a.pic(cx, src, core.Contain, "", fillAbs)},
	}
}

func rankBadge(rank int) core.Node {
	if rank <= 0 {
		return core.Group{}
	}
	class := "bg-surface-container-highest text-on-surface-variant"
	if rank <= 3 {
		class = "bg-primary text-on-primary"
	}
	return core.Text{Content: strconv.Itoa(rank), Class: class, Style: textStyle(textCaption, 600).Merge(core.Style{
		Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), MinWidth: core.Pt(16),
		Padding: core.Symmetric(0, 3), CornerRadius: 4, TextAlign: core.TextAlignCenter,
	})}
}

func (a *App) rankedPicture(cx *core.Scope, src func() string, size core.Points, rank func() int) core.Node {
	return core.View{Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), FlexShrink: 0}, Children: []core.Node{
		a.digitalPicture(cx, src, size, size), dynamicShape(cx, rank, func(_ *core.Scope, n int) core.Node { return rankBadge(n) }),
	}}
}

func itemScore(r backend.ItemRating) core.Node {
	center := core.Style{Gap: 2, AlignItems: core.AlignCenter, FlexShrink: 0}
	switch {
	case r.Label != "":
		return column(center, "", label(r.Score, textTitle, 600, "text-star"),
			label(r.Label, textCaption, 400, "text-on-surface-variant"))
	case r.Unreleased && r.ReleaseDate != "" && r.ReleaseDate != "未公布":
		return column(center, "", ratingTag(r.ReleaseDate, "bg-primary-container text-on-primary-container"),
			label("发布时间", textCaption, 400, "text-primary"))
	case r.Score != "":
		n, _ := strconv.ParseFloat(r.Score, 64)
		return column(center, "", label(r.Score, textTitle, 600, "text-on-surface"), stars(n/2, 12))
	default:
		return label("暂无评分", textCaption, 400, "text-on-surface-variant")
	}
}

func (a *App) ratedItemRow(cx *core.Scope, get func() backend.CardItem, rank func() int) core.Node {
	view := tap("", func() { a.openCardItem(get()) }, core.Style{
		Direction: core.Row, Padding: core.Symmetric(10, 12), Gap: 10, AlignItems: core.AlignCenter, MinHeight: core.Pt(76),
	}, "bg-background",
		a.rankedPicture(cx, func() string { return get().Pic }, 56, rank),
		column(fill.Merge(core.Style{MinWidth: core.Pt(0), Gap: 4}), "",
			core.Text{ContentFunc: func() string { return get().Title }, MaxLines: 2, Class: "text-on-surface", Style: textStyle(textBody, 500)},
			heatLine(func() string { return get().Hot }, func() string {
				return itemActivity(get().Hot, get().Discussions)
			}, "text-on-surface-variant")),
		column(core.Style{Width: core.Pt(92), FlexShrink: 0, AlignItems: core.AlignCenter}, "",
			dynamicShape(cx, func() backend.ItemRating { return get().Rating }, func(cx *core.Scope, r backend.ItemRating) core.Node {
				return itemScore(r)
			})),
	)
	view.LabelFunc = func() string { return get().Title }
	return view
}

func catalogItem(e backend.PageEntry) backend.CardItem {
	link := e.URL
	if e.Kind == backend.EntryProduct {
		link = "/product/" + e.ID
	}
	return backend.CardItem{Title: e.Title, Pic: e.LogoURL, URL: link, Hot: e.Hot, Discussions: e.Discussions, Rating: e.Rating}
}

func (a *App) ratedListCard(cx *core.Scope, c *backend.Card) core.Node {
	items := make([]core.Node, 0, len(c.Items))
	for i, item := range c.Items {
		rank := 0
		if c.Ranked {
			rank = i + 1
		}
		items = append(items, a.ratedItemRow(cx, func() backend.CardItem { return item }, func() int { return rank }))
	}
	return column(core.Style{Padding: core.Symmetric(4, 0)}, "", items...)
}

func releaseDateParts(release string) (string, string) {
	if date, err := time.Parse("2006年01月02日", release); err == nil {
		return date.Format("01.02"), date.Format("2006")
	}
	if year, period, ok := strings.Cut(release, "年"); ok && period != "" {
		return strings.TrimPrefix(period, "0"), year
	}
	return release, ""
}

func (a *App) timelineRow(cx *core.Scope, get func() backend.CardItem, first, last func() bool) core.Node {
	date := func() string { date, _ := releaseDateParts(get().Rating.ReleaseDate); return date }
	year := func() string { _, year := releaseDateParts(get().Rating.ReleaseDate); return year }
	view := tap("", func() { a.openCardItem(get()) }, core.Style{
		Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, MinHeight: core.Pt(56), Padding: core.Symmetric(0, 12),
	}, "bg-background",
		column(core.Style{Width: core.Pt(48), AlignItems: core.AlignCenter, FlexShrink: 0}, "",
			core.Text{ContentFunc: date, MaxLines: 2, Style: textStyle(textSecondary, 500), Class: "text-on-surface"},
			core.Text{ContentFunc: year, Style: textStyle(textCaption, 400), Class: "text-on-surface-variant"}),
		core.View{Style: core.Style{Width: core.Pt(8), AlignSelf: core.AlignStretch, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			Children: []core.Node{
				core.View{Class: "bg-outline-variant", Style: core.Style{Position: core.Absolute, Left: core.Pt(3), Width: core.Pt(2)},
					StyleFunc: func() core.Style {
						st := core.Style{Top: core.Pt(0), Bottom: core.Pt(0)}
						if first() {
							st.Top = core.Pct(50)
						}
						if last() {
							st.Bottom = core.Pct(50)
						}
						return st
					}},
				core.View{Class: "bg-primary", Style: core.Style{Width: core.Pt(7), Height: core.Pt(7), CornerRadius: 4}},
			}},
		a.digitalPicture(cx, func() string { return get().Pic }, 40, 40),
		core.Text{ContentFunc: func() string { return get().Title }, MaxLines: 2, Class: "text-on-surface",
			Style: fill.Merge(textStyle(textSecondary, 500))},
		heatCount(func() string {
			it := get()
			if it.Rating.Unreleased {
				return ""
			}
			return it.Hot
		}),
	)
	view.LabelFunc = func() string { return get().Title }
	return view
}

func (a *App) timelineCard(cx *core.Scope, c *backend.Card) core.Node {
	items := make([]core.Node, len(c.Items))
	for i, item := range c.Items {
		items[i] = a.timelineRow(cx, func() backend.CardItem { return item },
			func() bool { return i == 0 }, func() bool { return i == len(c.Items)-1 })
	}
	return column(core.Style{Padding: core.Symmetric(4, 0)}, "", items...)
}

func (a *App) productHighlights(cx *core.Scope, c *backend.Card) core.Node {
	items := make([]core.Node, 0, len(c.Items))
	for _, it := range c.Items {
		summary := ""
		if it.Rating.Score != "" {
			summary = it.Rating.Score + "分"
		}
		if it.Rating.Count != "" {
			summary = strings.TrimSpace(summary + "  " + it.Rating.Count + "人点评")
		}
		path := a.imageFile(cx, func() string { return it.Pic })
		background := core.NewSignal(cx, highlightBase)
		cx.Effect(func() {
			if file := path(); file != "" {
				go func() {
					tint := productHighlightColor(file)
					cx.Post(func() { background.Set(tint) })
				}()
			}
		})
		white := core.RGB(0xFFFFFF)
		tile := tap(it.Title, func() { a.openCardItem(it) }, core.Style{
			Width: core.Pt(130), Padding: core.All(10), FlexShrink: 0, Gap: 8, CornerRadius: 12,
		}, "",
			cachedPicture(path, core.Contain, "", core.Style{Height: core.Pt(92)}),
			core.Text{Content: it.Title, MaxLines: 1, Style: textStyle(textSecondary, 600).Merge(core.Style{Color: white})},
			core.Text{Content: summary, MaxLines: 1, Style: textStyle(textCaption, 400).Merge(core.Style{Color: white})})
		tile.StyleFunc = func() core.Style { return core.Style{Background: background.Get()} }
		items = append(items, tile)
	}
	return sideScroll{Children: []core.Node{row(core.Style{Gap: 8, Padding: core.Symmetric(8, 12)}, "", items...)}}
}

const highlightBase core.Color = 0x24242AFF

func productHighlightColor(path string) core.Color {
	f, err := os.Open(path)
	if err != nil {
		return highlightBase
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return highlightBase
	}
	b := img.Bounds()
	var red, green, blue, total uint32
	for y := b.Min.Y; y < b.Max.Y; y += max(1, b.Dy()/32) {
		for x := b.Min.X; x < b.Max.X; x += max(1, b.Dx()/32) {
			p := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if p.A < 192 {
				continue
			}
			r, g, b := uint32(p.R), uint32(p.G), uint32(p.B)
			chroma := max(r, g, b) - min(r, g, b)
			weight := 16 + chroma*chroma/32
			red, green, blue = red+r*weight, green+g*weight, blue+b*weight
			total += weight
		}
	}
	if total == 0 {
		return highlightBase
	}
	// Keep the image's hue while holding the surface dark enough for white text.
	return core.RGBA(red/total<<16|green/total<<8|blue/total, 0.32).Over(highlightBase)
}

func (a *App) miniLinksCard(cx *core.Scope, c *backend.Card) core.Node {
	height := core.Points(40)
	if c.Inset {
		height = 56
	}
	items := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		items[i] = tap(it.Title, func() { a.openCardItem(it) }, fill.Merge(core.Style{
			Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Height: core.Pt(height), CornerRadius: 8,
		}), "", a.digitalPicture(cx, func() string { return it.Pic }, 18, 18),
			label(it.Title, textCaption, 500, "text-on-surface"))
	}
	return row(core.Style{Padding: core.Symmetric(0, 8)}, "", items...)
}

func (a *App) cardEntries(cx *core.Scope, c *backend.Card) core.Node {
	items := make([]core.Node, 0, len(c.Results))
	for _, e := range c.Results {
		if e.Kind == backend.EntryNews {
			items = append(items, a.newsRow(cx, func() backend.PageEntry { return e }, true))
		} else {
			items = append(items, a.entryView(cx, func() backend.PageEntry { return e }, nil, nil))
		}
	}
	return column(core.Style{Padding: core.Symmetric(4, 0)}, "", items...)
}

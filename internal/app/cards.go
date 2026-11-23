package app

import (
	"context"
	"sync"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// pageList is a server list whose selector card, where it has one, chooses
// what the rows after it show: the cards up to the selector stay, and the
// rest come from the chosen item's url.
type pageList struct {
	p      *pager[backend.PageEntry]
	chosen *core.Signal[int]
	// onChoose, when set, hears the chosen item, as the follow tab keeps it.
	onChoose func(i int)

	mu   sync.Mutex
	url  string // the chosen item's, "" for the list's own
	head []backend.PageEntry
}

// newPageList lists url. head, when given, leads the list as the cards the
// server would send, and source is the url its chosen item lists.
func (a *App) newPageList(cx *core.Scope, url, title string, head []backend.PageEntry, chosen int, source string) *pageList {
	l := &pageList{chosen: core.NewSignal(cx, chosen), head: head, url: source}
	l.p = newPager(cx, func(ctx context.Context, page int) ([]backend.PageEntry, bool, error) {
		l.mu.Lock()
		src, head := l.url, l.head
		l.mu.Unlock()
		if src == "" {
			r, err := a.backend.PageEntries(ctx, backend.PageQuery{URL: url, Title: title, Page: page})
			if err == nil && page == 1 {
				l.keepHead(r.Entries)
			}
			return r.Entries, r.HasMore, err
		}
		r, err := a.backend.PageEntries(ctx, backend.PageQuery{URL: src, Title: title, Page: page})
		if page == 1 {
			r.Entries = append(append([]backend.PageEntry(nil), head...), r.Entries...)
		}
		return r.Entries, r.HasMore, err
	}, entryKey)
	return l
}

// keepHead keeps the rows up to the first selector of the list's own first
// page, which lead the list whatever is chosen.
func (l *pageList) keepHead(entries []backend.PageEntry) {
	for i, e := range entries {
		if e.Card != nil && e.Card.Kind == backend.CardSelector {
			l.mu.Lock()
			l.head = append([]backend.PageEntry(nil), entries[:i+1]...)
			l.mu.Unlock()
			return
		}
	}
}

// choose shows the list of item i of the selector: the rows above it stay
// while the new ones load.
func (l *pageList) choose(c *backend.Card, i int) {
	if i == l.chosen.GetUntracked() || i >= len(c.Items) {
		return
	}
	l.chosen.Set(i)
	if l.onChoose != nil {
		l.onChoose(i)
	}
	l.mu.Lock()
	l.url = c.Items[i].URL
	head := l.head
	l.mu.Unlock()
	l.p.items.Set(head)
	l.p.hasMore.Set(true)
	l.p.moreErr.Set("")
	l.p.reload()
}

// openCardItem follows an item of a card: to its page in the app where it has
// one, else in the browser.
func (a *App) openCardItem(it backend.CardItem) {
	target := backend.ResolveLink(it.URL)
	if target.Title == "" {
		target.Title = it.Title
	}
	if !a.openInApp(target) && target.URL != "" {
		a.cx.OpenURL(target.URL, nil)
	}
}

// cardView shows a card of a server page, rebuilt as its row moves to
// another card.
func (a *App) cardView(cx *core.Scope, get func() backend.PageEntry, l *pageList) core.Node {
	card := core.NewMemo(cx, func() *backend.Card { return get().Card })
	return core.Dynamic(func(cx *core.Scope) core.Node {
		c := card.Get()
		if c == nil {
			return core.Group{}
		}
		var body core.Node
		switch c.Kind {
		case backend.CardBanner:
			body = a.bannerCard(cx, c)
		case backend.CardIcons:
			body = a.iconsCard(cx, c)
		case backend.CardChips:
			body = a.chipsCard(c)
		case backend.CardList:
			body = a.listCard(cx, c)
		case backend.CardScroll:
			body = a.scrollCard(cx, c)
		case backend.CardTitle:
			return cardTitle(c.Title, core.Edges{Top: 16, Right: 16, Bottom: 4, Left: 16})
		case backend.CardSelector:
			return selectorCard(c, l)
		}
		parts := []core.Node{}
		if c.Title != "" {
			parts = append(parts, cardTitle(c.Title, core.Edges{Top: 12, Right: 16, Left: 16}))
		}
		parts = append(parts, body, core.View{Style: core.Style{Height: core.Pt(8)}, Class: "bg-surface-container-low"})
		return column(core.Style{}, "bg-background", parts...)
	})
}

func cardTitle(title string, pad core.Edges) core.Node {
	return core.View{Class: "bg-background", Style: core.Style{Padding: pad}, Children: []core.Node{
		label(title, textBody, 600, "text-on-surface")}}
}

// bannerCard pages through its pictures, as wide as the screen less the
// margins and as tall as the first picture's shape.
func (a *App) bannerCard(cx *core.Scope, c *backend.Card) core.Node {
	ratio := c.Items[0].Ratio
	if ratio <= 0 {
		ratio = 3
	}
	pos := core.NewSignal(cx, 0)
	width := core.NewSignal(cx, core.Points(0))
	pages := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		it := it
		pages[i] = core.View{
			StyleFunc: func() core.Style {
				return core.Style{Width: core.Pt(width.Get()), Height: core.Pct(100), Padding: core.Symmetric(0, 16)}
			},
			Children: []core.Node{tap(it.Title, func() { a.openCardItem(it) },
				fill.Merge(core.Style{CornerRadius: 12}), "bg-surface-container-high",
				a.picOf(cx, it.Pic, core.Cover, "", fillAbs))},
		}
	}
	pager := core.Scroll{
		Horizontal: true, HideIndicator: true,
		Behavior: core.ScrollBehavior{Paging: true},
		Style:    core.Style{Direction: core.Row, AspectRatio: ratio * 1.08},
		OnLayout: func(b core.Bounds) {
			if b.Size.Width > 0 {
				width.Set(b.Size.Width)
			}
		},
		OnScroll: func(e core.ScrollEvent) {
			if !e.Stale && e.Size.Width > 0 {
				pos.Set(int(e.Offset.X/e.Size.Width + 0.5))
			}
		},
		Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, Height: core.Pct(100)}, Children: pages}},
	}
	parts := []core.Node{pager}
	if len(c.Items) > 1 {
		dots := make([]core.Node, len(c.Items))
		for i := range c.Items {
			i := i
			dots[i] = core.View{
				Style: core.Style{Height: core.Pt(6), CornerRadius: 3, Transition: core.Transition{Duration: 200e6, Properties: core.TransitionLayout}},
				StyleFunc: func() core.Style {
					if pos.Get() == i {
						return core.Style{Width: core.Pt(16)}
					}
					return core.Style{Width: core.Pt(6)}
				},
				ClassFunc: func() string {
					if pos.Get() == i {
						return "bg-primary"
					}
					return "bg-outline-variant"
				},
			}
		}
		parts = append(parts, row(core.Style{JustifyContent: core.JustifyCenter, Gap: 6, Padding: core.Edges{Top: 8}}, "", dots...))
	}
	return column(core.Style{Padding: core.Symmetric(12, 0)}, "", parts...)
}

// iconsCard is a grid of icons: five to a row for links, four for topics,
// products and people, whose names are longer and which have a line more.
func (a *App) iconsCard(cx *core.Scope, c *backend.Card) core.Node {
	cols := 4
	if c.Items[0].Subtitle == "" && !c.Items[0].Round {
		cols = 5
	}
	size := core.Points(48)
	if cols == 5 {
		size = 40
	}
	var rows []core.Node
	for start := 0; start < len(c.Items); start += cols {
		cells := make([]core.Node, cols)
		for j := range cells {
			if start+j >= len(c.Items) {
				cells[j] = core.View{Style: fill}
				continue
			}
			it := c.Items[start+j]
			radius := size / 4
			if it.Round {
				radius = size / 2
			}
			parts := []core.Node{
				a.clippedPic(cx, it.Pic, size, size, radius),
				core.Text{Content: it.Title, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textCaption, 500).Merge(core.Style{TextAlign: core.TextAlignCenter})},
			}
			if it.Subtitle != "" {
				parts = append(parts, core.Text{Content: it.Subtitle, MaxLines: 1, Class: "text-on-surface-variant",
					Style: textStyle(textCaption, 400).Merge(core.Style{TextAlign: core.TextAlignCenter})})
			}
			cells[j] = tap(it.Title, func() { a.openCardItem(it) },
				fill.Merge(core.Style{AlignItems: core.AlignCenter, Gap: 6, Padding: core.Symmetric(8, 2), CornerRadius: 12}), "", parts...)
		}
		rows = append(rows, row(core.Style{}, "", cells...))
	}
	return column(core.Style{Padding: core.Symmetric(4, 8)}, "", rows...)
}

// chipsCard is a row of assist chips that scrolls.
func (a *App) chipsCard(c *backend.Card) core.Node {
	chips := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		it := it
		chips[i] = tap(it.Title, func() { a.openCardItem(it) },
			core.Style{Height: core.Pt(32), Padding: core.Symmetric(0, 12), CornerRadius: 8, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			"bg-surface-container-high",
			core.Text{Content: it.Title, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textSecondary, 500)})
	}
	return core.Scroll{Horizontal: true, HideIndicator: true, Children: []core.Node{
		row(core.Style{Gap: 8, Padding: core.Symmetric(12, 16)}, "", chips...)}}
}

// listCard is a few rows with an icon, a name and a line under it.
func (a *App) listCard(cx *core.Scope, c *backend.Card) core.Node {
	rows := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		it := it
		radius := core.Points(10)
		if it.Round {
			radius = 20
		}
		rows[i] = tap(it.Title, func() { a.openCardItem(it) },
			core.Style{Direction: core.Row, Padding: core.Symmetric(8, 16), Gap: 12, AlignItems: core.AlignCenter}, "",
			a.clippedPic(cx, it.Pic, 40, 40, radius),
			column(fill.Merge(core.Style{Gap: 2}), "",
				core.Text{Content: it.Title, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textSecondary, 500)},
				core.Text{Content: it.Subtitle, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)}),
		)
	}
	return column(core.Style{Padding: core.Symmetric(4, 0)}, "", rows...)
}

// scrollCard is pictures with titles that scroll sideways.
func (a *App) scrollCard(cx *core.Scope, c *backend.Card) core.Node {
	const w = 148
	items := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		it := it
		items[i] = tap(it.Title, func() { a.openCardItem(it) },
			core.Style{Width: core.Pt(w), Gap: 6, CornerRadius: 12}, "",
			a.clippedPic(cx, it.Pic, w, w*3/4, 12),
			core.Text{Content: it.Title, MaxLines: 2, Class: "text-on-surface", Style: textStyle(textCaption, 500)},
			core.Text{Content: it.Subtitle, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)},
		)
	}
	return core.Scroll{Horizontal: true, HideIndicator: true, Children: []core.Node{
		row(core.Style{Gap: 12, Padding: core.Symmetric(12, 16)}, "", items...)}}
}

// selectorCard is filter chips over the list they choose.
func selectorCard(c *backend.Card, l *pageList) core.Node {
	chips := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		i := i
		on := func() bool { return l != nil && l.chosen.Get() == i }
		chips[i] = filterChip(it.Title, on, func() {
			if l != nil {
				l.choose(c, i)
			}
		})
	}
	return column(core.Style{}, "bg-background",
		core.Scroll{Horizontal: true, HideIndicator: true, Children: []core.Node{
			row(core.Style{Gap: 8, Padding: core.Edges{Top: 12, Right: 16, Bottom: 8, Left: 16}}, "", chips...)}},
	)
}

// clippedPic is a picture of a fixed size with rounded corners: an image's own
// corner radius does not clip what it draws, the view around it does.
func (a *App) clippedPic(cx *core.Scope, url string, w, h, radius core.Points) core.Node {
	return core.View{Class: "bg-surface-container-high",
		Style:    core.Style{Width: core.Pt(w), Height: core.Pt(h), CornerRadius: radius, Overflow: core.Hidden, FlexShrink: 0},
		Children: []core.Node{a.picOf(cx, url, core.Cover, "", fillAbs)}}
}

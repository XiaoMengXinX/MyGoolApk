package app

import (
	"context"
	"slices"
	"strconv"
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
	// openCardTitle handles links that belong to the page holding this list.
	openCardTitle func(backend.CardItem)
	// onChoose, when set, hears the chosen item, as the follow tab keeps it.
	onChoose func(i int)

	mu sync.Mutex
	// url and title are the chosen item's, "" for the list's own.
	url, title string
	// split leaves a page that ends with its tabs without the chosen tab's
	// rows: whoever shows it gives each tab a list of its own.
	split bool
	head  []backend.PageEntry
}

// newPageList lists url. head, when given, leads the list as the cards the
// server would send, and source is the url its chosen item lists.
func (a *App) newPageList(cx *core.Scope, url, title string, head []backend.PageEntry, chosen int, source string) *pageList {
	l := &pageList{chosen: core.NewSignal(cx, chosen), head: head, url: source}
	l.p = newQueryPager(cx, l.chosen.GetUntracked, func(ctx context.Context, picked, page int) ([]backend.PageEntry, bool, error) {
		// chosen lists the chosen item under the rows that lead the list.
		chosen := func(page int) ([]backend.PageEntry, bool, error) {
			l.mu.Lock()
			src, named, head := l.url, l.title, l.head
			l.mu.Unlock()
			if named == "" {
				named = title
			}
			r, err := a.backend.PageEntries(ctx, backend.PageQuery{URL: src, Title: named, Page: page})
			if page == 1 {
				r.Entries = append(append([]backend.PageEntry(nil), head...), belowHead(r.Entries, head)...)
			}
			return r.Entries, r.HasMore, err
		}
		l.mu.Lock()
		src := l.url
		l.mu.Unlock()
		if src != "" {
			return chosen(page)
		}
		r, err := a.backend.PageEntries(ctx, backend.PageQuery{URL: url, Title: title, Page: page})
		if err != nil || page != 1 {
			return r.Entries, r.HasMore, err
		}
		// A page that ends with its tabs (iconTabLinkGridCard) lists
		// nothing of its own: its chosen tab's list follows them.
		if n := len(r.Entries) - 1; l.split && n >= 0 && r.Entries[n].Card != nil && r.Entries[n].Card.Kind == backend.CardTabs {
			return r.Entries, false, nil
		}
		if l.keepHead(r.Entries, picked) {
			return chosen(1)
		}
		return r.Entries, r.HasMore, err
	}, entryKey)
	a.trackPageList(cx, l)
	return l
}

func (a *App) trackPageList(cx *core.Scope, l *pageList) {
	a.lists = append(a.lists, l)
	cx.Cleanup(func() { a.lists = slices.DeleteFunc(a.lists, func(x *pageList) bool { return x == l }) })
}

// belowHead leaves out of a chosen item's rows the cards that lead the list
// already, which the server sends again above them.
func belowHead(entries, head []backend.PageEntry) []backend.PageEntry {
	if len(head) == 0 {
		return entries
	}
	last := entryKey(head[len(head)-1])
	for i, e := range entries {
		if entryKey(e) == last {
			return entries[i+1:]
		}
	}
	return entries
}

// feedChanged shows what a post's page changed of it, as its likes, on the
// cards of the lists that show it.
func (a *App) feedChanged(f backend.Feed) {
	for _, l := range a.lists {
		l.p.update(entryKey(backend.PageEntry{Kind: backend.EntryFeed, ID: f.ID}), func(e backend.PageEntry) backend.PageEntry {
			if e.Feed != nil {
				g := *e.Feed
				g.Liked, g.Stats.Likes = f.Liked, f.Stats.Likes
				e.Feed = &g
			}
			return e
		})
	}
}

// keepHead keeps the rows up to the first selector of the list's own first
// page, which lead the list whatever is chosen. It reports whether the
// chosen item's list is to follow them: when nothing follows the selector,
// or an item other than its first is chosen.
func (l *pageList) keepHead(entries []backend.PageEntry, n int) bool {
	for i, e := range entries {
		if e.Card == nil || !e.Card.Chooses() {
			continue
		}
		l.mu.Lock()
		defer l.mu.Unlock()
		l.head = append([]backend.PageEntry(nil), entries[:i+1]...)
		if n >= len(e.Card.Items) {
			// A choice remembered from a selector that had more items: the
			// first is shown, and chosen.
			n = 0
			l.p.cx.Post(func() {
				l.chosen.Set(0)
				if l.onChoose != nil {
					l.onChoose(0)
				}
			})
		}
		if n == 0 && i < len(entries)-1 {
			return false
		}
		l.url, l.title = e.Card.Items[n].URL, e.Card.Items[n].Title
		return true
	}
	return false
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
	l.url, l.title = c.Items[i].URL, c.Items[i].Title
	head := l.head
	l.mu.Unlock()
	l.p.items.Set(head)
	l.p.hasMore.Set(true)
	l.p.moreErr.Set("")
	l.p.restart()
}

// openCardItem follows an item of a card: to its page in the app where it has
// one, to a web view for the site's own tools, else in the browser.
func (a *App) openCardItem(it backend.CardItem) {
	target := backend.ResolveLink(it.URL)
	if target.Title == "" {
		target.Title = it.Title
	}
	switch {
	case a.openInApp(target) || target.URL == "":
	case showsInApp(target.URL):
		a.openWebPage(it.Title, target.URL)
	default:
		a.openWeb(target.URL)
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
			if len(c.Results) > 0 {
				open := a.openCardItem
				if l != nil && l.openCardTitle != nil {
					open = l.openCardTitle
				}
				return a.searchPreview(cx, c, open)
			}
			body = a.listCard(cx, c)
		case backend.CardAvatars:
			return a.avatarsCard(cx, c)
		case backend.CardPairs:
			body = a.pairsCard(cx, c)
		case backend.CardRanking:
			body = a.rankingCard(cx, c)
		case backend.CardTiles:
			return a.tilesCard(cx, c)
		case backend.CardLogos:
			body = a.logosCard(cx, c)
		case backend.CardScroll:
			body = a.scrollCard(cx, c)
		case backend.CardTitle:
			return cardTitle(c.Title, core.Edges{Top: 16, Right: 16, Bottom: 4, Left: 16})
		case backend.CardSelector:
			return selectorCard(c, l)
		case backend.CardTabs:
			return tabsCard(c, l)
		case backend.CardButtons:
			body = a.buttonsCard(cx, c)
		case backend.CardConfigs:
			body = a.configsCard(c)
		case backend.CardNote:
			body = core.View{Style: core.Style{Padding: core.Symmetric(12, 16)}, Children: []core.Node{
				a.richText(cx, func() ([]backend.RichSpan, string) { return c.Note, "" },
					richOptions{size: textSecondary, class: "text-on-surface-variant"})}}
		}
		parts := []core.Node{}
		switch {
		case c.Title != "" && c.URL != "":
			// The title leads to all of what the card shows a few of.
			open := a.openCardItem
			if l != nil && l.openCardTitle != nil {
				open = l.openCardTitle
			}
			parts = append(parts, tap(c.Title, func() { open(backend.CardItem{Title: c.Title, URL: c.URL}) },
				core.Style{Direction: core.Row, Padding: core.Edges{Top: 12, Right: 12, Left: 16}, Gap: 4, AlignItems: core.AlignCenter}, "",
				core.Text{Content: c.Title, MaxLines: 1, Class: "text-on-surface", Style: fill.Merge(textStyle(textBody, 600))},
				label(c.More, textSecondary, 400, "text-on-surface-variant"),
				icon(&symbols.ChevronRight, 24, "text-on-surface-variant")))
		case c.Title != "":
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
// margins, at the shape the card asks for or else the first picture's.
func (a *App) bannerCard(cx *core.Scope, c *backend.Card) core.Node {
	const margin = 8
	ratio := c.Ratio
	if ratio <= 0 {
		ratio = c.Items[0].Ratio
	}
	if ratio <= 0 {
		ratio = 3
	}
	pos := core.NewSignal(cx, 0)
	width := core.NewSignal(cx, core.Points(0))
	pages := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		pages[i] = core.View{
			StyleFunc: func() core.Style {
				return core.Style{Width: core.Pt(width.Get()), Height: core.Pct(100), Padding: core.Symmetric(0, margin)}
			},
			Children: []core.Node{tap(it.Title, func() { a.openCardItem(it) },
				fill.Merge(core.Style{CornerRadius: 12}), "bg-surface-container-high",
				a.picOf(cx, it.Pic, core.Cover, "", fillAbs))},
		}
	}
	pager := core.Scroll{
		Horizontal: true, HideIndicator: true,
		// A single picture leaves a sideways drag to the views around it.
		Behavior: core.ScrollBehavior{Paging: true, NoScroll: len(c.Items) == 1},
		Style:    core.Style{Direction: core.Row},
		// The pictures keep the shape between the margins.
		StyleFunc: func() core.Style {
			return core.Style{Height: core.Pt(max(width.Get()-2*margin, 0) / core.Points(ratio))}
		},
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
	return column(core.Style{Padding: core.Symmetric(8, 0)}, "", parts...)
}

// avatarsCard is people in a row that scrolls: the first ten of them, as the
// official client shows a part, then the way to them all.
func (a *App) avatarsCard(cx *core.Scope, c *backend.Card) core.Node {
	const shown, size = 10, 48
	cell := func(name string, open func(), pic core.Node) core.Node {
		return tap(name, open, core.Style{Width: core.Pt(68), Gap: 4, Padding: core.Symmetric(4, 2), AlignItems: core.AlignCenter, CornerRadius: 12}, "", pic,
			core.Text{Content: name, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400).Merge(core.Style{TextAlign: core.TextAlignCenter})})
	}
	var cells []core.Node
	for _, it := range c.Items[:min(shown, len(c.Items))] {
		cells = append(cells, cell(it.Title, func() { a.openCardItem(it) }, a.clippedPic(cx, it.Pic, size, size, size/2)))
	}
	if c.URL != "" {
		cells = append(cells, cell("查看更多", func() { a.openCardItem(backend.CardItem{Title: c.Title, URL: c.URL}) },
			core.View{Class: "bg-secondary-container", Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				Children: []core.Node{icon(&symbols.ChevronRight, 24, "text-on-secondary-container")}}))
	}
	return column(core.Style{}, "bg-background", core.Scroll{Horizontal: true, HideIndicator: true, Children: []core.Node{
		row(core.Style{Padding: core.Edges{Top: 8, Right: 8, Bottom: 4, Left: 8}}, "", cells...)}})
}

// pairsCard is names with small icons, two to a row.
func (a *App) pairsCard(cx *core.Scope, c *backend.Card) core.Node {
	var rows []core.Node
	for start := 0; start < len(c.Items); start += 2 {
		cells := make([]core.Node, 2)
		for j := range cells {
			if start+j >= len(c.Items) {
				cells[j] = core.View{Style: fill}
				continue
			}
			it := c.Items[start+j]
			cells[j] = tap(it.Title, func() { a.openCardItem(it) },
				fill.Merge(core.Style{Direction: core.Row, Height: core.Pt(40), Padding: core.Symmetric(0, 8), Gap: 10, AlignItems: core.AlignCenter, CornerRadius: 8}), "",
				a.clippedPic(cx, it.Pic, 24, 24, 6),
				core.Text{Content: it.Title, MaxLines: 1, Class: "text-on-surface", Style: fill.Merge(textStyle(textSecondary, 400))})
		}
		rows = append(rows, row(core.Style{}, "", cells...))
	}
	return column(core.Style{Padding: core.Edges{Top: 4, Right: 8, Bottom: 8, Left: 8}}, "", rows...)
}

// rankingCard is products four to a row in the order of their rank, each
// under its place.
func (a *App) rankingCard(cx *core.Scope, c *backend.Card) core.Node {
	const cols, size = 4, 56
	var rows []core.Node
	for start := 0; start < len(c.Items); start += cols {
		cells := make([]core.Node, cols)
		for j := range cells {
			n := start + j
			if n >= len(c.Items) {
				cells[j] = core.View{Style: fill}
				continue
			}
			it := c.Items[n]
			place := "bg-surface-container-highest text-on-surface-variant"
			if n < 3 {
				place = "bg-primary text-on-primary"
			}
			cells[j] = tap(it.Title, func() { a.openCardItem(it) },
				fill.Merge(core.Style{AlignItems: core.AlignCenter, Gap: 6, Padding: core.Symmetric(8, 4), CornerRadius: 12}), "",
				core.View{Style: core.Style{Width: core.Pt(size), Height: core.Pt(size)}, Children: []core.Node{
					a.clippedPic(cx, it.Pic, size, size, 12),
					core.Text{Content: strconv.Itoa(n + 1), Class: place, Style: textStyle(textCaption, 600).Merge(core.Style{
						Position: core.Absolute, Top: core.Pt(-4), Left: core.Pt(-8), MinWidth: core.Pt(18), Padding: core.Symmetric(0, 4), CornerRadius: 8, TextAlign: core.TextAlignCenter})},
				}},
				core.Text{Content: it.Title, MaxLines: 2, Class: "text-on-surface", Style: textStyle(textCaption, 400).Merge(core.Style{TextAlign: core.TextAlignCenter})})
		}
		rows = append(rows, row(core.Style{AlignItems: core.AlignStart}, "", cells...))
	}
	return column(core.Style{Padding: core.Edges{Top: 4, Right: 8, Bottom: 8, Left: 8}}, "", rows...)
}

// tilesCard is square pictures under their names, in a row that scrolls.
func (a *App) tilesCard(cx *core.Scope, c *backend.Card) core.Node {
	const size = 76
	tiles := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		tiles[i] = tap(it.Title, func() { a.openCardItem(it) },
			core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: 12}, "bg-surface-container-high",
			a.picOf(cx, it.Pic, core.Cover, "", fillAbs),
			core.View{Class: "bg-black/40", Style: fillAbs.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(4)}),
				PointerEvents: core.PointerEventsNone,
				Children: []core.Node{core.Text{Content: it.Title, MaxLines: 2, Class: "text-white",
					Style: textStyle(textSecondary, 600).Merge(core.Style{TextAlign: core.TextAlignCenter})}}})
	}
	return column(core.Style{}, "bg-background", core.Scroll{Horizontal: true, HideIndicator: true, Children: []core.Node{
		row(core.Style{Gap: 8, Padding: core.Symmetric(8, 16)}, "", tiles...)}})
}

// logosCard is the logos of brands, two to a column, in columns that scroll
// sideways. The logos come with white backgrounds of their own (JPEGs and
// PNGs without alpha), so each sits on a white tile of its shape.
func (a *App) logosCard(cx *core.Scope, c *backend.Card) core.Node {
	const w, h = 64, 32
	cell := func(it backend.CardItem) core.Node {
		return tap(it.Title, func() { a.openCardItem(it) }, core.Style{Width: core.Pt(w + 8), Gap: 4, Padding: core.Symmetric(4, 0), AlignItems: core.AlignCenter, CornerRadius: 8}, "",
			core.View{Class: "bg-white", Style: core.Style{Width: core.Pt(w), Height: core.Pt(h), CornerRadius: 8, Overflow: core.Hidden},
				Children: []core.Node{a.picOf(cx, it.Pic, core.Contain, "", fillAbs)}},
			core.Text{Content: it.Title, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400).Merge(core.Style{TextAlign: core.TextAlignCenter})})
	}
	var cols []core.Node
	for i := 0; i < len(c.Items); i += 2 {
		kids := []core.Node{cell(c.Items[i])}
		if i+1 < len(c.Items) {
			kids = append(kids, cell(c.Items[i+1]))
		}
		cols = append(cols, column(core.Style{Gap: 4}, "", kids...))
	}
	return core.Scroll{Horizontal: true, HideIndicator: true, Children: []core.Node{
		row(core.Style{Gap: 4, Padding: core.Symmetric(4, 12)}, "", cols...)}}
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

// configsCard is the versions of a product: the name and price of each, and
// the page of its specifications.
func (a *App) configsCard(c *backend.Card) core.Node {
	rows := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		rows[i] = row(core.Style{Padding: core.Edges{Top: 12, Right: 12, Bottom: 12, Left: 16}, Gap: 12, AlignItems: core.AlignCenter, CornerRadius: 12, BorderWidth: 1}, "border-outline-variant",
			column(fill.Merge(core.Style{Gap: 2}), "",
				core.Text{Content: it.Title, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textBody, 500)},
				core.Text{Content: it.Subtitle, MaxLines: 1, Class: "text-error", Style: textStyle(textSecondary, 500)}),
			tonalButton("参数", func() { a.openCardItem(it) }))
	}
	return column(core.Style{Padding: core.Symmetric(12, 16), Gap: 8}, "", rows...)
}

// scrollCard is pictures with titles that scroll sideways.
func (a *App) scrollCard(cx *core.Scope, c *backend.Card) core.Node {
	const w = 148
	items := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
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

// tabsCard is the tabs of a page: MD3 primary tabs as the home's, which
// choose the list below them.
func tabsCard(c *backend.Card, l *pageList) core.Node {
	tabs := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		on := func() bool { return l != nil && l.chosen.Get() == i }
		tabs[i] = pressable(core.View{
			Role: core.RoleTab, Label: it.Title,
			Style: core.Style{Height: core.Pct(100), Padding: core.Symmetric(0, 16), JustifyContent: core.JustifyEnd, AlignItems: core.AlignCenter, Overflow: core.Hidden},
			OnPress: func(core.PressEvent) {
				if l != nil {
					l.choose(c, i)
				}
			},
			Children: []core.Node{
				column(core.Style{Gap: 12, AlignItems: core.AlignCenter}, "",
					core.Text{Content: it.Title, MaxLines: 1, Style: textStyle(textSecondary, 500),
						ClassFunc: func() string {
							if on() {
								return "text-primary"
							}
							return "text-on-surface-variant"
						}},
					core.View{Class: "bg-primary", Style: core.Style{Height: core.Pt(3), Width: core.Pct(100), CornerRadii: core.Corners{TopLeft: 3, TopRight: 3}},
						StyleFunc: func() core.Style {
							if on() {
								return core.Style{Opacity: core.Alpha(1)}
							}
							return core.Style{Opacity: core.Alpha(0)}
						}},
				),
			},
		})
	}
	return column(core.Style{}, "bg-background",
		core.Scroll{Horizontal: true, HideIndicator: true, Style: core.Style{Height: core.Pt(48), FlexGrow: 0},
			Children: []core.Node{row(core.Style{Height: core.Pct(100), Padding: core.Symmetric(0, 8)}, "", tabs...)}},
		core.View{Style: core.Style{Height: core.Pt(1)}, Class: "bg-outline-variant"},
	)
}

// buttonsCard is pictures side by side, each as wide as the others and as
// tall as its shape.
func (a *App) buttonsCard(cx *core.Scope, c *backend.Card) core.Node {
	cells := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		ratio := it.Ratio
		if ratio <= 0 {
			ratio = 2.4
		}
		// The shape is on a child of the cell: a cell that grows along the row
		// with a shape of its own came out with no height.
		cells[i] = column(fill, "", tap(it.Title, func() { a.openCardItem(it) },
			core.Style{AspectRatio: ratio, CornerRadius: 12}, "bg-surface-container-high",
			a.picOf(cx, it.Pic, core.Cover, "", fillAbs)))
	}
	return row(core.Style{Gap: 8, Padding: core.Symmetric(12, 16)}, "", cells...)
}

func (a *App) clippedPic(cx *core.Scope, url string, w, h, radius core.Points) core.Node {
	return a.clippedPicFrom(cx, func() string { return url }, w, h, radius)
}

// An image's own corner radius does not clip what it draws, the view around it does.
func (a *App) clippedPicFrom(cx *core.Scope, url func() string, w, h, radius core.Points) core.Node {
	return core.View{Class: "bg-surface-container-high",
		Style:    core.Style{Width: core.Pt(w), Height: core.Pt(h), CornerRadius: radius, Overflow: core.Hidden, FlexShrink: 0},
		Children: []core.Node{a.pic(cx, url, core.Cover, "", fillAbs)}}
}

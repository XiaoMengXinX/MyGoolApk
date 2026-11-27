package app

import (
	"context"
	"slices"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// pageList is a server list whose selector card, where it has one, chooses
// what the rows after it show: the cards up to the selector stay, and the
// rest come from the chosen item's url.
type pageList struct {
	p                *pager[backend.PageEntry]
	chosen           *core.Signal[int]
	pageTitle        *core.Signal[string]
	catalogSelection *pageList
	// openCardTitle handles links that belong to the page holding this list.
	openCardTitle func(backend.CardItem)
	// onChoose, when set, hears the chosen item, as the follow tab keeps it.
	onChoose func(i int)

	// url and title name the selected child, if it has its own list.
	url, title        string
	cursor            backend.ListCursor
	contentURL        string
	deviceEnvironment *core.Signal[*backend.Card]
	// split lets the caller display the tabs of a page as separate lists.
	split     bool
	head      []backend.PageEntry
	inlineTab int
}

// newPageList lists url and fetches a selector's child only when it has its own list.
func (a *App) newPageList(cx *core.Scope, url, title string, head []backend.PageEntry, chosen int, source string) *pageList {
	l := &pageList{chosen: core.NewSignal(cx, chosen), pageTitle: core.NewSignal(cx, title), head: head, url: source, inlineTab: -1}
	if url == backend.DigitalPhoneURL && (cx.Env().OS() == core.OSIOS || cx.Env().OS() == core.OSAndroid) {
		l.deviceEnvironment = core.NewSignal[*backend.Card](cx, nil)
	}
	device := l.deviceEnvironment
	l.p = newBatchPager(cx, func(page int) func(context.Context) (pageBatch[backend.PageEntry], error) {
		q := backend.PageQuery{URL: url, Title: title, Page: page, Cursor: l.cursor, ContentURL: l.contentURL}
		src, named, head, inlineTab := l.url, l.title, l.head, l.inlineTab
		picked, remembered, split := l.chosen.GetUntracked(), l.onChoose != nil, l.split
		if src != "" {
			q.URL = src
			if named != "" {
				q.Title = named
			}
		}
		return func(ctx context.Context) (pageBatch[backend.PageEntry], error) {
			r, err := a.backend.PageEntries(ctx, q)
			if err != nil {
				return pageBatch[backend.PageEntry]{}, err
			}
			if page == 1 && len(r.Entries) == 0 && q.Cursor.FirstItem != "" {
				return pageBatch[backend.PageEntry]{unchanged: true}, nil
			}
			var pageTitle string
			if page == 1 && src == "" {
				pageTitle = r.Title
				for i, e := range r.Entries {
					c := e.Card
					if c == nil || !c.Chooses() {
						continue
					}
					if split && c.Kind == backend.CardTabs && i == len(r.Entries)-1 {
						r.HasMore = false
						break
					}
					if !remembered || picked < 0 || picked >= len(c.Items) {
						picked = c.DefaultTab
					}
					head = slices.Clone(r.Entries[:i+1])
					inlineTab = -1
					if i < len(r.Entries)-1 {
						inlineTab = c.DefaultTab
					}
					if picked != inlineTab {
						item := c.Items[picked]
						src, named = item.URL, item.Title
						r, err = a.backend.PageEntries(ctx, backend.PageQuery{URL: src, Title: named, Page: 1})
						if err != nil {
							return pageBatch[backend.PageEntry]{}, err
						}
					}
					break
				}
			}
			if page == 1 && src != "" {
				r.Entries = append(slices.Clone(head), belowHead(r.Entries, head)...)
			}
			var environment *backend.Card
			if device != nil {
				r.Entries = slices.DeleteFunc(r.Entries, func(e backend.PageEntry) bool {
					if e.Card != nil && e.Card.Kind == backend.CardDeviceEnvironment {
						environment = e.Card
						return true
					}
					return false
				})
			}
			return pageBatch[backend.PageEntry]{items: r.Entries, hasMore: r.HasMore, apply: func() {
				l.cursor, l.contentURL = r.Cursor, r.ContentURL
				if page == 1 {
					if device != nil {
						device.Set(environment)
					}
					l.url, l.title, l.head, l.inlineTab = src, named, head, inlineTab
					l.setChosen(picked)
					if pageTitle != "" {
						l.pageTitle.Set(pageTitle)
					}
				}
			}}, nil
		}
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
				g.AuthorFollowed = f.AuthorFollowed
				e.Feed = &g
			}
			return e
		})
	}
}

func (l *pageList) setChosen(i int) {
	if i != l.chosen.GetUntracked() {
		l.chosen.Set(i)
		if l.onChoose != nil {
			l.onChoose(i)
		}
	}
}

// choose keeps the leading cards while the new selection loads.
func (l *pageList) choose(c *backend.Card, i int) {
	if i == l.chosen.GetUntracked() || i >= len(c.Items) {
		return
	}
	l.setChosen(i)
	l.url, l.title = "", ""
	if i != l.inlineTab {
		l.url, l.title = c.Items[i].URL, c.Items[i].Title
	}
	l.cursor, l.contentURL = backend.ListCursor{}, ""
	l.p.items.Set(l.head)
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
		case backend.CardImageGrid:
			body = a.imageGridCard(cx, c)
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
		case backend.CardTimeline:
			body = a.timelineCard(cx, c)
		case backend.CardProductList:
			body = a.ratedListCard(cx, c)
		case backend.CardProductHighlights:
			body = a.productHighlights(cx, c)
		case backend.CardEntries:
			body = a.cardEntries(cx, c)
		case backend.CardMiniLinks:
			body = a.miniLinksCard(cx, c)
		case backend.CardFollowedProducts:
			body = a.pairsCard(cx, c)
		case backend.CardDeviceEnvironment:
			return core.Group{}
		case backend.CardAvatars:
			return a.avatarsCard(cx, c)
		case backend.CardTopicStrip:
			body = a.topicStrip(cx, c)
		case backend.CardPairs:
			body = a.pairsCard(cx, c)
		case backend.CardRanking:
			body = a.rankingCard(cx, c)
		case backend.CardTiles:
			body = a.tilesCard(cx, c)
			if c.Title == "" {
				return body
			}
		case backend.CardLogos:
			body = a.logosCard(cx, c)
		case backend.CardScroll:
			body = a.scrollCard(cx, c)
		case backend.CardEditorial:
			body = a.editorialCard(cx, c)
		case backend.CardTitle:
			if c.URL == "" {
				return cardTitle(c.Title, core.Edges{Top: 16, Right: 16, Bottom: 4, Left: 16})
			}
			body = core.Group{}
		case backend.CardSelector:
			return selectorCard(c, l)
		case backend.CardTabs:
			return tabsCard(c, l)
		case backend.CardButtons:
			body = a.buttonsCard(cx, c)
		case backend.CardConfigs:
			body = a.configsCard(c)
		case backend.CardNote:
			var subtitle core.Node = core.Group{}
			if c.Subtitle != "" {
				subtitle = label(c.Subtitle, textSecondary, 400, "text-on-surface-variant")
			}
			body = core.View{Style: core.Style{Padding: core.Symmetric(12, 16), Gap: 16}, Children: []core.Node{subtitle,
				a.richText(cx, func() ([]backend.RichSpan, string) { return c.Note, "" },
					richOptions{size: textBody, lineHeight: feedLineHeight, class: "text-on-surface"})}}
		}
		parts := []core.Node{}
		switch {
		case c.Title != "" && c.URL != "":
			// The title leads to all of what the card shows a few of.
			open := a.openCardItem
			if l != nil && l.openCardTitle != nil {
				open = l.openCardTitle
			}
			heading := tap(c.Title, func() { open(backend.CardItem{Title: c.Title, URL: c.URL}) },
				core.Style{Direction: core.Row, MinHeight: core.Pt(44), Padding: core.Symmetric(0, 16), Gap: 4, AlignItems: core.AlignCenter}, "bg-background",
				core.Text{Content: c.Title, MaxLines: 1, Class: "text-on-surface", Style: fill.Merge(textStyle(textBody, 600))},
				core.Text{Content: c.More, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textSecondary, 400)},
				icon(&symbols.ChevronRight, 20, "text-on-surface-variant"))
			if c.Kind == backend.CardTitle {
				return heading
			}
			parts = append(parts, heading)
		case c.Title != "":
			parts = append(parts, cardTitle(c.Title, core.Edges{Top: 12, Right: 16, Left: 16}))
		}
		parts = append(parts, body)
		if c.Inset {
			return column(core.Style{Padding: core.All(8)}, "bg-surface-container-low",
				column(core.Style{CornerRadius: 16, Overflow: core.Hidden}, "bg-background", parts...))
		}
		if !c.Continues {
			parts = append(parts, core.View{Style: core.Style{Height: core.Pt(8)}, Class: "bg-surface-container-low"})
		}
		return column(core.Style{}, "bg-background", parts...)
	})
}

func cardTitle(title string, pad core.Edges) core.Node {
	return core.View{Class: "bg-background", Style: core.Style{Padding: pad}, Children: []core.Node{
		label(title, textBody, 600, "text-on-surface")}}
}

// bannerCard pages through a loop at the card's aspect ratio.
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
	var ref core.Ref
	count := len(c.Items)
	items := c.Items
	page := 0
	if count > 1 {
		// Each end has a copy of its neighbor across the seam. Once that
		// copy is fully visible, scroll back to the identical original.
		items = make([]backend.CardItem, 0, count+2)
		items = append(items, c.Items[count-1])
		items = append(items, c.Items...)
		items = append(items, c.Items[0])
		page = 1
	}
	scrollTo := func(i int, animated bool) {
		ref.ScrollTo(core.Point{X: core.Points(i) * width.GetUntracked()}, animated)
	}
	var wheel func(core.WheelEvent)
	if count > 1 && hoverable {
		wheel = pagedWheel(&ref, width.GetUntracked, len(items), func() int { return page }, func(delta int) {
			if next := max(0, min(page+delta, len(items)-1)); next != page {
				scrollTo(next, true)
			}
		})
	}
	pages := make([]core.Node, len(items))
	for i, it := range items {
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
		Horizontal: true, HideIndicator: true, Ref: &ref,
		// A single picture leaves a sideways drag to the views around it.
		Behavior: core.ScrollBehavior{Paging: true, NoScroll: count == 1},
		Style:    core.Style{Direction: core.Row},
		// The pictures keep the shape between the margins.
		StyleFunc: func() core.Style {
			return core.Style{Height: core.Pt(max(width.Get()-2*margin, 0) / core.Points(ratio))}
		},
		OnLayout: func(b core.Bounds) {
			if b.Size.Width > 0 && b.Size.Width != width.GetUntracked() {
				width.Set(b.Size.Width)
				if count > 1 {
					scrollTo(page, false)
				}
			}
		},
		OnScroll: func(e core.ScrollEvent) {
			if count > 1 && !e.Stale && e.Size.Width > 0 {
				page = max(0, min(int(e.Offset.X/e.Size.Width+0.5), count+1))
				pos.Set((page + count - 1) % count)
			}
		},
		OnScrollEnd: func(e core.ScrollEvent) {
			if count == 1 || e.Stale || e.Size.Width <= 0 {
				return
			}
			switch {
			case e.Offset.X < 0.5:
				page = count
				scrollTo(page, false)
			case e.Offset.X >= core.Points(count+1)*e.Size.Width-0.5:
				page = 1
				scrollTo(page, false)
			}
		},
		Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, Height: core.Pct(100)}, OnWheel: wheel, Children: pages}},
	}
	parts := []core.Node{pager}
	if count > 1 {
		dots := make([]core.Node, count)
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
	const shown = 10
	const size, width, radius = 48, 68, 24
	cell := func(name string, open func(), pic core.Node) core.Node {
		return tap(name, open, core.Style{Width: core.Pt(width), FlexShrink: 0, Gap: 6, Padding: core.Symmetric(4, 2), AlignItems: core.AlignCenter, CornerRadius: 12}, "", pic,
			core.Text{Content: name, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400).Merge(core.Style{TextAlign: core.TextAlignCenter})})
	}
	var cells []core.Node
	for _, it := range c.Items[:min(shown, len(c.Items))] {
		cells = append(cells, cell(it.Title, func() { a.openCardItem(it) }, a.clippedPic(cx, it.Pic, size, size, radius)))
	}
	if c.URL != "" {
		cells = append(cells, cell("查看更多", func() { a.openCardItem(backend.CardItem{Title: c.Title, URL: c.URL}) },
			core.View{Class: "bg-secondary-container", Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				Children: []core.Node{icon(&symbols.ChevronRight, 24, "text-on-secondary-container")}}))
	}
	return column(core.Style{}, "bg-background", sideScroll{Children: []core.Node{
		row(core.Style{Padding: core.Edges{Top: 8, Right: 8, Bottom: 4, Left: 8}}, "", cells...)}})
}

func (a *App) topicStrip(cx *core.Scope, c *backend.Card) core.Node {
	width := core.NewSignal(cx, core.Points(0))
	cells := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		cell := tap(it.Title, func() { a.openCardItem(it) },
			core.Style{MinWidth: core.Pt(64), FlexShrink: 0, Gap: 6, Padding: core.Symmetric(4, 4),
				AlignItems: core.AlignCenter, CornerRadius: 12}, "",
			a.clippedPic(cx, it.Pic, 56, 56, 12),
			core.Text{Content: it.Title, MaxLines: 1, Class: "text-on-surface",
				Style: textStyle(textSecondary, 500).Merge(core.Style{TextAlign: core.TextAlignCenter})})
		cell.StyleFunc = func() core.Style { return core.Style{Width: core.Pt(width.Get() / 4)} }
		cells[i] = cell
	}
	return core.View{OnLayout: func(b core.Bounds) { width.Set(b.Size.Width) },
		Children: []core.Node{sideScroll{Children: []core.Node{
			row(core.Style{Padding: core.Symmetric(4, 0)}, "", cells...),
		}}}}
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
			cells[j] = tap(it.Title, func() { a.openCardItem(it) },
				fill.Merge(core.Style{AlignItems: core.AlignCenter, Gap: 6, Padding: core.Symmetric(8, 4), CornerRadius: 12}), "",
				a.rankedPicture(cx, func() string { return it.Pic }, size, func() int { return n + 1 }),
				column(core.Style{Width: core.Pct(100), AlignItems: core.AlignCenter, Gap: 3}, "",
					core.Text{Content: it.Title, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textCaption, 400).Merge(core.Style{TextAlign: core.TextAlignCenter})},
					heatCount(func() string { return it.Hot })))
		}
		rows = append(rows, row(core.Style{AlignItems: core.AlignStart}, "", cells...))
	}
	return column(core.Style{Padding: core.Edges{Top: 4, Right: 8, Bottom: 8, Left: 8}}, "", rows...)
}

// tilesCard overlays names on pictures in a horizontal strip.
func (a *App) tilesCard(cx *core.Scope, c *backend.Card) core.Node {
	width, height := core.Points(76), core.Points(76)
	if c.Ratio > 0 {
		width = 104
		height = width / core.Points(c.Ratio)
	}
	tiles := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		tiles[i] = tap(it.Title, func() { a.openCardItem(it) },
			core.Style{Width: core.Pt(width), Height: core.Pt(height), FlexShrink: 0, CornerRadius: 12}, "bg-surface-container-high",
			a.picOf(cx, it.Pic, core.Cover, "", fillAbs),
			core.View{Class: "bg-black/40", Style: fillAbs.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(4)}),
				PointerEvents: core.PointerEventsNone,
				Children: []core.Node{core.Text{Content: it.Title, MaxLines: 2, Class: "text-white",
					Style: textStyle(textSecondary, 600).Merge(core.Style{TextAlign: core.TextAlignCenter})}}})
	}
	return column(core.Style{}, "bg-background", sideScroll{Children: []core.Node{
		row(core.Style{Gap: 8, Padding: core.Symmetric(8, 8)}, "", tiles...)}})
}

// logosCard is the logos of brands, two to a column, in columns that scroll
// sideways. The logos come with white backgrounds of their own (JPEGs and
// PNGs without alpha), so each sits on a white tile of its shape.
func (a *App) logosCard(cx *core.Scope, c *backend.Card) core.Node {
	const w, h = 64, 32
	cell := func(it backend.CardItem) core.Node {
		return tap(it.Title, func() { a.openCardItem(it) }, core.Style{Width: core.Pt(w + 8), Gap: 4, Padding: core.Symmetric(4, 0), AlignItems: core.AlignCenter, CornerRadius: 8}, "",
			core.View{Class: "bg-white", Style: core.Style{Width: core.Pt(w), Height: core.Pt(h), CornerRadius: 8, Overflow: core.Hidden},
				Children: []core.Node{a.picOf(cx, it.Pic, core.Cover, "", fillAbs)}},
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
	return sideScroll{Children: []core.Node{
		row(core.Style{Gap: 4, Padding: core.Symmetric(4, 12)}, "", cols...)}}
}

// iconsCard is a grid of icons: five to a row for links, four for topics,
// products and people, whose names are longer and which have a line more.
func (a *App) iconsCard(cx *core.Scope, c *backend.Card) core.Node {
	cols := c.Columns
	if cols == 0 {
		cols = 4
		if it := c.Items[0]; it.Subtitle == "" && it.Hot == "" && !it.Round {
			cols = 5
		}
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
			if it.Hot != "" && it.Hot != "0" {
				parts = append(parts, heatCount(func() string { return it.Hot }))
			} else if it.Subtitle != "" {
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
	return sideScroll{Children: []core.Node{
		row(core.Style{Gap: 8, Padding: core.Symmetric(12, 16)}, "", chips...)}}
}

// listCard is a few rows with an icon, a name and a line under it.
func (a *App) listCard(cx *core.Scope, c *backend.Card) core.Node {
	thumbnails := slices.ContainsFunc(c.Items, func(item backend.CardItem) bool { return item.Pic != "" })
	rows := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		var thumbnail core.Node = core.Group{}
		if it.Pic != "" {
			radius := core.Points(10)
			if it.Round {
				radius = 20
			}
			thumbnail = a.clippedPic(cx, it.Pic, 40, 40, radius)
		} else if thumbnails {
			thumbnail = column(core.Style{Width: core.Pt(40), Height: core.Pt(40), FlexShrink: 0, CornerRadius: 10,
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, "bg-surface-container-low",
				icon(&symbols.Article, 20, "text-on-surface-variant"))
		}
		rows[i] = tap(it.Title, func() { a.openCardItem(it) },
			core.Style{Direction: core.Row, Padding: core.Symmetric(8, 16), Gap: 12, AlignItems: core.AlignCenter}, "",
			thumbnail,
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

func (a *App) editorialCard(cx *core.Scope, c *backend.Card) core.Node {
	const width = 216
	items := make([]core.Node, 0, len(c.Items))
	for _, it := range c.Items {
		items = append(items, tap(it.Title, func() { a.openCardItem(it) },
			core.Style{Width: core.Pt(width), CornerRadius: 12, FlexShrink: 0}, "bg-surface-container-high",
			a.clippedPic(cx, it.Pic, width, width*0.45, 0),
			core.Text{Content: it.Title, MaxLines: 2, Class: "text-on-surface",
				Style: textStyle(textSecondary, 600).Merge(core.Style{Padding: core.Symmetric(8, 12), MinHeight: core.Pt(52)})}))
	}
	return sideScroll{Children: []core.Node{
		row(core.Style{Gap: 10, Padding: core.Symmetric(12, 8)}, "", items...),
	}}
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
	return sideScroll{Children: []core.Node{
		row(core.Style{Gap: 12, Padding: core.Symmetric(12, 16)}, "", items...)}}
}

// selectorCard is filter chips over the list they choose.
func selectorCard(c *backend.Card, l *pageList) core.Node {
	chips := make([]core.Node, len(c.Items))
	for i, it := range c.Items {
		on := func() bool { return l != nil && l.chosen.Get() == i }
		chips[i] = compactChip(it.Title, on, func() {
			if l != nil {
				l.choose(c, i)
			}
		})
	}
	return column(core.Style{}, "bg-background",
		sideScroll{Children: []core.Node{
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
		sideScroll{Style: core.Style{Height: core.Pt(48), FlexGrow: 0},
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
	return core.View{
		Style:    core.Style{Width: core.Pt(w), Height: core.Pt(h), CornerRadius: radius, Overflow: core.Hidden, FlexShrink: 0},
		Children: []core.Node{a.pic(cx, url, core.Cover, "", fillAbs)}}
}

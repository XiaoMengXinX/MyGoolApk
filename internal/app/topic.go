package app

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// The route id of a topic holds the product id and the name, which may hold
// "/", as base64url of "id\x00name".
func topicParam(name, productID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(productID + "\x00" + name))
}

// openTopic shows a topic above the current page.
func (a *App) openTopic(name string) {
	if name = strings.Trim(strings.TrimSpace(name), "#"); name != "" {
		a.push(routeTopic, topicParam(name, ""))
	}
}

// openTarget shows the topic or product a post is filed under.
func (a *App) openTarget(t *backend.Target) {
	a.push(routeTopic, topicParam(t.Title, t.ID))
}

// topicPage is the page of a topic or a product: its head with the follow
// button over the lists the server names, which swipe as the home tabs do.
// A topic whose page tells no lists shows its posts alone.
func (a *App) topicPage(cx *core.Scope, param string) core.Node {
	raw, _ := base64.RawURLEncoding.DecodeString(param)
	productID, name, _ := strings.Cut(string(raw), "\x00")
	title := "#" + name + "#"
	if productID != "" {
		title = name
	}
	detail := core.NewSignalFunc(cx, backend.TopicDetail{}, nil)
	loaded, errText := core.NewSignal(cx, false), core.NewSignal(cx, "")
	load := func() {
		background(a, func(ctx context.Context) (backend.TopicDetail, error) { return a.backend.Topic(ctx, name, productID) },
			func(d backend.TopicDetail, err error) {
				if err != nil {
					errText.Set(err.Error())
					return
				}
				detail.Set(d)
				loaded.Set(true)
			})
	}
	load()
	busy := false
	follow := func() {
		if busy || !a.requireLogin() {
			return
		}
		busy = true
		to := !detail.GetUntracked().Followed
		backgroundErr(a, func(ctx context.Context) error { return a.backend.SetTopicFollow(ctx, name, productID, to) },
			func(err error) {
				busy = false
				if err != nil {
					a.notice("操作失败：" + err.Error())
					return
				}
				d := detail.GetUntracked()
				d.Followed = to
				if to {
					d.Followers++
				} else {
					d.Followers--
				}
				detail.Set(d)
			})
	}
	// A product a link named by its id alone is titled once loaded.
	shown := func() string {
		if name == "" {
			return detail.Get().Title
		}
		return title
	}
	return a.screen(shown, nil, core.Dynamic(func(cx *core.Scope) core.Node {
		switch {
		case !loaded.Get() && errText.Get() != "":
			return statusBlock("加载失败", errText.Get(), "重试", func() { errText.Set(""); load() })
		case !loaded.Get():
			return column(fill.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}), "", spinner())
		}
		d := core.Untracked(cx, detail.Get)
		head := a.topicHead(cx, detail.Get, follow)
		if len(d.Tabs) == 0 {
			feeds := newPager(cx, func(ctx context.Context, page int) ([]backend.Feed, bool, error) {
				r, err := a.backend.Feed(ctx, backend.FeedQuery{Kind: backend.FeedTopic, Topic: name, ProductID: productID, Page: page})
				return r.Feeds, r.HasMore, err
			}, func(f backend.Feed) string { return f.ID })
			return pagerList(cx, feeds, listSpec{pushed: true, header: head, empty: "暂无内容", emptyHint: "这里还没有帖子。", bottom: 24, skeleton: skeletonCard},
				func(cx *core.Scope, get func() backend.Feed, r core.ListRow) core.Node { return a.feedCard(cx, get) })
		}
		pages := make([]pageTab, len(d.Tabs))
		for i, t := range d.Tabs {
			pages[i] = pageTab{Title: t.Title, Kind: tabPage, URL: t.URL}
		}
		g := a.newTabGroup(cx, pages, d.Selected)
		g.pushed, g.head = true, head
		return g.view(cx, nil)
	}))
}

// topicHead shows what a topic is: its logo, name and counts, the button
// that follows it, and its introduction.
func (a *App) topicHead(cx *core.Scope, get func() backend.TopicDetail, follow func()) core.Node {
	counts := func() string {
		d := get()
		hot := ""
		if d.Hot != "" {
			hot = d.Hot + " 热度"
		}
		return joinNonEmpty([]string{hot, countLabel(d.Comments, "讨论"), countLabel(d.Followers, "关注")}, "  ")
	}
	followed := core.NewMemo(cx, func() bool { return get().Followed })
	// The rating and the tags are the page's own, which a follow leaves alone.
	d := core.Untracked(cx, get)
	var more []core.Node
	if d.Rating != nil {
		more = append(more, ratingCard(d.Rating))
	}
	if len(d.Tags) > 0 {
		chips := make([]core.Node, len(d.Tags))
		for i, t := range d.Tags {
			chips[i] = core.View{Class: "bg-surface-container-high",
				Style:    core.Style{Height: core.Pt(32), Padding: core.Symmetric(0, 12), CornerRadius: 8, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				Children: []core.Node{core.Text{Content: t, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textSecondary, 500)}}}
		}
		more = append(more, core.Scroll{Horizontal: true, HideIndicator: true, Style: core.Style{Margin: core.Symmetric(0, -16)},
			Children: []core.Node{row(core.Style{Gap: 8, Padding: core.Symmetric(0, 16)}, "", chips...)}})
	}
	return column(core.Style{Padding: core.Edges{Top: 8, Right: 16, Bottom: 12, Left: 16}, Gap: 12}, "bg-background",
		row(core.Style{Gap: 12, AlignItems: core.AlignCenter}, "",
			a.pic(cx, func() string { return get().LogoURL }, core.Cover, "bg-surface-container-highest",
				core.Style{Width: core.Pt(56), Height: core.Pt(56), CornerRadius: 14, Overflow: core.Hidden, FlexShrink: 0}),
			column(fill.Merge(core.Style{Gap: 4}), "",
				core.Text{ContentFunc: func() string { return get().Title }, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textBody, 600)},
				core.Text{ContentFunc: counts, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)}),
			core.Dynamic(func(cx *core.Scope) core.Node {
				if followed.Get() {
					return tonalButton("已关注", follow)
				}
				return filledButton("关注", func() bool { return true }, follow)
			})),
		dynamicShape(cx, func() bool { return get().Intro != "" }, func(cx *core.Scope, has bool) core.Node {
			if !has {
				return core.Group{}
			}
			return core.Text{ContentFunc: func() string { return get().Intro }, MaxLines: 3, Class: "text-on-surface-variant", Style: textStyle(textSecondary, 400)}
		}),
		core.Group(more))
}

// ratingCard is a product's score from its owners beside the score of each
// of its sides, over how many rated it.
func ratingCard(r *backend.Rating) core.Node {
	sides := make([]core.Node, len(r.Sides))
	for i, s := range r.Sides {
		sides[i] = row(core.Style{Gap: 8, AlignItems: core.AlignCenter}, "",
			core.Text{Content: s.Name, MaxLines: 1, Class: "text-on-surface-variant",
				Style: textStyle(textCaption, 400).Merge(core.Style{Width: core.Pt(52), TextAlign: core.TextAlignRight})},
			core.View{Class: "bg-surface-container-highest", Style: fill.Merge(core.Style{Height: core.Pt(6), CornerRadius: 3, Overflow: core.Hidden}),
				Children: []core.Node{core.View{Class: "bg-primary", Style: core.Style{Width: core.Pct(s.Score * 10), Height: core.Pct(100), CornerRadius: 3}}}},
			core.Text{Content: strconv.FormatFloat(float64(s.Score), 'f', 1, 32), Class: "text-on-surface-variant",
				Style: textStyle(textCaption, 400).Merge(core.Style{Width: core.Pt(28)})})
	}
	return column(core.Style{Padding: core.All(16), Gap: 12, CornerRadius: 16}, "bg-surface-container-low",
		row(core.Style{Gap: 16, AlignItems: core.AlignCenter}, "",
			column(core.Style{Width: core.Pt(88), Gap: 2, AlignItems: core.AlignCenter}, "",
				label(r.Score, textTitle, 600, "text-on-surface"),
				label("机主评分", textCaption, 400, "text-on-surface-variant")),
			column(fill.Merge(core.Style{Gap: 6}), "", sides...)),
		label(joinNonEmpty([]string{countLabel(r.Owners, "机主点评"), countLabel(r.Detailed, "详细点评")}, "  "), textCaption, 400, "text-on-surface-variant"))
}

// listPage shows a server list that a row opened, such as the products of a
// category.
func (a *App) listPage(cx *core.Scope, param string) core.Node {
	raw, _ := base64.RawURLEncoding.DecodeString(param)
	title, url, _ := strings.Cut(string(raw), "\x00")
	switch url {
	case backend.TradeBrandsURL:
		return a.tradeBrandsPage(cx, title)
	case backend.TradeCityURL:
		return a.tradeCityPage(cx, title)
	}
	l := a.newPageList(cx, url, title, nil, 0, "")
	l.split = true
	// at is the row of the tabs the page ends with, -1 when it has none.
	at := core.NewMemo(cx, func() int {
		rows := l.p.items.Get()
		if n := len(rows) - 1; n >= 0 && rows[n].Card != nil && rows[n].Card.Kind == backend.CardTabs {
			return n
		}
		return -1
	})
	// A page that ends with tabs swipes between their lists as the home tabs
	// do, under the cards before them.
	return a.screen(titled(title), nil, core.Dynamic(func(cx *core.Scope) core.Node {
		n := at.Get()
		if n < 0 {
			return a.entryList(cx, l, listSpec{pushed: true})
		}
		rows := l.p.items.GetUntracked()
		c := rows[n].Card
		pages := make([]pageTab, len(c.Items))
		for i, it := range c.Items {
			pages[i] = pageTab{Title: it.Title, Kind: tabPage, URL: it.URL}
		}
		g := a.newTabGroup(cx, pages, 0)
		g.pushed = true
		if n > 0 {
			head := make([]core.Node, n)
			for i := range head {
				head[i] = a.entryView(cx, func() backend.PageEntry { return rows[i] }, nil)
			}
			g.head = column(core.Style{}, "bg-background", head...)
		}
		return g.view(cx, nil)
	}))
}

func (a *App) openList(title, url string) {
	a.push(routeList, base64.RawURLEncoding.EncodeToString([]byte(title+"\x00"+url)))
}

const topicColumns = 3

// topicRow is one row of the topic grid.
type topicRow struct {
	ID    string
	Items []backend.TopicItem
}

// topicHome is the topic tab: a rail of categories and the grid of the
// selected one, as the official clients lay it out.
type topicHome struct {
	a        *App
	cats     *core.Signal[[]backend.TopicCategory]
	loading  bool
	err      *core.Signal[string]
	selected *core.Signal[int]
	pagers   map[int]*pager[topicRow]
	started  bool
}

func (a *App) newTopicHome(cx *core.Scope) *topicHome {
	return &topicHome{
		a: a, cats: core.NewSignalFunc(cx, []backend.TopicCategory(nil), nil),
		err: core.NewSignal(cx, ""), selected: core.NewSignal(cx, 0), pagers: map[int]*pager[topicRow]{},
	}
}

func (t *topicHome) load() {
	if t.loading {
		return
	}
	t.loading = true
	t.started = true
	background(t.a, t.a.backend.TopicCategories, func(cats []backend.TopicCategory, err error) {
		t.loading = false
		if err != nil {
			t.err.Set(err.Error())
			return
		}
		t.cats.Set(cats)
		t.err.Set("")
	})
}

// pager returns the grid of category i, made when first shown.
func (t *topicHome) pager(i int) *pager[topicRow] {
	if p := t.pagers[i]; p != nil {
		return p
	}
	cat := t.cats.GetUntracked()[i]
	p := newPager(t.a.cx, func(ctx context.Context, page int) ([]topicRow, bool, error) {
		r, err := t.a.backend.TopicList(ctx, cat, page)
		var rows []topicRow
		for j := 0; j < len(r.Items); j += topicColumns {
			row := topicRow{Items: r.Items[j:min(j+topicColumns, len(r.Items))]}
			row.ID = row.Items[0].ID
			rows = append(rows, row)
		}
		return rows, r.HasMore, err
	}, func(r topicRow) string { return r.ID })
	t.pagers[i] = p
	return p
}

func (t *topicHome) view(cx *core.Scope) core.Node {
	a := t.a
	return core.Dynamic(func(cx *core.Scope) core.Node {
		if len(t.cats.Get()) == 0 {
			if !t.started {
				t.load()
			}
			if t.err.Get() != "" {
				return statusBlock("加载失败", t.err.Get(), "重试", func() { t.err.Set(""); t.load() })
			}
			return column(fill.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}), "", spinner())
		}
		var rail []core.Node
		for i, cat := range t.cats.Get() {
			i := i
			on := func() bool { return t.selected.Get() == i }
			rail = append(rail, core.View{
				Role: core.RoleTab, Label: cat.Title, OnPress: func(core.PressEvent) { t.selected.Set(i) },
				Android: core.AndroidViewProps{Ripple: &core.Ripple{}},
				ClassFunc: func() string {
					if on() {
						return "bg-secondary-container"
					}
					return ""
				},
				Style: core.Style{Height: core.Pt(48), CornerRadius: 24, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Overflow: core.Hidden},
				Children: []core.Node{core.Text{Content: cat.Title, MaxLines: 1, Style: textStyle(textSecondary, 500), ClassFunc: func() string {
					if on() {
						return "text-on-secondary-container"
					}
					return "text-on-surface-variant"
				}}},
			})
		}
		return row(fill, "",
			core.Scroll{HideIndicator: true, Style: core.Style{Width: core.Pt(88), FlexShrink: 0}, Class: "bg-surface-container-low",
				Children: []core.Node{column(core.Style{Padding: core.Symmetric(8, 8), Gap: 4}, "", rail...)}},
			core.Dynamic(func(cx *core.Scope) core.Node {
				return column(fill, "", pagerList(cx, t.pager(t.selected.Get()), listSpec{pushed: true, empty: "暂无话题"},
					func(cx *core.Scope, get func() topicRow, r core.ListRow) core.Node { return a.topicGridRow(cx, get) }))
			}),
		)
	})
}

func (a *App) topicGridRow(cx *core.Scope, get func() topicRow) core.Node {
	var cells []core.Node
	for i := 0; i < topicColumns; i++ {
		i := i
		item := func() (backend.TopicItem, bool) {
			r := get()
			if i < len(r.Items) {
				return r.Items[i], true
			}
			return backend.TopicItem{}, false
		}
		has := core.NewMemo(cx, func() bool { _, ok := item(); return ok })
		cell := core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0)}
		cells = append(cells, core.Dynamic(func(cx *core.Scope) core.Node {
			if !has.Get() {
				return core.View{Style: cell}
			}
			return tap("话题", func() {
				if it, ok := item(); ok {
					a.openTopic(it.Title)
				}
			}, cell.Merge(core.Style{Padding: core.Symmetric(8, 4), Gap: 6, AlignItems: core.AlignCenter, CornerRadius: 12, Overflow: core.Hidden}), "",
				a.pic(cx, func() string { it, _ := item(); return it.LogoURL }, core.Cover, "bg-surface-container-highest",
					core.Style{Width: core.Pt(56), Height: core.Pt(56), CornerRadius: 14}),
				core.Text{ContentFunc: func() string { it, _ := item(); return it.Title }, MaxLines: 2, Class: "text-on-surface", Style: func() core.Style {
					s := textStyle(textSecondary, 500)
					s.TextAlign = core.TextAlignCenter
					return s
				}()},
				core.Text{ContentFunc: func() string { it, _ := item(); return it.Subtitle }, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)},
			)
		}))
	}
	return row(core.Style{Padding: core.Edges{Top: 12, Right: 8, Bottom: 4, Left: 8}, AlignItems: core.AlignStart}, "", cells...)
}

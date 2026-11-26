package app

import (
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

type searchResultTab struct {
	query *core.Signal[backend.SearchQuery]
	list  *pageList
}

func (t searchResultTab) apply(update func(*backend.SearchQuery)) {
	previous := t.query.GetUntracked()
	next := previous
	update(&next)
	if next != previous {
		t.query.Set(next)
		t.list.p.list.ScrollTo(core.Point{}, false)
		t.list.p.reset()
	}
}

type searchOption struct {
	label, value string
}

var feedSearchOrders = []searchOption{
	{"综合", ""}, {"实时", "dateline"}, {"热度", "hot"}, {"评论", "reply"}, {"精确", "none"},
}

var feedSearchTypes = []searchOption{
	{"全部", ""}, {"动态", "feed"}, {"图文", "feedArticle"}, {"点评", "rating"},
	{"好物推荐", "goods"}, {"酷图", "picture"}, {"提问", "question"}, {"回答", "answer"}, {"评论", "comment"},
	{"视频", "video"}, {"二手", "ershou"}, {"投票", "vote"},
}

var questionSearchTypes = []searchOption{
	{"全部问答", ""}, {"只看提问", "question"}, {"只看回答", "answer"},
}

var tradeSearchOrders = []searchOption{
	{"综合排序", ""}, {"按发布时间", "dateline"}, {"价格从低到高", "priceDown"},
	{"价格从高到低", "priceUp"}, {"只看面议", backend.SearchTradeNegotiable},
}

func (a *App) searchResultView(cx *core.Scope, tab searchResultTab, spec listSpec) core.Node {
	kind := tab.query.GetUntracked().Kind
	switch kind {
	case backend.SearchProducts, backend.SearchTopics:
		spec.empty, spec.emptyHint, spec.bottom, spec.skeleton = "暂无内容", "换个关键词试试。", 24, skeletonCard
		return pagerList(cx, tab.list.p, spec, func(cx *core.Scope, get func() backend.PageEntry, _ core.ListRow) core.Node {
			return a.searchMatchRow(cx, kind == backend.SearchProducts, get)
		})
	case backend.SearchFeeds:
		sort := searchFilterChips(feedSearchOrders, func() string { return tab.query.Get().Sort }, func(value string) {
			tab.apply(func(q *backend.SearchQuery) { q.Sort = value })
		})
		types := searchFilterMenu("动态类型", feedSearchTypes, func() string { return tab.query.Get().FeedType }, func(value string) {
			tab.apply(func(q *backend.SearchQuery) { q.FeedType = value })
		})
		return column(fill, "bg-background", searchFilterBar(sort, types), a.entryList(cx, tab.list, spec))
	case backend.SearchQuestions:
		filters := searchFilterChips(questionSearchTypes, func() string { return tab.query.Get().FeedType }, func(value string) {
			tab.apply(func(q *backend.SearchQuery) { q.FeedType = value })
		})
		return column(fill, "bg-background", searchFilterBar(filters), a.entryList(cx, tab.list, spec))
	case backend.SearchTrades:
		return column(fill, "bg-background", a.tradeSearchFilters(cx, tab), a.entryList(cx, tab.list, spec))
	}
	return a.entryList(cx, tab.list, spec)
}

func searchFilterBar(children ...core.Node) core.Node {
	return row(core.Style{Height: core.Pt(52), FlexShrink: 0, Padding: core.Symmetric(0, 12), Gap: 8, AlignItems: core.AlignCenter}, "bg-background", children...)
}

func searchFilterChips(options []searchOption, selected func() string, apply func(string)) core.Node {
	choices := make([]core.Node, len(options))
	for i, opt := range options {
		active := func() bool { return selected() == opt.value }
		choice := tap(opt.label, func() { apply(opt.value) },
			core.Style{Height: core.Pt(40), Padding: core.Symmetric(0, 9), CornerRadius: 20, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, "",
			core.Text{Content: opt.label, MaxLines: 1, Style: textStyle(textSecondary, 500), ClassFunc: func() string {
				return filterTone(active(), "text-on-secondary-container", "text-on-surface-variant")
			}})
		choice.Selected = active
		choice.ClassFunc = func() string { return filterTone(active(), "bg-secondary-container", "") }
		choices[i] = choice
	}
	return core.Scroll{Horizontal: true, HideIndicator: true, Style: fill.Merge(core.Style{Height: core.Pt(44), MinWidth: core.Pt(0)}),
		Children: []core.Node{row(core.Style{Height: core.Pct(100), Gap: 2, AlignItems: core.AlignCenter}, "", choices...)}}
}

func filterTone(active bool, selected, idle string) string {
	if active {
		return selected
	}
	return idle
}

func searchFilterMenu(name string, options []searchOption, selected func() string, apply func(string)) core.Node {
	items := make([]core.Node, len(options))
	for i, opt := range options {
		items[i] = base.MenuItem{Label: opt.label, Role: core.RoleMenuItemRadio,
			OnSelect: func() { apply(opt.value) }, Checked: func() bool { return selected() == opt.value },
			Style: core.Style{Height: core.Pt(48), Padding: core.Symmetric(0, 16), Direction: core.Row, Gap: 12, AlignItems: core.AlignCenter, MinWidth: core.Pt(156)},
			Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(18), Height: core.Pt(18)}, Children: []core.Node{
					core.Show{When: func() bool { return selected() == opt.value }, Then: func(*core.Scope) core.Node { return icon(&symbols.Check, 18, "text-primary") }},
				}},
				label(opt.label, textSecondary, 500, "text-on-surface"),
			}}
	}
	rest := core.Style{Opacity: core.Alpha(0), Transform: core.Transform{TranslateY: -6}}
	return base.Menu{Align: core.AlignEnd, Offset: 4, Margin: 8,
		LabelFunc: func() string { return name + "，" + searchOptionLabel(options, selected()) },
		Style:     core.Style{FlexShrink: 0, Height: core.Pt(40), Padding: core.Symmetric(0, 10), CornerRadius: 20, JustifyContent: core.JustifyCenter},
		Class:     "bg-surface-container", PopupClass: "bg-surface-container-high",
		PopupStyle: core.Style{CornerRadius: 16, Padding: core.Symmetric(6, 0),
			Transition: core.Transition{Duration: 180 * time.Millisecond, Easing: emphasized, Properties: core.TransitionOpacity | core.TransitionTransform}},
		StartingStyle: rest, EndingStyle: rest,
		Trigger: row(core.Style{Gap: 4, AlignItems: core.AlignCenter}, "",
			core.Text{ContentFunc: func() string { return searchOptionLabel(options, selected()) }, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textSecondary, 500)},
			core.View{Style: core.Style{Width: core.Pt(16), Height: core.Pt(16), Transform: core.Transform{Rotate: 90}},
				Children: []core.Node{icon(&symbols.ChevronRight, 16, "text-on-surface-variant")}}),
		Children: items}
}

func searchOptionLabel(options []searchOption, value string) string {
	for _, opt := range options {
		if opt.value == value {
			return opt.label
		}
	}
	return options[0].label
}

func (a *App) tradeSearchFilters(cx *core.Scope, tab searchResultTab) core.Node {
	order := searchFilterMenu("二手排序", tradeSearchOrders, func() string { return tab.query.Get().Sort }, func(value string) {
		tab.apply(func(q *backend.SearchQuery) { q.Sort = value })
	})
	cityOpen := core.NewSignal(cx, false)
	cityInput := core.NewSignal(cx, "")
	currentArea := func() string {
		if tab.query.Get().City != "" {
			return "city"
		}
		return ""
	}
	area := searchFilterMenu("二手地区", []searchOption{{"全部", ""}, {"只看同城", "city"}}, currentArea, func(value string) {
		if value == "" {
			tab.apply(func(q *backend.SearchQuery) { q.City = "" })
			return
		}
		city := tab.query.GetUntracked().City
		if city == "" {
			city = a.tradeCity()
		}
		cityInput.Set(city)
		cityOpen.Set(true)
	})
	name := func() string { return strings.TrimSuffix(strings.TrimSpace(cityInput.Get()), "市") }
	apply := func() {
		city := name()
		if city == "" {
			return
		}
		cx.DismissKeyboard()
		cityOpen.Set(false)
		tab.apply(func(q *backend.SearchQuery) { q.City = city })
	}
	return core.Group{
		searchFilterBar(order, area),
		a.bottomSheet(cityOpen.Get, func() { cityOpen.Set(false) },
			label("只看同城", textTitle, 500, "text-on-surface"),
			core.Input{Value: cityInput.Get, OnChange: cityInput.Set, OnSubmit: func(string) { apply() },
				Placeholder: "城市，如深圳", Label: "城市", ReturnKeyType: core.ReturnKeySearch, Class: "bg-surface-container-high text-on-surface",
				Style: textStyle(textBody, 400).Merge(core.Style{Height: core.Pt(48), Padding: core.Symmetric(0, 12), CornerRadius: 8})},
			filledButton("应用", func() bool { return name() != "" }, apply)),
	}
}

func (a *App) searchPreview(cx *core.Scope, c *backend.Card, open func(backend.CardItem)) core.Node {
	parts := []core.Node{cardTitle(c.Title, core.Edges{Top: 12, Right: 16, Bottom: 4, Left: 16})}
	for _, result := range c.Results {
		parts = append(parts, a.searchMatchRow(cx, result.Kind == backend.EntryProduct, func() backend.PageEntry { return result }))
	}
	if c.More != "" {
		parts = append(parts, tap(c.More, func() { open(backend.CardItem{Title: c.Title, URL: c.URL}) },
			core.Style{Direction: core.Row, Height: core.Pt(44), Padding: core.Symmetric(0, 16), Gap: 4, AlignItems: core.AlignCenter}, "",
			label(c.More, textSecondary, 500, "text-primary"), icon(&symbols.ChevronRight, 18, "text-primary")))
	}
	return column(core.Style{Padding: core.Symmetric(4, 8)}, "bg-surface-container-low",
		column(core.Style{CornerRadius: 16, Overflow: core.Hidden}, "bg-background", parts...))
}

func (a *App) searchMatchRow(cx *core.Scope, product bool, get func() backend.PageEntry) core.Node {
	var thumbnail core.Node
	if product {
		thumbnail = core.View{Class: "bg-white", Style: core.Style{Width: core.Pt(56), Height: core.Pt(56), CornerRadius: 10, Overflow: core.Hidden, FlexShrink: 0},
			Children: []core.Node{a.pic(cx, func() string { return get().LogoURL }, core.Contain, "", fillAbs)}}
	} else {
		thumbnail = a.clippedPicFrom(cx, func() string { return get().LogoURL }, 48, 48, 10)
	}
	view := tap("", func() {
		e := get()
		if e.Kind == backend.EntryProduct {
			a.openTarget(&backend.Target{ID: e.ID, Title: e.Title, Product: true})
		} else {
			a.openCardItem(backend.CardItem{Title: e.Title, URL: e.URL})
		}
	}, core.Style{Direction: core.Row, Padding: core.Symmetric(10, 16), Gap: 12, AlignItems: core.AlignCenter}, "bg-background",
		thumbnail,
		column(fill.Merge(core.Style{MinWidth: core.Pt(0), Gap: 4}), "",
			core.Text{ContentFunc: func() string { return get().Title }, MaxLines: 2, Class: "text-on-surface", Style: textStyle(textBody, 600)},
			core.Text{ContentFunc: func() string {
				e := get()
				metrics := []string{e.Hot, e.Discussions}
				for i, unit := range []string{"热度", "讨论"} {
					if metrics[i] != "" {
						metrics[i] += unit
					}
				}
				return strings.TrimSpace(strings.Join(metrics, "  "))
			}, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textSecondary, 400)}),
		dynamicShape(cx, func() string { return get().Score }, func(cx *core.Scope, score string) core.Node {
			if score == "" {
				return icon(&symbols.ChevronRight, 20, "text-on-surface-variant")
			}
			value, _ := strconv.ParseFloat(score, 64)
			return column(core.Style{Gap: 2, AlignItems: core.AlignCenter, FlexShrink: 0}, "",
				label(score, textTitle, 600, "text-on-surface"), stars(value/2, 12))
		}))
	view.LabelFunc = func() string { return get().Title }
	return view
}

package app

import (
	"context"
	"strings"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// tradeBrandsPage browses what second-hand posts sell: a rail of the kinds
// of goods and the product categories, beside the list of the picked one. A
// product opens its second-hand posts.
func (a *App) tradeBrandsPage(cx *core.Scope, title string) core.Node {
	rail := core.NewSignalFunc(cx, []backend.PageEntry{{Title: "商品类型", URL: backend.TradeBrandsURL}}, nil)
	picked := core.NewSignal(cx, 0)
	background(a, func(ctx context.Context) (backend.PageEntriesResult, error) {
		return a.backend.PageEntries(ctx, backend.PageQuery{URL: backend.ProductCategoriesURL, Page: 1})
	}, func(r backend.PageEntriesResult, err error) {
		if err != nil {
			a.notice("加载分类失败：" + err.Error())
			return
		}
		rows := rail.GetUntracked()
		for _, e := range r.Entries {
			if e.Kind == backend.EntryCategory {
				rows = append(rows, e)
			}
		}
		rail.Set(rows)
	})
	names := core.Dynamic(func(cx *core.Scope) core.Node {
		rows := rail.Get()
		items := make([]core.Node, len(rows))
		for i, e := range rows {
			on := func() bool { return picked.Get() == i }
			items[i] = pressable(core.View{
				Role: core.RoleTab, Label: e.Title, OnPress: func(core.PressEvent) { picked.Set(i) },
				Style: core.Style{Height: core.Pt(48), Padding: core.Symmetric(0, 12), JustifyContent: core.JustifyCenter, Overflow: core.Hidden},
				ClassFunc: func() string {
					if on() {
						return "bg-background"
					}
					return ""
				},
				Children: []core.Node{core.Text{Content: e.Title, MaxLines: 1, Style: textStyle(textSecondary, 500), ClassFunc: func() string {
					if on() {
						return "text-primary"
					}
					return "text-on-surface-variant"
				}}},
			})
		}
		return core.Scroll{HideIndicator: true, Class: "bg-surface-container-low", Style: core.Style{Width: core.Pt(96), FlexGrow: 0, FlexShrink: 0},
			Children: []core.Node{column(core.Style{PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}}, "", items...)}}
	})
	list := core.Dynamic(func(cx *core.Scope) core.Node {
		rows := rail.GetUntracked()
		e := rows[picked.Get()]
		l := a.newPageList(cx, e.URL, e.Title, nil, 0, "")
		return pagerRows(cx, l.p, tradeRows, func(r tradeRow) string { return r.key },
			listSpec{pushed: true, empty: "暂无内容", bottom: 8},
			func(cx *core.Scope, get func() tradeRow, _ core.ListRow) core.Node {
				return dynamicShape(cx, func() bool { return get().entry != nil }, func(cx *core.Scope, card bool) core.Node {
					if card {
						return a.entryView(cx, func() backend.PageEntry {
							if e := get().entry; e != nil {
								return *e
							}
							return backend.PageEntry{}
						}, l)
					}
					return a.tileRow(cx, func() []tradeTile { return get().tiles })
				})
			})
	})
	return a.screen(titled(title), nil, row(fill, "", names, column(fill, "bg-surface-container-low", list)))
}

// tradeTile is a kind of goods or a product on the brand page: its picture
// and name, a product's hot count, and the second-hand posts it opens.
type tradeTile struct {
	title, pic, hot, url string
}

// tradeRow is a row of the brand page: a card across it, or tiles three to a
// row.
type tradeRow struct {
	key   string
	entry *backend.PageEntry
	tiles []tradeTile
}

// tradeRows lays out a list of the brand page: the kinds of goods and the
// products as tiles, three to a row, and the other cards (the brands, the
// order of the products) across the list.
func tradeRows(entries []backend.PageEntry) []tradeRow {
	var out []tradeRow
	var tiles []tradeTile
	flush := func() {
		for i := 0; i < len(tiles); i += 3 {
			line := tiles[i:min(i+3, len(tiles))]
			out = append(out, tradeRow{key: "tiles:" + line[0].url, tiles: line})
		}
		tiles = nil
	}
	for i := range entries {
		e := &entries[i]
		switch {
		case e.Kind == backend.EntryProduct:
			tiles = append(tiles, tradeTile{title: e.Title, pic: e.LogoURL, hot: e.Hot, url: backend.TradeListURL(e.ID, "")})
		case e.Card != nil && e.Card.Kind == backend.CardIcons:
			for _, it := range e.Card.Items {
				tiles = append(tiles, tradeTile{title: it.Title, pic: it.Pic, url: it.URL})
			}
		default:
			flush()
			out = append(out, tradeRow{key: entryKey(*e), entry: e})
		}
	}
	flush()
	return out
}

// tileRow is up to three tiles side by side, each on a card: the picture on
// white, as the pictures come with white backgrounds of their own.
func (a *App) tileRow(cx *core.Scope, get func() []tradeTile) core.Node {
	cells := make([]core.Node, 3)
	for i := range cells {
		tile := func() tradeTile {
			if ts := get(); i < len(ts) {
				return ts[i]
			}
			return tradeTile{}
		}
		cells[i] = dynamicShape(cx, func() bool { return tile().url != "" }, func(cx *core.Scope, shown bool) core.Node {
			if !shown {
				return core.View{Style: fill}
			}
			return tap("条目", func() { a.openList(tile().title, tile().url) },
				fill.Merge(core.Style{Padding: core.Edges{Top: 12, Right: 6, Bottom: 10, Left: 6}, Gap: 6, AlignItems: core.AlignCenter, CornerRadius: 12}), "bg-white",
				core.View{Style: core.Style{Width: core.Pt(64), Height: core.Pt(64)}, Children: []core.Node{
					a.pic(cx, func() string { return tile().pic }, core.Contain, "", fillAbs)}},
				core.Text{ContentFunc: func() string { return tile().title }, MaxLines: 2, Class: "text-black",
					Style: textStyle(textCaption, 500).Merge(core.Style{TextAlign: core.TextAlignCenter})},
				dynamicShape(cx, func() bool { return tile().hot != "" }, func(cx *core.Scope, hot bool) core.Node {
					if !hot {
						return core.Group{}
					}
					return core.Text{ContentFunc: func() string { return "热度 " + tile().hot }, MaxLines: 1, Class: "text-black/60",
						Style: textStyle(textCaption, 400).Merge(core.Style{TextAlign: core.TextAlignCenter})}
				}))
		})
	}
	return row(core.Style{Gap: 8, Padding: core.Edges{Top: 4, Right: 8, Bottom: 4, Left: 8}, AlignItems: core.AlignStretch}, "", cells...)
}

func (a *App) tradeCity() string {
	city := a.local.TradeCity
	if city == "" {
		// A profile's region is its province and city.
		if parts := strings.Fields(a.account.profile.GetUntracked().Region); len(parts) > 0 {
			city = parts[len(parts)-1]
		}
	}
	return city
}

// tradeCityPage lists the second-hand posts of one city: the one the
// account last asked for, else the city of its profile.
func (a *App) tradeCityPage(cx *core.Scope, title string) core.Node {
	city := a.tradeCity()
	shown := core.NewSignal(cx, city)
	typed := core.NewSignal(cx, city)
	var field core.Ref
	show := func(string) {
		field.Blur()
		name := strings.TrimSuffix(strings.TrimSpace(typed.GetUntracked()), "市")
		if name == "" || name == shown.GetUntracked() {
			return
		}
		a.local.TradeCity = name
		a.saveLocalState()
		shown.Set(name)
	}
	bar := row(core.Style{Padding: core.Edges{Top: 4, Right: 16, Bottom: 8, Left: 16}, Gap: 8, AlignItems: core.AlignCenter}, "",
		row(fill.Merge(core.Style{Height: core.Pt(40), Padding: core.Symmetric(0, 16), CornerRadius: 20, AlignItems: core.AlignCenter}), "bg-surface-container-high",
			core.Input{Ref: &field, Value: typed.Get, OnChange: typed.Set, OnSubmit: show, Placeholder: "城市，如 大连", Label: "城市",
				ReturnKeyType: core.ReturnKeySearch, Class: "text-on-surface",
				Style: func() core.Style {
					st := textStyle(textSecondary, 400)
					st.FlexGrow, st.MinWidth = 1, core.Pt(0)
					return st
				}()}),
		tonalButton("查看", func() { show("") }))
	list := core.Dynamic(func(cx *core.Scope) core.Node {
		name := shown.Get()
		if name == "" {
			return statusBlock("选择城市", "输入城市名查看当地的二手帖子。", "", nil)
		}
		return a.entryList(cx, a.newPageList(cx, backend.TradeListURL("", name), title, nil, 0, ""), listSpec{pushed: true})
	})
	return a.screen(titled(title), nil, bar, column(fill, "", list))
}

package app

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/nsgo-dev/nsgo/base"
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

// topicPage lists the posts of one topic or product.
func (a *App) topicPage(cx *core.Scope, param string) base.NavPage {
	raw, _ := base64.RawURLEncoding.DecodeString(param)
	productID, name, _ := strings.Cut(string(raw), "\x00")
	feeds := newPager(cx, func(ctx context.Context, page int) ([]backend.Feed, bool, error) {
		r, err := a.backend.Feed(ctx, backend.FeedQuery{Kind: backend.FeedTopic, Topic: name, ProductID: productID, Page: page})
		return r.Feeds, r.HasMore, err
	}, func(f backend.Feed) string { return f.ID })
	title := "#" + name + "#"
	if productID != "" {
		title = name
	}
	return base.NavPage{Title: title, HeaderStyleFunc: a.headerStyle(cx), Class: "bg-background", Children: []core.Node{
		pagerList(cx, feeds, listSpec{empty: "暂无内容", emptyHint: "这里还没有帖子。", bottom: 24, skeleton: skeletonCard},
			func(cx *core.Scope, get func() backend.Feed, r core.ListRow) core.Node { return a.feedCard(cx, get) }),
	}}
}

// listPage shows a server list that a row opened, such as the products of a
// category.
func (a *App) listPage(cx *core.Scope, param string) base.NavPage {
	raw, _ := base64.RawURLEncoding.DecodeString(param)
	title, url, _ := strings.Cut(string(raw), "\x00")
	entries := newPager(cx, func(ctx context.Context, page int) ([]backend.PageEntry, bool, error) {
		r, err := a.backend.PageEntries(ctx, backend.PageQuery{URL: url, Title: title, Page: page})
		return r.Entries, r.HasMore, err
	}, entryKey)
	return base.NavPage{Title: title, HeaderStyleFunc: a.headerStyle(cx), Class: "bg-background", Children: []core.Node{a.entryPane(cx, entries)}}
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
				return column(fill, "", pagerList(cx, t.pager(t.selected.Get()), listSpec{empty: "暂无话题"},
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

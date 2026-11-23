package app

import (
	"strconv"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// tabKind is where the posts of a tab come from.
type tabKind int

const (
	tabTopics tabKind = iota
	tabFollow
	tabHeadline
	tabPage
)

// pageTab is a tab of the home, digital or discovery page. The lists are
// those of the official iOS client's tab configuration
// (mainpage_tab_config_*.json in the app bundle).
type pageTab struct {
	Title string
	Kind  tabKind
	URL   string
}

func page(title, name string) pageTab {
	return pageTab{Title: title, Kind: tabPage, URL: "/page?url=" + name}
}

// The home tabs the iOS client fixes (话题, 关注, 头条) and offers, in its
// order.
var homeTabs = []pageTab{
	{Title: "话题", Kind: tabTopics},
	{Title: "关注", Kind: tabFollow},
	{Title: "头条", Kind: tabHeadline},
	page("热榜", "V9_HOME_TAB_RANKING"),
	page("快讯", "V11_HOME_TAB_NEWS"),
	page("新机", "V11_HOME_NEW"),
	page("开箱", "V13_IOSHOME_OPENSHOW"),
	page("摄影", "V13_HOME_SHEYING"),
	page("游戏", "V15_YOUXI"),
	page("教程", "V11_HOME_TAB_JC"),
	page("汽车", "V11_HOME_CAR"),
	page("外设", "V14_WAISHE"),
	page("视频", "V9_HOME_TAB_SHIPIN"),
	page("美化", "V11_HOME_MEIHUA"),
	page("问答", "V9_HOME_TAB_WENDA"),
}

var digitalTabs = []pageTab{
	{Title: "数码库", Kind: tabPage, URL: backend.ProductCategoriesURL},
	page("数码", "V10_DIGITAL_HOME"),
	page("手机", "V10_CHANNEL_SJB"),
	page("排行榜", "V10_CHANNEL_SMB_TOP"),
	page("系统", "V13_DIGITAL_ROM"),
	page("平板", "V15_PAD"),
	page("电脑", "V8_ZHUANTI_COMPUTER_20230413"),
}

// The discovery tabs, without the shop's 酷品 and 好物榜, which list goods.
var discoveryTabs = []pageTab{
	page("生活", "V15_ZHUANTI_SHENGHUO"),
	page("二手", "IOS_FIND_ERSHOU"),
	page("酷图", "V11_FIND_COOLPIC"),
	page("宠物", "V15_CHONGWU"),
	page("运动", "V15_YUNDONG"),
}

func entryKey(e backend.PageEntry) string { return strconv.Itoa(int(e.Kind)) + ":" + e.ID }

// tabGroup is a row of tabs over pages that swipe, each tab with its own
// list.
type tabGroup struct {
	a     *App
	tabs  []pageTab
	lists []*pageList
	// selected is the tab the strip marks; pos the pages' offset in pages,
	// which moves the strip's indicator as a finger drags them.
	selected *core.Signal[int]
	pos      *core.Signal[float32]
	width    *core.Signal[core.Points]
	pages    core.Ref
	tabRefs  []*core.Ref
	// jump shows page i, as the strip's taps and the back key do.
	jump func(i int, animated bool)
}

// scrollToTop shows the first row of the selected tab.
func (g *tabGroup) scrollToTop() {
	i := g.selected.GetUntracked()
	if g.tabs[i].Kind == tabTopics {
		return
	}
	if l := g.lists[i]; l != nil {
		l.p.list.ScrollTo(core.Point{}, true)
	}
}

// select shows tab i.
func (g *tabGroup) selectTab(i int) {
	if g.selected.GetUntracked() != i && g.jump != nil {
		g.selected.Set(i)
		g.jump(i, true)
	}
}

func (a *App) newTabGroup(cx *core.Scope, tabs []pageTab, selected int) *tabGroup {
	g := &tabGroup{
		a: a, tabs: tabs,
		lists:    make([]*pageList, len(tabs)),
		selected: core.NewSignal(cx, selected),
		pos:      core.NewSignal(cx, float32(selected)),
		width:    core.NewSignal(cx, core.Points(0)),
		tabRefs:  make([]*core.Ref, len(tabs)),
	}
	for i := range g.tabRefs {
		g.tabRefs[i] = new(core.Ref)
	}
	return g
}

// list returns the list of tab i, made when first shown.
func (g *tabGroup) list(cx *core.Scope, i int) *pageList {
	if l := g.lists[i]; l != nil {
		return l
	}
	a, tab := g.a, g.tabs[i]
	var l *pageList
	switch tab.Kind {
	case tabHeadline:
		l = a.newPageList(cx, backend.HeadlineURL, tab.Title, nil, 0, "")
	case tabFollow:
		// The follow tab leads with its groups, as c001apk's does.
		chosen := min(max(a.local.FollowGroup, 0), len(followGroups.Items)-1)
		head := []backend.PageEntry{{Kind: backend.EntryCard, ID: "card:follow-groups", Card: followGroups}}
		l = a.newPageList(cx, backend.FollowURL, tab.Title, head, chosen, followGroups.Items[chosen].URL)
		l.onChoose = func(i int) {
			a.local.FollowGroup = i
			a.saveLocalState()
		}
	default:
		l = a.newPageList(cx, tab.URL, tab.Title, nil, 0, "")
	}
	g.lists[i] = l
	return l
}

// followGroups are the groups the follow tab lists, without the apps'.
var followGroups = &backend.Card{Kind: backend.CardSelector, Items: []backend.CardItem{
	{Title: "全部关注", URL: backend.FollowURL},
	{Title: "好友关注", URL: "/page?url=V9_HOME_TAB_FOLLOW&type=circle"},
	{Title: "话题关注", URL: "/page?url=V9_HOME_TAB_FOLLOW&type=topic"},
	{Title: "数码关注", URL: "/page?url=V9_HOME_TAB_FOLLOW&type=product"},
}}

// reload fetches again the tabs of kind that were shown, as after signing in
// or out.
func (g *tabGroup) reload(kinds ...tabKind) {
	for i, tab := range g.tabs {
		for _, kind := range kinds {
			if tab.Kind == kind && g.lists[i] != nil {
				g.lists[i].p.reload()
			}
		}
	}
}

// view shows the tabs of g over its swiping pages.
func (g *tabGroup) view(cx *core.Scope, topics func(cx *core.Scope) core.Node) core.Node {
	pages := make([]core.Node, len(g.tabs))
	for i := range g.tabs {
		i := i
		built := false
		pages[i] = core.View{
			StyleFunc: func() core.Style {
				return core.Style{Width: core.Pt(g.width.Get()), Height: core.Pct(100), Overflow: core.Hidden}
			},
			Children: []core.Node{core.Show{
				When: func() bool {
					near := g.width.Get() > 0 && abs32(g.pos.Get()-float32(i)) < 1.6
					built = built || near
					return built
				},
				Then: func(cx *core.Scope) core.Node {
					if g.tabs[i].Kind == tabTopics {
						return topics(cx)
					}
					return g.a.entryPane(cx, g.list(cx, i), false)
				},
			}},
		}
	}
	jump := func(i int, animated bool) {
		g.pages.ScrollTo(core.Point{X: core.Points(i) * g.width.GetUntracked()}, animated)
	}
	g.jump = jump
	pager := core.Scroll{
		Horizontal: true, HideIndicator: true, Ref: &g.pages,
		Behavior: core.ScrollBehavior{Paging: true},
		Style:    fill.Merge(core.Style{Direction: core.Row}),
		// A page a back gesture previews is laid out at 0 for a moment; its
		// pages keep the width they had, or they would empty and the offset
		// would shrink to the first page. A width that really changes, as
		// turning the phone does, leaves the offset where the old width put it:
		// it is set again to the selected tab.
		OnLayout: func(b core.Bounds) {
			if w := b.Size.Width; w > 0 && w != g.width.GetUntracked() {
				g.width.Set(w)
				jump(g.selected.GetUntracked(), false)
			}
		},
		OnScroll: func(e core.ScrollEvent) {
			if e.Stale || e.Size.Width == 0 {
				return
			}
			pos := e.Offset.X / e.Size.Width
			g.pos.Set(float32(pos))
			if i := int(pos + 0.5); i != g.selected.GetUntracked() {
				g.selected.Set(i)
				g.tabRefs[i].Reveal(core.RevealOptions{})
			}
		},
		Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, Height: core.Pct(100)}, Children: pages}},
	}
	return column(fill, "", g.strip(cx, jump), pager)
}

// strip is MD3 primary tabs: each as wide as its label, the selected one in
// primary with a 3dp indicator as wide as its label, over an
// outline-variant rule. The strip scrolls when the tabs do not fit.
func (g *tabGroup) strip(cx *core.Scope, jump func(i int, animated bool)) core.Node {
	tabs := make([]core.Node, len(g.tabs))
	for i, tab := range g.tabs {
		i := i
		near := func() float32 { return min(1, max(0, 1-abs32(g.pos.Get()-float32(i)))) }
		tabs[i] = core.View{
			Ref: g.tabRefs[i], Role: core.RoleTab, Label: tab.Title,
			Style: core.Style{Height: core.Pct(100), Padding: core.Symmetric(0, 16), JustifyContent: core.JustifyEnd, AlignItems: core.AlignCenter, Overflow: core.Hidden},
			OnPress: func(core.PressEvent) {
				if g.selected.GetUntracked() == i {
					g.list(cx, i).p.list.ScrollTo(core.Point{}, true)
					return
				}
				g.selected.Set(i)
				jump(i, true)
			},
			Android: core.AndroidViewProps{Ripple: &core.Ripple{}},
			Children: []core.Node{
				column(core.Style{Gap: 12, AlignItems: core.AlignCenter}, "",
					core.Text{Content: tab.Title, MaxLines: 1, Style: textStyle(textSecondary, 500), Class: "text-on-surface-variant",
						ClassFunc: func() string {
							if near() >= 0.5 {
								return "text-primary"
							}
							return "text-on-surface-variant"
						}},
					core.View{Class: "bg-primary", Style: core.Style{Height: core.Pt(3), CornerRadii: core.Corners{TopLeft: 3, TopRight: 3}},
						StyleFunc: func() core.Style {
							return core.Style{Width: core.Pct(100 * near()), Opacity: core.Alpha(near())}
						}},
				),
			},
		}
	}
	return column(core.Style{FlexShrink: 0}, "bg-background",
		core.Scroll{Horizontal: true, HideIndicator: true, Style: core.Style{Height: core.Pt(48), FlexGrow: 0},
			Children: []core.Node{row(core.Style{Height: core.Pct(100), Padding: core.Symmetric(0, 8)}, "", tabs...)}},
		core.View{Style: core.Style{Height: core.Pt(1)}, Class: "bg-surface-container-highest"},
	)
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// entryPane lists the rows of a server page.
func (a *App) entryPane(cx *core.Scope, l *pageList, pushed bool) core.Node {
	return pagerList(cx, l.p, listSpec{pushed: pushed, empty: "暂无内容", emptyHint: "这里还没有可显示的内容。", bottom: 24, skeleton: skeletonCard},
		func(cx *core.Scope, get func() backend.PageEntry, r core.ListRow) core.Node {
			kind := core.NewMemo(cx, func() backend.EntryKind { return get().Kind })
			return core.Dynamic(func(cx *core.Scope) core.Node {
				switch kind.Get() {
				case backend.EntryCard:
					return a.cardView(cx, get, l)
				case backend.EntryFeed:
					return a.feedCard(cx, func() backend.Feed {
						if f := get().Feed; f != nil {
							return *f
						}
						return backend.Feed{}
					})
				}
				return a.entryRow(cx, get)
			})
		})
}

// entryRow shows a product or a category as a list row.
func (a *App) entryRow(cx *core.Scope, get func() backend.PageEntry) core.Node {
	return tap("条目", func() {
		e := get()
		if e.Kind == backend.EntryProduct {
			a.openTarget(&backend.Target{Title: e.Title, ID: e.ID, Product: true})
		} else {
			a.openList(e.Title, e.URL)
		}
	}, core.Style{Direction: core.Row, Padding: core.Symmetric(12, 16), Gap: 16, AlignItems: core.AlignCenter}, "bg-background",
		a.pic(cx, func() string { return get().LogoURL }, core.Cover, "bg-surface-container-highest",
			core.Style{Width: core.Pt(48), Height: core.Pt(48), CornerRadius: 8, FlexShrink: 0}),
		column(fill.Merge(core.Style{Gap: 2}), "",
			core.Text{ContentFunc: func() string { return get().Title }, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textBody, 500)},
			core.Text{ContentFunc: func() string { return get().Subtitle }, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textSecondary, 400)},
		),
		icon(&symbols.ChevronRight, 24, "text-on-surface-variant"),
	)
}

// skeletonCard is a comment-shaped placeholder that keeps the list roughly
// as tall while its first page loads.
func skeletonCard() core.Node {
	bar := func(width float32) core.Node {
		return core.View{Class: "bg-surface-container-high", Style: core.Style{Width: core.Pct(width), Height: core.Pt(12), CornerRadius: 6}}
	}
	return row(core.Style{Padding: core.Edges{Top: 12, Right: 16, Bottom: 8, Left: 16}, Gap: 12}, "bg-background",
		core.View{Class: "bg-surface-container-high", Style: core.Style{Width: core.Pt(36), Height: core.Pt(36), CornerRadius: 18, FlexShrink: 0}},
		column(fill.Merge(core.Style{Gap: 8}), "", bar(40), bar(100), bar(70)),
	)
}

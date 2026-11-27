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
	tabWeb
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
	webTab func(*core.Scope, int) core.Node
	a      *App
	tabs   []pageTab
	lists  []*pageList
	// A search supplies its own lists and starts them when selected.
	newList  func(int) *pageList
	listView func(*core.Scope, int, listSpec) core.Node
	// pushed says the group is on a page pushed over the shell.
	pushed bool
	// head, when set, is above the tabs and scrolls away with the shown list.
	head core.Node
	// selected is the tab the strip marks; pos the pages' offset in pages,
	// which moves the strip's indicator as a finger drags them.
	selected *core.Signal[int]
	pos      *core.Signal[float32]
	width    *core.Signal[core.Points]
	slide    *pageSlide
	pages    core.Ref
	tabRefs  []*core.Ref
	// jump shows page i, as the strip's taps and the back key do.
	jump func(i int, animated bool)
	// first is the tab the group opens on.
	first int
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

// refresh loads the selected tab again, as a pull on its list does.
func (g *tabGroup) refresh() {
	i := g.selected.GetUntracked()
	if g.tabs[i].Kind == tabTopics {
		return
	}
	if l := g.lists[i]; l != nil {
		l.p.refreshing.Set(true)
		l.p.reload()
	}
}

// home shows the tab the group opens on, at its first row.
func (g *tabGroup) home() {
	g.selectTab(g.first)
	g.scrollToTop()
}

// select shows tab i.
func (g *tabGroup) selectTab(i int) {
	if g.selected.GetUntracked() != i && g.jump != nil {
		g.selected.Set(i)
		g.tabRefs[i].Reveal(core.RevealOptions{})
		g.jump(i, true)
	}
}

func (a *App) newTabGroup(cx *core.Scope, tabs []pageTab, selected int) *tabGroup {
	g := &tabGroup{
		a: a, tabs: tabs,
		lists:    make([]*pageList, len(tabs)),
		selected: core.NewSignal(cx, selected),
		first:    selected,
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
	if g.newList != nil {
		l := g.newList(i)
		g.lists[i] = l
		return l
	}
	a, tab := g.a, g.tabs[i]
	var l *pageList
	switch tab.Kind {
	case tabHeadline:
		l = a.newPageList(cx, backend.HeadlineURL, tab.Title, nil, 0, "")
	case tabFollow:
		// The follow tab leads with the server's own cards: the followed
		// accounts with news, over the groups that choose its posts.
		l = a.newPageList(cx, backend.FollowURL, tab.Title, nil, a.local.FollowGroup, "")
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
	var over *headed
	if g.head != nil {
		over = newHeaded(cx, g.selected, len(g.tabs), func(i int) *core.Ref {
			if l := g.lists[i]; l != nil {
				return &l.p.list
			}
			return nil
		})
	}
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
					if g.tabs[i].Kind == tabWeb {
						return g.webTab(cx, i)
					}
					l, spec := g.list(cx, i), listSpec{pushed: g.pushed}
					if g.newList != nil {
						spec.manualStart = true
						cx.Effect(func() {
							if g.selected.Get() == i {
								l.p.ensure()
							}
						})
					}
					if over != nil {
						spec = over.under(cx, i, spec, func() { l.p.items.Get(); l.p.loaded.Get() })
					}
					if g.listView != nil {
						return g.listView(cx, i, spec)
					}
					return g.a.entryList(cx, l, spec)
				},
			}},
		}
	}
	// The offset chooses the tab as the user drags the pages, except that
	// iOS sets the offset of a page another covers back to 0: an offset more
	// than half a page from the last one in a single report came from
	// elsewhere, and the pages go back to the selected tab. A tap's animated
	// jump (glide) moves the indicator without choosing on the way.
	last, glide := core.Points(-1), -1
	slide := newPageSlide(cx, g.width, len(g.tabs))
	g.slide = slide
	jump := func(i int, animated bool) {
		w := g.width.GetUntracked()
		if slide.byPan {
			g.pos.Set(float32(i))
			slide.show(&g.pages, i, animated)
			return
		}
		if animated {
			glide = i
		} else {
			glide, last = -1, core.Points(i)*w
		}
		g.pages.ScrollTo(core.Point{X: core.Points(i) * w}, animated)
	}
	g.jump = jump
	placed := false
	row := slide.row(pages, func(i int) {
		g.pos.Set(float32(i))
		if i != g.selected.GetUntracked() {
			g.selected.Set(i)
			g.tabRefs[i].Reveal(core.RevealOptions{})
		}
	})
	pager := core.Scroll{
		TestID:     "pages",
		Horizontal: true, HideIndicator: true, Ref: &g.pages,
		Behavior:     core.ScrollBehavior{Paging: true, NoScroll: slide.byPan},
		OffsetValues: slide.scroll(),
		Style:        fill.Merge(core.Style{Direction: core.Row}),
		// A page a back gesture previews is laid out at 0 for a moment; its
		// pages keep the width they had, or they would empty and the offset
		// would shrink to the first page. A new pager, and a width that
		// really changes, as turning the phone does, are set to the selected
		// tab.
		OnLayout: func(b core.Bounds) {
			if b.Size.Width == 0 {
				placed = false // iOS moves the offset to 0 meanwhile
				return
			}
			if w := b.Size.Width; w != g.width.GetUntracked() || !placed {
				placed = true
				g.width.Set(w)
				jump(g.selected.GetUntracked(), false)
				g.tabRefs[g.selected.GetUntracked()].Reveal(core.RevealOptions{})
			}
		},
		OnScrollDragStart: func(core.ScrollEvent) { glide = -1 }, // a drag takes over a glide
		OnScroll: func(e core.ScrollEvent) {
			if e.Size.Width == 0 {
				placed = false
				return
			}
			if e.Stale {
				return
			}
			off, w := e.Offset.X, e.Size.Width
			pos := off / w
			if glide >= 0 {
				g.pos.Set(float32(pos))
				if d := pos - core.Points(glide); d > -0.01 && d < 0.01 {
					glide, last = -1, off
				}
				return
			}
			if last >= 0 && (off-last > w/2 || last-off > w/2) {
				jump(g.selected.GetUntracked(), false)
				return
			}
			last = off
			g.pos.Set(float32(pos))
			if i := int(pos + 0.5); i != g.selected.GetUntracked() {
				g.selected.Set(i)
				g.tabRefs[i].Reveal(core.RevealOptions{})
			}
		},
		Children: []core.Node{row},
	}
	if over != nil {
		return over.view(g.head, g.strip(cx), pager)
	}
	return column(fill, "", g.strip(cx), pager)
}

// strip is MD3 primary tabs: each as wide as its label, the selected one in
// primary, over an outline-variant rule. The 3dp indicator slides between
// the tabs as the pages do. The strip scrolls when the tabs do not fit.
func (g *tabGroup) strip(cx *core.Scope) core.Node {
	// centers are the middles of the tabs in the strip, known once laid out.
	centers := make([]core.Points, len(g.tabs))
	laid := core.NewSignal(cx, 0)
	tabs := make([]core.Node, len(g.tabs), len(g.tabs)+1)
	for i, tab := range g.tabs {
		i := i
		tabs[i] = pressable(core.View{
			Ref: g.tabRefs[i], Role: core.RoleTab, Label: tab.Title,
			Style: core.Style{Height: core.Pct(100), Padding: core.Symmetric(0, 16), JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter, Overflow: core.Hidden},
			OnLayout: func(b core.Bounds) {
				if c := b.Origin.X + b.Size.Width/2; c != centers[i] {
					centers[i] = c
					laid.Set(laid.GetUntracked() + 1)
				}
			},
			OnPress: func(core.PressEvent) {
				if g.selected.GetUntracked() == i {
					g.scrollToTop()
					return
				}
				g.selectTab(i)
			},
			Children: []core.Node{
				core.Text{Content: tab.Title, MaxLines: 1, Style: textStyle(textSecondary, 500), Class: "text-on-surface-variant",
					ClassFunc: func() string {
						if abs32(g.pos.Get()-float32(i)) < 0.5 {
							return "text-primary"
						}
						return "text-on-surface-variant"
					}},
			},
		})
	}
	const bar = 32
	// The indicator goes from tab to tab as the pages' offset goes from page
	// to page, on the platform's main thread.
	move := core.NewMemoFunc(cx, func() core.AnimatedNode {
		laid.Get()
		w := float64(g.width.Get())
		if len(centers) < 2 || w <= 0 {
			return nil
		}
		in, out := make([]float64, len(centers)), make([]float64, len(centers))
		for i, c := range centers {
			in[i], out[i] = float64(i)*w, float64(c-bar/2)
		}
		return core.Interpolate(g.slide.offset, core.Interpolation{Input: in, Output: out, Extrapolate: core.ExtrapolateClamp})
	}, func(a, b core.AnimatedNode) bool { return false })
	tabs = append(tabs, core.View{
		PointerEvents: core.PointerEventsNone, Class: "bg-primary",
		Style: core.Style{Position: core.Absolute, Bottom: core.Pt(0), Left: core.Pt(0), Width: core.Pt(bar), Height: core.Pt(3), CornerRadii: core.Corners{TopLeft: 3, TopRight: 3}},
		StyleFunc: func() core.Style {
			if m := move.Get(); m != nil {
				return core.Style{Animated: &core.AnimatedStyle{TranslateX: m}}
			}
			return core.Style{Left: core.Pt(centers[0] - bar/2)}
		},
	})
	return column(core.Style{FlexShrink: 0}, "bg-background",
		sideScroll{Style: core.Style{Height: core.Pt(48), FlexGrow: 0},
			Children: []core.Node{row(core.Style{Height: core.Pct(100), Padding: core.Symmetric(0, 8)}, "", tabs...)}},
		core.View{Style: core.Style{Height: core.Pt(1)}, Class: "bg-outline-variant"},
	)
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// entryList lists the rows of a server page.
// spec tells where the list is and what is around it.
func (a *App) entryList(cx *core.Scope, l *pageList, spec listSpec) core.Node {
	spec.empty, spec.emptyHint, spec.bottom, spec.skeleton = "暂无内容", "这里还没有可显示的内容。", 24, skeletonCard
	return pagerList(cx, l.p, spec,
		func(cx *core.Scope, get func() backend.PageEntry, r core.ListRow) core.Node {
			return a.entryView(cx, get, l)
		})
}

// entryView shows a row of a server page, rebuilt as it moves to a row of
// another kind.
func (a *App) entryView(cx *core.Scope, get func() backend.PageEntry, l *pageList) core.Node {
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
}

// entryRow shows a product, a category or a link as a list row.
func (a *App) entryRow(cx *core.Scope, get func() backend.PageEntry) core.Node {
	return tap("条目", func() {
		e := get()
		switch e.Kind {
		case backend.EntryProduct:
			a.openTarget(&backend.Target{Title: e.Title, ID: e.ID, Product: true})
		case backend.EntryLink:
			a.openCardItem(backend.CardItem{Title: e.Title, URL: e.URL})
		default:
			a.openList(e.Title, e.URL)
		}
	}, core.Style{Direction: core.Row, Padding: core.Symmetric(12, 16), Gap: 16, AlignItems: core.AlignCenter}, "bg-background",
		core.View{Class: "bg-surface-container-highest",
			Style: core.Style{Width: core.Pt(48), Height: core.Pt(48), Overflow: core.Hidden, FlexShrink: 0},
			StyleFunc: func() core.Style {
				if get().Round {
					return core.Style{CornerRadius: 24}
				}
				return core.Style{CornerRadius: 8}
			},
			Children: []core.Node{a.pic(cx, func() string { return get().LogoURL }, core.Cover, "", fillAbs)}},
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

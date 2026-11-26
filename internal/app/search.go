package app

import (
	"context"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

const suggestDelay = 250 * time.Millisecond

var searchTabs = [...]pageTab{
	backend.SearchAll:       {Title: "综合", Kind: tabPage},
	backend.SearchProducts:  {Title: "数码", Kind: tabPage, URL: "tab://product"},
	backend.SearchFeeds:     {Title: "动态", Kind: tabPage, URL: "tab://feed"},
	backend.SearchTopics:    {Title: "话题", Kind: tabPage, URL: "tab://feedTopic"},
	backend.SearchUsers:     {Title: "用户", Kind: tabPage, URL: "tab://user"},
	backend.SearchTrades:    {Title: "二手", Kind: tabPage, URL: "tab://ershou"},
	backend.SearchQuestions: {Title: "问答", Kind: tabPage},
	backend.SearchChannels:  {Title: "看看号", Kind: tabPage, URL: "tab://dyh"},
}

type searchSubmission struct {
	keyword string
	kind    backend.SearchKind
}

type searchPageState struct {
	a         *App
	cx        *core.Scope
	field     core.Ref
	input     *core.Signal[string]
	submitted *core.Signal[searchSubmission]

	hot         *core.Signal[[]string]
	hotLoading  *core.Signal[bool]
	hotErr      *core.Signal[string]
	suggestions *core.Signal[[]string]
	suggestFor  string
	suggestGen  int
	stopTimer   func() bool
}

// searchPage searches; a keyword, as a hot search names, is searched at once.
func (a *App) searchPage(cx *core.Scope, keyword string) core.Node {
	s := &searchPageState{
		a: a, cx: cx,
		input:       core.NewSignal(cx, ""),
		submitted:   core.NewSignalFunc(cx, searchSubmission{}, nil),
		hot:         core.NewSignalFunc(cx, []string(nil), nil),
		hotLoading:  core.NewSignal(cx, false),
		hotErr:      core.NewSignal(cx, ""),
		suggestions: core.NewSignalFunc(cx, []string(nil), nil),
	}
	if keyword != "" {
		s.submit(keyword)
	}
	s.loadHot()

	input := core.Input{
		Ref:   &s.field,
		Value: s.input.Get, Placeholder: "搜索酷安", Label: "搜索", AutoFocus: keyword == "",
		ReturnKeyType: core.ReturnKeySearch,
		OnChange:      s.typed, OnSubmit: s.submit,
		Class: "text-on-surface",
		Style: func() core.Style { st := textStyle(textBody, 400); st.FlexGrow, st.MinWidth = 1, core.Pt(0); return st }(),
	}
	bar := a.topBar(
		iconAction(&symbols.Back, "返回", func() {
			s.cx.DismissKeyboard()
			a.nav.Pop()
		}),
		row(fill.Merge(core.Style{Height: core.Pt(48), Padding: core.Edges{Right: 4, Left: 20}, Margin: core.Edges{Right: 12}, CornerRadius: 24, AlignItems: core.AlignCenter}), "bg-surface-container-high",
			input,
			core.Dynamic(func(cx *core.Scope) core.Node {
				if s.input.Get() == "" {
					return core.Group{}
				}
				return iconAction(&symbols.Close, "清除", s.clear)
			})),
	)
	showSuggestions := core.NewMemo(cx, func() bool {
		query := strings.TrimSpace(s.input.Get())
		return query != "" && query != s.submitted.Get().keyword && (isNumericFeedID(query) || len(s.suggestions.Get()) > 0)
	})
	body := core.Dynamic(func(cx *core.Scope) core.Node {
		search := s.submitted.Get()
		if search.keyword != "" {
			return s.searchResults(cx, search, showSuggestions.Get)
		}
		return core.Show{When: showSuggestions.Get, Then: s.suggestionList, Else: s.discovery}
	})
	return column(fill, "bg-background", bar, body)
}

func (s *searchPageState) searchResults(cx *core.Scope, search searchSubmission, suggesting func() bool) core.Node {
	a := s.a
	selected := int(search.kind)
	var tabs [len(searchTabs)]searchResultTab
	for i := range tabs {
		query := core.NewSignal(cx, backend.SearchQuery{Keyword: search.keyword, Kind: backend.SearchKind(i)})
		l := &pageList{
			p: newQueryPager(cx, query.GetUntracked, func(ctx context.Context, q backend.SearchQuery, page int) ([]backend.PageEntry, bool, error) {
				q.Page = page
				r, err := a.backend.Search(ctx, q)
				return r.Entries, r.HasMore, err
			}, entryKey),
		}
		tabs[i] = searchResultTab{query: query, list: l}
		a.trackPageList(cx, l)
	}
	return core.Show{When: suggesting, Then: s.suggestionList, Else: func(cx *core.Scope) core.Node {
		g := a.newTabGroup(cx, searchTabs[:], selected)
		g.pushed = true
		cx.Effect(func() { selected = g.selected.Get() })
		g.newList = func(i int) *pageList {
			l := tabs[i].list
			l.openCardTitle = func(item backend.CardItem) {
				for i, tab := range searchTabs {
					if tab.URL != "" && tab.URL == item.URL {
						g.selectTab(i)
						return
					}
				}
				a.openCardItem(item)
			}
			return l
		}
		g.listView = func(cx *core.Scope, i int, spec listSpec) core.Node {
			return a.searchResultView(cx, tabs[i], spec)
		}
		return g.view(cx, nil)
	}}
}

func (s *searchPageState) loadHot() {
	s.hotLoading.Set(true)
	s.hotErr.Set("")
	background(s.a, func(ctx context.Context) (backend.SearchPage, error) {
		return s.a.backend.Search(ctx, backend.SearchQuery{})
	},
		func(r backend.SearchPage, err error) {
			s.hotLoading.Set(false)
			if err != nil {
				s.hotErr.Set(err.Error())
				return
			}
			s.hot.Set(r.HotSearches)
		})
}

// typed asks for suggestions once typing pauses.
func (s *searchPageState) typed(text string) {
	s.input.Set(text)
	query := strings.TrimSpace(text)
	if query == s.suggestFor {
		return
	}
	s.clearSuggestions()
	if query == "" || query == s.submitted.GetUntracked().keyword {
		return
	}
	s.suggestFor = query
	gen := s.suggestGen
	s.stopTimer = s.cx.AfterFunc(suggestDelay, func() {
		background(s.a, func(ctx context.Context) ([]string, error) { return s.a.backend.SearchSuggestions(ctx, query) },
			func(items []string, err error) {
				if gen != s.suggestGen {
					return
				}
				if err != nil {
					s.suggestFor = ""
					return
				}
				s.suggestions.Set(items)
			})
	})
}

// clear empties the field and shows the hot searches again.
func (s *searchPageState) clear() {
	s.input.Set("")
	s.submitted.Set(searchSubmission{})
	s.clearSuggestions()
}

func (s *searchPageState) clearSuggestions() {
	s.suggestions.Set(nil)
	s.suggestFor = ""
	s.suggestGen++ // drops suggestions still on the way
	if s.stopTimer != nil {
		s.stopTimer()
		s.stopTimer = nil
	}
}

func (s *searchPageState) submit(raw string) {
	s.search(raw, backend.SearchAll)
}

func (s *searchPageState) search(raw string, kind backend.SearchKind) {
	q := strings.TrimSpace(raw)
	if q == "" {
		return
	}
	s.cx.DismissKeyboard()
	s.clearSuggestions()
	// A pasted link of the site opens the page it names.
	if strings.HasPrefix(q, "http://") || strings.HasPrefix(q, "https://") {
		if t := backend.ResolveLink(q); t.Kind != backend.LinkSearch && s.a.openInApp(t) {
			return
		}
	}
	s.input.Set(q)
	s.submitted.Set(searchSubmission{keyword: q, kind: kind})
	s.a.keepSearches(prependUnique(s.a.searchHistory.GetUntracked(), q, 10))
}

func isNumericFeedID(query string) bool {
	return query != "" && strings.Trim(query, "0123456789") == ""
}

func (s *searchPageState) suggestionList(cx *core.Scope) core.Node {
	return core.Scroll{Style: fill, Behavior: core.ScrollBehavior{KeyboardShouldPersistTaps: core.KeyboardTapsHandled}, Children: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
		var rows []core.Node
		add := func(title string, open func()) {
			rows = append(rows, tap(title, open, core.Style{Direction: core.Row, Height: core.Pt(56), Padding: core.Symmetric(0, 16), Gap: 16, AlignItems: core.AlignCenter}, "",
				icon(&symbols.Search, 24, "text-on-surface-variant"),
				core.Text{Content: title, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textBody, 400)}))
		}
		if id := strings.TrimSpace(s.input.Get()); isNumericFeedID(id) {
			add("动态ID："+id, func() {
				s.cx.DismissKeyboard()
				s.a.openFeed(backend.Feed{ID: id})
			})
		}
		for _, item := range s.suggestions.Get() {
			keyword, user := strings.CutPrefix(item, "搜索用户：")
			kind := backend.SearchAll
			if user {
				kind = backend.SearchUsers
			}
			add(item, func() { s.search(keyword, kind) })
		}
		return column(core.Style{}, "", rows...)
	})}}
}

func (s *searchPageState) discovery(cx *core.Scope) core.Node {
	a := s.a
	return core.Scroll{Style: fill, Behavior: core.ScrollBehavior{KeyboardShouldPersistTaps: core.KeyboardTapsHandled}, Children: []core.Node{
		column(core.Style{Padding: core.Edges{Top: 8, Right: 16, Bottom: 24, Left: 16}, Gap: 24}, "",
			core.Dynamic(func(cx *core.Scope) core.Node {
				h := a.searchHistory.Get()
				if len(h) == 0 {
					return core.Group{}
				}
				return chipSection("最近搜索", h, s.submit, func() { a.keepSearches(nil) })
			}),
			core.Dynamic(func(cx *core.Scope) core.Node {
				switch {
				case len(s.hot.Get()) > 0:
					return chipSection("热门搜索", s.hot.Get(), s.submit, nil)
				case s.hotLoading.Get():
					return spinner()
				case s.hotErr.Get() != "":
					return row(core.Style{Gap: 8, AlignItems: core.AlignCenter}, "",
						core.Text{Content: "热门搜索加载失败", Class: "text-on-surface-variant", Style: fill.Merge(textStyle(textSecondary, 400))},
						textButton("重试", s.loadHot))
				}
				return core.Group{}
			}),
		),
	}}
}

func chipSection(title string, values []string, pick func(string), clear func()) core.Node {
	head := []core.Node{core.Text{Content: title, Class: "text-on-surface-variant", Style: fill.Merge(textStyle(textSecondary, 500))}}
	if clear != nil {
		head = append(head, tap("清空"+title, clear, core.Style{Width: core.Pt(40), Height: core.Pt(40), CornerRadius: 20, AlignItems: core.AlignCenter,
			JustifyContent: core.JustifyCenter, Overflow: core.Hidden}, "", icon(&symbols.Delete, 22, "text-on-surface-variant")))
	}
	var chips []core.Node
	for _, v := range values {
		v := v
		chips = append(chips, tap(v, func() { pick(v) }, core.Style{Height: core.Pt(36), Padding: core.Symmetric(0, 16), CornerRadius: 18, AlignItems: core.AlignCenter,
			JustifyContent: core.JustifyCenter, Overflow: core.Hidden}, "bg-surface-container-high",
			core.Text{Content: v, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textSecondary, 400)}))
	}
	return column(core.Style{Gap: 8}, "",
		row(core.Style{Height: core.Pt(40), AlignItems: core.AlignCenter}, "", head...),
		core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 8}, Children: chips})
}

// prependUnique moves s to the front of list, keeping at most n entries.
func prependUnique(list []string, s string, n int) []string {
	out := []string{s}
	for _, item := range list {
		if item != s && len(out) < n {
			out = append(out, item)
		}
	}
	return out
}

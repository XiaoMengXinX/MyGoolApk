package app

import (
	"context"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

const suggestDelay = 250 * time.Millisecond

// searchPageState is one search page: the query, hot searches, suggestions
// and two result tabs.
type searchPageState struct {
	a       *App
	cx      *core.Scope
	input   *core.Signal[string]
	keyword *core.Signal[string]
	tab     *core.Signal[int]
	feeds   *pager[backend.Feed]
	users   *pager[backend.Author]

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
		keyword:     core.NewSignal(cx, ""),
		tab:         core.NewSignal(cx, 0),
		hot:         core.NewSignalFunc(cx, []string(nil), nil),
		hotLoading:  core.NewSignal(cx, false),
		hotErr:      core.NewSignal(cx, ""),
		suggestions: core.NewSignalFunc(cx, []string(nil), nil),
	}
	s.feeds = newPager(cx, func(ctx context.Context, page int) ([]backend.Feed, bool, error) {
		r, err := a.backend.Search(ctx, backend.SearchQuery{Keyword: s.keyword.GetUntracked(), Kind: backend.SearchFeeds, Page: page})
		return r.Feeds, r.HasMore, err
	}, func(f backend.Feed) string { return f.ID })
	s.users = newPager(cx, func(ctx context.Context, page int) ([]backend.Author, bool, error) {
		r, err := a.backend.Search(ctx, backend.SearchQuery{Keyword: s.keyword.GetUntracked(), Kind: backend.SearchUsers, Page: page})
		return r.Users, r.HasMore, err
	}, func(u backend.Author) string { return u.ID })
	s.loadHot()
	if keyword != "" {
		s.submit(keyword)
	}

	var field core.Ref
	input := core.Input{
		Ref:   &field,
		Value: s.input.Get, Placeholder: "搜索动态和用户", Label: "搜索", AutoFocus: keyword == "",
		ReturnKeyType: core.ReturnKeySearch,
		OnChange:      s.typed, OnSubmit: func(q string) { s.submit(q) },
		Class: "text-on-surface",
		Style: func() core.Style { st := textStyle(textBody, 400); st.FlexGrow, st.MinWidth = 1, core.Pt(0); return st }(),
	}
	bar := topBar(
		iconAction(&symbols.Back, "返回", func() {
			field.Blur()
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
	body := core.Dynamic(func(cx *core.Scope) core.Node {
		query := strings.TrimSpace(s.input.Get())
		switch {
		case query != "" && query != s.keyword.Get() && len(s.suggestions.Get()) > 0:
			return s.suggestionList(cx)
		case s.keyword.Get() == "":
			return s.discovery(cx)
		}
		return column(fill, "", simpleTabs(s.tab, "动态", "用户"), core.Dynamic(func(cx *core.Scope) core.Node {
			if s.tab.Get() == 0 {
				return pagerList(cx, s.feeds, listSpec{pushed: true, empty: "没有找到相关动态", emptyHint: "换个关键词试试。", bottom: 24, skeleton: skeletonCard},
					func(cx *core.Scope, get func() backend.Feed, r core.ListRow) core.Node { return a.feedCard(cx, get) })
			}
			return pagerList(cx, s.users, listSpec{pushed: true, empty: "没有找到相关用户", emptyHint: "换个关键词试试。"},
				func(cx *core.Scope, get func() backend.Author, r core.ListRow) core.Node { return a.personRow(cx, get) })
		}))
	})
	return column(fill, "bg-background", bar, body)
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
	if s.stopTimer != nil {
		s.stopTimer()
	}
	query := strings.TrimSpace(text)
	if query == "" {
		s.suggestions.Set(nil)
		s.suggestFor = ""
		return
	}
	s.stopTimer = s.cx.AfterFunc(suggestDelay, func() {
		if query == s.keyword.GetUntracked() || query == s.suggestFor {
			return
		}
		s.suggestGen++
		gen := s.suggestGen
		background(s.a, func(ctx context.Context) ([]string, error) { return s.a.backend.SearchSuggestions(ctx, query) },
			func(items []string, err error) {
				if gen != s.suggestGen || err != nil {
					return
				}
				s.suggestions.Set(items)
				s.suggestFor = query
			})
	})
}

// clear empties the field and shows the hot searches again.
func (s *searchPageState) clear() {
	s.input.Set("")
	s.keyword.Set("")
	s.suggestions.Set(nil)
	s.suggestFor = ""
	s.suggestGen++ // drops suggestions still on the way
	if s.stopTimer != nil {
		s.stopTimer()
	}
}

func (s *searchPageState) submit(raw string) {
	q := strings.TrimSpace(raw)
	if q == "" {
		return
	}
	s.input.Set(q)
	s.keyword.Set(q)
	s.tab.Set(0)
	s.suggestions.Set(nil)
	s.suggestFor = ""
	s.suggestGen++
	s.a.searchHistory.Update(func(h []string) []string { return prependUnique(h, q, 10) })
	s.feeds.reset()
	s.users.reset()
}

func (s *searchPageState) suggestionList(cx *core.Scope) core.Node {
	var rows []core.Node
	for _, item := range s.suggestions.Get() {
		item := item
		rows = append(rows, tap(item, func() { s.submit(item) }, core.Style{Direction: core.Row, Height: core.Pt(56), Padding: core.Symmetric(0, 16), Gap: 16, AlignItems: core.AlignCenter}, "",
			icon(&symbols.Search, 24, "text-on-surface-variant"),
			core.Text{Content: item, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textBody, 400)}))
	}
	return core.Scroll{Style: fill, Behavior: core.ScrollBehavior{KeyboardShouldPersistTaps: core.KeyboardTapsHandled}, Children: []core.Node{column(core.Style{}, "", rows...)}}
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
				return chipSection("最近搜索", h, s.submit, func() { a.searchHistory.Set(nil) })
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

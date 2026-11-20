package app

import (
	"context"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// userPageState is one user page: the profile and three independently paged
// tabs.
type userPageState struct {
	a          *App
	uid        string
	profile    *core.Signal[backend.UserProfile]
	loaded     *core.Signal[bool]
	err        *core.Signal[string]
	loading    bool
	gen        int
	followBusy bool

	tab       *core.Signal[int]
	feeds     *pager[backend.Feed]
	following *pager[backend.Author]
	fans      *pager[backend.Author]
}

func (a *App) userPage(cx *core.Scope, uid string) base.NavPage {
	u := &userPageState{
		a: a, uid: uid,
		profile: core.NewSignalFunc(cx, backend.UserProfile{Author: backend.Author{ID: uid}}, nil),
		loaded:  core.NewSignal(cx, false),
		err:     core.NewSignal(cx, ""),
		tab:     core.NewSignal(cx, 0),
	}
	u.feeds = newPager(cx, func(ctx context.Context, page int) ([]backend.Feed, bool, error) {
		r, err := a.backend.UserFeeds(ctx, uid, page)
		return r.Feeds, r.HasMore, err
	}, func(f backend.Feed) string { return f.ID })
	people := func(kind backend.UserConnectionKind) *pager[backend.Author] {
		return newPager(cx, func(ctx context.Context, page int) ([]backend.Author, bool, error) {
			r, err := a.backend.UserConnections(ctx, uid, kind, page)
			return r.Users, r.HasMore, err
		}, func(p backend.Author) string { return p.ID })
	}
	u.following, u.fans = people(backend.UserFollowing), people(backend.UserFans)
	u.load()

	refresh := func() {
		u.load()
		switch u.tab.GetUntracked() {
		case 1:
			u.following.reload()
		case 2:
			u.fans.reload()
		default:
			u.feeds.reload()
		}
	}
	header := column(core.Style{}, "", u.header(cx), simpleTabs(u.tab, "动态", "关注", "粉丝"))
	body := core.Dynamic(func(cx *core.Scope) core.Node {
		if !u.loaded.Get() && u.err.Get() != "" {
			return statusBlock("加载失败", u.err.Get(), "重试", func() { u.err.Set(""); u.load() })
		}
		switch u.tab.Get() {
		case 1:
			return pagerList(cx, u.following, listSpec{header: header, empty: "暂无关注", onRefresh: refresh, bottom: 24},
				func(cx *core.Scope, get func() backend.Author, r core.ListRow) core.Node { return a.personRow(cx, get) })
		case 2:
			return pagerList(cx, u.fans, listSpec{header: header, empty: "暂无粉丝", onRefresh: refresh, bottom: 24},
				func(cx *core.Scope, get func() backend.Author, r core.ListRow) core.Node { return a.personRow(cx, get) })
		}
		return pagerList(cx, u.feeds, listSpec{header: header, empty: "暂无动态", onRefresh: refresh, bottom: 24, skeleton: skeletonCard},
			func(cx *core.Scope, get func() backend.Feed, r core.ListRow) core.Node { return a.feedCard(cx, get) })
	})
	return base.NavPage{
		TitleFunc: func() string { return u.profile.Get().Author.Name }, HeaderStyleFunc: a.headerStyle(cx), Class: "bg-background",
		Children: []core.Node{column(fill, "", core.Dynamic(func(cx *core.Scope) core.Node {
			if u.loaded.Get() && u.err.Get() != "" {
				return errorBanner(u.err.Get(), u.load, func() { u.err.Set("") })
			}
			return core.Group{}
		}), body)},
	}
}

func (u *userPageState) load() {
	if u.loading {
		return
	}
	u.loading = true
	u.gen++
	gen := u.gen
	background(u.a, func(ctx context.Context) (backend.UserProfile, error) { return u.a.backend.User(ctx, u.uid) },
		func(profile backend.UserProfile, err error) {
			if gen != u.gen {
				return
			}
			u.loading = false
			if err != nil {
				u.err.Set(err.Error())
				return
			}
			u.profile.Set(profile)
			u.loaded.Set(true)
			u.err.Set("")
		})
}

func (u *userPageState) header(cx *core.Scope) core.Node {
	a := u.a
	p := func() backend.UserProfile { return u.profile.Get() }
	author := func() backend.Author { return p().Author }
	cover := core.View{Class: "bg-secondary-container", Style: core.Style{Width: core.Pct(100), Height: core.Pt(128), Overflow: core.Hidden},
		Children: []core.Node{a.pic(cx, func() string {
			if c := p().CoverURL; c != "" {
				return thumbURL(backend.Image{URL: c}, thumbMedium)
			}
			return ""
		}, core.Cover, "", core.Style{Width: core.Pct(100), Height: core.Pct(100)})}}
	stat := func(i int, n func() int, name string) core.Node {
		return tap(name, func() { u.tab.Set(i) }, core.Style{Direction: core.Row, Gap: 4, AlignItems: core.AlignBaseline}, "",
			core.Text{ContentFunc: func() string { return formatCount(n()) }, Class: "text-on-surface", Style: textStyle(textBody, 600)},
			label(name, textSecondary, 400, "text-on-surface-variant"))
	}
	info := []core.Node{
		row(core.Style{Gap: 8, AlignItems: core.AlignCenter}, "",
			authorName(author, textTitle),
			core.Dynamic(func(cx *core.Scope) core.Node {
				if p().Level <= 0 {
					return core.Group{}
				}
				return tonalPill(func() string { return "Lv." + formatCount(p().Level) })
			})),
		core.Dynamic(func(cx *core.Scope) core.Node {
			if p().VerifyTitle == "" {
				return core.Group{}
			}
			return core.Text{ContentFunc: func() string { return p().VerifyTitle }, MaxLines: 1, Class: "text-primary", Style: textStyle(textSecondary, 400)}
		}),
	}
	return column(core.Style{}, "bg-background",
		cover,
		column(core.Style{Padding: core.Edges{Right: 16, Bottom: 16, Left: 16}, Gap: 12}, "",
			row(core.Style{Gap: 12, AlignItems: core.AlignEnd, Margin: core.Edges{Top: -36}}, "",
				column(core.Style{Width: core.Pt(80), Height: core.Pt(80), CornerRadius: 40, Padding: core.All(4), AlignItems: core.AlignCenter,
					JustifyContent: core.JustifyCenter, FlexShrink: 0}, "bg-background", a.avatar(cx, author, 72)),
				spacer,
				u.followButton(),
			),
			column(core.Style{Gap: 4}, "", info...),
			core.Dynamic(func(cx *core.Scope) core.Node {
				if p().Bio == "" {
					return core.Group{}
				}
				return core.Text{ContentFunc: func() string { return p().Bio }, MaxLines: 4, Class: "text-on-surface-variant", Style: textStyle(textSecondary, 400)}
			}),
			core.Dynamic(func(cx *core.Scope) core.Node {
				if !u.loaded.Get() {
					return core.Group{}
				}
				return row(core.Style{Gap: 24}, "",
					stat(0, func() int { return p().FeedCount }, "动态"),
					stat(1, func() int { return p().FollowCount }, "关注"),
					stat(2, func() int { return p().FansCount }, "粉丝"))
			}),
		),
	)
}

func (u *userPageState) followButton() core.Node {
	a := u.a
	followed := core.NewMemo(a.cx, func() bool { return u.profile.Get().IsFollowed })
	return core.Dynamic(func(cx *core.Scope) core.Node {
		if !u.loaded.Get() || u.uid == a.backend.SessionUID() {
			return core.Group{}
		}
		if followed.Get() {
			return tap("取消关注", u.toggleFollow, core.Style{Padding: core.Symmetric(10, 24), CornerRadius: 20, BorderWidth: 1, Overflow: core.Hidden}, "border-outline",
				label("已关注", textSecondary, 500, "text-on-surface"))
		}
		return tap("关注", u.toggleFollow, core.Style{Direction: core.Row, Padding: core.Symmetric(10, 24), CornerRadius: 20, Gap: 8, Overflow: core.Hidden}, "bg-primary",
			icon(&symbols.Add, 18, "text-on-primary"), label("关注", textSecondary, 500, "text-on-primary"))
	})
}

func (u *userPageState) toggleFollow() {
	a := u.a
	if u.followBusy || !a.requireLogin() {
		return
	}
	follow := !u.profile.GetUntracked().IsFollowed
	u.followBusy = true
	backgroundErr(a, func(ctx context.Context) error { return a.backend.SetUserFollow(ctx, u.uid, follow) },
		func(err error) {
			u.followBusy = false
			if err != nil {
				a.notice("关注失败：" + err.Error())
				return
			}
			p := u.profile.GetUntracked()
			p.IsFollowed = follow
			if follow {
				p.FansCount++
			} else {
				p.FansCount--
			}
			u.profile.Set(p)
		})
}

// personRow is a one-line Material 3 list item for an account.
func (a *App) personRow(cx *core.Scope, get func() backend.Author) core.Node {
	return tap("用户", func() { a.openUser(get()) }, core.Style{Direction: core.Row, Height: core.Pt(72), Padding: core.Symmetric(0, 16), Gap: 16, AlignItems: core.AlignCenter}, "bg-background",
		a.avatar(cx, get, 40),
		column(fill, "", authorName(get, textBody)),
		icon(&symbols.ChevronRight, 24, "text-on-surface-variant"),
	)
}

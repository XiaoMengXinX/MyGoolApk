package app

import (
	"context"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// userPageState is one user page: the profile over the account's posts,
// long posts, and questions and answers, each paged on its own.
type userPageState struct {
	a          *App
	uid        string
	profile    *core.Signal[backend.UserProfile]
	loaded     *core.Signal[bool]
	err        *core.Signal[string]
	loading    bool
	gen        int
	followBusy bool

	tab   *core.Signal[int]
	feeds [3]*pager[backend.Feed]
}

// userTabs are the lists of a user page, in the order of UserFeedKind.
var userTabs = []string{"动态", "图文", "问答"}

func (a *App) userPage(cx *core.Scope, uid string) core.Node {
	u := &userPageState{
		a: a, uid: uid,
		profile: core.NewSignalFunc(cx, backend.UserProfile{Author: backend.Author{ID: uid}}, nil),
		loaded:  core.NewSignal(cx, false),
		err:     core.NewSignal(cx, ""),
		tab:     core.NewSignal(cx, 0),
	}
	for i := range u.feeds {
		kind := backend.UserFeedKind(i)
		u.feeds[i] = newPager(cx, func(ctx context.Context, page int) ([]backend.Feed, bool, error) {
			r, err := a.backend.UserFeeds(ctx, uid, kind, page)
			return r.Feeds, r.HasMore, err
		}, func(f backend.Feed) string { return f.ID })
	}
	u.load()

	// The profile is over the tabs' lists and scrolls away with the shown one.
	over := newHeaded(cx, u.tab, len(userTabs), func(i int) *core.Ref { return &u.feeds[i].list })
	slide := newPageSlide(cx, core.NewSignal(cx, core.Points(0)), len(userTabs))
	pages := tabPages(cx, u.tab, slide, func(cx *core.Scope, i int) core.Node {
		p := u.feeds[i]
		spec := over.under(cx, i, listSpec{pushed: true, empty: "暂无内容", bottom: 24, skeleton: skeletonCard,
			onRefresh: func() {
				u.load()
				p.reload()
			}}, func() { p.items.Get(); p.loaded.Get() })
		return pagerList(cx, p, spec,
			func(cx *core.Scope, get func() backend.Feed, r core.ListRow) core.Node { return a.feedCard(cx, get) })
	})
	// The page is built again only as loading fails or recovers: a profile
	// that arrives after the lists leaves them where they are.
	failed := core.NewMemo(cx, func() bool { return !u.loaded.Get() && u.err.Get() != "" })
	body := core.Dynamic(func(cx *core.Scope) core.Node {
		if failed.Get() {
			return statusBlock("加载失败", u.err.Get(), "重试", func() { u.err.Set(""); u.load() })
		}
		return over.view(u.header(cx), simpleTabs(cx, u.tab, slide, userTabs...), pages)
	})
	return a.screen(func() string { return u.profile.Get().Author.Name }, nil,
		core.Dynamic(func(cx *core.Scope) core.Node {
			if u.loaded.Get() && u.err.Get() != "" {
				return errorBanner(u.err.Get(), u.load, func() { u.err.Set("") })
			}
			return core.Group{}
		}), body)
}

// peoplePage lists the accounts a user follows, or their followers.
func (a *App) peoplePage(cx *core.Scope, uid string, kind backend.UserConnectionKind) core.Node {
	title, empty := "关注", "暂无关注"
	if kind == backend.UserFans {
		title, empty = "粉丝", "暂无粉丝"
	}
	people := newPager(cx, func(ctx context.Context, page int) ([]backend.Author, bool, error) {
		r, err := a.backend.UserConnections(ctx, uid, kind, page)
		return r.Users, r.HasMore, err
	}, func(p backend.Author) string { return p.ID })
	return a.screen(titled(title), nil,
		pagerList(cx, people, listSpec{pushed: true, empty: empty, bottom: 24},
			func(cx *core.Scope, get func() backend.Author, r core.ListRow) core.Node { return a.personRow(cx, get) }))
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

// The header's text is white over the cover under a scrim, whatever the
// appearance.
var (
	onCover    = core.RGB(0xFFFFFF)
	onCoverDim = core.RGBA(0xFFFFFF, 0.8)
)

func coverText(get func() string, size core.Points, weight core.FontWeight, color core.Color, lines int) core.Node {
	return core.Text{ContentFunc: get, MaxLines: lines, Style: textStyle(size, weight).Merge(core.Style{Color: color, FlexShrink: 1})}
}

// header is the profile over the account's cover, laid out as the official
// client's: the avatar and the follow button, the name with the level, the
// verification, the bio, the counts, and where and when the account was seen.
func (u *userPageState) header(cx *core.Scope) core.Node {
	a := u.a
	p := func() backend.UserProfile { return u.profile.Get() }
	author := func() backend.Author { return p().Author }
	cover := a.pic(cx, func() string {
		if c := p().CoverURL; c != "" {
			return thumbURL(backend.Image{URL: c}, thumbMedium)
		}
		return ""
	}, core.Cover, "", fillAbs)
	stat := func(n func() int, name string, open func()) core.Node {
		kids := []core.Node{
			coverText(func() string { return formatCount(n()) }, textBody, 600, onCover, 1),
			coverText(func() string { return name }, textSecondary, 400, onCoverDim, 1),
		}
		s := core.Style{Direction: core.Row, Gap: 4, AlignItems: core.AlignBaseline, Padding: core.Symmetric(6, 0)}
		if open == nil {
			return core.View{Style: s, Children: kids}
		}
		return tap(name, open, s, "", kids...)
	}
	chip := func(text func() string) core.Node {
		return core.Dynamic(func(cx *core.Scope) core.Node {
			if text() == "" {
				return core.Group{}
			}
			return core.View{Style: core.Style{Background: core.RGBA(0xFFFFFF, 0.2), CornerRadius: 8, Padding: core.Symmetric(2, 8)},
				Children: []core.Node{coverText(text, textCaption, 400, onCover, 1)}}
		})
	}
	line := func(glyph *symbol, text func() string, lines int) core.Node {
		return core.Dynamic(func(cx *core.Scope) core.Node {
			if text() == "" {
				return core.Group{}
			}
			kids := []core.Node{coverText(text, textSecondary, 400, onCoverDim, lines)}
			if glyph != nil {
				kids = []core.Node{core.View{Style: core.Style{Height: core.Pt(20), JustifyContent: core.JustifyCenter},
					Children: []core.Node{icon(glyph, 16, "text-white")}}, kids[0]}
			}
			return row(core.Style{Gap: 6, AlignItems: core.AlignStart}, "", kids...)
		})
	}
	return column(core.Style{Background: core.RGB(0x2E312F), Overflow: core.Hidden}, "",
		cover,
		core.View{Style: fillAbs.Merge(core.Style{Background: core.RGBA(0x000000, 0.45)})},
		column(core.Style{Padding: core.Edges{Top: 16, Right: 16, Bottom: 12, Left: 16}, Gap: 12}, "",
			row(core.Style{AlignItems: core.AlignCenter}, "",
				column(core.Style{Width: core.Pt(76), Height: core.Pt(76), CornerRadius: 38, AlignItems: core.AlignCenter,
					JustifyContent: core.JustifyCenter, FlexShrink: 0, Background: core.RGBA(0xFFFFFF, 0.9)}, "", a.avatar(cx, author, 72)),
				spacer,
				u.followButton(cx),
			),
			column(core.Style{Gap: 6}, "",
				row(core.Style{Gap: 8, AlignItems: core.AlignCenter}, "",
					coverText(func() string { return displayName(author().Name) }, textTitle, 500, onCover, 1),
					core.Dynamic(func(cx *core.Scope) core.Node {
						if p().Level <= 0 {
							return core.Group{}
						}
						return tonalPill(func() string { return "Lv." + formatCount(p().Level) })
					})),
				line(&symbols.Verified, func() string { return p().VerifyTitle }, 1),
				line(nil, func() string { return p().Bio }, 4),
			),
			core.Dynamic(func(cx *core.Scope) core.Node {
				if !u.loaded.Get() {
					return core.Group{}
				}
				return column(core.Style{Gap: 6}, "",
					row(core.Style{Gap: 20}, "",
						stat(func() int { return p().LikeCount }, "获赞", nil),
						stat(func() int { return p().FollowCount }, "关注", func() { a.push(routeFollows, u.uid) }),
						stat(func() int { return p().FansCount }, "粉丝", func() { a.push(routeFans, u.uid) })),
					row(core.Style{Gap: 8, Wrap: core.Wrap}, "",
						chip(func() string {
							switch p().Gender {
							case 1:
								return "♂"
							case 0:
								return "♀"
							}
							return ""
						}),
						chip(func() string { return p().Region }),
						chip(func() string {
							if t := p().LastActive; !t.IsZero() {
								return formatTimeNow(t) + "活跃"
							}
							return ""
						})))
			}),
		),
	)
}

func (u *userPageState) followButton(cx *core.Scope) core.Node {
	a := u.a
	followed := core.NewMemo(cx, func() bool { return u.profile.Get().IsFollowed })
	return core.Dynamic(func(cx *core.Scope) core.Node {
		if !u.loaded.Get() || u.uid == a.backend.SessionUID() {
			return core.Group{}
		}
		if followed.Get() {
			return tap("取消关注", u.toggleFollow, core.Style{Padding: core.Symmetric(10, 24), CornerRadius: 20, BorderWidth: 1,
				BorderColor: onCoverDim}, "", coverText(func() string { return "已关注" }, textSecondary, 500, onCover, 1))
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
		column(fill, "", authorName(cx, get, textBody)),
		icon(&symbols.ChevronRight, 24, "text-on-surface-variant"),
	)
}

package app

import (
	"context"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// accountState is the signed-in profile and the message-center badges. The
// session itself lives in the backend; uid mirrors it for the views.
type accountState struct {
	uid     *core.Signal[string]
	profile *core.Signal[backend.UserProfile]
	loaded  *core.Signal[bool]
	err     *core.Signal[string]
	unread  *core.Signal[backend.UnreadCounts]
	loading bool
	gen     int
}

func (a *App) newAccountState(cx *core.Scope) {
	a.account = accountState{
		uid:     core.NewSignal(cx, a.backend.SessionUID()),
		profile: core.NewSignalFunc(cx, backend.UserProfile{}, nil),
		loaded:  core.NewSignal(cx, false),
		err:     core.NewSignal(cx, ""),
		unread:  core.NewSignal(cx, backend.UnreadCounts{}),
	}
}

// requireLogin sends a guest to the login page and reports whether a
// write may go ahead.
func (a *App) requireLogin() bool {
	if a.backend.SessionUID() != "" {
		return true
	}
	a.openLogin()
	return false
}

// ensureAccount loads the profile of a restored session once.
func (a *App) ensureAccount() {
	s := &a.account
	if s.uid.GetUntracked() == "" || s.loaded.GetUntracked() || s.loading || s.err.GetUntracked() != "" {
		return
	}
	a.loadAccount()
}

func (a *App) loadAccount() {
	s := &a.account
	uid := a.backend.SessionUID()
	s.uid.Set(uid)
	s.loading = true
	s.gen++
	gen := s.gen
	go func() {
		profile, err := a.backend.User(context.Background(), uid)
		unread, unreadErr := a.backend.UnreadCounts(context.Background())
		a.cx.Post(func() {
			if gen != s.gen {
				return
			}
			s.loading = false
			if err != nil {
				s.err.Set(err.Error())
			} else {
				s.profile.Set(profile)
				s.loaded.Set(true)
				s.err.Set("")
			}
			if unreadErr == nil {
				s.unread.Set(unread)
			}
		})
	}()
}

// resetAccount forgets the signed-in profile after signing in or out.
func (a *App) resetAccount() {
	s := &a.account
	s.gen++
	s.loading = false
	s.uid.Set(a.backend.SessionUID())
	s.profile.Set(backend.UserProfile{})
	s.loaded.Set(false)
	s.err.Set("")
	s.unread.Set(backend.UnreadCounts{})
	a.messages.reset()
}

// accountAvatar is the signed-in user's picture, or a gray disc for a guest.
func (a *App) accountAvatar(cx *core.Scope, size core.Points) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		a.ensureAccount()
		if !a.account.loaded.Get() {
			return guestAvatar(size)
		}
		return a.avatar(cx, func() backend.Author { return a.account.profile.Get().Author }, size)
	})
}

// guestAvatar stands for an account that is not signed in: a gray disc, not
// the theme's color, which an account's own picture would take the place of.
func guestAvatar(size core.Points) core.Node {
	return column(core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2, AlignItems: core.AlignCenter,
		JustifyContent: core.JustifyCenter, FlexShrink: 0}, "bg-surface-container-highest",
		icon(&symbols.Person, size*0.6, "text-on-surface-variant"))
}

func (a *App) logout() {
	if err := a.backend.Logout(); err != nil {
		a.notice("退出失败：" + err.Error())
		return
	}
	a.resetAccount()
	a.home.reload(tabFollow, tabHeadline)
}

// mePage is the account tab: the profile, then the lists and settings.
func (a *App) mePage(cx *core.Scope) core.Node {
	s := &a.account
	signedIn := func() bool { return s.uid.Get() != "" }
	head := core.Dynamic(func(cx *core.Scope) core.Node {
		a.ensureAccount()
		switch {
		case !signedIn():
			return a.guestCard()
		case s.loaded.Get():
			return a.profileCard(cx)
		case s.err.Get() != "":
			return errorBanner(s.err.Get(), func() { s.err.Set(""); a.loadAccount() }, nil)
		}
		return column(core.Style{Height: core.Pt(96), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, "", spinner())
	})
	group := core.Dynamic(func(cx *core.Scope) core.Node {
		var items []core.Node
		if signedIn() {
			items = append(items, listItem(&symbols.Mail, "消息", badge(func() int { return s.unread.Get().Notifications + s.unread.Get().Messages }),
				func() { a.push(routeMessages, "") }))
		}
		items = append(items,
			listItem(&symbols.History, "浏览历史", nil, a.openHistory),
			listItem(&symbols.Settings, "设置", nil, a.openSettings),
		)
		return column(core.Style{CornerRadius: 28, Overflow: core.Hidden}, "bg-surface-container-low", items...)
	})
	logout := core.Dynamic(func(cx *core.Scope) core.Node {
		if !signedIn() {
			return core.Group{}
		}
		return row(core.Style{}, "", textButton("退出登录", a.logout))
	})
	return column(fill, "bg-background",
		topBar(label("我的", textTitle, 400, "text-on-surface")),
		core.Scroll{Style: fill, Children: []core.Node{
			column(core.Style{Padding: core.Edges{Top: 8, Right: 16, Left: 16}, Gap: 16}, "", head, group, logout, bottomSpace(24, false)),
		}},
	)
}

func (a *App) guestCard() core.Node {
	return row(core.Style{Padding: core.All(16), Gap: 16, AlignItems: core.AlignCenter, CornerRadius: 28}, "bg-surface-container-low",
		guestAvatar(56),
		column(fill.Merge(core.Style{Gap: 4}), "",
			label("未登录", textBody, 500, "text-on-surface"),
			label("登录后参与互动", textSecondary, 400, "text-on-surface-variant")),
		tonalButton("登录", a.openLogin),
	)
}

func (a *App) profileCard(cx *core.Scope) core.Node {
	profile := func() backend.UserProfile { return a.account.profile.Get() }
	author := func() backend.Author { return profile().Author }
	stat := func(n func() int, name string) core.Node {
		return column(fill.Merge(core.Style{Gap: 2, AlignItems: core.AlignCenter}), "",
			core.Text{ContentFunc: func() string { return formatCount(n()) }, Class: "text-on-surface", Style: textStyle(textBody, 600)},
			label(name, textCaption, 400, "text-on-surface-variant"))
	}
	return tap("我的主页", func() { a.openUser(author()) }, core.Style{Padding: core.All(16), Gap: 16, CornerRadius: 28, Overflow: core.Hidden}, "bg-surface-container-low",
		row(core.Style{Gap: 16, AlignItems: core.AlignCenter}, "",
			a.avatar(cx, author, 56),
			column(fill.Merge(core.Style{Gap: 4}), "",
				row(core.Style{Gap: 8, AlignItems: core.AlignCenter}, "",
					authorName(author, textTitle),
					core.Dynamic(func(cx *core.Scope) core.Node {
						if profile().Level <= 0 {
							return core.Group{}
						}
						return tonalPill(func() string { return "Lv." + formatCount(profile().Level) })
					})),
				label("查看个人主页", textSecondary, 400, "text-on-surface-variant")),
			icon(&symbols.ChevronRight, 24, "text-on-surface-variant")),
		row(core.Style{}, "",
			stat(func() int { return profile().FeedCount }, "动态"),
			stat(func() int { return profile().FollowCount }, "关注"),
			stat(func() int { return profile().FansCount }, "粉丝")),
	)
}

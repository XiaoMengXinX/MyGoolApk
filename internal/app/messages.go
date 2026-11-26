package app

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// Lists of the message center opened from its entries, as the ids of
// routeNotices.
const (
	noticesLikes   = "likes"
	noticesFollows = "follows"
	noticesChats   = "chats"
	noticesService = "service"
)

// messageCenter is the notices and private messages of the account: what
// other accounts wrote to it, under the entries to its likes, new
// followers, private messages and service notices.
type messageCenter struct {
	notices, likes, follows *pager[backend.Notification]
	conversations           *pager[backend.Conversation]
	updating                *core.Signal[bool]
	// senders are the senders of service notices, sender the kind of the one
	// shown, and service its notices.
	senders *core.Signal[[]backend.NoticeSender]
	sender  *core.Signal[string]
	service *pager[backend.Notification]
}

func (a *App) newMessageCenter(cx *core.Scope) *messageCenter {
	notices := func(kind backend.NoticeKind) *pager[backend.Notification] {
		return newPager(cx, func(ctx context.Context, page int) ([]backend.Notification, bool, error) {
			r, err := a.backend.Notifications(ctx, kind, page)
			return r.Items, r.HasMore, err
		}, func(n backend.Notification) string { return n.ID })
	}
	m := &messageCenter{
		notices: notices(backend.NoticesAll),
		likes:   notices(backend.NoticesLikes),
		follows: notices(backend.NoticesFollows),
		conversations: newPager(cx, func(ctx context.Context, page int) ([]backend.Conversation, bool, error) {
			r, err := a.backend.Conversations(ctx, page)
			return r.Items, r.HasMore, err
		}, func(m backend.Conversation) string { return m.Ukey }),
		updating: core.NewSignal(cx, false),
		senders:  core.NewSignalFunc(cx, []backend.NoticeSender(nil), nil),
		sender:   core.NewSignal(cx, ""),
	}
	m.service = newQueryPager(cx, m.sender.GetUntracked, func(ctx context.Context, sender string, page int) ([]backend.Notification, bool, error) {
		// The senders come first: the notices are those of the first one
		// until another is picked.
		if sender == "" {
			senders, err := a.backend.NoticeSenders(ctx)
			if err != nil || len(senders) == 0 {
				return nil, false, err
			}
			sender = senders[0].Kind
			cx.Post(func() {
				m.senders.Set(senders)
				if m.sender.GetUntracked() == "" {
					m.sender.Set(sender)
				}
			})
		}
		r, err := a.backend.ServiceNotices(ctx, sender, page)
		return r.Items, r.HasMore, err
	}, func(n backend.Notification) string { return n.ID })
	return m
}

// reset drops what the lists loaded, as after signing in or out.
func (m *messageCenter) reset() {
	m.senders.Set(nil)
	m.sender.Set("")
	for _, p := range []*pager[backend.Notification]{m.notices, m.likes, m.follows, m.service} {
		if p.started {
			p.reset()
		}
	}
	if m.conversations.started {
		m.conversations.reset()
	}
}

// messagesPage is the message center.
func (a *App) messagesPage(cx *core.Scope) core.Node {
	unread := a.account.unread.Get
	entry := func(glyph *symbol, title, id string, count func() int) core.Node {
		return tap(title, func() { a.push(routeNotices, id) }, fill.Merge(core.Style{Padding: core.Symmetric(8, 0), Gap: 6, AlignItems: core.AlignCenter, CornerRadius: 12}), "",
			core.View{Style: core.Style{Width: core.Pt(48), Height: core.Pt(48)}, Children: []core.Node{
				core.View{Class: "bg-secondary-container", Style: insetAbs(0).Merge(core.Style{CornerRadius: 16, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}),
					Children: []core.Node{icon(glyph, 24, "text-on-secondary-container")}},
				core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(-4), Right: core.Pt(-6)}, Children: []core.Node{badge(count)}},
			}},
			label(title, textSecondary, 400, "text-on-surface"))
	}
	entries := row(core.Style{Padding: core.Symmetric(8, 8)}, "",
		entry(&symbols.ThumbUp, "点赞", noticesLikes, func() int { return unread().Likes }),
		entry(&symbols.PersonFill, "关注", noticesFollows, func() int { return unread().Follows }),
		entry(&symbols.MailFill, "私信", noticesChats, func() int { return unread().Messages }),
		entry(&symbols.Campaign, "服务通知", noticesService, func() int { return unread().Service }))
	a.refreshUnread()
	notices := a.messages.notices
	list := pagerList(cx, notices, listSpec{pushed: true, empty: "暂无通知", header: entries,
		onRefresh: func() { a.refreshUnread(); notices.reload() }}, a.noticeRow)
	return a.screen(titled("通知中心"), a.notificationActions(cx), list)
}

func (a *App) notificationActions(cx *core.Scope) core.Node {
	button := func(glyph *symbol, title string, press func()) core.Node {
		view := iconAction(glyph, title, press)
		view.Disabled = a.messages.updating.Get
		view.Style.Disabled = &core.Style{Opacity: core.Alpha(0.4)}
		return view
	}
	return row(core.Style{AlignItems: core.AlignCenter, FlexShrink: 0}, "",
		button(&symbols.DoneAll, "全部已读", func() { a.updateNotifications(false) }),
		button(&symbols.Delete, "清除通知", func() {
			cx.ShowAlert(core.AlertConfig{Title: "清除通知？", Message: "将删除通知中心的评论、回复和提及通知。", Cancelable: true,
				Actions: []core.AlertAction{{Label: "取消", Role: core.AlertCancel}, {Label: "清除", Role: core.AlertDestructive}},
			}, func(choice int) {
				if choice == 1 {
					a.updateNotifications(true)
				}
			})
		}))
}

func (a *App) updateNotifications(clear bool) {
	m := a.messages
	uid := a.account.uid.GetUntracked()
	if m.updating.GetUntracked() || uid == "" {
		return
	}
	work, title, success := a.backend.MarkNotificationsRead, "标记已读", "已全部标为已读"
	if clear {
		work, title, success = a.backend.ClearNotifications, "清除通知", "通知已清除"
	}
	m.updating.Set(true)
	backgroundErr(a, work, func(err error) {
		m.updating.Set(false)
		if a.account.uid.GetUntracked() != uid {
			return
		}
		if err != nil {
			a.notice(title + "失败：" + err.Error())
			return
		}
		if clear {
			m.notices.reset()
		} else {
			m.notices.restart()
		}
		a.refreshUnread()
		a.notice(success)
	})
}

// noticesPage is a list of the message center opened from its entries.
func (a *App) noticesPage(cx *core.Scope, id string) core.Node {
	m := a.messages
	switch id {
	case noticesLikes:
		return a.screen(titled("点赞"), nil, pagerList(cx, m.likes, listSpec{pushed: true, empty: "暂无点赞"}, a.noticeRow))
	case noticesFollows:
		return a.screen(titled("关注"), nil, pagerList(cx, m.follows, listSpec{pushed: true, empty: "暂无新关注"}, a.noticeRow))
	case noticesService:
		return a.screen(titled("服务通知"), nil, a.serviceNotices(cx))
	}
	return a.screen(titled("私信"), nil, pagerList(cx, m.conversations, listSpec{pushed: true, empty: "暂无私信"},
		func(cx *core.Scope, get func() backend.Conversation, r core.ListRow) core.Node {
			return a.messageRow(cx,
				func() backend.Author { c := get(); return backend.Author{Name: c.Title, AvatarURL: c.AvatarURL} },
				func() string { return get().Title }, func() string { return get().LastMessage },
				func() string { return formatTime(time.Now(), get().LastTime) }, func() bool { return get().Unread > 0 },
				func() {
					c := get()
					m.conversations.update(c.Ukey, func(c backend.Conversation) backend.Conversation { c.Unread = 0; return c })
					a.push(routeChat, base64.RawURLEncoding.EncodeToString([]byte(c.Ukey+"\x00"+c.UID+"\x00"+c.Title)))
				})
		}))
}

// noticeRow is a notice of an account: who wrote, what, and when.
func (a *App) noticeRow(cx *core.Scope, get func() backend.Notification, _ core.ListRow) core.Node {
	return a.messageRow(cx,
		func() backend.Author { n := get(); return backend.Author{Name: n.Title, AvatarURL: n.AvatarURL} },
		func() string { return get().Title }, func() string { return get().Summary },
		func() string { return formatTime(time.Now(), get().CreatedAt) }, func() bool { return get().Unread },
		func() { a.openURL(get().URL) })
}

// serviceNotices are the notices of the site itself: a chip for each of
// their senders over the notices of the picked one.
func (a *App) serviceNotices(cx *core.Scope) core.Node {
	m := a.messages
	chips := core.Dynamic(func(cx *core.Scope) core.Node {
		senders := m.senders.Get()
		if len(senders) == 0 {
			return core.Group{}
		}
		kids := make([]core.Node, len(senders))
		for i, sender := range senders {
			kids[i] = filterChip(sender.Name, func() bool { return m.sender.Get() == sender.Kind }, func() {
				if m.sender.GetUntracked() != sender.Kind {
					m.sender.Set(sender.Kind)
					m.service.reset()
				}
			})
		}
		return sideScroll{Style: core.Style{FlexGrow: 0}, Children: []core.Node{
			row(core.Style{Padding: core.Symmetric(8, 16), Gap: 8}, "", kids...)}}
	})
	return column(fill, "", chips,
		pagerList(cx, m.service, listSpec{pushed: true, empty: "暂无通知"},
			func(cx *core.Scope, get func() backend.Notification, _ core.ListRow) core.Node {
				body := column(core.Style{Padding: core.Symmetric(12, 16), Gap: 6}, "",
					row(core.Style{Gap: 8, AlignItems: core.AlignCenter}, "",
						core.Text{ContentFunc: func() string { return get().Title }, MaxLines: 1, Class: "text-on-surface", Style: fill.Merge(textStyle(textBody, 500))},
						labelFunc(func() string { return formatTime(time.Now(), get().CreatedAt) }, textCaption, 400, "text-on-surface-variant")),
					core.Text{ContentFunc: func() string { return get().Summary }, Class: "text-on-surface-variant", Style: textStyle(textSecondary, 400)},
					core.Dynamic(func(cx *core.Scope) core.Node {
						if get().Action == "" {
							return core.Group{}
						}
						return row(core.Style{Padding: core.Edges{Top: 4}, AlignItems: core.AlignCenter}, "",
							core.Text{ContentFunc: func() string { return get().Action }, Class: "text-primary", Style: fill.Merge(textStyle(textSecondary, 500))},
							icon(&symbols.ChevronRight, 20, "text-on-surface-variant"))
					}))
				return column(core.Style{Padding: core.Symmetric(4, 16)}, "",
					tap("通知", func() { a.openURL(get().URL) }, core.Style{CornerRadius: 12}, "bg-surface-container-low", body))
			}))
}

// messageRow is a three-line Material 3 list item with an unread dot.
func (a *App) messageRow(cx *core.Scope, who func() backend.Author, title, summary, when func() string, unread func() bool, open func()) core.Node {
	body := row(core.Style{Padding: core.Symmetric(12, 16), Gap: 16}, "bg-background",
		a.avatar(cx, who, 40),
		column(fill.Merge(core.Style{Gap: 4}), "",
			row(core.Style{Gap: 8, AlignItems: core.AlignCenter}, "",
				core.Text{ContentFunc: func() string { return displayName(title()) }, MaxLines: 1, Class: "text-on-surface", Style: fill.Merge(textStyle(textBody, 500))},
				core.Text{ContentFunc: when, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)},
				core.Dynamic(func(cx *core.Scope) core.Node {
					if !unread() {
						return core.Group{}
					}
					return core.View{Class: "bg-primary", Style: core.Style{Width: core.Pt(8), Height: core.Pt(8), CornerRadius: 4}}
				})),
			core.Text{ContentFunc: summary, MaxLines: 2, Class: "text-on-surface-variant", Style: textStyle(textSecondary, 400)},
		),
	)
	if open == nil {
		return body
	}
	return tap("消息", open, core.Style{}, "", body)
}

// chatPage is a private-message thread: its messages, the newest at the
// bottom, over the field that writes one.
func (a *App) chatPage(cx *core.Scope, param string) core.Node {
	raw, _ := base64.RawURLEncoding.DecodeString(param)
	parts := strings.SplitN(string(raw), "\x00", 3)
	ukey, uid, name := parts[0], parts[1], parts[2]
	chat := newPager(cx, func(ctx context.Context, page int) ([]backend.ChatMessage, bool, error) {
		r, err := a.backend.Chat(ctx, ukey, page)
		return r.Items, r.HasMore, err
	}, func(m backend.ChatMessage) string { return m.ID })
	text, sending := core.NewSignal(cx, ""), core.NewSignal(cx, false)
	send := func() {
		sending.Set(true)
		msg := strings.TrimSpace(text.GetUntracked())
		backgroundErr(a, func(ctx context.Context) error { return a.backend.SendMessage(ctx, uid, msg) },
			func(err error) {
				sending.Set(false)
				if err != nil {
					a.notice("发送失败：" + err.Error())
					return
				}
				text.Set("")
				chat.reload()
			})
	}
	field := core.Input{
		Multiline: true, Placeholder: "发私信…", Label: "私信",
		Value: text.Get, OnChange: text.Set, Disabled: sending.Get,
		Class: "bg-surface-container-highest text-on-surface",
		Style: fill.Merge(textStyle(textBody, 400)).Merge(core.Style{MaxHeight: core.Pt(120), Padding: core.Symmetric(12, 16), CornerRadius: 24}),
	}
	// The bar stands over the keyboard, or over the system's bar without one.
	env := a.cx.Env()
	bar := core.View{Class: "bg-surface-container",
		Style: core.Style{FlexShrink: 0},
		StyleFunc: func() core.Style {
			return core.Style{Padding: core.Edges{Bottom: max(env.SafeArea().Bottom, env.KeyboardHeight())}}
		},
		Children: []core.Node{row(core.Style{Padding: core.Edges{Top: 8, Right: 12, Bottom: 8, Left: 16}, Gap: 8, AlignItems: core.AlignEnd}, "",
			field,
			filledButton("发送", func() bool { return !sending.Get() && strings.TrimSpace(text.Get()) != "" }, send))}}
	return a.screen(titled(displayName(name)), nil,
		pagerList(cx, chat, listSpec{inverted: true, empty: "暂无消息", emptyHint: "发一条私信开始聊天。"},
			func(cx *core.Scope, get func() backend.ChatMessage, r core.ListRow) core.Node {
				return a.chatRow(cx, get)
			}),
		bar)
}

// chatRow is a message in its bubble: the account's own at the right, the
// other's at the left beside its avatar.
func (a *App) chatRow(cx *core.Scope, get func() backend.ChatMessage) core.Node {
	body := func() string {
		if m := get(); m.Picture && m.Text == "" {
			return "[图片]"
		}
		return get().Text
	}
	return dynamicShape(cx, func() bool { return get().Mine }, func(cx *core.Scope, mine bool) core.Node {
		class, text, side := "bg-surface-container-high", "text-on-surface", core.AlignStart
		if mine {
			class, text, side = "bg-primary-container", "text-on-primary-container", core.AlignEnd
		}
		bubble := column(core.Style{FlexShrink: 1, Gap: 4, AlignItems: side}, "",
			core.View{Class: class, Style: core.Style{Padding: core.Symmetric(10, 14), CornerRadius: 20},
				Children: []core.Node{core.Text{ContentFunc: body, Selectable: core.SelectableOn, Class: text, Style: textStyle(textBody, 400)}}},
			core.Text{ContentFunc: func() string { return formatTimeNow(get().CreatedAt) }, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)})
		if mine {
			return row(core.Style{Padding: core.Edges{Top: 6, Right: 16, Bottom: 6, Left: 64}, JustifyContent: core.JustifyEnd}, "", bubble)
		}
		return row(core.Style{Padding: core.Edges{Top: 6, Right: 64, Bottom: 6, Left: 16}, Gap: 8}, "",
			tap("主页", func() { a.openUser(get().Author) }, core.Style{FlexShrink: 0, CornerRadius: 18, AlignSelf: core.AlignStart}, "",
				a.avatar(cx, func() backend.Author { return get().Author }, 36)),
			bubble)
	})
}

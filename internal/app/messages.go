package app

import (
	"context"
	"time"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// messageCenter is the notifications and private messages of the account.
type messageCenter struct {
	tab           *core.Signal[int]
	notifications *pager[backend.Notification]
	conversations *pager[backend.Conversation]
}

func (a *App) newMessageCenter(cx *core.Scope) *messageCenter {
	return &messageCenter{
		tab: core.NewSignal(cx, 0),
		notifications: newPager(cx, func(ctx context.Context, page int) ([]backend.Notification, bool, error) {
			r, err := a.backend.Notifications(ctx, page)
			return r.Items, r.HasMore, err
		}, func(n backend.Notification) string { return n.ID }),
		conversations: newPager(cx, func(ctx context.Context, page int) ([]backend.Conversation, bool, error) {
			r, err := a.backend.Conversations(ctx, page)
			return r.Items, r.HasMore, err
		}, func(m backend.Conversation) string { return m.Ukey }),
	}
}

// reset drops what the pages loaded, as after signing in or out.
func (m *messageCenter) reset() {
	m.tab.Set(0)
	if m.notifications.started {
		m.notifications.reset()
	}
	if m.conversations.started {
		m.conversations.reset()
	}
}

func (a *App) messagesPage(cx *core.Scope) core.Node {
	m := a.messages
	return column(fill, "bg-background",
		simpleTabs(m.tab, "通知", "私信"),
		core.Show{When: func() bool { return m.tab.Get() == 0 },
			Then: func(cx *core.Scope) core.Node {
				return pagerList(cx, m.notifications, listSpec{empty: "暂无通知", emptyHint: "评论、点赞和关注会显示在这里。"},
					func(cx *core.Scope, get func() backend.Notification, r core.ListRow) core.Node {
						return a.messageRow(cx,
							func() backend.Author { n := get(); return backend.Author{Name: n.Title, AvatarURL: n.AvatarURL} },
							func() string { return get().Title }, func() string { return get().Summary },
							func() string { return formatTime(time.Now(), get().CreatedAt) }, func() bool { return get().Unread },
							func() {
								if n := get(); n.URL != "" {
									a.openLink(n.URL)
								}
							})
					})
			},
			Else: func(cx *core.Scope) core.Node {
				return pagerList(cx, m.conversations, listSpec{empty: "暂无私信"},
					func(cx *core.Scope, get func() backend.Conversation, r core.ListRow) core.Node {
						return a.messageRow(cx,
							func() backend.Author { c := get(); return backend.Author{Name: c.Title, AvatarURL: c.AvatarURL} },
							func() string { return get().Title }, func() string { return get().LastMessage },
							func() string { return formatTime(time.Now(), get().LastTime) }, func() bool { return get().Unread > 0 }, nil)
					})
			}},
	)
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

package app

import (
	"context"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"

	"app/internal/backend"
)

type replyShape struct {
	images   int
	children bool
}

func (d *detail) replyRow(cx *core.Scope, get func() backend.Reply) core.Node {
	a := d.a
	author := func() backend.Author { return get().Author }
	v := row(core.Style{Padding: core.Edges{Top: 12, Right: 16, Bottom: 8, Left: 16}, Gap: 12}, "bg-background",
		link("作者主页", func() { a.openUser(author()) }, core.Style{FlexShrink: 0, CornerRadius: 18, AlignSelf: core.AlignStart},
			a.avatar(cx, author, 36)),
		column(fill.Merge(core.Style{Gap: 6}), "",
			row(core.Style{Gap: 8, AlignItems: core.AlignCenter}, "",
				// The name opens the author, as the avatar does.
				link("作者主页", func() { a.openUser(author()) }, core.Style{FlexShrink: 1, MinWidth: core.Pt(0)},
					authorName(cx, author, textSecondary)),
				dynamicShape(cx, func() bool { return get().IsAuthor }, func(cx *core.Scope, owner bool) core.Node {
					if !owner {
						return core.Group{}
					}
					return tonalPill(func() string { return "楼主" })
				}),
				dynamicShape(cx, func() bool { return get().Pinned }, func(cx *core.Scope, pinned bool) core.Node {
					if !pinned {
						return core.Group{}
					}
					return tonalPill(func() string { return "置顶" })
				}),
				dynamicShape(cx, func() bool { return get().Stance != "" }, func(cx *core.Scope, opinion bool) core.Node {
					if !opinion {
						return core.Group{}
					}
					return tonalPill(func() string { return get().Stance })
				})),
			dynamicShape(cx, func() bool { return replyText(get()) != "" }, func(cx *core.Scope, has bool) core.Node {
				if !has {
					return core.Group{} // a comment of pictures alone
				}
				// A long press opens the comment's menu, which copies it.
				return a.richText(cx, func() ([]backend.RichSpan, string) {
					r := get()
					rich, text := r.Rich, r.Content
					if len(r.Images) > 0 {
						rich, text = withoutPicturePlaceholder(r.Rich), replyText(r)
					}
					if len(rich) == 0 && text != "" {
						rich = []backend.RichSpan{{Kind: backend.SpanText, Text: text}}
					}
					return append(replyLead(r), rich...), text
				}, richOptions{size: textSecondary, class: "text-on-surface", hold: func() { d.menu.Set(new(get())) }})
			}),
			dynamicShape(cx, func() replyShape {
				r := get()
				return replyShape{len(r.Images), !r.Post && len(r.Children) > 0}
			},
				func(cx *core.Scope, s replyShape) core.Node {
					var kids []core.Node
					switch {
					case s.images == 1:
						kids = append(kids, a.replyImage(cx, func() backend.Image { return get().Images[0] }, func() { a.openImages(get().Images, 0) }))
					case s.images > 1:
						kids = append(kids, a.imageGrid(cx, func() []backend.Image { return get().Images }, s.images, 3))
					}
					if s.children {
						kids = append(kids, d.threadBox(cx, get))
					}
					return column(core.Style{Gap: 6}, "", kids...)
				}),
			row(core.Style{AlignItems: core.AlignCenter}, "",
				core.Text{ContentFunc: func() string { return formatTimeNow(get().CreatedAt) }, Class: "text-on-surface-variant", Style: fill.Merge(textStyle(textCaption, 400))},
				dynamicShape(cx, func() bool { return get().Post }, func(cx *core.Scope, post bool) core.Node {
					if post {
						// An opinion or an answer is liked and answered on its own page.
						return compactAction(&symbols.Comment, "text-on-surface-variant", func() string {
							if n := get().ChildrenTotal; n > 0 {
								return formatCount(n)
							}
							return ""
						}, "查看全文", func() { a.push(routeFeed, get().ID) })
					}
					return row(core.Style{AlignItems: core.AlignCenter}, "",
						d.replyLike(cx, get),
						compactAction(&symbols.Comment, "text-on-surface-variant", nil, "回复", func() { d.openComposer(get().ID, displayName(get().Author.Name)) }),
						compactAction(&symbols.More, "text-on-surface-variant", nil, "更多", func() { d.menu.Set(new(get())) }))
				}),
			),
		),
	).(core.View)
	// A secondary click opens the actions of the ⋮ as the desktop's menu.
	v.OnContextMenu = func(e core.ContextMenuEvent) { d.popupReplyMenu(get(), e.PagePosition) }
	return v
}

// replyLead names whom a nested reply answers, before its text.
func replyLead(r backend.Reply) []backend.RichSpan {
	if r.ReplyToName == "" {
		return nil
	}
	to := backend.RichSpan{Kind: backend.SpanText, Text: r.ReplyToName}
	if r.ReplyToID != "" {
		to = backend.RichSpan{Kind: backend.SpanUser, Text: r.ReplyToName, URL: "/u/" + r.ReplyToID}
	}
	return []backend.RichSpan{{Kind: backend.SpanText, Text: "回复 "}, to, {Kind: backend.SpanText, Text: "："}}
}

// replyAction is something to do with a comment, from its menu.
type replyAction struct {
	glyph *symbol
	name  string
	run   func(backend.Reply)
}

// replyActions are what else there is to do with comment r: copy its text,
// see the replies under it, delete it when it is the account's, and else
// report it on the site's page.
func (d *detail) replyActions(r backend.Reply) []replyAction {
	a := d.a
	actions := []replyAction{{&symbols.Copy, "复制", func(r backend.Reply) {
		a.cx.SetClipboard(replyText(r))
		a.notice("已复制")
	}}}
	if r.ChildrenTotal > 0 && d.thread.GetUntracked() == nil {
		actions = append(actions, replyAction{&symbols.Comment, "查看回复", d.openThread})
	}
	if r.Author.ID != "" && r.Author.ID == a.backend.SessionUID() {
		actions = append(actions, replyAction{&symbols.Delete, "删除", d.deleteReply})
	} else {
		actions = append(actions, replyAction{&symbols.Flag, "举报", func(r backend.Reply) {
			a.openWeb("https://m.coolapk.com/mp/do?c=feed&m=report&type=feed_reply&id=" + r.ID)
		}})
	}
	return actions
}

// popupReplyMenu shows comment r's actions as the desktop's own menu at
// at, as a secondary click asks.
func (d *detail) popupReplyMenu(r backend.Reply, at core.Point) {
	actions := d.replyActions(r)
	items := make([]desktop.MenuItem, len(actions))
	for i, act := range actions {
		items[i] = desktop.MenuItem{Label: act.name, OnSelect: func() { act.run(r) }}
	}
	desktop.PopupMenu(d.cx, items, at, nil)
}

// replyMenu is the sheet of the actions of the comment d.menu holds.
func (d *detail) replyMenu() core.Node {
	closeMenu := func() { d.menu.Set(nil) }
	item := func(act replyAction) core.Node {
		return tap(act.name, func() {
			r := *d.menu.GetUntracked()
			closeMenu()
			act.run(r)
		}, core.Style{Direction: core.Row, Height: core.Pt(56), Gap: 16, AlignItems: core.AlignCenter}, "",
			icon(act.glyph, 24, "text-on-surface-variant"), label(act.name, textBody, 400, "text-on-surface"))
	}
	return d.a.bottomSheet(func() bool { return d.menu.Get() != nil }, closeMenu,
		core.Dynamic(func(cx *core.Scope) core.Node {
			r := d.menu.Get()
			if r == nil {
				return core.Group{}
			}
			actions := d.replyActions(*r)
			items := make([]core.Node, len(actions))
			for i, act := range actions {
				items[i] = item(act)
			}
			return column(core.Style{}, "", items...)
		}))
}

func (d *detail) deleteReply(r backend.Reply) {
	backgroundErr(d.a, func(ctx context.Context) error { return d.a.backend.DeleteReply(ctx, r.ID) },
		func(err error) {
			if err != nil {
				d.a.notice("删除失败：" + err.Error())
				return
			}
			gone := func(x backend.Reply) bool { return x.ID == r.ID }
			d.nested.items.Set(slices.DeleteFunc(slices.Clone(d.nested.items.GetUntracked()), gone))
			items := slices.Clone(d.replies.items.GetUntracked())
			if kept := slices.DeleteFunc(items, gone); len(kept) != len(items) {
				d.replies.items.Set(kept)
				f := d.feed.GetUntracked()
				if r.Pinned {
					f.Pinned = slices.DeleteFunc(slices.Clone(f.Pinned), gone)
				}
				f.Stats.Comments--
				d.feed.Set(f)
				return
			}
			for i := range items {
				if kept := slices.DeleteFunc(slices.Clone(items[i].Children), gone); len(kept) != len(items[i].Children) {
					items[i].Children, items[i].ChildrenTotal = kept, items[i].ChildrenTotal-1
					d.replies.items.Set(items)
					return
				}
			}
		})
}

// dynamicShape builds body again when the shape of its item changes.
func dynamicShape[S comparable](cx *core.Scope, shape func() S, body func(cx *core.Scope, s S) core.Node) core.Node {
	m := core.NewMemo(cx, shape)
	return core.Dynamic(func(cx *core.Scope) core.Node { return body(cx, m.Get()) })
}

func (a *App) replyImage(cx *core.Scope, get func() backend.Image, open func()) core.Node {
	return row(core.Style{}, "", tap("查看图片", open, core.Style{Width: core.Pct(60), CornerRadius: 8, Overflow: core.Hidden}, "bg-surface-container-highest",
		a.pic(cx, func() string { return thumbURL(get(), thumbSmall) }, core.Cover, "", core.Style{Width: core.Pct(100), AspectRatio: 4.0 / 3})))
}

// compactAction is a 32dp icon action with an optional count, for rows
// dense with actions such as comments.
func compactAction(glyph *symbol, class string, count func() string, name string, fn func()) core.Node {
	kids := []core.Node{icon(glyph, 18, class)}
	if count != nil {
		kids = append(kids, core.Text{ContentFunc: count, Style: textStyle(textCaption, 400)})
	}
	return tap(name, fn, core.Style{Direction: core.Row, Height: core.Pt(32), MinWidth: core.Pt(40), Padding: core.Symmetric(0, 10), Gap: 4,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, CornerRadius: 16, Overflow: core.Hidden}, class, kids...)
}

func (d *detail) replyLike(cx *core.Scope, get func() backend.Reply) core.Node {
	liked := core.NewMemo(cx, func() bool { return get().Liked })
	count := func() string {
		if n := get().Likes; n > 0 {
			return formatCount(n)
		}
		return ""
	}
	toggle := func() { d.toggleReplyLike(get().ID, !get().Liked) }
	return core.Dynamic(func(cx *core.Scope) core.Node {
		if liked.Get() {
			return compactAction(&symbols.Favorite, "text-primary", count, "取消赞", toggle)
		}
		return compactAction(&symbols.FavoriteOutline, "text-on-surface-variant", count, "赞", toggle)
	})
}

func (d *detail) toggleReplyLike(id string, liked bool) {
	if d.replyLikeBusy[id] || !d.a.requireLogin() {
		return
	}
	d.replyLikeBusy[id] = true
	background(d.a, func(ctx context.Context) (int, error) { return d.a.backend.SetReplyLike(ctx, id, liked) },
		func(likes int, err error) {
			delete(d.replyLikeBusy, id)
			if err != nil {
				d.a.notice("点赞失败：" + err.Error())
				return
			}
			d.updateReply(id, func(r *backend.Reply) { r.Liked, r.Likes = liked, likes })
		})
}

// updateReply changes the reply with id where it shows: among the comments,
// among the replies a comment came with, and in the thread sheet.
func (d *detail) updateReply(id string, fn func(*backend.Reply)) {
	change := func(r backend.Reply) backend.Reply {
		fn(&r)
		return r
	}
	d.nested.update(id, change)
	items := slices.Clone(d.replies.items.GetUntracked())
	for i := range items {
		if items[i].ID == id {
			items[i] = change(items[i])
			d.replies.items.Set(items)
			return
		}
		if j := slices.IndexFunc(items[i].Children, func(c backend.Reply) bool { return c.ID == id }); j >= 0 {
			items[i].Children = slices.Clone(items[i].Children)
			items[i].Children[j] = change(items[i].Children[j])
			d.replies.items.Set(items)
			return
		}
	}
}

// threadBox shows the nested replies a comment came with, and opens the
// sheet of all of them.
func (d *detail) threadBox(cx *core.Scope, get func() backend.Reply) core.Node {
	a := d.a
	return core.Dynamic(func(cx *core.Scope) core.Node {
		r := get()
		var kids []core.Node
		for _, child := range r.Children {
			child := child
			spans := []backend.RichSpan{mentionSpan(child.Author)}
			if child.ReplyToName != "" {
				spans = append(spans, backend.RichSpan{Kind: backend.SpanText, Text: " 回复 "})
				if child.ReplyToID != "" {
					spans = append(spans, backend.RichSpan{Kind: backend.SpanUser, Text: child.ReplyToName, URL: "/u/" + child.ReplyToID})
				} else {
					spans = append(spans, backend.RichSpan{Kind: backend.SpanText, Text: child.ReplyToName})
				}
			}
			spans = append(spans, backend.RichSpan{Kind: backend.SpanText, Text: "："})
			body := child.Rich
			if len(body) == 0 && child.Content != "" {
				body = []backend.RichSpan{{Kind: backend.SpanText, Text: child.Content}}
			}
			if len(child.Images) > 0 {
				// The server writes "[图片]" where the pictures go; they show
				// under the text.
				body = withoutPicturePlaceholder(body)
			}
			spans = append(spans, body...)
			// A press on a reply answers it, and a long one opens its menu.
			kids = append(kids, a.richText(cx, func() ([]backend.RichSpan, string) { return spans, "" },
				richOptions{size: textSecondary, class: "text-on-surface",
					press: func() { d.openComposer(child.ID, displayName(child.Author.Name)) },
					hold:  func() { d.menu.Set(&child) }}))
			if len(child.Images) > 0 {
				kids = append(kids, a.threadPictures(cx, child.Images))
			}
		}
		if r.ChildrenHasMore {
			text := "查看更多回复"
			if r.ChildrenTotal > len(r.Children) {
				text = "查看全部 " + formatCount(r.ChildrenTotal) + " 条回复"
			}
			kids = append(kids, tap(text, func() { d.openThread(r) }, core.Style{}, "", label(text, textSecondary, 500, "text-primary")))
		}
		return column(core.Style{Padding: core.All(12), Gap: 8, CornerRadius: 8}, "bg-surface-container-low", kids...)
	})
}

// openThread lists all the replies under comment r in the thread sheet.
func (d *detail) openThread(r backend.Reply) {
	d.threadY = 0
	d.thread.Set(&r)
	d.nested.reset()
}

// fetchNested lists the replies under the comment parent names, none without one.
func (d *detail) fetchNested(ctx context.Context, parent string, page int) ([]backend.Reply, bool, error) {
	if parent == "" {
		return nil, false, nil
	}
	r, err := d.a.backend.Replies(ctx, backend.ReplyQuery{FeedID: d.id, ParentID: parent, Page: page})
	return r.Replies, r.HasMore, err
}

// threadSheet is the sheet of a comment and all the replies under it.
func (d *detail) threadSheet(cx *core.Scope) core.Node {
	a := d.a
	// The comment leads the list without the replies it came with, which the
	// list shows.
	parent := func() backend.Reply {
		p := d.thread.Get()
		if p == nil {
			return backend.Reply{}
		}
		r, _ := d.replies.find(p.ID)
		if r.ID == "" {
			r = *p
		}
		r.Children, r.ChildrenTotal = nil, 0
		return r
	}
	title := func() string {
		if p := d.thread.Get(); p != nil && p.ChildrenTotal > 0 {
			return formatCount(p.ChildrenTotal) + " 条回复"
		}
		return "回复"
	}
	// The sheet hides while a page it opened covers the post, and comes back
	// as it was, scrolled where it was.
	return a.sheet(func() bool { return d.thread.Get() != nil && !d.covered() }, func() {
		if !d.covered() {
			d.thread.Set(nil)
			d.threadY = 0
		}
	}, "bg-background", 0,
		core.Text{ContentFunc: title, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textBody, 500).Merge(core.Style{Padding: core.Symmetric(0, 16)})},
		core.View{
			// The sheet is as wide as what it holds, which a list is not: it
			// gets the window's width, or the dialog's in a wide window, and its
			// rows wrap in it.
			StyleFunc: func() core.Style {
				w := a.cx.Env().Window()
				width := w.Width
				if a.wide() {
					width = min(dialogWidth, w.Width*0.9)
				}
				return core.Style{Width: core.Pt(width), Height: core.Pt(w.Height * 0.7)}
			},
			// Shown again, the list goes back to where it was.
			OnLayout: func(core.Bounds) {
				if y := d.threadY; y > 0 {
					d.nested.list.ScrollTo(core.Point{Y: y}, false)
				}
			},
			Children: []core.Node{pagerList(cx, d.nested, listSpec{empty: "暂无回复", bottom: 8, inSheet: true,
				onScroll: func(e core.ScrollEvent) {
					if !e.Stale {
						d.threadY = e.Offset.Y
					}
				},
				header: column(core.Style{}, "", d.replyRow(cx, parent), core.View{Class: "bg-surface-container-low", Style: core.Style{Height: core.Pt(8)}}),
			}, func(cx *core.Scope, get func() backend.Reply, r core.ListRow) core.Node { return d.replyRow(cx, get) })},
		})
}

// composer is the bottom sheet that writes a comment or a reply.
type composer struct {
	isOpen     *core.Signal[bool]
	text       *core.Signal[string]
	parentID   string
	replyTo    *core.Signal[string]
	submitting *core.Signal[bool]
	err        *core.Signal[string]
}

func newComposer(cx *core.Scope) *composer {
	return &composer{
		isOpen: core.NewSignal(cx, false), text: core.NewSignal(cx, ""), replyTo: core.NewSignal(cx, ""),
		submitting: core.NewSignal(cx, false), err: core.NewSignal(cx, ""),
	}
}

func (d *detail) openComposer(parentID, replyTo string) {
	if !d.a.requireLogin() {
		return
	}
	c := d.composer
	if c.parentID != parentID {
		c.text.Set("")
	}
	c.parentID = parentID
	c.replyTo.Set(replyTo)
	c.err.Set("")
	c.isOpen.Set(true)
}

func (c *composer) sheet(d *detail) core.Node {
	title := func() string {
		if to := c.replyTo.Get(); to != "" {
			return "回复 " + to
		}
		return "评论"
	}
	// The keyboard goes with the sheet, however it closes: a press on the
	// backdrop leaves the field focused otherwise.
	var field core.Ref
	d.cx.Effect(func() {
		if !c.isOpen.Get() {
			field.Blur()
		}
	})
	return d.a.bottomSheet(c.isOpen.Get, func() {
		if !c.submitting.GetUntracked() {
			c.isOpen.Set(false)
		}
	},
		core.Text{ContentFunc: title, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textBody, 500)},
		core.Input{
			Ref: &field, Multiline: true, AutoFocus: true, Placeholder: "说点什么…", Label: "评论",
			Value: c.text.Get, OnChange: func(s string) { c.text.Set(s); c.err.Set("") },
			Disabled: c.submitting.Get, Class: "bg-surface-container-highest text-on-surface",
			Style: func() core.Style {
				s := textStyle(textBody, 400)
				s.Height, s.Padding, s.CornerRadius = core.Pt(120), core.Symmetric(12, 16), 12
				return s
			}(),
		},
		row(core.Style{Gap: 8, AlignItems: core.AlignCenter}, "",
			core.Text{ContentFunc: c.err.Get, MaxLines: 2, Class: "text-error", Style: fill.Merge(textStyle(textCaption, 400))},
			filledButton("发送", func() bool { return !c.submitting.Get() && strings.TrimSpace(c.text.Get()) != "" }, d.submitReply)),
	)
}

func (d *detail) submitReply() {
	c := d.composer
	c.submitting.Set(true)
	draft := backend.ReplyDraft{FeedID: d.id, ParentID: c.parentID, Message: strings.TrimSpace(c.text.GetUntracked())}
	background(d.a, func(ctx context.Context) (backend.Reply, error) { return d.a.backend.CreateReply(ctx, draft) },
		func(reply backend.Reply, err error) {
			c.submitting.Set(false)
			if err != nil {
				c.err.Set("发送失败：" + err.Error())
				return
			}
			c.isOpen.Set(false)
			c.text.Set("")
			switch {
			case draft.ParentID == "":
				items := slices.Clone(d.replies.items.GetUntracked())
				index := 0
				for index < len(items) && items[index].Pinned {
					index++
				}
				d.replies.items.Set(slices.Insert(items, index, reply))
				f := d.feed.GetUntracked()
				f.Stats.Comments++
				d.feed.Set(f)
			default:
				if _, ok := d.replies.find(draft.ParentID); ok {
					d.updateReply(draft.ParentID, func(p *backend.Reply) {
						p.Children, p.ChildrenTotal = append(slices.Clone(p.Children), reply), p.ChildrenTotal+1
					})
				}
				if d.thread.GetUntracked() != nil {
					d.nested.reload()
				} else {
					d.a.notice("回复已发送")
				}
			}
		})
}

// favorites lists the account's folders and whether the post is in each.
type favorites struct {
	d       *detail
	isOpen  *core.Signal[bool]
	folders *pager[backend.Collection]
	busy    bool
}

func (a *App) newFavorites(cx *core.Scope, d *detail) *favorites {
	f := &favorites{d: d, isOpen: core.NewSignal(cx, false)}
	f.folders = newPager(cx, func(ctx context.Context, page int) ([]backend.Collection, bool, error) {
		r, err := a.backend.Collections(ctx, d.id, page)
		return r.Items, r.HasMore, err
	}, func(col backend.Collection) string { return col.ID })
	return f
}

func (f *favorites) open() {
	if !f.d.a.requireLogin() {
		return
	}
	f.isOpen.Set(true)
	if f.folders.started && !f.busy {
		f.folders.reset()
	}
}

func (f *favorites) sheet() core.Node {
	return f.d.a.bottomSheet(f.isOpen.Get, func() { f.isOpen.Set(false) },
		core.Show{When: f.isOpen.Get, Then: func(cx *core.Scope) core.Node {
			return column(core.Style{Height: core.Pt(360)}, "",
				label("收藏到", textBody, 500, "text-on-surface"),
				pagerList(cx, f.folders, listSpec{empty: "暂无收藏夹", emptyHint: "在酷安中创建收藏夹后可在这里选择。"},
					func(cx *core.Scope, get func() backend.Collection, r core.ListRow) core.Node { return f.folderRow(get) }))
		}})
}

func (f *favorites) folderRow(get func() backend.Collection) core.Node {
	return tap("收藏夹", func() {
		col := get()
		if !f.busy {
			f.toggle(col.ID, !col.Contains)
		}
	}, core.Style{Direction: core.Row, Height: core.Pt(64), Padding: core.Symmetric(0, 8), Gap: 16, AlignItems: core.AlignCenter}, "",
		icon(&symbols.Folder, 24, "text-on-surface-variant"),
		column(fill.Merge(core.Style{Gap: 2}), "",
			core.Text{ContentFunc: func() string { return get().Name }, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textBody, 400)},
			core.Text{ContentFunc: func() string { return formatCount(get().Count) + " 个内容" }, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)}),
		core.Dynamic(func(cx *core.Scope) core.Node {
			if get().Contains {
				return icon(&symbols.Check, 24, "text-primary")
			}
			return core.Group{}
		}),
	)
}

func (f *favorites) toggle(folderID string, add bool) {
	f.busy = true
	d := f.d
	backgroundErr(d.a, func(ctx context.Context) error { return d.a.backend.SetCollection(ctx, folderID, d.id, add) }, func(err error) {
		if err != nil {
			f.busy = false
			d.a.notice("收藏失败：" + err.Error())
			return
		}
		f.folders.update(folderID, func(col backend.Collection) backend.Collection {
			if col.Contains != add {
				col.Contains = add
				if add {
					col.Count++
				} else {
					col.Count--
				}
			}
			return col
		})
		// The post carries the total across collections, including pages the sheet hasn't loaded.
		background(d.a, func(ctx context.Context) (backend.Feed, error) { return d.a.backend.FeedDetail(ctx, d.id) },
			func(latest backend.Feed, err error) {
				f.busy = false
				if err != nil {
					d.a.notice("收藏数量刷新失败：" + err.Error())
					return
				}
				feed := d.feed.GetUntracked()
				feed.Favorited = latest.Favorited
				feed.Stats.Favorites = latest.Stats.Favorites
				d.feed.Set(feed)
			})
	})
}

// withoutPicturePlaceholder drops the "[图片]" a nested reply's text holds
// for its pictures.
func withoutPicturePlaceholder(spans []backend.RichSpan) []backend.RichSpan {
	out := make([]backend.RichSpan, 0, len(spans))
	for _, sp := range spans {
		if sp.Kind == backend.SpanText {
			sp.Text = strings.ReplaceAll(sp.Text, "[图片]", "")
			if sp.Text == "" {
				continue
			}
		}
		out = append(out, sp)
	}
	return out
}

// threadPictures are a nested reply's pictures, small, each opening the
// viewer over all of them.
func (a *App) threadPictures(cx *core.Scope, images []backend.Image) core.Node {
	cells := make([]core.Node, 0, len(images))
	for i, im := range images {
		cells = append(cells, tap("查看图片", func() { a.openImages(images, i) },
			core.Style{Width: core.Pt(72), Height: core.Pt(72), CornerRadius: 8}, "bg-surface-container-highest",
			a.picOf(cx, thumbURL(im, thumbSmall), core.Cover, "", fillAbs)))
	}
	return row(core.Style{Gap: 6, Wrap: core.Wrap}, "", cells...)
}

// replyText is a comment's text, without the "[图片]" the server writes for
// the pictures it shows.
func replyText(r backend.Reply) string {
	if len(r.Images) == 0 {
		return r.Content
	}
	return strings.TrimSpace(strings.ReplaceAll(r.Content, "[图片]", ""))
}

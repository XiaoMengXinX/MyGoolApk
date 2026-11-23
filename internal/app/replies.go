package app

import (
	"context"
	"strings"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// thread pages the nested replies of one comment.
type thread struct {
	page    int
	loading *core.Signal[bool]
	err     *core.Signal[string]
}

func (d *detail) thread(id string) *thread {
	t := d.threads[id]
	if t == nil {
		t = &thread{loading: core.NewSignal(d.cx, false), err: core.NewSignal(d.cx, "")}
		d.threads[id] = t
	}
	return t
}

type replyShape struct {
	images   int
	children bool
}

func (d *detail) replyRow(cx *core.Scope, get func() backend.Reply) core.Node {
	a := d.a
	author := func() backend.Author { return get().Author }
	return row(core.Style{Padding: core.Edges{Top: 12, Right: 16, Bottom: 8, Left: 16}, Gap: 12}, "bg-background",
		tap("作者主页", func() { a.openUser(author()) }, core.Style{FlexShrink: 0, CornerRadius: 18, Overflow: core.Hidden, AlignSelf: core.AlignStart}, "",
			a.avatar(cx, author, 36)),
		column(fill.Merge(core.Style{Gap: 6}), "",
			row(core.Style{Gap: 8, AlignItems: core.AlignCenter}, "",
				authorName(cx, author, textSecondary),
				dynamicShape(cx, func() bool { return get().IsAuthor }, func(cx *core.Scope, owner bool) core.Node {
					if !owner {
						return core.Group{}
					}
					return tonalPill(func() string { return "楼主" })
				})),
			a.richText(cx, func() ([]backend.RichSpan, string) { r := get(); return r.Rich, r.Content },
				richOptions{size: textSecondary, class: "text-on-surface", selectable: true}),
			dynamicShape(cx, func() replyShape {
				r := get()
				return replyShape{len(r.Images), len(r.Children) > 0 || r.ChildrenTotal > 0}
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
				d.replyLike(cx, get),
				compactAction(&symbols.Comment, "text-on-surface-variant", nil, "回复", func() { d.openComposer(get().ID, displayName(get().Author.Name)) }),
			),
		),
	)
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
	kids := []core.Node{icon(glyph, 18, "")}
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

// updateReply changes the reply with id, a comment or a nested reply.
func (d *detail) updateReply(id string, fn func(*backend.Reply)) {
	items := append([]backend.Reply(nil), d.replies.items.GetUntracked()...)
	for i := range items {
		if items[i].ID == id {
			fn(&items[i])
			d.replies.items.Set(items)
			return
		}
		for j := range items[i].Children {
			if items[i].Children[j].ID == id {
				children := append([]backend.Reply(nil), items[i].Children...)
				fn(&children[j])
				items[i].Children = children
				d.replies.items.Set(items)
				return
			}
		}
	}
}

// threadBox shows the nested replies under a comment.
func (d *detail) threadBox(cx *core.Scope, get func() backend.Reply) core.Node {
	a := d.a
	return core.Dynamic(func(cx *core.Scope) core.Node {
		r := get()
		t := d.thread(r.ID)
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
			if len(child.Rich) > 0 {
				spans = append(spans, child.Rich...)
			} else if child.Content != "" {
				spans = append(spans, backend.RichSpan{Kind: backend.SpanText, Text: child.Content})
			}
			kids = append(kids, a.richText(cx, func() ([]backend.RichSpan, string) { return spans, "" },
				richOptions{size: textSecondary, class: "text-on-surface", selectable: true}))
		}
		parentID := r.ID
		kids = append(kids, core.Dynamic(func(cx *core.Scope) core.Node {
			switch {
			case t.loading.Get():
				return spinner()
			case t.err.Get() != "":
				return column(core.Style{Gap: 4}, "",
					core.Text{Content: t.err.Get(), MaxLines: 2, Class: "text-error", Style: textStyle(textCaption, 400)},
					textButton("重试", func() { d.loadThread(parentID) }))
			case r.ChildrenHasMore:
				text := "查看更多回复"
				if r.ChildrenTotal > len(r.Children) {
					text = "查看全部 " + formatCount(r.ChildrenTotal) + " 条回复"
				}
				return tap(text, func() { d.loadThread(parentID) }, core.Style{}, "", label(text, textSecondary, 500, "text-primary"))
			}
			return core.Group{}
		}))
		return column(core.Style{Padding: core.All(12), Gap: 8, CornerRadius: 8}, "bg-surface-container-low", kids...)
	})
}

func (d *detail) loadThread(parentID string) {
	t := d.thread(parentID)
	if t.loading.GetUntracked() {
		return
	}
	t.loading.Set(true)
	t.err.Set("")
	page, gen := t.page+1, d.replies.gen
	background(d.a, func(ctx context.Context) (backend.ReplyPage, error) {
		return d.a.backend.Replies(ctx, backend.ReplyQuery{FeedID: d.id, ParentID: parentID, Page: page})
	}, func(r backend.ReplyPage, err error) {
		if gen != d.replies.gen {
			return
		}
		t.loading.Set(false)
		if err != nil {
			t.err.Set(err.Error())
			return
		}
		t.page = page
		// The first page repeats the preview the comment came with.
		d.updateReply(parentID, func(p *backend.Reply) {
			p.Children = mergeByID(p.Children, r.Replies, func(r backend.Reply) string { return r.ID })
			p.ChildrenHasMore = r.HasMore
		})
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
	return d.a.bottomSheet(c.isOpen.Get, func() {
		if !c.submitting.GetUntracked() {
			c.isOpen.Set(false)
		}
	},
		core.Text{ContentFunc: title, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textBody, 500)},
		core.Input{
			Multiline: true, AutoFocus: true, Placeholder: "说点什么…", Label: "评论",
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
				d.replies.items.Set(append([]backend.Reply{reply}, d.replies.items.GetUntracked()...))
				f := d.feed.GetUntracked()
				f.Stats.Comments++
				d.feed.Set(f)
			default:
				if _, ok := d.replies.find(draft.ParentID); ok {
					d.updateReply(draft.ParentID, func(p *backend.Reply) { p.Children = append(append([]backend.Reply(nil), p.Children...), reply) })
				} else {
					// A reply to a nested reply shows in its thread on refresh.
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
	busy    map[string]bool
}

func (a *App) newFavorites(cx *core.Scope, d *detail) *favorites {
	f := &favorites{d: d, isOpen: core.NewSignal(cx, false), busy: make(map[string]bool)}
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
	f.busy = make(map[string]bool)
	f.isOpen.Set(true)
	if f.folders.started {
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
		if !f.busy[col.ID] {
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
	f.busy[folderID] = true
	d := f.d
	backgroundErr(d.a, func(ctx context.Context) error { return d.a.backend.SetCollection(ctx, folderID, d.id, add) },
		func(err error) {
			delete(f.busy, folderID)
			if err != nil {
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
			any := false
			for _, col := range f.folders.items.GetUntracked() {
				any = any || col.Contains
			}
			feed := d.feed.GetUntracked()
			feed.Favorited = any
			d.feed.Set(feed)
		})
}

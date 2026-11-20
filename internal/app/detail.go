package app

import (
	"context"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// detail is one post page: the post, its comments and its sheets.
type detail struct {
	a       *App
	cx      *core.Scope
	id      string
	feed    *core.Signal[backend.Feed]
	has     *core.Signal[bool] // seeded from a card or loaded
	err     *core.Signal[string]
	loading bool
	gen     int

	sort    *core.Signal[int] // 0 hot, 1 latest
	replies *pager[backend.Reply]
	threads map[string]*thread

	likeBusy      bool
	replyLikeBusy map[string]bool
	composer      *composer
	favorites     *favorites
}

func (a *App) newDetail(cx *core.Scope, id string, seed backend.Feed) *detail {
	d := &detail{
		a: a, cx: cx, id: id,
		feed:          core.NewSignalFunc(cx, seed, nil),
		has:           core.NewSignal(cx, seed.Author.ID != ""),
		err:           core.NewSignal(cx, ""),
		sort:          core.NewSignal(cx, 0),
		threads:       make(map[string]*thread),
		replyLikeBusy: make(map[string]bool),
	}
	d.replies = newPager(cx, d.fetchReplies, func(r backend.Reply) string { return r.ID })
	d.composer = newComposer(cx)
	d.favorites = a.newFavorites(cx, d)
	d.load()
	return d
}

func (d *detail) fetchReplies(ctx context.Context, page int) ([]backend.Reply, bool, error) {
	sort := backend.ReplySort(d.sort.GetUntracked())
	r, err := d.a.backend.Replies(ctx, backend.ReplyQuery{FeedID: d.id, Sort: sort, Page: page})
	if err == nil && page == 1 && len(r.Replies) == 0 && sort == backend.RepliesHot {
		// No comment is hot (yet): the tab shows the newest instead of nothing.
		// The later pages follow in the same order.
		r, err = d.a.backend.Replies(ctx, backend.ReplyQuery{FeedID: d.id, Sort: backend.RepliesLatest, Page: page})
		d.cx.Post(func() { d.sort.Set(int(backend.RepliesLatest)) })
	}
	return r.Replies, r.HasMore, err
}

func (d *detail) setSort(i int) {
	if d.sort.GetUntracked() != i {
		d.sort.Set(i)
		d.threads = make(map[string]*thread)
		d.replies.reset()
	}
}

func (d *detail) load() {
	if d.loading {
		return
	}
	d.loading = true
	d.gen++
	gen := d.gen
	background(d.a, func(ctx context.Context) (backend.Feed, error) { return d.a.backend.FeedDetail(ctx, d.id) },
		func(feed backend.Feed, err error) {
			if gen != d.gen {
				return
			}
			d.loading = false
			if err != nil {
				d.err.Set(err.Error())
				return
			}
			d.feed.Set(feed)
			d.has.Set(true)
			d.err.Set("")
			d.a.recordHistory(feed)
		})
}

func (d *detail) refresh() {
	d.load()
	d.replies.reload()
}

func (a *App) detailPage(cx *core.Scope, id string) core.Node {
	d := a.newDetail(cx, id, a.seeds[id])
	delete(a.seeds, id)
	content := core.Dynamic(func(cx *core.Scope) core.Node {
		switch {
		case !d.has.Get() && d.err.Get() != "":
			return statusBlock("加载失败", d.err.Get(), "重试", func() { d.err.Set(""); d.load() })
		case !d.has.Get():
			return column(fill.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}), "", spinner())
		}
		return column(fill, "",
			core.Dynamic(func(cx *core.Scope) core.Node {
				if !d.has.Get() || d.err.Get() == "" {
					return core.Group{}
				}
				return errorBanner(d.err.Get(), func() { d.err.Set(""); d.load() }, func() { d.err.Set("") })
			}),
			pagerList(cx, d.replies, listSpec{pushed: true,
				empty: "暂无评论", emptyHint: "来说点什么吧。", bottom: 24, skeleton: skeletonCard,
				header:    column(core.Style{}, "", d.postBody(cx), d.commentsHeader()),
				onRefresh: d.refresh,
			}, func(cx *core.Scope, get func() backend.Reply, r core.ListRow) core.Node { return d.replyRow(cx, get) }),
			d.actionBar(cx),
		)
	})
	return a.screen(titled("帖子"), nil, content, d.composer.sheet(d), d.favorites.sheet())
}

func (d *detail) postBody(cx *core.Scope) core.Node {
	a := d.a
	get := func() backend.Feed { return d.feed.Get() }
	return core.Dynamic(func(cx *core.Scope) core.Node {
		f := core.Untracked(cx, get)
		s := shapeOfFeed(f, 0)
		kids := []core.Node{a.feedHeader(cx, get, 40)}
		if s.title {
			kids = append(kids, core.Text{ContentFunc: func() string { return get().Title }, Class: "text-on-surface", Style: func() core.Style {
				st := textStyle(textTitle, 500)
				st.LineHeight = 31
				return st
			}()})
		}
		if f.Kind == backend.PostQuestion && (f.Answers > 0 || f.Followers > 0) {
			kids = append(kids, core.Text{ContentFunc: func() string {
				f := get()
				return joinNonEmpty([]string{countLabel(f.Answers, "个回答"), countLabel(f.Followers, "人关注")}, "  ")
			}, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)})
		}
		switch {
		case len(f.Blocks) > 0:
			kids = append(kids, a.articleBlocks(cx, f))
		case len(f.Rich) > 0 || f.Content != "":
			kids = append(kids, a.feedRich(cx, get, richOptions{size: textBody, class: "text-on-surface", selectable: true}))
		}
		if s.video || s.images > 0 || s.link || s.vote || s.fw {
			kids = append(kids, a.feedMedia(cx, get, s, true))
		}
		if f.Target != nil {
			kids = append(kids, row(core.Style{}, "", a.targetChip(cx, func() *backend.Target { return get().Target })))
		}
		return column(core.Style{Padding: core.Edges{Top: 8, Right: 16, Bottom: 20, Left: 16}, Gap: 16}, "bg-background", kids...)
	})
}

// articleBlocks shows an article's body in order: paragraphs, pictures with
// their captions, link cards. A tap on a picture opens the viewer over all of
// the article's pictures.
func (a *App) articleBlocks(cx *core.Scope, f backend.Feed) core.Node {
	var images []backend.Image
	for _, b := range f.Blocks {
		if b.Kind == backend.BlockImage {
			images = append(images, b.Image)
		}
	}
	shown := 0
	var kids []core.Node
	for _, b := range f.Blocks {
		b := b
		switch b.Kind {
		case backend.BlockText:
			kids = append(kids, a.richText(cx, func() ([]backend.RichSpan, string) { return b.Rich, b.Text }, richOptions{size: textBody, class: "text-on-surface", selectable: true}))
		case backend.BlockImage:
			index := shown
			shown++
			part := []core.Node{a.articleImage(cx, b.Image, func() { a.openImages(images, index) })}
			if b.Caption != "" {
				part = append(part, core.Text{Content: b.Caption, Class: "text-on-surface-variant", Style: func() core.Style {
					s := textStyle(textCaption, 400)
					s.TextAlign = core.TextAlignCenter
					return s
				}()})
			}
			kids = append(kids, column(core.Style{Gap: 6}, "", part...))
		case backend.BlockLink:
			link := b.Link
			kids = append(kids, a.linkCard(cx, func() *backend.Link { return link }, true))
		}
	}
	return column(core.Style{Gap: 16}, "", kids...)
}

// articleImage shows a picture of an article's body, centered: at the
// body's width and its own ratio, however wide it is, and a picture taller
// than 22:9 whole in a frame of that ratio.
func (a *App) articleImage(cx *core.Scope, im backend.Image, open func()) core.Node {
	ratio := max(mediaRatio(im, 4.0/3, 0, 1e6), longImageRatio)
	return row(core.Style{Width: core.Pct(100), AspectRatio: ratio, JustifyContent: core.JustifyCenter}, "",
		a.pictureButton(cx, func() backend.Image { return im }, thumbMedium, open))
}

func (d *detail) commentsHeader() core.Node {
	return column(core.Style{}, "bg-background",
		core.View{Class: "bg-surface-container-low", Style: core.Style{Height: core.Pt(8)}},
		row(core.Style{Height: core.Pt(56), Padding: core.Symmetric(0, 16), Gap: 8, AlignItems: core.AlignCenter}, "",
			label("评论", textBody, 500, "text-on-surface"),
			core.Dynamic(func(cx *core.Scope) core.Node {
				return core.Text{ContentFunc: func() string {
					if n := d.feed.Get().Stats.Comments; n > 0 {
						return formatCount(n)
					}
					return ""
				}, Class: "text-on-surface-variant", Style: textStyle(textSecondary, 400)}
			}),
			spacer,
			filterChip("热门", func() bool { return d.sort.Get() == 0 }, func() { d.setSort(0) }),
			filterChip("最新", func() bool { return d.sort.Get() == 1 }, func() { d.setSort(1) }),
		),
	)
}

// actionBar is the bottom bar of a post: a comment field that opens the
// composer, and like, favorite and share actions.
func (d *detail) actionBar(cx *core.Scope) core.Node {
	a := d.a
	feed := func() backend.Feed { return d.feed.Get() }
	likes := core.NewMemo(cx, func() bool { return feed().Liked })
	marked := core.NewMemo(cx, func() bool { return feed().Favorited })
	return row(core.Style{MinHeight: core.Pt(64), Padding: core.Edges{Right: 4, Left: 16}, Gap: 4, AlignItems: core.AlignCenter, FlexShrink: 0,
		PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}}, "bg-surface-container",
		tap("写评论", func() { d.openComposer("", "") }, fill.Merge(core.Style{Direction: core.Row, Height: core.Pt(40), CornerRadius: 20,
			Padding: core.Symmetric(0, 16), AlignItems: core.AlignCenter, Overflow: core.Hidden}), "bg-surface-container-highest",
			label("说点什么…", textSecondary, 400, "text-on-surface-variant")),
		core.Dynamic(func(cx *core.Scope) core.Node {
			if likes.Get() {
				return barAction(&symbols.Favorite, "text-primary", func() string { return formatCount(feed().Stats.Likes) }, "取消点赞", d.toggleLike)
			}
			return barAction(&symbols.FavoriteOutline, "text-on-surface-variant", func() string { return formatCount(feed().Stats.Likes) }, "点赞", d.toggleLike)
		}),
		core.Dynamic(func(cx *core.Scope) core.Node {
			if marked.Get() {
				return barAction(&symbols.Bookmark, "text-primary", nil, "收藏", d.favorites.open)
			}
			return barAction(&symbols.BookmarkOutline, "text-on-surface-variant", nil, "收藏", d.favorites.open)
		}),
		barAction(&symbols.Share, "text-on-surface-variant", nil, "分享", func() { a.shareFeed(feed()) }),
	)
}

func barAction(glyph *symbol, class string, count func() string, name string, fn func()) core.Node {
	kids := []core.Node{icon(glyph, 24, "")}
	if count != nil {
		kids = append(kids, core.Text{ContentFunc: count, Style: textStyle(textCaption, 400)})
	}
	return tap(name, fn, core.Style{Direction: core.Row, Height: core.Pt(48), MinWidth: core.Pt(48), Padding: core.Symmetric(0, 12), Gap: 4,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, CornerRadius: 24, Overflow: core.Hidden, FlexShrink: 0}, class, kids...)
}

func (d *detail) toggleLike() {
	if d.likeBusy || !d.a.requireLogin() {
		return
	}
	d.likeBusy = true
	liked := !d.feed.GetUntracked().Liked
	background(d.a, func(ctx context.Context) (int, error) { return d.a.backend.SetFeedLike(ctx, d.id, liked) },
		func(likes int, err error) {
			d.likeBusy = false
			if err != nil {
				d.a.notice("点赞失败：" + err.Error())
				return
			}
			f := d.feed.GetUntracked()
			f.Liked, f.Stats.Likes = liked, likes
			d.feed.Set(f)
		})
}

// shareFeed shares the post's link alone, on the coolapk1s.com mirror.
func (a *App) shareFeed(f backend.Feed) {
	a.cx.Share(core.ShareOptions{Text: "https://www.coolapk1s.com/feed/" + f.ID}, nil)
}

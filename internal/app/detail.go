package app

import (
	"context"
	"slices"

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

	sort    *core.Signal[int] // a backend.ReplySort
	replies *pager[backend.Reply]
	// thread is the comment whose replies the thread sheet lists in nested,
	// and threadY how far that list is scrolled.
	thread  *core.Signal[*backend.Reply]
	nested  *pager[backend.Reply]
	threadY core.Points
	// depth is how many pages the stack holds with this one on top.
	depth int

	likeBusy      bool
	voteBusy      bool
	replyLikeBusy map[string]bool
	// menu is the comment whose menu is open.
	menu      *core.Signal[*backend.Reply]
	composer  *composer
	favorites *favorites
}

func (a *App) newDetail(cx *core.Scope, id string, seed backend.Feed) *detail {
	d := &detail{
		a: a, cx: cx, id: id, depth: a.nav.Depth(),
		feed:          core.NewSignalFunc(cx, seed, nil),
		has:           core.NewSignal(cx, seed.Author.ID != ""),
		err:           core.NewSignal(cx, ""),
		sort:          core.NewSignal(cx, 0),
		thread:        core.NewSignalFunc[*backend.Reply](cx, nil, nil),
		replyLikeBusy: make(map[string]bool),
		menu:          core.NewSignalFunc[*backend.Reply](cx, nil, nil),
	}
	replyID := func(r backend.Reply) string { return r.ID }
	d.replies = newQueryPager(cx, func() backend.ReplyQuery {
		return backend.ReplyQuery{FeedID: d.id, Sort: backend.ReplySort(d.sort.GetUntracked()), Of: repliesOf(d.feed.GetUntracked())}
	}, d.fetchReplies, replyID)
	d.nested = newQueryPager(cx, func() string {
		if p := d.thread.GetUntracked(); p != nil {
			return p.ID
		}
		return ""
	}, d.fetchNested, replyID)
	d.composer = newComposer(cx)
	d.favorites = a.newFavorites(cx, d)
	cx.Effect(d.pinFirst)
	d.load()
	return d
}

func (d *detail) fetchReplies(ctx context.Context, q backend.ReplyQuery, page int) ([]backend.Reply, bool, error) {
	q.Page = page
	r, err := d.a.backend.Replies(ctx, q)
	return r.Replies, r.HasMore, err
}

// pinFirst leads the comments in the default order with the pinned ones the
// post tells, whichever of the two came first.
func (d *detail) pinFirst() {
	pinned, items := d.feed.Get().Pinned, d.replies.items.Get()
	if len(pinned) == 0 || !d.replies.loaded.Get() || d.sort.Get() != int(backend.RepliesRecent) {
		return
	}
	led := len(items) >= len(pinned)
	for i := 0; led && i < len(pinned); i++ {
		led = items[i].ID == pinned[i].ID && items[i].Pinned
	}
	if led {
		return
	}
	rest := slices.DeleteFunc(slices.Clone(items), func(r backend.Reply) bool {
		return slices.ContainsFunc(pinned, func(p backend.Reply) bool { return p.ID == r.ID })
	})
	d.replies.items.Set(append(slices.Clone(pinned), rest...))
}

func (d *detail) setSort(i int) {
	if d.sort.GetUntracked() != i {
		d.sort.Set(i)
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
			// A vote lists its opinions and a question its answers, other
			// lists than the one asked for before the post told what it is.
			of := repliesOf(d.feed.GetUntracked())
			d.feed.Set(feed)
			if of != repliesOf(feed) {
				d.replies.reset()
			}
			d.has.Set(true)
			d.err.Set("")
			d.a.recordHistory(feed)
		})
}

// repliesOf is what f lists under it.
func repliesOf(f backend.Feed) backend.ReplyOf {
	switch {
	case f.Vote != nil:
		return backend.RepliesOfVote
	case f.Kind == backend.PostQuestion:
		return backend.RepliesOfQuestion
	}
	return backend.RepliesOfPost
}

// castVote votes for options of the post's vote, and loads the post again,
// which then tells the counts.
func (d *detail) castVote(ids []string) {
	if d.voteBusy || !d.a.requireLogin() {
		return
	}
	d.voteBusy = true
	backgroundErr(d.a, func(ctx context.Context) error { return d.a.backend.CastVote(ctx, d.id, ids) },
		func(err error) {
			d.voteBusy = false
			if err != nil {
				d.a.notice("投票失败：" + err.Error())
				return
			}
			d.load()
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
	return a.screen(titled("帖子"), nil, content, d.threadSheet(cx), d.composer.sheet(d), d.favorites.sheet(), d.replyMenu())
}

func (d *detail) postBody(cx *core.Scope) core.Node {
	a := d.a
	get := func() backend.Feed { return d.feed.Get() }
	type shape struct {
		feedShape
		blocks, rich          int
		target, counted, text bool
		voted                 bool
	}
	return dynamicShape(cx, func() shape {
		f := get()
		return shape{shapeOfFeed(f, 0), len(f.Blocks), len(f.Rich), f.Target != nil, f.Answers > 0 || f.Followers > 0, f.Content != "", f.Vote != nil && f.Vote.Counted}
	}, func(cx *core.Scope, _ shape) core.Node {
		f := core.Untracked(cx, get)
		s := shapeOfFeed(f, 0)
		kids := []core.Node{a.feedHeader(cx, get, 40)}
		// The cover leads an article edge to edge, over its title.
		if c := f.Cover; c != nil && !slices.ContainsFunc(f.Blocks, func(b backend.Block) bool { return b.Kind == backend.BlockImage && b.Image.URL == c.URL }) {
			cover := *c
			kids = append(kids, tap("查看封面", func() { a.openImages([]backend.Image{cover}, 0) },
				core.Style{Margin: core.Symmetric(0, -16), AspectRatio: mediaRatio(cover, 16.0/9, 1, 3)}, "bg-surface-container-highest",
				a.pic(cx, func() string { return thumbURL(cover, thumbMedium) }, core.Cover, "", fillAbs)))
		}
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
		case f.Rating != nil:
			kids = append(kids, a.ratingDetail(cx, f))
		case len(f.Blocks) > 0:
			kids = append(kids, a.articleBlocks(cx, f))
		case len(f.Rich) > 0 || f.Content != "":
			kids = append(kids, a.feedRich(cx, get, richOptions{size: textBody, lineHeight: feedLineHeight, class: "text-on-surface", selectable: true}))
		}
		if s.video || s.images > 0 || s.link || s.vote || s.fw {
			kids = append(kids, a.feedMedia(cx, get, s, true, d.castVote))
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
			kids = append(kids, a.richText(cx, func() ([]backend.RichSpan, string) { return b.Rich, b.Text }, richOptions{size: textBody, lineHeight: feedLineHeight, class: "text-on-surface", selectable: true}))
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
			kids = append(kids, a.linkCard(cx, func() *backend.Link { return link }, true, nil))
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
			core.Text{ContentFunc: func() string {
				switch repliesOf(d.feed.Get()) {
				case backend.RepliesOfVote:
					return "观点"
				case backend.RepliesOfQuestion:
					return "回答"
				}
				return "评论"
			}, Class: "text-on-surface", Style: textStyle(textBody, 500)},
			core.Dynamic(func(cx *core.Scope) core.Node {
				return core.Text{ContentFunc: func() string {
					f := d.feed.Get()
					n := f.Stats.Comments
					if f.Kind == backend.PostQuestion {
						n = f.Answers
					}
					if n > 0 {
						return formatCount(n)
					}
					return ""
				}, Class: "text-on-surface-variant", Style: textStyle(textSecondary, 400)}
			}),
			spacer,
			dynamicShape(d.cx, func() backend.ReplyOf { return repliesOf(d.feed.Get()) }, func(cx *core.Scope, of backend.ReplyOf) core.Node {
				if of == backend.RepliesOfVote {
					return core.Group{}
				}
				chips := []core.Node{
					filterChip("默认", func() bool { return d.sort.Get() == 0 }, func() { d.setSort(0) }),
					filterChip("最新", func() bool { return d.sort.Get() == 1 }, func() { d.setSort(1) }),
					filterChip("热门", func() bool { return d.sort.Get() == 2 }, func() { d.setSort(2) }),
				}
				if of == backend.RepliesOfPost {
					chips = append(chips, filterChip("楼主", func() bool { return d.sort.Get() == 3 }, func() { d.setSort(3) }))
				}
				return row(core.Style{Gap: 8}, "", chips...)
			}),
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
	// The bar's 64dp stand above the system's bar, whose room is the padding
	// of the view around it.
	return column(core.Style{FlexShrink: 0, PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}}, "bg-surface-container", row(
		core.Style{Height: core.Pt(64), Padding: core.Edges{Right: 4, Left: 16}, Gap: 4, AlignItems: core.AlignCenter}, "",
		tap("写评论", func() { d.openComposer("", "") }, fill.Merge(core.Style{Direction: core.Row, Height: core.Pt(48), CornerRadius: 24,
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
	))
}

func barAction(glyph *symbol, class string, count func() string, name string, fn func()) core.Node {
	kids := []core.Node{icon(glyph, 24, class)}
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
			d.a.feedChanged(f)
		})
}

// shareFeed shares the post's link alone, on the coolapk1s.com mirror.
func (a *App) shareFeed(f backend.Feed) {
	a.cx.Share(core.ShareOptions{Text: "https://www.coolapk1s.com/feed/" + f.ID}, nil)
}

// covered reports whether another page is over the post's page. The sheets
// are over the whole app, so they hide meanwhile.
func (d *detail) covered() bool { return d.a.nav.Depth() > d.depth }

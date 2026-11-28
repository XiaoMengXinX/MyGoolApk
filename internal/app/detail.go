package app

import (
	"context"
	"slices"
	"strconv"
	"unicode/utf16"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// detail is one post page: the post, its comments and its sheets.
type detail struct {
	a              *App
	cx             *core.Scope
	id             string
	feed           *core.Signal[backend.Feed]
	has            *core.Signal[bool] // seeded from a card or loaded
	err            *core.Signal[string]
	loading        bool
	refreshReplies bool
	documents      *core.Signal[*postDocuments]
	markdown       *core.Signal[bool]

	sort     *core.Signal[int] // a backend.ReplySort
	replies  *pager[backend.Reply]
	comments core.Ref

	// summary holds the AI summary as far as it has streamed in; summaryError
	// hides the card when an automatic summary is unavailable. summaryAction
	// picks whether the card starts automatically, starts on tap inline, or
	// opens the Markdown sheet for Coolapk's raw first-turn summary.
	summary          *core.Signal[string]
	summaryModel     *core.Signal[string]
	summaryError     *core.Signal[bool]
	summaryErrText   *core.Signal[string]
	summaryAction    *core.Signal[summaryAction]
	summarySheetOpen *core.Signal[bool]
	summaryLoading   bool
	asked            *core.Signal[bool]

	// thread is the comment whose replies the thread sheet lists in nested,
	// and threadY how far that list is scrolled.
	thread  *core.Signal[*backend.Reply]
	nested  *pager[backend.Reply]
	threadY core.Points
	// depth is how many pages the stack holds with this one on top.
	depth int

	likeBusy      bool
	followBusy    bool
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
		feed:             core.NewSignalFunc(cx, seed, nil),
		has:              core.NewSignal(cx, seed.Author.ID != "" && seed.Kind != backend.PostArticle),
		err:              core.NewSignal(cx, ""),
		sort:             core.NewSignal(cx, 0),
		thread:           core.NewSignalFunc[*backend.Reply](cx, nil, nil),
		replyLikeBusy:    make(map[string]bool),
		menu:             core.NewSignalFunc[*backend.Reply](cx, nil, nil),
		summary:          core.NewSignal(cx, ""),
		summaryModel:     core.NewSignal(cx, ""),
		summaryError:     core.NewSignal(cx, false),
		summaryErrText:   core.NewSignal(cx, ""),
		summaryAction:    core.NewSignal(cx, summaryNone),
		summarySheetOpen: core.NewSignal(cx, false),
		asked:            core.NewSignal(cx, false),
		documents:        core.NewSignal(cx, new(postDocuments)),
		markdown:         core.NewSignal(cx, true),
	}
	replyID := func(r backend.Reply) string { return r.ID }
	d.replies = newQueryPager(cx, func() detailReplyQuery {
		feed := d.feed.GetUntracked()
		sort := backend.ReplySort(d.sort.GetUntracked())
		query := detailReplyQuery{ReplyQuery: backend.ReplyQuery{FeedID: d.id, Sort: sort, Of: repliesOf(feed)}}
		if sort == backend.RepliesRecent {
			query.pinned = feed.Pinned
		}
		return query
	}, d.fetchReplies, replyID)
	d.nested = newQueryPager(cx, func() string {
		if p := d.thread.GetUntracked(); p != nil {
			return p.ID
		}
		return ""
	}, d.fetchNested, replyID)
	d.composer = newComposer(cx)
	d.favorites = a.newFavorites(cx, d)
	d.load()
	return d
}

// summaryTextLength is the post's visible plain text in UTF-16 units,
// counted as coolapk1s.com does for its summaries (its getSummaryInput):
// an article counts its text blocks, other posts their content, both
// already stripped of markup when the backend parsed them.
func summaryTextLength(f backend.Feed) int {
	n := 0
	if f.Kind == backend.PostArticle && len(f.Blocks) > 0 {
		for _, block := range f.Blocks {
			if block.Kind == backend.BlockText {
				n += len(utf16.Encode([]rune(block.Text)))
			}
		}
		return n
	}
	return len(utf16.Encode([]rune(f.Content)))
}

type summaryAction int

const (
	summaryNone summaryAction = iota
	summaryAutoCoolapk1s
	summaryAutoOfficialConcise
	summaryTapOfficialConcise
	summaryTapOfficialRaw
)

// summaryActionFor decides how a post's AI summary should be presented and
// fetched from the current login state, summary settings, and post metadata.
func (a *App) summaryActionFor(feed backend.Feed) summaryAction {
	if !a.aiSummaryEnabled.GetUntracked() {
		return summaryNone
	}
	loggedIn := a.backend.SessionUID() != ""
	useOfficial := loggedIn && a.officialAISummary.GetUntracked()
	rawMode := useOfficial && a.officialAIRawMode.GetUntracked()
	hasAssistant := feed.ShowAIAssistant && feed.AIAccessKey != ""
	longEnough := summaryTextLength(feed) >= 800

	if rawMode {
		if hasAssistant {
			return summaryTapOfficialRaw
		}
		return summaryNone
	}
	if useOfficial {
		switch {
		case hasAssistant && longEnough:
			return summaryAutoOfficialConcise
		case !hasAssistant && longEnough:
			return summaryAutoCoolapk1s
		case hasAssistant && !longEnough:
			return summaryTapOfficialConcise
		default:
			return summaryNone
		}
	}
	if longEnough {
		return summaryAutoCoolapk1s
	}
	return summaryNone
}

// askSummary configures the summary card once the post's detail arrives, and
// starts the automatic request when the active mode calls for one.
func (d *detail) askSummary(feed backend.Feed) {
	if d.asked.GetUntracked() || d.summaryAction.GetUntracked() != summaryNone {
		return
	}
	action := d.a.summaryActionFor(feed)
	d.summaryAction.Set(action)
	switch action {
	case summaryAutoCoolapk1s:
		d.startSummary(feed, backend.SummaryCoolapk1s, true)
	case summaryAutoOfficialConcise:
		d.startSummary(feed, backend.SummaryOfficialConcise, true)
	}
}

func (d *detail) tapSummaryCard() {
	feed := d.feed.GetUntracked()
	switch d.summaryAction.GetUntracked() {
	case summaryTapOfficialConcise:
		if d.asked.GetUntracked() || d.summaryLoading {
			return
		}
		d.startSummary(feed, backend.SummaryOfficialConcise, false)
	case summaryTapOfficialRaw:
		d.summarySheetOpen.Set(true)
		if !d.asked.GetUntracked() && !d.summaryLoading {
			d.startSummary(feed, backend.SummaryOfficialRaw, false)
		}
	}
}

func (d *detail) retryRawSummary() {
	if d.summaryLoading {
		return
	}
	d.summary.Set("")
	d.summaryErrText.Set("")
	d.asked.Set(false)
	d.startSummary(d.feed.GetUntracked(), backend.SummaryOfficialRaw, false)
}

func (d *detail) startSummary(feed backend.Feed, mode backend.SummaryMode, auto bool) {
	d.asked.Set(true)
	d.summaryLoading = true
	d.summaryError.Set(false)
	d.summaryErrText.Set("")
	req := backend.SummaryRequest{
		FeedID:      d.id,
		Mode:        mode,
		HasTitle:    feed.Title != "",
		AIAccessKey: feed.AIAccessKey,
	}
	ctx := d.cx.Context()
	go func() {
		s, err := d.a.backend.FeedSummary(ctx, req, func(piece string) {
			d.cx.Post(func() { d.summary.Set(d.summary.GetUntracked() + piece) })
		})
		d.cx.Post(func() {
			d.summaryLoading = false
			if err == nil {
				d.summary.Set(s.Text)
				d.summaryModel.Set(s.Model)
				d.summaryErrText.Set("")
				d.summaryError.Set(d.summary.GetUntracked() == "")
				return
			}
			if d.summary.GetUntracked() != "" {
				return
			}
			if auto {
				d.summaryError.Set(true)
				return
			}
			d.asked.Set(false)
			if mode == backend.SummaryOfficialRaw {
				d.summaryErrText.Set(err.Error())
				return
			}
			d.a.notice("获取 AI 总结失败：" + err.Error())
		})
	}()
}

type detailReplyQuery struct {
	backend.ReplyQuery
	pinned []backend.Reply
}

func (d *detail) fetchReplies(ctx context.Context, query detailReplyQuery, page int) ([]backend.Reply, bool, error) {
	query.Page = page
	result, err := d.a.backend.Replies(ctx, query.ReplyQuery)
	if page == 1 && len(query.pinned) > 0 {
		result.Replies = slices.Concat(query.pinned, result.Replies)
	}
	return result.Replies, result.HasMore, err
}

func (d *detail) setSort(i int) {
	if d.sort.GetUntracked() != i {
		d.sort.Set(i)
		if d.replies.started {
			d.replies.reset()
		}
	}
}

func (d *detail) load() {
	if d.loading {
		return
	}
	d.loading = true
	var ctx context.Context
	d.cx.Run(func() { ctx = d.cx.Context() })
	go func() {
		feed, err := d.a.backend.FeedDetail(ctx, d.id)
		var docs postDocuments
		if err == nil {
			docs = preparePostDocuments(feed)
		}
		d.cx.Post(func() {
			d.loading = false
			refreshReplies := d.refreshReplies
			d.refreshReplies = false
			of := repliesOf(d.feed.GetUntracked())
			if err != nil {
				d.err.Set(err.Error())
			} else {
				d.feed.Set(feed)
				d.documents.Set(&docs)
				d.has.Set(true)
				d.err.Set("")
				d.a.recordHistory(feed)
				d.askSummary(feed)
			}
			switch {
			case !d.has.GetUntracked():
			case !d.replies.started:
				d.replies.ensure()
			case err == nil && of != repliesOf(feed):
				d.replies.reset()
			case refreshReplies:
				d.replies.restart()
			}
		})
	}()
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
	d.refreshReplies = true
	d.load()
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
				onRefresh: d.refresh, manualStart: true,
			}, func(cx *core.Scope, get func() backend.Reply, r core.ListRow) core.Node { return d.replyRow(cx, get) }),
			d.actionBar(cx),
		)
	})
	markdown := core.Show{
		When: func() bool { return d.documents.Get().hasMarkdown },
		Then: func(*core.Scope) core.Node {
			button := compactChip("Markdown", d.markdown.Get, func() { d.markdown.Set(!d.markdown.GetUntracked()) })
			button.Style.CornerRadius = 16
			button.Style.FlexShrink = 0
			button.HitSlop = core.Symmetric(8, 0)
			return button
		},
	}
	return a.screen(titled("帖子"), markdown, content, d.threadSheet(cx), d.summarySheet(cx), d.composer.sheet(d), d.favorites.sheet(), d.replyMenu())
}

func (d *detail) postBody(cx *core.Scope) core.Node {
	a := d.a
	get := func() backend.Feed { return d.feed.Get() }
	// Each part is built again only as its own shape changes: the full post
	// that replaces the list's copy rebuilds what it changes, and pictures
	// built again show their placeholder for a frame.
	type coverShape struct {
		url   string
		ratio float32
	}
	cover := dynamicShape(cx, func() coverShape {
		f := get()
		c := f.Cover
		if c == nil || slices.ContainsFunc(f.Blocks, func(b backend.Block) bool { return b.Kind == backend.BlockImage && b.Image.URL == c.URL }) {
			return coverShape{}
		}
		if f.Kind == backend.PostArticle {
			return coverShape{c.URL, articleCoverRatio}
		}
		return coverShape{c.URL, mediaRatio(*c, 16.0/9, 1, 3)}
	}, func(cx *core.Scope, cs coverShape) core.Node {
		if cs.url == "" {
			return core.Group{}
		}
		cover := *core.Untracked(cx, get).Cover
		return tap("查看封面", func() { a.openImages([]backend.Image{cover}, 0) },
			core.Style{Margin: core.Symmetric(0, -16), AspectRatio: cs.ratio}, "bg-surface-container-highest",
			a.pic(cx, func() string { return thumbURL(cover, thumbMedium) }, core.Cover, "", fillAbs))
	})
	title := core.Show{
		When: func() bool { return get().Title != "" },
		Then: func(*core.Scope) core.Node {
			st := textStyle(textTitle, 500)
			st.LineHeight = 31
			return core.Text{ContentFunc: func() string { return get().Title }, Class: "text-on-surface", Style: st}
		},
	}
	question := core.Show{
		When: func() bool { f := get(); return f.Kind == backend.PostQuestion && (f.Answers > 0 || f.Followers > 0) },
		Then: func(*core.Scope) core.Node {
			return core.Text{ContentFunc: func() string {
				f := get()
				return joinNonEmpty([]string{countLabel(f.Answers, "个回答"), countLabel(f.Followers, "人关注")}, "  ")
			}, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)}
		},
	}
	type textShape struct {
		rating       bool
		blocks, rich int
		text         bool
		documents    *postDocuments
	}
	text := dynamicShape(cx, func() textShape {
		f := get()
		return textShape{f.Rating != nil, len(f.Blocks), len(f.Rich), f.Content != "" || f.Markup != "", d.documents.Get()}
	}, func(cx *core.Scope, ts textShape) core.Node {
		f := core.Untracked(cx, get)
		switch {
		case ts.rating:
			return d.ratingDetail(cx, f, ts.documents.parts)
		case ts.blocks > 0:
			return d.articleBlocks(cx, f, ts.documents.parts)
		case ts.rich > 0 || ts.text:
			return a.postDocument(cx, ts.documents.body, d.markdown.Get,
				func() ([]backend.RichSpan, string) { f := get(); return f.Rich, f.Content })
		}
		return core.Group{}
	})
	type mediaShape struct {
		feedShape
		voted bool
	}
	media := dynamicShape(cx, func() mediaShape {
		f := get()
		s := shapeOfFeed(f, 0)
		if s.kind == backend.PostArticle {
			s.images = 0 // Article body images are rendered by articleBlocks.
		}
		return mediaShape{feedShape{kind: s.kind, images: s.images, video: s.video, link: s.link, vote: s.vote, fw: s.fw, bigCover: s.bigCover}, f.Vote != nil && f.Vote.Counted}
	}, func(cx *core.Scope, ms mediaShape) core.Node {
		s := ms.feedShape
		if !s.video && s.images == 0 && !s.link && !s.vote && !s.fw {
			return core.Group{}
		}
		return a.feedMedia(cx, get, s, true, d.castVote)
	})
	target := core.Show{
		When: func() bool { f := get(); return f.Target != nil && f.Rating == nil },
		Then: func(cx *core.Scope) core.Node {
			return row(core.Style{}, "", a.targetChip(cx, func() *backend.Target { return get().Target }))
		},
	}
	summary := core.Show{When: func() bool { return d.summaryAction.Get() != summaryNone && !d.summaryError.Get() }, Then: d.summaryCard}
	return column(core.Style{Padding: core.Edges{Top: 8, Right: 16, Bottom: 20, Left: 16}, Gap: 16}, "bg-background",
		d.authorHeader(cx), cover, title, question,
		dynamicShape(cx, func() bool { return get().BigCover }, func(_ *core.Scope, big bool) core.Node {
			if big {
				return core.Group{media, summary, text}
			}
			return core.Group{summary, text, media}
		}), target)
}

func (d *detail) authorHeader(cx *core.Scope) core.Node {
	type followState struct{ visible, followed bool }
	return row(core.Style{Gap: 12, AlignItems: core.AlignCenter}, "",
		core.View{Style: fill, Children: []core.Node{d.a.feedHeader(cx, d.feed.Get, 40)}},
		dynamicShape(cx, func() followState {
			f := d.feed.Get()
			uid := d.a.account.uid.Get()
			return followState{f.Author.ID != "" && f.Author.ID != uid, f.AuthorFollowed}
		}, func(_ *core.Scope, state followState) core.Node {
			if !state.visible {
				return core.Group{}
			}
			action, text, border, foreground := "关注", "关注", "border-primary", "text-primary"
			if state.followed {
				action, text, border, foreground = "取消关注", "已关注", "border-outline-variant", "text-on-surface-variant"
			}
			button := tap(action, d.toggleAuthorFollow, core.Style{Height: core.Pt(32), Padding: core.Symmetric(0, 16),
				CornerRadius: 16, BorderWidth: 1, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				border, label(text, textSecondary, 500, foreground))
			button.HitSlop = core.Symmetric(8, 0)
			return button
		}))
}

func (d *detail) toggleAuthorFollow() {
	if d.followBusy || !d.a.requireLogin() {
		return
	}
	f := d.feed.GetUntracked()
	follow := !f.AuthorFollowed
	d.followBusy = true
	backgroundErr(d.a, func(ctx context.Context) error { return d.a.backend.SetUserFollow(ctx, f.Author.ID, follow) }, func(err error) {
		d.followBusy = false
		if err != nil {
			d.a.notice("关注失败：" + err.Error())
			return
		}
		f := d.feed.GetUntracked()
		f.AuthorFollowed = follow
		d.feed.Set(f)
		d.a.feedChanged(f)
	})
}

func (d *detail) summaryCard(*core.Scope) core.Node {
	bodyStyle := textStyle(textSecondary, 400)
	bodyStyle.LineHeight = 22
	bodyText := func() string {
		switch d.summaryAction.Get() {
		case summaryTapOfficialRaw:
			if d.summary.Get() != "" {
				return "点击查看AI总结"
			}
			return "点击获取AI总结"
		case summaryTapOfficialConcise:
			if !d.asked.Get() {
				return "点击获取AI总结"
			}
		}
		if text := d.summary.Get(); text != "" {
			return text
		}
		return "生成中…"
	}
	bodyClass := func() string {
		switch d.summaryAction.Get() {
		case summaryTapOfficialRaw:
			return "text-primary"
		case summaryTapOfficialConcise:
			if !d.asked.Get() {
				return "text-primary"
			}
		}
		if d.summary.Get() == "" {
			return "text-on-surface-variant"
		}
		return "text-on-surface"
	}
	content := []core.Node{
		row(core.Style{Gap: 6, AlignItems: core.AlignCenter}, "",
			icon(&symbols.AutoAwesome, 16, "text-primary"),
			label("AI 总结", textSecondary, 500, "text-primary"),
			spacer,
			core.Text{ContentFunc: d.summaryModel.Get, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)},
		),
		core.Text{
			ContentFunc: bodyText,
			ClassFunc:   bodyClass,
			Style:       bodyStyle,
		},
		label("摘要由 AI 生成，仅供参考", textCaption, 400, "text-on-surface-variant"),
	}
	style := core.Style{Padding: core.Symmetric(12, 16), Gap: 6, CornerRadius: 16, Overflow: core.Hidden}
	action := d.summaryAction.GetUntracked()
	if action == summaryTapOfficialConcise || action == summaryTapOfficialRaw {
		return tap("获取 AI 总结", d.tapSummaryCard, style, "bg-surface-container-high", content...)
	}
	return column(style, "bg-surface-container-high", content...)
}

// summarySheet streams Coolapk's first-turn AI assistant summary as Markdown
// in a modal sheet sized like the nested-replies sheet.
func (d *detail) summarySheet(cx *core.Scope) core.Node {
	a := d.a
	type summarySheetShape struct {
		hasText bool
		errText string
	}
	return a.sheet(func() bool { return d.summarySheetOpen.Get() && !d.covered() }, func() {
		if !d.covered() {
			d.summarySheetOpen.Set(false)
		}
	}, "bg-background", 0,
		row(core.Style{Padding: core.Symmetric(0, 16), Gap: 6, AlignItems: core.AlignCenter}, "",
			icon(&symbols.AutoAwesome, 18, "text-primary"),
			label("AI 总结", textBody, 500, "text-on-surface"),
			spacer,
			core.Text{ContentFunc: d.summaryModel.Get, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)},
		),
		core.View{
			StyleFunc: func() core.Style {
				w := a.cx.Env().Window()
				width := w.Width
				if a.wide() {
					width = min(dialogWidth, w.Width*0.9)
				}
				return core.Style{Width: core.Pt(width), Height: core.Pt(w.Height * 0.7)}
			},
			Children: []core.Node{
				dynamicShape(cx, func() summarySheetShape {
					return summarySheetShape{hasText: d.summary.Get() != "", errText: d.summaryErrText.Get()}
				}, func(cx *core.Scope, s summarySheetShape) core.Node {
					if !s.hasText {
						if s.errText != "" {
							return column(fill.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}), "",
								statusBlock("获取失败", s.errText, "重试", d.retryRawSummary))
						}
						return column(fill.Merge(core.Style{Gap: 12, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}), "",
							spinner(), label("生成中…", textSecondary, 400, "text-on-surface-variant"))
					}
					return core.Scroll{Style: fill, Children: []core.Node{
						column(core.Style{Padding: core.Edges{Top: 4, Right: 16, Bottom: 20, Left: 16}, Gap: 12}, "",
							a.markdownView(cx, d.summary.Get),
							label("摘要由 AI 生成，仅供参考", textCaption, 400, "text-on-surface-variant"),
						),
					}}
				}),
			},
		})
}

// articleBlocks shows an article's body in order: paragraphs, pictures with
// their captions, link cards. A tap on a picture opens the viewer over all of
// the article's pictures.
func (d *detail) articleBlocks(cx *core.Scope, f backend.Feed, docs map[int]postDocument) core.Node {
	a := d.a
	var images []backend.Image
	for _, b := range f.Blocks {
		if b.Kind == backend.BlockImage {
			images = append(images, b.Image)
		}
	}
	shown := 0
	var kids []core.Node
	for i, b := range f.Blocks {
		switch b.Kind {
		case backend.BlockText:
			kids = append(kids, a.postDocument(cx, docs[i], d.markdown.Get,
				func() ([]backend.RichSpan, string) { return b.Rich, b.Text }))
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
	return core.View{Ref: &d.comments, Class: "bg-background", Children: []core.Node{
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
				// The values are backend.ReplySort's; only a post's replies can be its author's.
				sorts := []option{{"默认", "0"}, {"最新", "1"}, {"热门", "2"}}
				if of == backend.RepliesOfPost {
					sorts = append(sorts, option{"楼主", "3"})
				}
				return optionMenu(cx, "评论排序", core.AlignEnd, sorts, func() string { return strconv.Itoa(d.sort.Get()) }, func(v string) {
					i, _ := strconv.Atoi(v)
					d.setSort(i)
				})
			}),
		),
	}}
}

// actionBar is the bottom bar of a post: a comment field that opens the
// composer beside the like, comment, favorite and share actions.
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
			label("写评论", textSecondary, 400, "text-on-surface-variant")),
		core.Dynamic(func(cx *core.Scope) core.Node {
			if likes.Get() {
				return barAction(&symbols.Favorite, "text-primary", func() int { return feed().Stats.Likes }, "取消点赞", d.toggleLike)
			}
			return barAction(&symbols.FavoriteOutline, "text-on-surface-variant", func() int { return feed().Stats.Likes }, "点赞", d.toggleLike)
		}),
		barAction(&symbols.Comment, "text-on-surface-variant", func() int {
			f := feed()
			if f.Kind == backend.PostQuestion {
				return f.Answers
			}
			return f.Stats.Comments
		}, "跳到评论区", func() { d.comments.Reveal(core.RevealOptions{Animated: true}) }),
		core.Dynamic(func(cx *core.Scope) core.Node {
			if marked.Get() {
				return barAction(&symbols.Bookmark, "text-primary", func() int { return feed().Stats.Favorites }, "收藏", d.favorites.open)
			}
			return barAction(&symbols.BookmarkOutline, "text-on-surface-variant", func() int { return feed().Stats.Favorites }, "收藏", d.favorites.open)
		}),
		barAction(&symbols.Share, "text-on-surface-variant", func() int { return feed().Stats.Forwards }, "转发", func() { a.shareFeed(feed()) }),
	))
}

func barAction(glyph *symbol, class string, count func() int, name string, fn func()) core.Node {
	return tap(name, fn, core.Style{Height: core.Pt(52), MinWidth: core.Pt(44), Padding: core.Symmetric(0, 4), Gap: 2,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, CornerRadius: 12, FlexShrink: 0}, class,
		icon(glyph, 22, class), labelFunc(func() string { return formatCount(count()) }, textCaption, 400, class))
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

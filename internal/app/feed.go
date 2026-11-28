package app

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// feedShape is what decides the structure of a card's body; while it stays
// the same a list row only rebinds the card's text and pictures.
type feedShape struct {
	kind                         backend.PostKind
	images                       int
	cover, video, link, vote, fw bool
	title, text, more            bool
	bigCover                     bool
}

func shapeOfFeed(f backend.Feed, lines int) feedShape {
	return feedShape{
		kind: f.Kind, images: len(f.Images), cover: f.Cover != nil, video: f.Video != nil,
		bigCover: f.BigCover,
		link:     f.Link != nil, vote: f.Vote != nil, fw: f.Forward != nil,
		title: f.Title != "", text: strings.TrimSpace(f.Content) != "" || len(f.Rich) > 0, more: feedTextNeedsMore(f.Content, lines),
	}
}

// dynamicOnShape builds body again when the shape of the post changes.
func dynamicOnShape(cx *core.Scope, shape func() feedShape, body func(cx *core.Scope, s feedShape) core.Node) core.Node {
	m := core.NewMemo(cx, shape)
	return core.Dynamic(func(cx *core.Scope) core.Node { return body(cx, m.Get()) })
}

// feedCard is a post in a list: author, text and media, then the topic and
// counts. Cards sit edge to edge on the surface, split by an 8dp band.
func (a *App) feedCard(cx *core.Scope, get func() backend.Feed) core.Node {
	open := func() { a.openFeed(get()) }
	press := pointing(core.View{Role: core.RoleButton, Label: "查看帖子", OnPress: func(core.PressEvent) { open() }})
	card := core.View{Class: "bg-background", Children: []core.Node{
		through(column(core.Style{Padding: core.Edges{Top: 16, Right: 16, Bottom: 8, Left: 16}, Gap: 12}, "",
			a.feedHeader(cx, get, 40),
			dynamicOnShape(cx, func() feedShape { return shapeOfFeed(get(), 6) },
				func(cx *core.Scope, s feedShape) core.Node { return a.feedBody(cx, get, s) }),
		)),
		a.feedFooter(cx, get, open),
	}}
	if cx.Env().OS() == core.OSMacOS {
		// AppKit walks up from decorative children to the nearest pressable view.
		card.Role, card.Label, card.OnPress, card.Style.Cursor = press.Role, press.Label, press.OnPress, press.Style.Cursor
	} else {
		// On Android a pressable ancestor would take presses from its children.
		press.Style = fillAbs.Merge(core.Style{Overflow: core.Hidden, Cursor: press.Style.Cursor})
		press.Android.Ripple = &core.Ripple{}
		card.Children = append([]core.Node{press}, card.Children...)
	}
	return column(core.Style{}, "",
		hoverGroup(card, "card", fillAbs),
		core.View{Style: core.Style{Height: core.Pt(8)}, Class: "bg-surface-container-low"})
}

func (a *App) feedFooter(cx *core.Scope, get func() backend.Feed, open func()) core.Node {
	return dynamicShape(cx, func() [2]bool {
		f := get()
		return [2]bool{f.Target != nil && f.Rating == nil, f.Relation != nil}
	}, func(cx *core.Scope, has [2]bool) core.Node {
		var chips []core.Node
		if has[0] {
			chips = append(chips, a.targetChip(cx, func() *backend.Target { return get().Target }))
		}
		if has[1] {
			relation := func() backend.CardItem {
				if it := get().Relation; it != nil {
					return *it
				}
				return backend.CardItem{}
			}
			chips = append(chips, a.feedChip(cx, func() string { return relation().Title },
				func() string { return relation().Pic }, func() { a.openCardItem(relation()) },
				"bg-secondary-container", "text-on-secondary-container"))
		}
		var topics core.Node = core.Group{}
		if len(chips) > 0 {
			topics = through(row(core.Style{Gap: 6, MaxWidth: core.Pct(100), MinWidth: core.Pt(0)}, "", chips...))
		}
		// The counts open the post as the card does, under the card's hover.
		stats := link("查看帖子", open, core.Style{Height: core.Pt(32), AutoMargin: core.SideLeft,
			JustifyContent: core.JustifyCenter, FlexShrink: 0}, feedStats(get))
		return through(row(core.Style{
			Padding: core.Edges{Right: 16, Bottom: 8, Left: 16},
			Wrap:    core.Wrap, RowGap: 4, ColumnGap: 12, AlignItems: core.AlignCenter,
		}, "", topics, stats))
	})
}

// targetChip is an assist chip with the logo of the topic or product a post
// is filed under; it opens the feed of that name.
func (a *App) targetChip(cx *core.Scope, get func() *backend.Target) core.Node {
	title := func() string {
		if t := get(); t != nil {
			return t.Title
		}
		return ""
	}
	return a.feedChip(cx, title, func() string {
		if t := get(); t != nil {
			return t.LogoURL
		}
		return ""
	}, func() {
		if t := get(); t != nil {
			a.openTarget(t)
		}
	}, "bg-surface-container-high", "text-on-surface")
}

func (a *App) feedChip(cx *core.Scope, title, logo func() string, open func(), background, foreground string) core.Node {
	chip := tap("", open, core.Style{Direction: core.Row, Height: core.Pt(32), FlexShrink: 1, MinWidth: core.Pt(0),
		Padding: core.Edges{Left: 6, Right: 8}, Gap: 4, AlignItems: core.AlignCenter, CornerRadius: 8}, background,
		dynamicShape(cx, func() bool { return logo() != "" }, func(cx *core.Scope, has bool) core.Node {
			if has {
				return a.clippedPicFrom(cx, logo, 18, 18, 5)
			}
			return icon(&symbols.Tag, 18, foreground)
		}),
		core.Text{ContentFunc: title, MaxLines: 1, Class: foreground,
			Style: textStyle(textSecondary, 500).Merge(core.Style{FlexShrink: 1, MinWidth: core.Pt(0)})})
	chip.LabelFunc = title
	return chip
}

func (a *App) feedHeader(cx *core.Scope, get func() backend.Feed, size core.Points) core.Node {
	author := func() backend.Author { return get().Author }
	meta := func() string {
		f := get()
		return joinNonEmpty([]string{formatTime(time.Now(), f.PublishedAt), f.Device, f.Location}, "  ")
	}
	return through(row(core.Style{Gap: 12, AlignItems: core.AlignCenter}, "",
		// The avatar and the name open the author as one; the room beside them
		// is the card's.
		link("作者主页", func() { a.openUser(author()) }, core.Style{Direction: core.Row, Gap: 12, AlignItems: core.AlignCenter, FlexShrink: 1, MinWidth: core.Pt(0)},
			a.avatar(cx, author, size),
			column(core.Style{Gap: 2, FlexShrink: 1, MinWidth: core.Pt(0)}, "",
				authorName(cx, author, textBody),
				core.Text{ContentFunc: meta, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)},
			)),
		core.View{Style: core.Style{FlexGrow: 1}, PointerEvents: core.PointerEventsNone},
		// A rating heads its text with its own tags.
		dynamicShape(cx, func() backend.PostKind { return get().Kind }, func(cx *core.Scope, kind backend.PostKind) core.Node {
			switch kind {
			case backend.PostText, backend.PostRating:
				return core.Group{}
			case backend.PostQuestion:
				return pill(kind.Label, "bg-primary-container text-on-primary-container")
			case backend.PostAnswer:
				return pill(kind.Label, "bg-tertiary-container text-on-tertiary-container")
			default:
				return tonalPill(kind.Label)
			}
		}),
	))
}

// avatar is a user's picture in a circle, or a gray disc without one.
func (a *App) avatar(cx *core.Scope, get func() backend.Author, size core.Points) core.Node {
	return core.View{Class: "bg-surface-container-highest", Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2, Overflow: core.Hidden, FlexShrink: 0},
		Children: []core.Node{a.pic(cx, func() string { return get().AvatarURL }, core.Cover, "", fillAbs)}}
}

func authorName(cx *core.Scope, get func() backend.Author, size core.Points) core.Node {
	return row(core.Style{Gap: 4, AlignItems: core.AlignCenter}, "",
		core.Text{ContentFunc: func() string { return displayName(get().Name) }, MaxLines: 1, Class: "text-on-surface",
			Style: func() core.Style { s := textStyle(size, 500); s.FlexShrink = 1; return s }()},
		dynamicShape(cx, func() bool { return get().Verified }, func(cx *core.Scope, verified bool) core.Node {
			if verified {
				return icon(&symbols.Verified, size, "text-primary")
			}
			return core.Group{}
		}),
	)
}

// tonalPill is a small secondary-container label such as a level badge.
func tonalPill(text func() string) core.Node {
	return pill(text, "bg-secondary-container text-on-secondary-container")
}

// pill is a small label on a container of class's colors, its text centered
// by its line height as on every platform's font.
func pill(text func() string, class string) core.Node {
	return core.Text{ContentFunc: text, MaxLines: 1, Class: class,
		Style: func() core.Style {
			s := textStyle(textCaption, 500)
			s.Padding = core.Symmetric(2, 8)
			s.CornerRadius = 8
			s.FlexShrink = 0
			return s
		}()}
}

// feedBody arranges the text and media of a card by post kind.
func (a *App) feedBody(cx *core.Scope, get func() backend.Feed, s feedShape) core.Node {
	title := func() core.Node {
		if !s.title {
			return core.Group{}
		}
		return core.Text{ContentFunc: func() string { return get().Title }, MaxLines: 2, Class: "text-on-surface", Style: textStyle(textBody, 600)}
	}
	switch s.kind {
	case backend.PostArticle:
		const columns = 3
		var media core.Node
		if s.cover {
			media = a.coverImage(cx, get, s)
		} else if s.images > 0 {
			media = a.imageGrid(cx, func() []backend.Image { return get().Images }, min(s.images, columns), columns)
		}
		return through(column(core.Style{Gap: 12}, "", title(), a.feedContent(cx, get, s, 3), media))
	case backend.PostQuestion:
		return through(column(core.Style{Gap: 12}, "", title(), a.feedContent(cx, get, s, 3), core.Text{
			ContentFunc: func() string {
				f := get()
				return joinNonEmpty([]string{countLabel(f.Answers, "个回答"), countLabel(f.Followers, "人关注")}, "  ")
			},
			Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)}))
	case backend.PostAnswer:
		return through(column(core.Style{Gap: 12}, "", core.Dynamic(func(cx *core.Scope) core.Node {
			if !s.title {
				return core.Group{}
			}
			return column(core.Style{Padding: core.Symmetric(10, 12), CornerRadius: 8}, "bg-surface-container-high",
				core.Text{ContentFunc: func() string { return get().Title }, MaxLines: 2, Class: "text-on-surface", Style: textStyle(textSecondary, 600)})
		}), a.feedContent(cx, get, s, 5), a.feedMedia(cx, get, s, false, nil)))
	case backend.PostRating:
		target := dynamicShape(cx, func() *backend.Target { return get().Target }, func(cx *core.Scope, t *backend.Target) core.Node {
			if t == nil {
				return core.Group{}
			}
			return a.ratingTarget(cx, t, false)
		})
		return through(column(core.Style{Gap: 12}, "", a.ratingSummary(cx, get), a.feedMedia(cx, get, s, false, nil), target))
	case backend.PostVideo, backend.PostVote:
		return through(column(core.Style{Gap: 12}, "", title(), a.feedContent(cx, get, s, 3), a.feedMedia(cx, get, s, false, nil)))
	}
	price := dynamicShape(cx, func() bool { return get().Price != "" }, func(cx *core.Scope, has bool) core.Node {
		if !has {
			return core.Group{}
		}
		return labelFunc(func() string { return get().Price }, textBody, 600, "text-error")
	})
	media := a.feedMedia(cx, get, s, false, nil)
	if s.cover {
		media = a.coverImage(cx, get, s)
	}
	return through(column(core.Style{Gap: 12}, "", title(), price, a.feedContent(cx, get, s, 6), media))
}

func countLabel(n int, unit string) string {
	if n <= 0 {
		return ""
	}
	return formatCount(n) + " " + unit
}

// feedContent shows the text of a post, cut to lines, with the link that
// opens the rest.
func (a *App) feedContent(cx *core.Scope, get func() backend.Feed, s feedShape, lines int) core.Node {
	if !s.text {
		return core.Group{}
	}
	text := a.feedRich(cx, get, richOptions{size: textBody, lineHeight: feedLineHeight, class: "text-on-surface", maxLines: lines, press: func() { a.openFeed(get()) }})
	if !s.more {
		return text
	}
	return through(column(core.Style{Gap: 4}, "", text, link("查看更多", func() { a.openFeed(get()) }, core.Style{AlignSelf: core.AlignStart},
		label("查看更多", textSecondary, 500, "text-primary"))))
}

const articleCoverRatio = 20.0 / 9

func (a *App) coverImage(cx *core.Scope, get func() backend.Feed, s feedShape) core.Node {
	ratio, fit := float32(16.0/9), core.Cover
	if s.kind == backend.PostArticle {
		ratio, fit = articleCoverRatio, core.Contain
	}
	image := a.pic(cx, func() string {
		if c := get().Cover; c != nil {
			return thumbURL(*c, thumbSmall)
		}
		return ""
	}, fit, "", fillAbs)
	return core.View{
		Class:    "bg-surface-container-highest",
		Style:    core.Style{Width: core.Pct(100), AspectRatio: ratio, CornerRadius: 12, Overflow: core.Hidden},
		Children: []core.Node{image},
	}
}

// feedMedia shows a post's video, images, link, vote and repost. cast, on
// the post's page, votes for the options chosen.
func (a *App) feedMedia(cx *core.Scope, get func() backend.Feed, s feedShape, detail bool, cast func(ids []string)) core.Node {
	var kids []core.Node
	// A card opens its post from the media that has no page of its own.
	var open func()
	if !detail {
		open = func() { a.openFeed(get()) }
	}
	switch {
	case s.video:
		kids = append(kids, a.videoCover(cx, func() *backend.Video { return get().Video }, detail, open))
	case s.images > 0:
		if detail && s.bigCover {
			kids = append(kids, dynamicShape(cx, func() string { return get().ID }, func(cx *core.Scope, _ string) core.Node {
				// Detail text is inset 16dp; picture pages reach the pane edges.
				return core.View{Style: core.Style{Margin: core.Edges{Left: -16, Right: -16}}, Children: []core.Node{
					a.bigCoverImages(cx, func() []backend.Image { return get().Images }, s.images)}}
			}))
		} else {
			kids = append(kids, a.imageSet(cx, func() []backend.Image { return get().Images }, s.images, detail))
		}
	}
	if s.link {
		kids = append(kids, a.linkCard(cx, func() *backend.Link { return get().Link }, detail, open))
	}
	if s.vote {
		kids = append(kids, voteCard(cx, func() *backend.Vote { return get().Vote }, open, cast))
	}
	if s.fw {
		kids = append(kids, a.forwardCard(cx, func() *backend.Forward { return get().Forward }, open))
	}
	return through(column(core.Style{Gap: 12}, "", kids...))
}

// imageSet lays out one image at its ratio, and several as squares three to
// a row.
func (a *App) imageSet(cx *core.Scope, get func() []backend.Image, n int, detail bool) core.Node {
	if n == 1 {
		return a.singleImage(cx, func() backend.Image { return get()[0] }, detail, func() { a.openImages(get(), 0) })
	}
	return a.imageGrid(cx, get, n, 3)
}

// imageGrid shows square crops, which Coolapk serves at 480px.
func (a *App) imageGrid(cx *core.Scope, get func() []backend.Image, n, cols int) core.Node {
	const gap = 4
	cells := make([]core.Node, 0, n)
	for i := 0; i < n; i++ {
		i := i
		cells = append(cells, core.View{
			Style: core.Style{Width: core.Pct(100 / float32(cols)), AspectRatio: 1},
			Children: []core.Node{thumbnailButton(fmt.Sprintf("查看第 %d 张图片", i+1), func() { a.openImages(get(), i) },
				insetAbs(gap/2).Merge(core.Style{CornerRadius: 8}), "",
				a.pic(cx, func() string {
					if im := get(); i < len(im) {
						return thumbURL(im[i], thumbSquare)
					}
					return ""
				}, core.Cover, "bg-surface-container-highest", fillAbs),
			)},
		})
	}
	return core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Margin: core.All(-gap / 2)}, Children: cells}
}

func thumbnailButton(label string, open func(), style core.Style, class string, children ...core.Node) core.View {
	button := tap(label, open, style, class, children...)
	button.Android.Ripple.Foreground = true
	return button
}

// longImageRatio is the narrowest a single picture's frame goes: 9:22, as
// the official clients cap it. A taller picture fills a frame of that ratio
// from its top and is marked 长图.
const longImageRatio = 9.0 / 22

// singleImage shows the one picture of a post in the middle of the card, at
// the sizes of the official clients: a wide one 2/3 of the width tall (and no
// wider than the card), a square or a portrait up to 3:2 two thirds wide, and
// a taller portrait one third wide, up to 22:9 tall.
func (a *App) singleImage(cx *core.Scope, get func() backend.Image, detail bool, open func()) core.Node {
	frame := func() (width, ratio float32) {
		ratio = max(mediaRatio(get(), 4.0/3, 0, 1e6), longImageRatio)
		switch {
		case ratio > 1:
			width = min(100, 200*ratio/3)
		case ratio > 2.0/3:
			width = 200.0 / 3
		default:
			width = 100.0 / 3
		}
		return
	}
	return through(row(core.Style{JustifyContent: core.JustifyCenter}, "", core.View{
		StyleFunc: func() core.Style {
			w, r := frame()
			return core.Style{Width: core.Pct(w), AspectRatio: r}
		},
		Children: []core.Node{a.pictureButton(cx, get, pictureSize(detail), open)},
	}))
}

// pictureButton fills its frame with a picture in a rounded button. A
// picture taller than longImageRatio shows its top, as the small variant
// the server cuts at that ratio, and is marked 长图.
func (a *App) pictureButton(cx *core.Scope, get func() backend.Image, size string, open func()) core.Node {
	long := func() bool { return mediaRatio(get(), 4.0/3, 0, 1e6) < longImageRatio }
	return thumbnailButton("查看图片", open, fillAbs.Merge(core.Style{CornerRadius: 12}), "bg-surface-container-highest",
		a.pic(cx, func() string {
			if long() {
				return thumbURL(get(), thumbSmall)
			}
			return thumbURL(get(), size)
		}, core.Cover, "", fillAbs),
		core.Dynamic(func(cx *core.Scope) core.Node {
			if !long() {
				return core.Group{}
			}
			return core.Text{Content: "长图", Class: "bg-black/60 text-white", Style: func() core.Style {
				s := textStyle(textCaption, 400)
				s.Position, s.Right, s.Bottom = core.Absolute, core.Pt(6), core.Pt(6)
				s.Padding, s.CornerRadius = core.Symmetric(2, 8), 8
				return s
			}()}
		}),
	)
}

// videoCover shows a video's cover with a play mark and its length. The
// sources are other sites' players, so a tap on the post page opens the page.
func (a *App) videoCover(cx *core.Scope, get func() *backend.Video, detail bool, card func()) core.Node {
	cover := func() string {
		if v := get(); v != nil && v.CoverURL != "" {
			return thumbURL(backend.Image{URL: v.CoverURL}, thumbSmall)
		}
		return ""
	}
	onPress := func() {
		if !detail {
			if card != nil {
				card()
			}
			return
		}
		if v := get(); v != nil && v.URL != "" {
			a.openWeb(v.URL)
		}
	}
	return tap("播放视频", onPress, core.Style{Width: core.Pct(100), AspectRatio: 16.0 / 9, CornerRadius: 12, Overflow: core.Hidden}, "bg-surface-container-highest",
		a.pic(cx, cover, core.Cover, "", fillAbs),
		core.View{Class: "bg-black/50", Style: core.Style{Position: core.Absolute, Width: core.Pt(56), Height: core.Pt(56), CornerRadius: 28,
			Top: core.Pct(50), Left: core.Pct(50), Margin: core.Edges{Top: -28, Left: -28}, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			Children: []core.Node{icon(&symbols.PlayArrow, 32, "text-white")}},
		dynamicShape(cx, func() bool { v := get(); return v != nil && v.Duration > 0 }, func(cx *core.Scope, timed bool) core.Node {
			if !timed {
				return core.Group{}
			}
			return core.Text{ContentFunc: func() string {
				if v := get(); v != nil {
					return formatDuration(v.Duration)
				}
				return ""
			}, Class: "bg-black/60 text-white", Style: func() core.Style {
				s := textStyle(textCaption, 400)
				s.Position, s.Right, s.Bottom = core.Absolute, core.Pt(8), core.Pt(8)
				s.Padding, s.CornerRadius = core.Symmetric(2, 8), 8
				return s
			}()}
		}),
	)
}

// formatDuration is a length as 3:05 or 1:02:03.
func formatDuration(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func (a *App) linkCard(cx *core.Scope, get func() *backend.Link, detail bool, card func()) core.Node {
	link := func() backend.Link {
		if l := get(); l != nil {
			return *l
		}
		return backend.Link{}
	}
	lead := dynamicShape(cx, func() bool { return link().CoverURL != "" }, func(cx *core.Scope, cover bool) core.Node {
		if !cover {
			return core.View{Class: "bg-primary-container", Style: core.Style{Width: core.Pt(32), Height: core.Pt(32), CornerRadius: 16, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0},
				Children: []core.Node{icon(&symbols.Link, 18, "text-on-primary-container")}}
		}
		return a.clippedPicFrom(cx, func() string {
			return thumbURL(backend.Image{URL: link().CoverURL}, thumbSquare)
		}, 40, 40, 8)
	})
	lines := 1
	if detail {
		lines = 2
	}
	subtitle := core.NewMemo(cx, func() string {
		l := link()
		if l.Subtitle != "" || !detail {
			return l.Subtitle
		}
		u, err := url.Parse(l.URL)
		if err != nil {
			return ""
		}
		return strings.TrimPrefix(u.Hostname(), "www.")
	})
	words := column(fill.Merge(core.Style{Gap: 2, MinWidth: core.Pt(0)}), "",
		core.Text{ContentFunc: func() string { return link().Title }, MaxLines: lines, Class: "text-on-surface", Style: textStyle(textSecondary, 500)},
		dynamicShape(cx, func() bool { return subtitle.Get() != "" }, func(cx *core.Scope, has bool) core.Node {
			if !has {
				return core.Group{}
			}
			return core.Text{ContentFunc: subtitle.Get, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)}
		}))
	style := core.Style{Direction: core.Row, Padding: core.Edges{Top: 8, Right: 12, Bottom: 8, Left: 8}, Gap: 10, AlignItems: core.AlignCenter, CornerRadius: 24, Overflow: core.Hidden}
	onPress := func() {
		if !detail {
			if card != nil {
				card()
			}
			return
		}
		a.openURL(link().URL)
	}
	return tap("链接", onPress, style, "bg-surface-container-high", lead, words, icon(&symbols.ChevronRight, 20, "text-on-surface-variant"))
}

// voteCard shows a post's vote: two sides against each other, or a list of
// options, then how many voted and how long it runs. The counts show once the
// server tells them, which it does to an account that has voted. On a card
// the vote opens the post; on the post's page cast votes for the options
// chosen.
func voteCard(cx *core.Scope, get func() *backend.Vote, card func(), cast func(ids []string)) core.Node {
	// A card's row is reused for another post: the vote is made again when it
	// is another one.
	return dynamicShape(cx, func() string {
		v := get()
		if v == nil {
			return ""
		}
		return fmt.Sprint(*v)
	}, func(cx *core.Scope, _ string) core.Node {
		v := core.Untracked(cx, get)
		if v == nil {
			return core.Group{}
		}
		votes := 0
		for _, o := range v.Options {
			votes += o.Votes
		}
		pct := func(o backend.VoteOption) int {
			if votes == 0 {
				return 0
			}
			return (o.Votes*100 + votes/2) / votes
		}
		open := v.Counted || v.Expired || cast == nil // nothing to choose
		picked := core.NewSignalFunc(cx, map[string]bool{}, nil)
		pick := func(id string) {
			if card != nil {
				card()
				return
			}
			if open {
				return
			}
			if v.MaxSelect <= 1 {
				cast([]string{id})
				return
			}
			next := map[string]bool{}
			for k := range picked.GetUntracked() {
				next[k] = true
			}
			if next[id] {
				delete(next, id)
			} else if len(next) < v.MaxSelect {
				next[id] = true
			}
			picked.Set(next)
		}
		var kids []core.Node
		if v.Versus {
			kids = append(kids, versusOptions(v, pct, pick))
		} else {
			for _, o := range v.Options {
				kids = append(kids, voteOption(o, v.Counted, pct(o), func() bool { return picked.Get()[o.ID] }, func() { pick(o.ID) }))
			}
		}
		foot := []core.Node{core.Text{Content: joinNonEmpty([]string{countLabel(v.Voters, "人参与"), voteEnds(v)}, "  "),
			Class: "text-on-surface-variant", Style: fill.Merge(textStyle(textCaption, 400))}}
		if !open && v.MaxSelect > 1 {
			foot = append(foot, core.Dynamic(func(cx *core.Scope) core.Node {
				if len(picked.Get()) == 0 {
					return label(fmt.Sprintf("最多选 %d 项", v.MaxSelect), textCaption, 400, "text-on-surface-variant")
				}
				return tap("投票", func() {
					var ids []string
					for _, o := range v.Options {
						if picked.GetUntracked()[o.ID] {
							ids = append(ids, o.ID)
						}
					}
					cast(ids)
				}, core.Style{Height: core.Pt(40), Padding: core.Symmetric(0, 24), CornerRadius: 20, JustifyContent: core.JustifyCenter}, "bg-primary",
					label("投票", textSecondary, 500, "text-on-primary"))
			}))
		}
		kids = append(kids, row(core.Style{Gap: 8, AlignItems: core.AlignCenter, MinHeight: core.Pt(20)}, "", foot...))
		return column(core.Style{Gap: 8}, "", kids...)
	})
}

// voteEnds says how long a vote still runs.
func voteEnds(v *backend.Vote) string {
	switch left := time.Until(v.EndsAt); {
	case v.Expired:
		return "已结束"
	case v.EndsAt.IsZero():
		return ""
	case left >= 24*time.Hour:
		return fmt.Sprintf("剩余 %d 天", int(left/(24*time.Hour)))
	case left >= time.Hour:
		return fmt.Sprintf("剩余 %d 小时", int(left/time.Hour))
	default:
		return "即将结束"
	}
}

// versusOptions are the two sides of a vote in their colors, and their shares
// once known.
func versusOptions(v *backend.Vote, pct func(backend.VoteOption) int, pick func(id string)) core.Node {
	side := func(i int, fallback uint32) core.Node {
		o := v.Options[i]
		c := o.Color
		if c == 0 {
			c = fallback
		}
		text := o.Text
		if v.Counted {
			text = fmt.Sprintf("%s %d%%", o.Text, pct(o))
		}
		if o.Chosen {
			text = "✓ " + text
		}
		radii := core.Corners{TopLeft: 24, BottomLeft: 24, TopRight: 4, BottomRight: 4}
		if i == 1 {
			radii = core.Corners{TopLeft: 4, BottomLeft: 4, TopRight: 24, BottomRight: 24}
		}
		s := fill.Merge(core.Style{Height: core.Pt(48), Padding: core.Symmetric(0, 16), CornerRadii: radii,
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: core.RGB(c)})
		// The sides share the row by their votes, each keeping room for its name.
		if v.Counted {
			s.FlexGrow = float32(max(pct(o), 25))
		}
		return tap(o.Text, func() { pick(o.ID) }, s, "",
			core.Text{Content: text, MaxLines: 1, Style: textStyle(textSecondary, 500).Merge(core.Style{Color: core.RGB(0xFFFFFF)})})
	}
	return row(core.Style{Gap: 4, AlignItems: core.AlignCenter}, "",
		// The red and blue keep white labels at a 4.5:1 contrast.
		side(0, 0xD32F2F),
		label("VS", textSecondary, 600, "text-on-surface-variant"),
		side(1, 0x1976D2))
}

// voteOption is one option of a vote: an outlined row, filled to its share
// once the counts are known, and marked when chosen.
func voteOption(o backend.VoteOption, counted bool, pct int, picked func() bool, pick func()) core.Node {
	kids := []core.Node{}
	if counted {
		kids = append(kids, core.View{Class: "bg-secondary-container",
			Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Bottom: core.Pt(0), Left: core.Pt(0), Width: core.Pct(float32(pct))}})
	}
	line := []core.Node{core.Text{Content: o.Text, MaxLines: 2, Class: "text-on-surface", Style: fill.Merge(textStyle(textSecondary, 400))}}
	if o.Chosen {
		line = append(line, icon(&symbols.Check, 18, "text-primary"))
	}
	if counted {
		line = append(line, label(fmt.Sprintf("%d%%", pct), textSecondary, 500, "text-on-surface-variant"))
	}
	kids = append(kids, row(core.Style{Gap: 8, AlignItems: core.AlignCenter, MinHeight: core.Pt(44), Padding: core.Symmetric(8, 16)}, "", line...))
	return pressable(core.View{
		Role: core.RoleButton, Label: o.Text, OnPress: func(core.PressEvent) { pick() },
		Style: core.Style{CornerRadius: 12, Overflow: core.Hidden, BorderWidth: 1},
		ClassFunc: func() string {
			if picked() || o.Chosen {
				return "border-primary"
			}
			return "border-outline-variant"
		},
		Children: kids,
	})
}

func (a *App) forwardCard(cx *core.Scope, get func() *backend.Forward, open func()) core.Node {
	return dynamicShape(cx, func() int {
		f := get()
		if f == nil {
			return -1
		}
		return len(f.Images)
	}, func(cx *core.Scope, _ int) core.Node {
		f := core.Untracked(cx, get)
		if f == nil {
			return core.Group{}
		}
		fw := func() backend.Forward {
			if f := get(); f != nil {
				return *f
			}
			return backend.Forward{}
		}
		kids := []core.Node{
			link("作者主页", func() { a.openUser(fw().Author) }, core.Style{AlignSelf: core.AlignStart},
				core.Text{ContentFunc: func() string { return "@" + displayName(fw().Author.Name) }, MaxLines: 1, Class: "text-primary", Style: textStyle(textSecondary, 500)}),
			a.richText(cx, func() ([]backend.RichSpan, string) { f := fw(); return f.Rich, f.Content }, richOptions{size: textSecondary, class: "text-on-surface", maxLines: 4, press: open}),
		}
		if n := len(f.Images); n > 0 {
			kids = append(kids, a.imageGrid(cx, func() []backend.Image { return fw().Images }, n, 3))
		}
		return column(core.Style{Padding: core.All(12), Gap: 8, CornerRadius: 12}, "bg-surface-container-high", kids...)
	})
}

// feedStats shows a card's counts; the post page makes them actions.
func feedStats(get func() backend.Feed) core.Node {
	return row(core.Style{Gap: 16, AlignItems: core.AlignCenter, FlexShrink: 0}, "",
		core.Dynamic(func(cx *core.Scope) core.Node {
			m := core.NewMemo(cx, func() bool { return get().Liked })
			return core.Dynamic(func(cx *core.Scope) core.Node {
				if m.Get() {
					return statCount(&symbols.Favorite, "text-primary", func() int { return get().Stats.Likes })
				}
				return statCount(&symbols.FavoriteOutline, "text-on-surface-variant", func() int { return get().Stats.Likes })
			})
		}),
		statCount(&symbols.Comment, "text-on-surface-variant", func() int { return get().Stats.Comments }),
		statCount(&symbols.Share, "text-on-surface-variant", func() int { return get().Stats.Forwards }),
	)
}

func statCount(glyph *symbol, class string, n func() int) core.Node {
	return row(core.Style{Gap: 6, AlignItems: core.AlignCenter}, class,
		icon(glyph, 18, class),
		core.Text{ContentFunc: func() string { return formatCount(n()) }, Style: textStyle(textCaption, 400)},
	)
}

// pictureSize is the variant of a picture shown in a list or on a post page.
func pictureSize(detail bool) string {
	if detail {
		return thumbMedium
	}
	return thumbSmall
}

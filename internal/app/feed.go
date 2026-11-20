package app

import (
	"fmt"
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
	title, more                  bool
}

func shapeOfFeed(f backend.Feed, lines int) feedShape {
	return feedShape{
		kind: f.Kind, images: len(f.Images), cover: f.Cover != nil, video: f.Video != nil,
		link: f.Link != nil, vote: f.Vote != nil, fw: f.Forward != nil,
		title: f.Title != "", more: feedTextNeedsMore(f.Content, lines),
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
	return column(core.Style{}, "", tap("查看帖子", func() { a.openFeed(get()) }, core.Style{}, "bg-background",
		column(core.Style{Padding: core.Edges{Top: 16, Right: 16, Bottom: 8, Left: 16}, Gap: 12}, "",
			a.feedHeader(cx, get, 40),
			dynamicOnShape(cx, func() feedShape { return shapeOfFeed(get(), 6) },
				func(cx *core.Scope, s feedShape) core.Node { return a.feedBody(cx, get, s) }),
		),
		row(core.Style{Padding: core.Edges{Right: 16, Bottom: 8, Left: 16}, Gap: 16, AlignItems: core.AlignCenter}, "",
			core.Dynamic(func(cx *core.Scope) core.Node {
				if core.Untracked(cx, get).Target == nil {
					return core.Group{}
				}
				return a.targetChip(cx, func() *backend.Target { return get().Target })
			}),
			spacer,
			feedStats(get),
		),
	), core.View{Style: core.Style{Height: core.Pt(8)}, Class: "bg-surface-container-low"})
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
	return tap("话题", func() {
		if t := get(); t != nil {
			a.openTarget(t)
		}
	}, core.Style{Direction: core.Row, Height: core.Pt(32), FlexShrink: 1, Padding: core.Edges{Left: 6, Right: 12}, Gap: 6,
		AlignItems: core.AlignCenter, CornerRadius: 8, Overflow: core.Hidden}, "bg-surface-container-high",
		core.Dynamic(func(cx *core.Scope) core.Node {
			if t := core.Untracked(cx, get); t != nil && t.LogoURL != "" {
				return a.pic(cx, func() string {
					if t := get(); t != nil {
						return t.LogoURL
					}
					return ""
				}, core.Cover, "bg-surface-container-highest", core.Style{Width: core.Pt(20), Height: core.Pt(20), CornerRadius: 10, FlexShrink: 0})
			}
			return icon(&symbols.Tag, 18, "text-on-surface-variant")
		}),
		core.Text{ContentFunc: title, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textSecondary, 500)},
	)
}

func (a *App) feedHeader(cx *core.Scope, get func() backend.Feed, size core.Points) core.Node {
	author := func() backend.Author { return get().Author }
	meta := func() string {
		f := get()
		return joinNonEmpty([]string{formatTime(time.Now(), f.PublishedAt), f.Device, f.Location}, "  ")
	}
	return row(core.Style{Gap: 12, AlignItems: core.AlignCenter}, "",
		tap("作者主页", func() { a.openUser(author()) }, core.Style{FlexShrink: 0, CornerRadius: size / 2, Overflow: core.Hidden}, "",
			a.avatar(cx, author, size)),
		column(fill.Merge(core.Style{Gap: 2}), "",
			authorName(author, textSecondary),
			core.Text{ContentFunc: meta, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)},
		),
		core.Dynamic(func(cx *core.Scope) core.Node {
			if k := core.Untracked(cx, get).Kind; k != backend.PostText {
				return tonalPill(func() string { return get().Kind.Label() })
			}
			return core.Group{}
		}),
	)
}

// avatar is a user's picture in a circle, or a gray disc without one.
func (a *App) avatar(cx *core.Scope, get func() backend.Author, size core.Points) core.Node {
	return a.pic(cx, func() string { return get().AvatarURL }, core.Cover, "bg-surface-container-highest",
		core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2})
}

func authorName(get func() backend.Author, size core.Points) core.Node {
	return row(core.Style{Gap: 4, AlignItems: core.AlignCenter}, "",
		core.Text{ContentFunc: func() string { return displayName(get().Name) }, MaxLines: 1, Class: "text-on-surface",
			Style: func() core.Style { s := textStyle(size, 500); s.FlexShrink = 1; return s }()},
		core.Dynamic(func(cx *core.Scope) core.Node {
			if core.Untracked(cx, get).Verified {
				return icon(&symbols.Verified, size, "text-primary")
			}
			return core.Group{}
		}),
	)
}

// tonalPill is a small secondary-container label such as a level badge.
func tonalPill(text func() string) core.Node {
	return core.Text{ContentFunc: text, MaxLines: 1, Class: "bg-secondary-container text-on-secondary-container",
		Style: func() core.Style {
			s := textStyle(textCaption, 500)
			s.Padding = core.Symmetric(2, 8)
			s.CornerRadius = 8
			s.FlexShrink = 0
			return s
		}()}
}

// feedBody is what a card shows of a post, by what the post is: an article
// has its title, a summary and its cover; a question its title and counts;
// an answer the question it answers, over the answer; a video its cover;
// a vote its choices; the others their text and pictures.
func (a *App) feedBody(cx *core.Scope, get func() backend.Feed, s feedShape) core.Node {
	title := func() core.Node {
		if !s.title {
			return core.Group{}
		}
		return core.Text{ContentFunc: func() string { return get().Title }, MaxLines: 2, Class: "text-on-surface", Style: textStyle(textBody, 600)}
	}
	switch s.kind {
	case backend.PostArticle, backend.PostTrade:
		return column(core.Style{Gap: 12}, "", title(), a.feedContent(cx, get, s, 3), a.coverImage(cx, get, s))
	case backend.PostQuestion:
		return column(core.Style{Gap: 12}, "", title(), a.feedContent(cx, get, s, 3), core.Text{
			ContentFunc: func() string {
				f := get()
				return joinNonEmpty([]string{countLabel(f.Answers, "个回答"), countLabel(f.Followers, "人关注")}, "  ")
			},
			Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)})
	case backend.PostAnswer:
		return column(core.Style{Gap: 12}, "", core.Dynamic(func(cx *core.Scope) core.Node {
			if !s.title {
				return core.Group{}
			}
			return column(core.Style{Padding: core.Symmetric(10, 12), CornerRadius: 8}, "bg-surface-container-high",
				core.Text{ContentFunc: func() string { return get().Title }, MaxLines: 2, Class: "text-on-surface", Style: textStyle(textSecondary, 600)})
		}), a.feedContent(cx, get, s, 5), a.feedMedia(cx, get, s, false))
	case backend.PostVideo, backend.PostVote:
		return column(core.Style{Gap: 12}, "", title(), a.feedContent(cx, get, s, 3), a.feedMedia(cx, get, s, false))
	}
	return column(core.Style{Gap: 12}, "", title(), a.feedContent(cx, get, s, 6), a.feedMedia(cx, get, s, false))
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
	text := a.feedRich(cx, get, richOptions{size: textBody, class: "text-on-surface", maxLines: lines, press: func() { a.openFeed(get()) }})
	if !s.more {
		return text
	}
	return column(core.Style{Gap: 4}, "", text, tap("查看更多", func() { a.openFeed(get()) }, core.Style{},
		"", label("查看更多", textSecondary, 500, "text-primary")))
}

// coverImage shows the cover of an article, at its ratio between 4:3 and
// 2.4:1.
func (a *App) coverImage(cx *core.Scope, get func() backend.Feed, s feedShape) core.Node {
	if !s.cover {
		return core.Group{}
	}
	return a.pic(cx, func() string {
		if c := get().Cover; c != nil {
			return thumbURL(*c, thumbSmall)
		}
		return ""
	}, core.Cover, "bg-surface-container-highest", core.Style{Width: core.Pct(100), AspectRatio: 16.0 / 9, CornerRadius: 12, Overflow: core.Hidden})
}

// feedMedia shows a post's video, images, link, vote and repost.
func (a *App) feedMedia(cx *core.Scope, get func() backend.Feed, s feedShape, detail bool) core.Node {
	var kids []core.Node
	switch {
	case s.video:
		kids = append(kids, a.videoCover(cx, func() *backend.Video { return get().Video }, detail))
	case s.images > 0:
		kids = append(kids, a.imageSet(cx, func() []backend.Image { return get().Images }, s.images, detail))
	}
	if s.link {
		kids = append(kids, a.linkCard(cx, func() *backend.Link { return get().Link }, detail))
	}
	if s.vote {
		kids = append(kids, voteCard(func() *backend.Vote { return get().Vote }))
	}
	if s.fw {
		var open func()
		if !detail {
			open = func() { a.openFeed(get()) }
		}
		kids = append(kids, a.forwardCard(cx, func() *backend.Forward { return get().Forward }, open))
	}
	return column(core.Style{Gap: 12}, "", kids...)
}

// imageSet lays out one image at its ratio, and several in a square grid
// of two columns for two or four, three otherwise.
func (a *App) imageSet(cx *core.Scope, get func() []backend.Image, n int, detail bool) core.Node {
	if n == 1 {
		return a.singleImage(cx, func() backend.Image { return get()[0] }, detail, func() { a.openImages(get(), 0) })
	}
	cols := 3
	if n == 2 || n == 4 {
		cols = 2
	}
	return a.imageGrid(cx, get, n, cols)
}

// imageGrid shows square crops, which Coolapk serves at 480px.
func (a *App) imageGrid(cx *core.Scope, get func() []backend.Image, n, cols int) core.Node {
	const gap = 4
	cells := make([]core.Node, 0, n)
	for i := 0; i < n; i++ {
		i := i
		cells = append(cells, tap(fmt.Sprintf("查看第 %d 张图片", i+1), func() { a.openImages(get(), i) },
			core.Style{Width: core.Pct(100 / float32(cols)), AspectRatio: 1, Padding: core.All(gap / 2)}, "",
			core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), CornerRadius: 8, Overflow: core.Hidden}, Children: []core.Node{
				a.pic(cx, func() string {
					if im := get(); i < len(im) {
						return thumbURL(im[i], thumbSquare)
					}
					return ""
				}, core.Cover, "bg-surface-container-highest", core.Style{Width: core.Pct(100), Height: core.Pct(100)}),
			}}))
	}
	return core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Margin: core.All(-gap / 2)}, Children: cells}
}

// longImageRatio is the narrowest a single picture's frame goes: 9:22, as
// the Android clients cap it. A taller picture shows whole inside a frame of
// that ratio, centered, and is marked 长图.
const longImageRatio = 9.0 / 22

// singleImage shows the one picture of a post as the official clients size
// it, from the left: a wide one 2/3 of the width tall (and no wider than the
// card), a square or a portrait up to 3:2 two thirds wide, a taller portrait
// one third wide up to 22:9 tall, and a taller one whole and centered in a
// third by 22:9 frame, marked 长图.
func (a *App) singleImage(cx *core.Scope, get func() backend.Image, detail bool, open func()) core.Node {
	frame := func() (width, ratio float32) {
		r := mediaRatio(get(), 4.0/3, 0, 1e6)
		ratio = r
		switch {
		case r > 1:
			width = min(100, 200*r/3)
		case r > 2.0/3:
			width = 200.0 / 3
		case r >= longImageRatio:
			width = 100.0 / 3
		default:
			width, ratio = 100.0/3, longImageRatio
		}
		return
	}
	return row(core.Style{}, "", core.View{
		StyleFunc: func() core.Style {
			w, r := frame()
			return core.Style{Width: core.Pct(w), AspectRatio: r, Direction: core.Row, JustifyContent: core.JustifyCenter}
		},
		Children: []core.Node{a.pictureButton(cx, get, pictureSize(detail), open)},
	})
}

// pictureButton fills its frame with a picture in a rounded button: a
// picture taller than longImageRatio shows whole, centered at the frame's
// height, and is marked 长图.
func (a *App) pictureButton(cx *core.Scope, get func() backend.Image, size string, open func()) core.Node {
	ratio := func() float32 { return mediaRatio(get(), 4.0/3, 0, 1e6) }
	return tap("查看图片", open, core.Style{Height: core.Pct(100), CornerRadius: 12, Overflow: core.Hidden}, "bg-surface-container-highest",
		a.pic(cx, func() string { return thumbURL(get(), size) }, core.Cover, "", core.Style{Width: core.Pct(100), Height: core.Pct(100)}),
		core.Dynamic(func(cx *core.Scope) core.Node {
			if ratio() >= longImageRatio {
				return core.Group{}
			}
			return core.Text{Content: "长图", Class: "bg-scrim text-white", Style: func() core.Style {
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
func (a *App) videoCover(cx *core.Scope, get func() *backend.Video, open bool) core.Node {
	cover := func() string {
		if v := get(); v != nil && v.CoverURL != "" {
			return thumbURL(backend.Image{URL: v.CoverURL}, thumbSmall)
		}
		return ""
	}
	onPress := func() {
		if v := get(); open && v != nil && v.URL != "" {
			cx.OpenURL(v.URL, nil)
		}
	}
	return tap("播放视频", onPress, core.Style{Width: core.Pct(100), AspectRatio: 16.0 / 9, CornerRadius: 12, Overflow: core.Hidden}, "bg-surface-container-highest",
		a.pic(cx, cover, core.Cover, "", core.Style{Width: core.Pct(100), Height: core.Pct(100), Position: core.Absolute}),
		core.View{Class: "bg-scrim/50", Style: core.Style{Position: core.Absolute, Width: core.Pt(56), Height: core.Pt(56), CornerRadius: 28,
			Top: core.Pct(50), Left: core.Pct(50), Margin: core.Edges{Top: -28, Left: -28}, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			Children: []core.Node{icon(&symbols.PlayArrow, 32, "text-white")}},
		core.Dynamic(func(cx *core.Scope) core.Node {
			v := core.Untracked(cx, get)
			if v == nil || v.Duration <= 0 {
				return core.Group{}
			}
			return core.Text{ContentFunc: func() string {
				if v := get(); v != nil {
					return formatDuration(v.Duration)
				}
				return ""
			}, Class: "bg-scrim/60 text-white", Style: func() core.Style {
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

func (a *App) linkCard(cx *core.Scope, get func() *backend.Link, detail bool) core.Node {
	kids := []core.Node{
		core.Dynamic(func(cx *core.Scope) core.Node {
			if l := core.Untracked(cx, get); l != nil && l.CoverURL != "" {
				return a.pic(cx, func() string {
					if l := get(); l != nil {
						return thumbURL(backend.Image{URL: l.CoverURL}, thumbSquare)
					}
					return ""
				}, core.Cover, "bg-surface-container-highest", core.Style{Width: core.Pt(48), Height: core.Pt(48), CornerRadius: 8, FlexShrink: 0})
			}
			return core.Group{}
		}),
		column(fill.Merge(core.Style{Gap: 2}), "",
			core.Text{ContentFunc: func() string {
				if l := get(); l != nil {
					return l.Title
				}
				return ""
			}, MaxLines: 2, Class: "text-on-surface", Style: textStyle(textSecondary, 500)},
			core.Text{ContentFunc: func() string {
				if l := get(); l != nil {
					return l.Subtitle
				}
				return ""
			}, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)},
		),
	}
	if detail {
		kids = append(kids, icon(&symbols.OpenInNew, 20, "text-on-surface-variant"))
	}
	style := core.Style{Direction: core.Row, Padding: core.All(12), Gap: 12, AlignItems: core.AlignCenter, CornerRadius: 12, Overflow: core.Hidden}
	onPress := func() {
		if l := get(); detail && l != nil && l.URL != "" {
			cx.OpenURL(l.URL, nil)
		}
	}
	return tap("链接", onPress, style, "bg-surface-container-high", kids...)
}

func voteCard(get func() *backend.Vote) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		v := core.Untracked(cx, get)
		if v == nil {
			return core.Group{}
		}
		total := 0
		for _, o := range v.Options {
			total += o.Votes
		}
		header := []core.Node{core.Text{Content: v.Title, MaxLines: 2, Class: "text-on-surface", Style: fill.Merge(textStyle(textSecondary, 500))}}
		if v.Expired {
			header = append(header, label("已结束", textCaption, 400, "text-on-surface-variant"))
		}
		kids := []core.Node{row(core.Style{Gap: 8, AlignItems: core.AlignCenter}, "", header...)}
		for _, o := range v.Options {
			pct := 0
			if total > 0 {
				pct = (o.Votes*100 + total/2) / total
			}
			kids = append(kids, column(core.Style{Gap: 4}, "",
				row(core.Style{Gap: 8}, "",
					core.Text{Content: o.Text, MaxLines: 2, Class: "text-on-surface", Style: fill.Merge(textStyle(textSecondary, 400))},
					label(fmt.Sprintf("%d%%", pct), textCaption, 400, "text-on-surface-variant")),
				progressBar(float32(pct)/100)))
		}
		if total > 0 {
			kids = append(kids, label(formatCount(total)+" 人参与", textCaption, 400, "text-on-surface-variant"))
		}
		return column(core.Style{Padding: core.All(16), Gap: 12, CornerRadius: 12}, "bg-surface-container-high", kids...)
	})
}

// progressBar is a thin determinate bar filled to fraction.
func progressBar(fraction float32) core.Node {
	return core.View{Class: "bg-surface-container-highest", Style: core.Style{Height: core.Pt(4), CornerRadius: 2, Overflow: core.Hidden},
		Children: []core.Node{core.View{Class: "bg-primary", Style: core.Style{Height: core.Pct(100), Width: core.Pct(fraction * 100)}}}}
}

func (a *App) forwardCard(cx *core.Scope, get func() *backend.Forward, open func()) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
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
			tap("作者主页", func() { a.openUser(fw().Author) }, core.Style{}, "",
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
		icon(glyph, 18, ""),
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

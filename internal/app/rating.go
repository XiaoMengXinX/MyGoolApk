package app

import (
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// stars shows a rating out of five, with a partial fill for fractional scores.
func stars(n float64, size core.Points) core.Node {
	kids := make([]core.Node, 5)
	for i := range kids {
		amount := n - float64(i)
		switch {
		case amount >= 1:
			kids[i] = icon(&symbols.Star, size, "text-star")
		case amount <= 0:
			kids[i] = icon(&symbols.Star, size, "text-outline-variant")
		default:
			kids[i] = core.View{Style: core.Style{Width: core.Pt(size), Height: core.Pt(size)}, Children: []core.Node{
				icon(&symbols.Star, size, "text-outline-variant"),
				core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Width: core.Pt(size * core.Points(amount)), Height: core.Pt(size), Overflow: core.Hidden},
					Children: []core.Node{icon(&symbols.Star, size, "text-star")}},
			}}
		}
	}
	return row(core.Style{Gap: 2, AlignItems: core.AlignCenter}, "", kids...)
}

// ratingTag is the heading of a part of a rating's text, or of who rated.
func ratingTag(text string, class string) core.Node {
	return core.Text{Content: text, MaxLines: 1, Class: class,
		Style: textStyle(textCaption, 500).Merge(core.Style{Padding: core.Symmetric(1, 6), CornerRadius: 4, AlignSelf: core.AlignStart})}
}

// ratingScore is the line of a rating's stars: who gave them, the stars
// and their word.
func ratingScore(r *backend.FeedRating, size core.Points) core.Node {
	if r.Stars <= 0 {
		return core.Group{}
	}
	title, class := "评分", "bg-secondary-container text-on-secondary-container"
	if r.Owner {
		title, class = "机主评分", "bg-tertiary-container text-on-tertiary-container"
	}
	return row(core.Style{Gap: 8, AlignItems: core.AlignCenter, Wrap: core.Wrap}, "",
		column(core.Style{}, "", ratingTag(title, class)),
		stars(float64(r.Stars), size), ratingWord(r.Stars, r.Word))
}

func ratingWord(n int, word string) core.Node {
	if n < 1 || n > 5 || word == "" {
		return core.Group{}
	}
	faces := [...]string{"😣", "🙁", "😐", "🙂", "🤩"}
	return label(faces[n-1]+" "+word, textCaption, 500, "text-on-surface-variant")
}

func (a *App) ratingTarget(cx *core.Scope, t *backend.Target, owner bool) core.Node {
	title := []core.Node{core.Text{Content: t.Title, MaxLines: 2, Class: "text-on-surface",
		Style: textStyle(textSecondary, 600).Merge(core.Style{FlexShrink: 1, MinWidth: core.Pt(0)})}}
	if owner {
		title = append(title, column(core.Style{}, "", ratingTag("机主", "bg-tertiary-container text-on-tertiary-container")))
	}
	meta := []core.Node{row(core.Style{Gap: 6, AlignItems: core.AlignCenter, Wrap: core.Wrap}, "", title...)}
	if t.Rating.Count != "" {
		meta = append(meta, label(t.Rating.Count+"人点评", textCaption, 400, "text-on-surface-variant"))
	}
	var kids []core.Node
	if t.LogoURL != "" {
		kids = append(kids, a.digitalPicture(cx, func() string { return t.LogoURL }, 48, 48))
	}
	kids = append(kids, column(fill.Merge(core.Style{Gap: 2}), "", meta...))
	if t.Rating.Score != "" {
		kids = append(kids, itemScore(t.Rating))
	}
	return tap(t.Title, func() { a.openTarget(t) }, core.Style{Direction: core.Row, Padding: core.All(10),
		Gap: 10, AlignItems: core.AlignCenter, CornerRadius: 12}, "bg-surface-container-low", kids...)
}

// ratingSummary is what a card shows of a rating: its stars, the stars of
// each side in a line, and the first part of its text.
func (a *App) ratingSummary(cx *core.Scope, get func() backend.Feed) core.Node {
	return dynamicShape(cx, func() *backend.FeedRating { return get().Rating }, func(cx *core.Scope, r *backend.FeedRating) core.Node {
		if r == nil {
			return core.Group{}
		}
		kids := []core.Node{ratingScore(r, 16)}
		if len(r.Sides) > 0 {
			parts := make([]string, len(r.Sides))
			for i, s := range r.Sides {
				parts[i] = s.Name + " " + strconv.Itoa(s.Stars) + "★"
			}
			kids = append(kids, core.Text{Content: strings.Join(parts, "   "), MaxLines: 2, Class: "text-on-surface-variant", Style: textStyle(textCaption, 400)})
		}
		if len(r.Sections) > 0 {
			s := r.Sections[0]
			kids = append(kids, ratingTag(s.Title, "bg-primary text-on-primary"),
				a.richText(cx, func() ([]backend.RichSpan, string) { return s.Rich, s.Text },
					richOptions{size: textBody, lineHeight: feedLineHeight, class: "text-on-surface", maxLines: 4, press: func() { a.openFeed(get()) }}))
		}
		return through(column(core.Style{Gap: 8}, "", kids...))
	})
}

// ratingDetail is a rating on its post's page: a box of the product, the
// stars of the rating and of each side with their words, then each part of
// its text under its heading.
func (d *detail) ratingDetail(cx *core.Scope, f backend.Feed, docs map[int]postDocument) core.Node {
	a := d.a
	r := f.Rating
	box := []core.Node{}
	if t := f.Target; t != nil {
		box = append(box, a.ratingTarget(cx, t, r.Owner))
	}
	line := func(name string, n int, word core.Node, size core.Points) core.Node {
		return row(core.Style{Gap: 12, AlignItems: core.AlignCenter}, "",
			core.Text{Content: name, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textSecondary, 400).Merge(core.Style{Width: core.Pt(64)})},
			stars(float64(n), size), word)
	}
	scorer := "评分"
	if r.Owner {
		scorer = "机主评分"
	}
	if r.Stars > 0 {
		box = append(box, line(scorer, r.Stars, ratingWord(r.Stars, r.Word), 20))
	}
	for _, s := range r.Sides {
		box = append(box, line(s.Name, s.Stars, label(s.Word, textCaption, 500, "text-on-surface-variant"), 16))
	}
	kids := []core.Node{column(core.Style{Padding: core.All(16), Gap: 10, CornerRadius: 12}, "bg-surface-container-low", box...)}
	for i, s := range r.Sections {
		kids = append(kids, ratingTag(s.Title, "bg-primary text-on-primary"),
			a.postDocument(cx, docs[i], d.markdown.Get,
				func() ([]backend.RichSpan, string) { return s.Rich, s.Text }))
	}
	return column(core.Style{Gap: 10}, "", kids...)
}

package app

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

func (a *App) ratingDataPage(cx *core.Scope, topic *backend.TopicDetail) core.Node {
	var cursor backend.ListCursor
	filter := core.NewSignal(cx, backend.NodeRatingFilter{})
	p := newBatchPager(cx, func(page int) func(context.Context) (pageBatch[backend.NodeRating], error) {
		q := backend.NodeRatingQuery{TopicID: topic.ID, Page: page, Cursor: cursor, Filter: filter.GetUntracked()}
		return func(ctx context.Context) (pageBatch[backend.NodeRating], error) {
			rows, err := a.backend.TopicNodeRatings(ctx, q)
			return pageBatch[backend.NodeRating]{items: rows.Items, hasMore: rows.HasMore,
				apply: func() { cursor = rows.Cursor }}, err
		}
	}, func(r backend.NodeRating) string { return r.ID })
	filters := ratingFilters(filter.Get, func(next backend.NodeRatingFilter) {
		if filter.GetUntracked() == next {
			return
		}
		filter.Set(next)
		// The new first-page request captures the outgoing list's cursor.
		p.reset()
		cursor = backend.ListCursor{}
	})

	trends := core.NewSignalFunc(cx, backend.RatingTrends{Week: topic.Rating.Stats.Weekly}, nil)
	trendError := core.NewSignal(cx, false)
	loadingTrends := false
	loadTrends := func() {
		if loadingTrends {
			return
		}
		loadingTrends = true
		trendError.Set(false)
		background(a, func(ctx context.Context) (backend.RatingTrends, error) {
			return a.backend.TopicRatingTrends(ctx, topic.ID)
		}, func(data backend.RatingTrends, err error) {
			loadingTrends = false
			if err != nil {
				trendError.Set(true)
				return
			}
			trends.Set(data)
		})
	}
	loadTrends()

	return a.screen(titled("评分详情"), nil, pagerList(cx, p, listSpec{
		pushed: true, bottom: 16, empty: "暂无打分记录",
		onRefresh: func() { p.reload(); loadTrends() },
		header:    a.ratingDataHeader(cx, topic, trends.Get, trendError.Get, loadTrends, filters),
	}, func(cx *core.Scope, get func() backend.NodeRating, _ core.ListRow) core.Node {
		return a.nodeRatingRow(cx, get)
	}))
}

func (a *App) ratingDataHeader(cx *core.Scope, topic *backend.TopicDetail, trends func() backend.RatingTrends, failed func() bool, retry func(), filters core.Node) core.Node {
	r := topic.Rating
	stats := r.Stats
	head := row(core.Style{Padding: core.Symmetric(10, 4), Gap: 12, AlignItems: core.AlignCenter}, "",
		a.clippedPic(cx, topic.LogoURL, 48, 48, 12),
		core.Text{Content: topic.Title, MaxLines: 2, Class: "text-on-surface", Style: fill.Merge(textStyle(textTitle, 600))})
	parts := []core.Node{head, ratingDistribution(r)}
	if stats.RecentScore != "" {
		recent := []core.Node{ratingValue(stats.RecentScore, stats.RecentCount)}
		if stats.RecentPositive != "" {
			recent = append(recent, column(core.Style{AlignItems: core.AlignCenter, Gap: 4}, "",
				ratingMetric(stats.RecentPositive+"%", "text-primary"),
				label("30天好评率", textCaption, 400, "text-on-surface-variant")))
		}
		parts = append(parts, ratingSection("最近30天全部用户评分",
			row(core.Style{JustifyContent: core.JustifySpaceAround, AlignItems: core.AlignCenter, Gap: 8}, "", recent...)))
	}
	if len(r.Sides) > 0 {
		parts = append(parts, ratingBreakdown(cx, r))
	}
	parts = append(parts, ratingHistory(cx, trends, failed, retry), label("全部用户打分", textBody, 600, "text-on-surface"), filters)
	return column(core.Style{Padding: core.All(12), Gap: 12}, "bg-background", parts...)
}

func ratingFilters(get func() backend.NodeRatingFilter, change func(backend.NodeRatingFilter)) core.Node {
	choices := [...]string{"全部", "5星", "4星", "3星", "2星", "1星"}
	starChips := make([]core.Node, len(choices))
	for i, name := range choices {
		star := 0
		if i != 0 {
			star = len(choices) - i
		}
		chip := compactChip(name, func() bool { return get().Star == star }, func() {
			f := get()
			f.Star = star
			change(f)
		})
		chip.Style = fill.Merge(chip.Style).Merge(core.Style{Padding: core.All(0)})
		starChips[i] = chip
	}
	detailed := compactChip("只看详细打分", func() bool { return get().Detail == backend.RatingDetailOnly }, func() {
		f := get()
		if f.Detail == backend.RatingDetailOnly {
			f.Detail = backend.RatingDetailOff
		} else {
			f.Detail = backend.RatingDetailOnly
		}
		change(f)
	})
	commentChip := func(name string, option backend.RatingCommentFilter) core.View {
		return compactChip(name, func() bool { return get().Comment == option }, func() {
			f := get()
			if f.Comment == option {
				f.Comment = backend.RatingCommentAll
			} else {
				f.Comment = option
			}
			change(f)
		})
	}
	with := commentChip("只看有评语", backend.RatingWithComment)
	without := commentChip("只看无评语", backend.RatingWithoutComment)
	detailed.Style.Padding, with.Style.Padding, without.Style.Padding = core.Symmetric(0, 8), core.Symmetric(0, 8), core.Symmetric(0, 8)
	return column(core.Style{Gap: 12}, "",
		row(core.Style{Gap: 4}, "", starChips...),
		row(core.Style{Gap: 6, RowGap: 12, Wrap: core.Wrap}, "", detailed, with, without))
}

func ratingSection(title string, children ...core.Node) core.Node {
	return column(core.Style{Padding: core.All(12), Gap: 12, CornerRadius: 16}, "bg-surface-container-low",
		append([]core.Node{label(title, textBody, 600, "text-on-surface")}, children...)...)
}

func ratingValue(score string, count int) core.Node {
	n, _ := strconv.ParseFloat(score, 64)
	return column(core.Style{Gap: 4, AlignItems: core.AlignCenter}, "",
		ratingMetric(score, "text-on-surface"), stars(n/2, 12),
		label(fmt.Sprintf("%d名用户评分", count), textCaption, 400, "text-on-surface-variant"))
}

func ratingMetric(value, class string) core.Node {
	return core.Text{Content: value, Class: class, Style: core.Style{FontSize: 32, LineHeight: 40, FontWeight: 600}}
}

func ratingDistribution(r *backend.Rating) core.Node {
	return ratingSection("全部用户评分", row(core.Style{AlignItems: core.AlignCenter, Gap: 16}, "",
		ratingValue(r.Score, r.Users), ratingStarBars(r.Stats.Stars)))
}

func ratingStarBars(counts [5]int) core.Node {
	most := max(counts[0], counts[1], counts[2], counts[3], counts[4])
	lines := make([]core.Node, 0, len(counts))
	for i := len(counts) - 1; i >= 0; i-- {
		lines = append(lines, row(core.Style{Gap: 6, AlignItems: core.AlignCenter}, "",
			core.Text{Content: strconv.Itoa(i+1) + "星", Class: "text-on-surface-variant",
				Style: textStyle(textCaption, 400).Merge(core.Style{Width: core.Pt(24)})},
			core.View{Class: "bg-surface-container-highest", Style: fill.Merge(core.Style{Height: core.Pt(6), CornerRadius: 3, Overflow: core.Hidden}),
				Children: []core.Node{core.View{Class: "bg-star", Style: core.Style{Width: core.Pct(100 * float32(counts[i]) / float32(max(most, 1))), Height: core.Pt(6), CornerRadius: 3}}}},
			core.Text{Content: strconv.Itoa(counts[i]), Class: "text-on-surface-variant",
				Style: textStyle(textCaption, 400).Merge(core.Style{Width: core.Pt(44), TextAlign: core.TextAlignRight})}))
	}
	return column(fill.Merge(core.Style{Gap: 4}), "", lines...)
}

func ratingBreakdown(cx *core.Scope, r *backend.Rating) core.Node {
	rows := make([]core.Node, 0, (len(r.Sides)+1)/2)
	for i := 0; i < len(r.Sides); i += 2 {
		cells := []core.Node{}
		for _, s := range r.Sides[i:min(i+2, len(r.Sides))] {
			cells = append(cells, row(fill.Merge(core.Style{Gap: 6, AlignItems: core.AlignCenter}), "",
				core.Text{Content: s.Name, Class: "text-on-surface-variant", Style: fill.Merge(textStyle(textCaption, 400))},
				label(fmt.Sprintf("%.1f", s.Score), textSecondary, 600, "text-on-surface")))
		}
		if len(cells) == 1 {
			cells = append(cells, core.View{Style: fill})
		}
		rows = append(rows, row(core.Style{Gap: 16}, "", cells...))
	}
	return ratingSection("全部用户详细评分", ratingRadar(cx, r.Sides),
		column(core.Style{Gap: 6}, "", rows...),
		label(fmt.Sprintf("%d名用户详细点评", r.Detailed), textCaption, 400, "text-on-surface-variant"))
}

func ratingRadar(cx *core.Scope, sides []backend.RatingSide) core.Node {
	names := make([]string, len(sides))
	for i, side := range sides {
		names[i] = fmt.Sprintf("%s%.1f", side.Name, side.Score)
	}
	return core.Canvas{Label: joinNonEmpty(names, "，"), Style: core.Style{Height: core.Pt(136)},
		Draw: func(d *core.Drawing, size core.Size) {
			p := cx.Palette()
			outline, _ := p.Color("outline-variant")
			gold, _ := p.Color("star")
			x, y := float32(size.Width)/2, float32(size.Height)/2
			radius := min(x, y) - 4
			for i := 1; i <= 4; i++ {
				r := radius * float32(i) / 4
				d.BeginPath().Arc(x, y, r, 0, 2*math.Pi, false).SetStrokeColor(outline).SetLineWidth(1).Stroke()
			}
			xs, ys := make([]float32, len(sides)), make([]float32, len(sides))
			for i, s := range sides {
				angle := 2*math.Pi*float64(i)/float64(len(sides)) - math.Pi/2
				sin, cos := math.Sincos(angle)
				dx, dy := radius*float32(cos), radius*float32(sin)
				d.BeginPath().MoveTo(x, y).LineTo(x+dx, y+dy).Stroke()
				xs[i], ys[i] = x+dx*s.Score/10, y+dy*s.Score/10
			}
			d.BeginPath().MoveTo(xs[0], ys[0])
			for i := 1; i < len(xs); i++ {
				d.LineTo(xs[i], ys[i])
			}
			d.ClosePath().SetAlpha(.24).SetFillColor(gold).Fill().SetAlpha(1).SetStrokeColor(gold).SetLineWidth(2).Stroke()
		}}
}

func ratingHistory(cx *core.Scope, get func() backend.RatingTrends, failed func() bool, retry func()) core.Node {
	periods := [...]string{"日", "周", "月"}
	selected, group := core.NewSignal(cx, 1), core.NewSignal(cx, 0)
	return core.Dynamic(func(cx *core.Scope) core.Node {
		data := get()
		trends := [...]backend.RatingTrend{data.Day, data.Week, data.Month}
		current := selected.Get()
		parts := []core.Node{}
		tabs := []core.Node{}
		firstAvailable := -1
		for i, name := range periods {
			if len(trends[i].Periods) > 0 && trends[i].CountCeiling > 0 {
				if firstAvailable == -1 {
					firstAvailable = i
				}
				tabs = append(tabs, filterChip(name, func() bool { return current == i }, func() {
					group.Set(0)
					selected.Set(i)
				}))
			}
		}
		if firstAvailable >= 0 && (len(trends[current].Periods) == 0 || trends[current].CountCeiling <= 0) {
			current = firstAvailable
		}
		t := trends[current]
		if len(tabs) > 1 {
			parts = append(parts, row(core.Style{Gap: 6}, "", tabs...))
		}
		if firstAvailable >= 0 {
			const visible = 7 // The topic chart shows seven periods at a time.
			page := min(group.Get(), (len(t.Periods)-1)/visible)
			end := len(t.Periods) - page*visible
			start := max(0, end-visible)
			parts = append(parts, ratingTrendChart(cx, t, t.Periods[start:end]))
			actions := []core.Node{}
			if start > 0 {
				actions = append(actions, textButton("更早", func() { group.Set(page + 1) }))
			}
			if page > 0 {
				actions = append(actions, textButton("较新", func() { group.Set(page - 1) }))
			}
			if len(actions) > 0 {
				parts = append(parts, row(core.Style{Gap: 6}, "", actions...))
			}
		}
		if failed() {
			parts = append(parts, textButton("重试加载趋势", retry))
		} else if len(parts) == 0 {
			parts = append(parts, label("暂无趋势", textCaption, 400, "text-on-surface-variant"))
		}
		return ratingSection("全部用户评分趋势", parts...)
	})
}

func ratingTrendChart(cx *core.Scope, t backend.RatingTrend, periods []backend.RatingPeriod) core.Node {
	return column(core.Style{Gap: 8}, "",
		row(core.Style{Gap: 12}, "", label("评分", textCaption, 500, "text-primary"), label("点评人数", textCaption, 500, "text-star")),
		core.Canvas{Label: "评分和点评人数趋势", Style: core.Style{Height: core.Pt(176)},
			Draw: func(d *core.Drawing, size core.Size) {
				p := cx.Palette()
				primary, _ := p.Color("primary")
				gold, _ := p.Color("star")
				outline, _ := p.Color("outline-variant")
				muted, _ := p.Color("on-surface-variant")
				font := core.Font{Size: 12}
				left := d.MeasureText("10.0", font) + 8
				right := float32(size.Width) - d.MeasureText(strconv.Itoa(t.CountCeiling), font) - 8
				top, bottom := float32(10), float32(size.Height)-24
				step := (right - left) / float32(len(periods))
				h := bottom - top
				scoreSpan := t.ScoreCeiling - t.ScoreFloor
				for i := 0; i <= 5; i++ {
					f := float32(i) / 5
					y := bottom - f*h
					d.BeginPath().MoveTo(left, y).LineTo(right, y).SetStrokeColor(outline).SetLineWidth(1).Stroke()
					d.SetTextAlign(core.TextAlignEnd).SetFillColor(primary).FillText(fmt.Sprintf("%.1f", t.ScoreFloor+f*scoreSpan), left-4, y+4, font)
					d.SetTextAlign(core.TextAlignStart).SetFillColor(gold).FillText(strconv.Itoa(int(math.Round(float64(f)*float64(t.CountCeiling)))), right+4, y+4, font)
				}
				for i, point := range periods {
					x := left + (float32(i)+.5)*step
					bar := h * float32(point.Count) / float32(t.CountCeiling)
					d.SetFillColor(gold).FillRect(x-step*.31, bottom-bar, step*.62, bar)
					date := point.Date
					if len(date) == len("2026.11") && date[4] == '.' {
						date = date[2:]
					}
					d.SetTextAlign(core.TextAlignCenter).SetFillColor(muted).FillText(date, x, bottom+16, font)
				}
				d.SetFillColor(primary).SetStrokeColor(primary).SetLineWidth(2)
				prev := false
				var px, py float32
				for i, point := range periods {
					if point.Score <= 0 {
						prev = false
						continue
					}
					x := left + (float32(i)+.5)*step
					y := bottom - h*min(max((point.Score-t.ScoreFloor)/scoreSpan, 0), 1)
					if prev {
						d.BeginPath().MoveTo(px, py).LineTo(x, y).Stroke()
					}
					d.BeginPath().Arc(x, y, 2.5, 0, 2*math.Pi, false).Fill()
					px, py, prev = x, y, true
				}
			}})
}

func (a *App) nodeRatingRow(cx *core.Scope, get func() backend.NodeRating) core.Node {
	type scoreState struct {
		stars    int
		detailed bool
	}
	return dynamicShape(cx, func() scoreState {
		r := get()
		return scoreState{r.Stars, r.Detailed}
	}, func(cx *core.Scope, state scoreState) core.Node {
		score := []core.Node{stars(float64(state.stars), 12)}
		if state.detailed {
			score = append(score, label("详细", textCaption, 500, "text-primary"))
		}
		author := func() backend.Author { return get().Author }
		view := tap("", func() {
			r := get()
			if r.FeedID != "" {
				a.push(routeFeed, r.FeedID)
			} else {
				a.openUser(r.Author)
			}
		}, core.Style{Direction: core.Row, Padding: core.Symmetric(8, 16), MinHeight: core.Pt(56), AlignItems: core.AlignCenter, Gap: 12}, "bg-background",
			a.avatar(cx, author, 36),
			core.Text{ContentFunc: func() string { return displayName(author().Name) }, MaxLines: 1, Class: "text-on-surface", Style: fill.Merge(textStyle(textBody, 400))},
			column(core.Style{Gap: 2, AlignItems: core.AlignEnd}, "", score...))
		view.LabelFunc = func() string { return displayName(author().Name) }
		return view
	})
}

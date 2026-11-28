package app

import (
	"context"
	"slices"
	"time"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

func (a *App) offlineEventCard(cx *core.Scope, get func() backend.PageEntry) core.Node {
	tags := core.NewMemoFunc(cx, func() []string { return get().Event.Tags }, slices.Equal)
	card := tap("", func() {
		e := get()
		a.openCardItem(backend.CardItem{Title: e.Title, URL: e.URL})
	}, core.Style{Direction: core.Row, Gap: 12, Padding: core.All(12), CornerRadius: 16,
		Margin: core.Edges{Left: 12, Right: 12, Bottom: 8}, AlignItems: core.AlignStart}, "bg-surface-container-low",
		core.View{Style: core.Style{Width: core.Pt(76), Height: core.Pt(76), CornerRadius: 12, Overflow: core.Hidden, FlexShrink: 0},
			Children: []core.Node{a.pic(cx, func() string { return get().LogoURL }, core.Cover, "", fillAbs)}},
		column(fill.Merge(core.Style{Gap: 8}), "",
			core.Text{ContentFunc: func() string { return get().Title }, MaxLines: 2, Class: "text-on-surface", Style: textStyle(textBody, 600)},
			core.Dynamic(func(*core.Scope) core.Node {
				values := tags.Get()
				if len(values) == 0 {
					return core.Group{}
				}
				chips := make([]core.Node, len(values))
				for i, tag := range values {
					chips[i] = core.Text{Content: tag, MaxLines: 1, Class: "bg-surface-container-high text-on-surface-variant",
						Style: textStyle(textCaption, 400).Merge(core.Style{Padding: core.Symmetric(2, 6), CornerRadius: 6,
							MaxWidth: core.Pct(100), MinWidth: core.Pt(0)})}
				}
				return row(core.Style{Wrap: core.Wrap, RowGap: 4, ColumnGap: 4}, "", chips...)
			}),
			row(core.Style{Wrap: core.Wrap, RowGap: 4, ColumnGap: 8, AlignItems: core.AlignCenter}, "",
				core.Show{When: func() bool { return get().Event.Date != "" }, Then: func(*core.Scope) core.Node {
					return labelFunc(func() string { return get().Event.Date }, textCaption, 500, "text-primary")
				}},
				row(core.Style{AutoMargin: core.SideLeft, AlignItems: core.AlignCenter, Gap: 2}, "",
					label("查看详情", textCaption, 500, "text-primary"), icon(&symbols.ChevronRight, 16, "text-primary"))),
		))
	card.LabelFunc = func() string { return "查看详情 " + get().Title }
	return card
}

func (a *App) eventPage(cx *core.Scope, id string) core.Node {
	event := core.NewSignal[*backend.EventDetail](cx, nil)
	failed := core.NewSignal(cx, "")
	load := func() {
		failed.Set("")
		background(a, func(ctx context.Context) (backend.EventDetail, error) {
			return a.backend.Event(ctx, id)
		}, func(d backend.EventDetail, err error) {
			if err != nil {
				failed.Set(err.Error())
				return
			}
			event.Set(&d)
		})
	}
	load()
	return a.screen(titled("活动"), nil, core.Dynamic(func(cx *core.Scope) core.Node {
		d := event.Get()
		switch {
		case failed.Get() != "":
			return statusBlock("加载失败", failed.Get(), "重试", load)
		case d == nil:
			return column(fill.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}), "", spinner())
		}
		head := a.eventHead(cx, *d)
		if len(d.Tabs) == 0 {
			return core.Scroll{Style: fill, Children: []core.Node{head, bottomSpace(16, true)}}
		}
		tabs := make([]pageTab, len(d.Tabs))
		for i, tab := range d.Tabs {
			tabs[i] = pageTab{Title: tab.Title, Kind: tabPage, URL: tab.URL}
		}
		g := a.newTabGroup(cx, tabs, 0)
		g.pushed, g.head = true, head
		return g.view(cx, nil)
	}))
}

func (a *App) eventHead(cx *core.Scope, d backend.EventDetail) core.Node {
	var body []core.Node
	stage := "报名中"
	now := time.Now()
	switch {
	case d.Stage >= 3:
		stage = "活动已结束"
	case !d.RegistrationStart.IsZero() && now.Before(d.RegistrationStart):
		stage = "报名未开始"
	case d.Stage == 2 || !d.RegistrationEnd.IsZero() && !now.Before(d.RegistrationEnd):
		stage = "报名已结束"
	case d.Joined:
		stage = "已报名"
	}
	body = append(body, label(d.Title, textTitle, 600, "text-on-surface"),
		label(stage, textSecondary, 500, "text-primary"))
	if len(d.Description) > 0 {
		body = append(body, a.richText(cx, func() ([]backend.RichSpan, string) { return d.Description, "" },
			richOptions{size: textBody, lineHeight: feedLineHeight, class: "text-on-surface"}))
	}
	if len(d.Rules) > 0 {
		expanded := core.NewSignal(cx, false)
		body = append(body,
			row(core.Style{AlignItems: core.AlignCenter}, "",
				label("众测须知", textBody, 600, "text-on-surface"), spacer,
				core.Dynamic(func(cx *core.Scope) core.Node {
					if expanded.Get() {
						return tonalButton("收起", func() { expanded.Set(false) })
					}
					return tonalButton("展开", func() { expanded.Set(true) })
				})),
			core.Dynamic(func(cx *core.Scope) core.Node {
				lines := 4
				if expanded.Get() {
					lines = 0
				}
				return a.richText(cx, func() ([]backend.RichSpan, string) { return d.Rules, "" },
					richOptions{size: textSecondary, lineHeight: 24, maxLines: lines, class: "text-on-surface-variant"})
			}))
	}
	if len(d.Sponsors) > 0 {
		body = append(body, label("主办方", textBody, 600, "text-on-surface"))
		for _, sponsor := range d.Sponsors {
			body = append(body, tap(sponsor.Name, func() { a.openUser(sponsor) },
				core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, CornerRadius: 8}, "",
				a.avatar(cx, func() backend.Author { return sponsor }, 36),
				authorName(cx, func() backend.Author { return sponsor }, textSecondary),
				spacer, icon(&symbols.ChevronRight, 20, "text-on-surface-variant")))
		}
	}
	if len(d.Prizes) > 0 {
		body = append(body, label("活动奖品", textBody, 600, "text-on-surface"))
		for _, prize := range d.Prizes {
			body = append(body, row(core.Style{AlignItems: core.AlignCenter, Gap: 10}, "",
				a.clippedPic(cx, prize.Pic, 48, 48, 8),
				core.Text{Content: prize.Title, Style: fill.Merge(textStyle(textSecondary, 500)), Class: "text-on-surface"}))
		}
	}
	if !d.RegistrationStart.IsZero() && !d.RegistrationEnd.IsZero() {
		const date = "2006-01-02"
		body = append(body, label("报名时间："+d.RegistrationStart.Local().Format(date)+" 至 "+d.RegistrationEnd.Local().Format(date),
			textSecondary, 400, "text-on-surface-variant"))
	}
	if stage == "报名中" && d.ActionURL != "" {
		body = append(body, filledButton("报名", func() bool { return true }, func() {
			if a.requireLogin() {
				a.openURL(d.ActionURL)
			}
		}))
	} else if stage == "活动已结束" && d.WinnersURL != "" {
		body = append(body, tonalButton("查看入选名单", func() { a.openList("入选名单", d.WinnersURL) }))
	}
	var cover core.Node = core.Group{}
	if d.CoverURL != "" {
		cover = core.View{Style: core.Style{AspectRatio: 16.0 / 9}, Class: "bg-surface-container-highest",
			Children: []core.Node{a.pic(cx, func() string { return d.CoverURL }, core.Cover, "", fillAbs)}}
	}
	return column(core.Style{}, "bg-background", cover, column(core.Style{Padding: core.All(16), Gap: 12}, "", body...))
}

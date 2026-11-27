package app

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// currentDeviceRating matches the server's environment card or looks up
// the physical device independently of the model reported in requests.
func (a *App) currentDeviceRating(cx *core.Scope, l *pageList, selected func() bool) core.Node {
	device := physicalDevice(cx.Env())
	if device.Model == "" {
		return core.Group{}
	}
	osName := "Android"
	if cx.Env().OS() == core.OSIOS {
		osName = "iOS"
		if cx.Env().Idiom() == core.IdiomTablet {
			osName = "iPadOS"
		}
	}
	major, _, _ := strings.Cut(device.SystemVersion, ".")
	system := osName + major
	queries := []backend.SearchQuery{{Keyword: device.Model, Kind: backend.SearchProducts, Page: 1}}
	initial := []backend.CardItem{{Title: device.Model}}
	if major != "" {
		queries = append(queries, backend.SearchQuery{Keyword: osName + major, Kind: backend.SearchTopics, Page: 1})
		initial = append(initial, backend.CardItem{Title: system})
	}
	lookup := core.NewSignalFunc(cx, initial, nil)
	items := core.NewMemoFunc(cx, func() []backend.CardItem {
		current := slices.Clone(lookup.Get())
		if card := l.deviceEnvironment.Get(); card != nil {
			for i, q := range queries {
				for _, it := range card.Items {
					if sameDeviceName(it.Title, q.Keyword) {
						it.Title = initial[i].Title
						current[i] = it
						break
					}
				}
			}
		}
		return current
	}, slices.Equal)
	requested := false
	cx.Effect(func() {
		if requested || !selected() || !l.p.loaded.Get() {
			return
		}
		requested = true
		current := items.GetUntracked()
		if !slices.ContainsFunc(current, func(it backend.CardItem) bool { return it.URL == "" }) {
			return
		}
		background(a, func(ctx context.Context) ([]backend.CardItem, error) {
			found := slices.Clone(initial)
			for i, q := range queries {
				if current[i].URL != "" {
					found[i] = current[i]
					continue
				}
				page, err := a.backend.Search(ctx, q)
				if err != nil {
					continue
				}
				for _, entry := range page.Entries {
					if sameDeviceName(entry.Title, q.Keyword) && (entry.Kind == backend.EntryProduct || strings.HasPrefix(entry.ID, "topic:")) {
						found[i] = catalogItem(entry)
						found[i].Title = initial[i].Title
						break
					}
				}
			}
			return found, nil
		}, func(found []backend.CardItem, _ error) { lookup.Set(found) })
	})
	cards := []core.Node{a.deviceRatingTile(cx, func() backend.CardItem { return items.Get()[0] }, "我的机型", &symbols.Devices, true)}
	if len(initial) > 1 {
		cards = append(cards, a.deviceRatingTile(cx, func() backend.CardItem { return items.Get()[1] }, "我的系统", &symbols.Settings, false))
	}
	return row(core.Style{Padding: core.All(8), Gap: 8, AlignItems: core.AlignStretch}, "bg-surface-container-low", cards...)
}

func (a *App) deviceRatingTile(cx *core.Scope, item func() backend.CardItem, title string, glyph *symbol, product bool) core.Node {
	count := func() string {
		r := item().Rating
		if product && r.OwnerCount != "" && r.OwnerCount != "0" {
			return r.OwnerCount + "机主点评"
		}
		if r.Count != "" {
			return r.Count + "人点评"
		}
		return ""
	}
	identity := tap("", func() { a.openCardItem(item()) },
		core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter, CornerRadius: 8}, "",
		core.View{Style: core.Style{Width: core.Pt(44), Height: core.Pt(44), FlexShrink: 0, CornerRadius: 8, Overflow: core.Hidden,
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			Children: []core.Node{
				a.pic(cx, func() string { return item().Pic }, core.Contain, "", fillAbs),
				core.Show{When: func() bool { return item().Pic == "" }, Then: func(*core.Scope) core.Node {
					return icon(glyph, 24, "text-on-surface-variant")
				}},
			}},
		column(fill.Merge(core.Style{Gap: 2, MinWidth: core.Pt(0)}), "",
			column(core.Style{Height: core.Pt(36), JustifyContent: core.JustifyCenter}, "",
				core.Text{ContentFunc: func() string { return item().Title }, MaxLines: 2, Class: "text-on-surface",
					Style: textStyle(textSecondary, 600).Merge(core.Style{LineHeight: 18})}),
			dynamicShape(cx, func() string { return item().Rating.Score }, func(_ *core.Scope, score string) core.Node {
				if score == "" {
					return label("暂无评分", textCaption, 400, "text-on-surface-variant")
				}
				n, _ := strconv.ParseFloat(score, 64)
				return row(core.Style{Gap: 4, AlignItems: core.AlignCenter}, "",
					stars(n/2, 10), label(score, textCaption, 600, "text-star"))
			})))
	identity.LabelFunc = func() string { return item().Title }
	identity.Disabled = func() bool { return item().URL == "" }
	return column(fill.Merge(core.Style{Padding: core.Edges{Top: 10, Left: 10, Right: 10}, Gap: 8, CornerRadius: 12}), "bg-background",
		label(title, textSecondary, 500, "text-on-surface-variant"),
		identity,
		column(core.Style{}, "",
			core.View{Class: "bg-outline-variant", Style: core.Style{Height: core.Pt(1)}},
			row(core.Style{MinHeight: core.Pt(40), Gap: 2, AlignItems: core.AlignCenter}, "",
				core.Text{ContentFunc: count, Class: "text-on-surface-variant", MaxLines: 1, Style: fill.Merge(textStyle(textCaption, 400))},
				core.Show{When: func() bool { return item().URL != "" }, Then: func(*core.Scope) core.Node {
					return tap("查看"+title+"点评", func() {
						it := item()
						target := backend.ResolveLink(it.URL)
						switch target.Kind {
						case backend.LinkProduct:
							a.push(routeTopicRatings, topicParam(it.Title, target.Param))
						case backend.LinkTopic:
							a.push(routeTopicRatings, topicParam(target.Param, ""))
						}
					}, core.Style{Direction: core.Row, Height: core.Pt(40), AlignItems: core.AlignCenter, CornerRadius: 8}, "",
						label("去点评", textCaption, 500, "text-primary"), icon(&symbols.ChevronRight, 14, "text-primary"))
				}},
			)))
}

func sameDeviceName(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), ""), strings.Join(strings.Fields(b), ""))
}

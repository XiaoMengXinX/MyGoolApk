package app

import (
	"context"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

const catalogColumns = 3

// productCatalog keeps the category rail visible as the selected product grid scrolls.
func (a *App) productCatalog(cx *core.Scope, root *pageList, spec listSpec) core.Node {
	root.p.ensure()
	picked := core.NewSignal(cx, 0)
	followed := core.NewSignalFunc(cx, map[string]bool{}, nil)
	pending := core.NewSignalFunc(cx, map[string]bool{}, nil)
	toggle := func(e backend.PageEntry, isFollowed bool) {
		if pending.GetUntracked()[e.ID] || !a.requireLogin() {
			return
		}
		pending.GetUntracked()[e.ID] = true
		pending.Set(pending.GetUntracked())
		backgroundErr(a, func(ctx context.Context) error {
			return a.backend.SetTopicFollow(ctx, "", e.ID, !isFollowed)
		}, func(err error) {
			delete(pending.GetUntracked(), e.ID)
			pending.Set(pending.GetUntracked())
			if err != nil {
				a.notice(err.Error())
				return
			}
			followed.GetUntracked()[e.ID] = !isFollowed
			followed.Set(followed.GetUntracked())
		})
	}
	lists := map[string]*pageList{}
	return core.Dynamic(func(body *core.Scope) core.Node {
		if !root.p.loaded.Get() {
			if err := root.p.err.Get(); err != "" {
				return statusBlock("加载失败", err, "重试", root.p.reload)
			}
			return column(fill.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}), "", spinner())
		}
		categories := root.p.items.Get()
		if len(categories) == 0 {
			return statusBlock("暂无分类", "", "重试", root.p.reload)
		}
		active := func() int { return min(picked.Get(), len(categories)-1) }
		rail := make([]core.Node, len(categories))
		for i, e := range categories {
			rail[i] = categoryTab(e.Title, func() bool { return active() == i }, func() { picked.Set(i) })
		}
		content := core.Dynamic(func(body *core.Scope) core.Node {
			e := categories[active()]
			l := lists[e.URL]
			if l == nil {
				l = a.newPageList(cx, e.URL, e.Title, nil, 0, "")
				lists[e.URL] = l
			}
			root.catalogSelection = l
			spec.empty, spec.bottom = "暂无数码产品", 8
			return pagerRows(body, l.p, catalogRows, func(r catalogRow) string { return entryKey(r.entries[0]) }, spec,
				func(body *core.Scope, get func() catalogRow, _ core.ListRow) core.Node {
					return dynamicShape(body, func() bool { return get().entries[0].Kind == backend.EntryProduct }, func(body *core.Scope, products bool) core.Node {
						if products {
							return a.catalogGridRow(body, get, followed, pending, toggle)
						}
						return a.entryView(body, func() backend.PageEntry { return get().entries[0] }, l, nil)
					})
				})
		})
		return row(fill, "",
			core.Scroll{HideIndicator: true, Class: "bg-background", Style: core.Style{Width: core.Pt(86), FlexShrink: 0},
				Children: []core.Node{column(core.Style{Gap: 4, Padding: core.Symmetric(6, 6), PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}}, "", rail...)}},
			column(fill, "bg-surface-container-low", content))
	})
}

type catalogRow struct {
	entries []backend.PageEntry
}

func catalogRows(entries []backend.PageEntry) []catalogRow {
	var rows []catalogRow
	for i := 0; i < len(entries); {
		end := i + 1
		if entries[i].Kind == backend.EntryProduct {
			for end < len(entries) && end-i < catalogColumns && entries[end].Kind == backend.EntryProduct {
				end++
			}
		}
		rows = append(rows, catalogRow{entries: entries[i:end]})
		i = end
	}
	return rows
}

func (a *App) catalogGridRow(cx *core.Scope, get func() catalogRow, followed, pending *core.Signal[map[string]bool], toggle func(backend.PageEntry, bool)) core.Node {
	cells := make([]core.Node, catalogColumns)
	for i := range cells {
		frame := fill.Merge(core.Style{MinWidth: core.Pt(0)})
		cells[i] = dynamicShape(cx, func() bool { return i < len(get().entries) }, func(cx *core.Scope, present bool) core.Node {
			if !present {
				return core.View{Style: frame}
			}
			item := func() backend.PageEntry { return get().entries[i] }
			isFollowed := func() bool {
				e := item()
				if value, ok := followed.Get()[e.ID]; ok {
					return value
				}
				return e.Followed
			}
			button := tap("", func() { toggle(item(), isFollowed()) }, core.Style{
				Height: core.Pt(30), MinWidth: core.Pt(52), Padding: core.Symmetric(0, 6), AlignItems: core.AlignCenter,
				JustifyContent: core.JustifyCenter, CornerRadius: 15,
			}, "", core.Text{ContentFunc: func() string {
				if isFollowed() {
					return "已关注"
				}
				return "关注"
			}, MaxLines: 1, Style: textStyle(textCaption, 500)})
			button.LabelFunc = func() string {
				if isFollowed() {
					return "取消关注" + item().Title
				}
				return "关注" + item().Title
			}
			button.ClassFunc = func() string {
				if isFollowed() {
					return "bg-surface-container-high text-on-surface-variant"
				}
				return "bg-primary text-on-primary"
			}
			button.Disabled = func() bool { return pending.Get()[item().ID] }
			product := tap("", func() { a.openCardItem(catalogItem(item())) }, core.Style{
				AlignItems: core.AlignCenter, AlignSelf: core.AlignStretch, Gap: 4, Padding: core.Symmetric(0, 4), CornerRadius: 8,
			}, "",
				a.digitalPicture(cx, func() string { return item().LogoURL }, 54, 54),
				core.Text{ContentFunc: func() string { return item().Title }, MaxLines: 2, Class: "text-on-surface",
					Style: textStyle(textCaption, 500).Merge(core.Style{Height: core.Pt(36), TextAlign: core.TextAlignCenter})},
				heatCount(func() string { return item().Hot }),
			)
			product.LabelFunc = func() string { return item().Title }
			return column(frame.Merge(core.Style{CornerRadius: 12, Padding: core.Symmetric(8, 2), Gap: 6, AlignItems: core.AlignCenter}), "bg-background",
				product, button)
		})
	}
	return row(core.Style{Gap: 5, Padding: core.Symmetric(4, 5), AlignItems: core.AlignStretch}, "bg-surface-container-low", cells...)
}

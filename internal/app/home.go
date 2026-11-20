package app

import (
	"github.com/nsgo-dev/nsgo/core"
)

// topBarHeight is the Material 3 small top app bar, as Google Play draws it.
const topBarHeight core.Points = 56

// topBar is the strip above a root page: it keeps clear of the status bar
// and holds a 56dp row.
func topBar(content ...core.Node) core.Node {
	return column(core.Style{FlexShrink: 0, PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea}}, "bg-background",
		row(core.Style{Height: core.Pt(topBarHeight), Padding: core.Edges{Right: 4, Left: 16}, Gap: 8, AlignItems: core.AlignCenter}, "", content...))
}

// iconAction is a 48dp icon button of a top bar.
func iconAction(glyph *symbol, name string, fn func()) core.Node {
	return tap(name, fn, core.Style{Width: core.Pt(48), Height: core.Pt(48), CornerRadius: 24, AlignItems: core.AlignCenter,
		JustifyContent: core.JustifyCenter, FlexShrink: 0, Overflow: core.Hidden}, "", icon(glyph, 24, "text-on-surface-variant"))
}

func (a *App) homePage(cx *core.Scope) core.Node {
	return column(fill, "bg-background",
		a.homeTopBar(cx),
		a.home.view(cx, a.topics.view),
	)
}

func (a *App) homeTopBar(cx *core.Scope) core.Node {
	return topBar(
		tap("账号", func() { a.tab.Set(3) }, core.Style{Width: core.Pt(40), Height: core.Pt(40), CornerRadius: 20, AlignItems: core.AlignCenter,
			JustifyContent: core.JustifyCenter, FlexShrink: 0, Overflow: core.Hidden}, "", a.accountAvatar(cx, 32)),
		tap("搜索", a.openSearch, fill.Merge(core.Style{Direction: core.Row, Height: core.Pt(48), CornerRadius: 24, Padding: core.Symmetric(0, 16),
			Gap: 12, AlignItems: core.AlignCenter, Overflow: core.Hidden}), "bg-surface-container-high",
			icon(&symbols.Search, 24, "text-on-surface-variant"),
			core.Text{Content: "搜索动态和用户", MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textBody, 400)}),
		iconAction(&symbols.Add, "发帖", a.openCompose),
	)
}

// pageHeader is the top bar of the digital and discovery pages.
func (a *App) rootTopBar(title string) core.Node {
	return topBar(
		core.Text{Content: title, Class: "text-on-surface", Style: fill.Merge(textStyle(textTitle, 400))},
		iconAction(&symbols.Search, "搜索", a.openSearch),
	)
}

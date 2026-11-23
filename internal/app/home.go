package app

import (
	"github.com/nsgo-dev/nsgo/core"
)

// topBarHeight is the Material 3 small top app bar, as Google Play draws it.
const topBarHeight core.Points = 56

// topBar is the strip above a page: it keeps clear of the status bar and
// holds a 56dp row.
func (a *App) topBar(content ...core.Node) core.Node {
	return a.underStatusBar(
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
	return a.topBar(
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
	return a.topBar(
		core.Text{Content: title, Class: "text-on-surface", Style: fill.Merge(textStyle(textTitle, 400))},
		iconAction(&symbols.Search, "搜索", a.openSearch),
	)
}

// underStatusBar is the strip at the top of a full-screen page, which keeps
// its children clear of the window's status bar. It reads the window's safe
// area itself: a screen pushed on the stack reports no inset at its top.
func (a *App) underStatusBar(children ...core.Node) core.Node {
	env := a.cx.Env()
	return core.View{Class: "bg-background", Style: core.Style{FlexShrink: 0},
		StyleFunc: func() core.Style { return core.Style{Padding: core.Edges{Top: env.SafeArea().Top}} },
		Children:  children}
}

func titled(s string) func() string { return func() string { return s } }

// screen is a page pushed over the destinations: a small top app bar with
// the way back, the title and what trails it, over body. The bar is the
// app's, 56dp high as Google Play's is, so every page has the same one.
func (a *App) screen(title func() string, trailing core.Node, body ...core.Node) core.Node {
	bar := []core.Node{
		iconAction(&symbols.Back, "返回", func() { a.nav.Pop() }),
		core.Text{ContentFunc: title, MaxLines: 1, Class: "text-on-surface", Style: fill.Merge(textStyle(textTitle, 400))},
	}
	if trailing != nil {
		bar = append(bar, trailing)
	}
	top := a.underStatusBar(
		row(core.Style{Height: core.Pt(topBarHeight), Padding: core.Edges{Right: 12, Left: 4}, Gap: 4, AlignItems: core.AlignCenter}, "", bar...))
	return column(fill, "bg-background", append([]core.Node{top}, body...)...)
}

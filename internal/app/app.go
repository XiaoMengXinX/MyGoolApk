package app

import (
	"path/filepath"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"

	"app/internal/backend"
)

// Root builds the app: its backend, from newBackend, and its pages.
func Root(newBackend func(dir string) (backend.Backend, error)) func(cx *core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		cx.SetTheme(newTheme())
		b, err := newBackend(cx.DataDir())
		if err != nil {
			panic(err)
		}
		a := &App{backend: b, nav: base.NewNav(cx, route{}), images: newImageStore(filepath.Join(cx.CacheDir(), "img"), b.UserAgent())}
		return a.view(cx)
	}
}

type route struct{ kind, id string }

// App owns the state the pages share.
type App struct {
	backend backend.Backend
	images  *imageStore
	nav     *base.Nav[route]
	tab     *core.Signal[int]

	home, digital, discovery *tabGroup
}

func (a *App) view(cx *core.Scope) core.Node {
	a.tab = core.NewSignal(cx, 0)
	a.home = a.newTabGroup(cx, homeTabs, 2)
	a.digital = a.newTabGroup(cx, digitalTabs, 1)
	a.discovery = a.newTabGroup(cx, discoveryTabs, 0)
	groupTab := func(title string, glyph *symbol, content func(cx *core.Scope) core.Node) ui.ShellTab {
		return ui.ShellTab{Text: title, Icon: glyph.SVG, Content: content}
	}
	topics := func(cx *core.Scope) core.Node { return statusBlock("话题", "", "", nil) }
	tabs := []ui.ShellTab{
		groupTab("首页", &symbols.Home, a.homePage),
		groupTab("数码", &symbols.Devices, func(cx *core.Scope) core.Node {
			return column(fill, "bg-background", a.rootTopBar("数码"), a.digital.view(cx, topics))
		}),
		groupTab("发现", &symbols.Explore, func(cx *core.Scope) core.Node {
			return column(fill, "bg-background", a.rootTopBar("发现"), a.discovery.view(cx, topics))
		}),
		groupTab("我的", &symbols.Person, func(cx *core.Scope) core.Node {
			return core.Text{Content: "我的", Style: textStyle(textTitle, 500), Class: "text-on-surface p-4"}
		}),
	}
	return a.nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, r route, _ func() int) base.NavPage {
		if r.kind != "" {
			return a.page(cx, r)
		}
		return base.NavPage{HeaderHidden: true, Class: "bg-background", Children: []core.Node{
			ui.TabShell{Tabs: tabs, Selected: a.tab.Get, OnChange: a.tab.Set, Label: "酷安"},
		}}
	})
}

// page builds the page of a route pushed over the tabs.
func (a *App) page(cx *core.Scope, r route) base.NavPage {
	return base.NavPage{Title: r.kind, Class: "bg-background", Children: []core.Node{core.Text{Content: r.kind + " " + r.id, Class: "text-on-surface p-4"}}}
}

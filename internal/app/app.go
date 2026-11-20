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
		a := &App{
			cx: cx, dir: cx.DataDir(), backend: b, nav: base.NewNav(cx, route{}),
			images:  newImageStore(filepath.Join(cx.CacheDir(), "img"), b.UserAgent()),
			seeds:   make(map[string]backend.Feed),
			viewers: make(map[string]*viewer),
			local:   loadLocalState(cx.DataDir()),
			toasts:  ui.NewToaster(cx),
		}
		a.searchHistory = core.NewSignalFunc(cx, []string(nil), nil)
		a.history = core.NewSignalFunc(cx, a.local.History, nil)
		a.topics = a.newTopicHome(cx)
		a.applyAppearance()
		cx.OnOpenLinks(func(links []string) {
			for _, l := range links {
				a.openLink(l)
			}
		})
		a.newAccountState(cx)
		a.messages = a.newMessageCenter(cx)
		return a.view(cx)
	}
}

type route struct{ kind, id string }

// App owns the state the pages share.
type App struct {
	cx            *core.Scope
	dir           string
	backend       backend.Backend
	images        *imageStore
	nav           *base.Nav[route]
	tab           *core.Signal[int]
	seeds         map[string]backend.Feed // posts a card shows, until the page loads the full post
	local         localState
	toasts        *ui.Toaster
	account       accountState
	messages      *messageCenter
	viewers       map[string]*viewer
	searchHistory *core.Signal[[]string]
	history       *core.Signal[[]historyEntry]
	topics        *topicHome
	viewerSeq     int
	loginOpening  bool

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
	topics := a.topics.view
	tabs := []ui.ShellTab{
		groupTab("首页", &symbols.Home, a.homePage),
		groupTab("数码", &symbols.Devices, func(cx *core.Scope) core.Node {
			return column(fill, "bg-background", a.rootTopBar("数码"), a.digital.view(cx, topics))
		}),
		groupTab("发现", &symbols.Explore, func(cx *core.Scope) core.Node {
			return column(fill, "bg-background", a.rootTopBar("发现"), a.discovery.view(cx, topics))
		}),
		groupTab("我的", &symbols.Person, a.mePage),
	}
	stack := a.nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, r route, _ func() int) base.NavPage {
		if r.kind != "" {
			return a.page(cx, r)
		}
		return base.NavPage{HeaderHidden: true, Class: "bg-background", Children: []core.Node{
			ui.TabShell{Tabs: tabs, Selected: a.tab.Get, OnChange: a.tab.Set, Label: "酷安"},
		}}
	})
	return core.Group{stack, ui.Toasts{Toaster: a.toasts, Placement: ui.ToastBottom, Inset: 72}}
}

// page builds the page of a route pushed over the tabs.
func (a *App) page(cx *core.Scope, r route) base.NavPage {
	switch r.kind {
	case routeFeed:
		return a.detailPage(cx, r.id)
	case routeLogin:
		return a.loginPage(cx)
	case routeUser:
		return a.userPage(cx, r.id)
	case routeTopic:
		return a.topicPage(cx, r.id)
	case routeList:
		return a.listPage(cx, r.id)
	case routeCompose:
		return a.composePage(cx)
	case routeHistory:
		return a.historyPage(cx)
	case routeSettings:
		return a.settingsPage(cx)
	case routeDevice:
		return a.devicePage(cx)
	case routeSearch:
		return a.searchPage(cx)
	case routeImages:
		return a.viewerPage(cx, r.id)
	case routeMessages:
		return base.NavPage{Title: "消息", HeaderStyleFunc: a.headerStyle(cx), Class: "bg-background", Children: []core.Node{a.messagesPage(cx)}}
	}
	return base.NavPage{Title: r.kind, HeaderStyleFunc: a.headerStyle(cx), Class: "bg-background",
		Children: []core.Node{core.Text{Content: r.kind + " " + r.id, Class: "text-on-surface p-4"}}}
}

// headerStyle colors the system header from the theme.
func (a *App) headerStyle(cx *core.Scope) func() core.Style {
	return func() core.Style {
		p := cx.Palette()
		return core.Style{Background: p.Background, Color: p.Text, TintColor: p.Text}
	}
}

package app

import (
	"path/filepath"

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
			cx: cx, dir: cx.DataDir(), backend: b,
			images:  newImageStore(filepath.Join(cx.CacheDir(), "img"), b.UserAgent()),
			seeds:   make(map[string]backend.Feed),
			viewers: make(map[string]*viewer),
			local:   loadLocalState(cx.DataDir()),
			toasts:  ui.NewToaster(cx),
		}
		a.nav = a.newPageStack(cx)
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

// App owns the state the pages share.
type App struct {
	cx            *core.Scope
	dir           string
	backend       backend.Backend
	images        *imageStore
	nav           *pageStack
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
	topics := a.topics.view
	dests := []destination{
		{"首页", &symbols.Home, &symbols.HomeFill, a.homePage},
		{"数码", &symbols.Devices, &symbols.DevicesFill, func(cx *core.Scope) core.Node {
			return column(fill, "bg-background", a.rootTopBar("数码"), a.digital.view(cx, topics))
		}},
		{"发现", &symbols.Explore, &symbols.ExploreFill, func(cx *core.Scope) core.Node {
			return column(fill, "bg-background", a.rootTopBar("发现"), a.discovery.view(cx, topics))
		}},
		{"我的", &symbols.Person, &symbols.PersonFill, a.mePage},
	}
	cx.OnBackPress(a.backFromRoot)
	stack := a.nav.view(a.shell(cx, dests), a.page)
	return core.Group{stack, ui.Toasts{Toaster: a.toasts, Placement: ui.ToastBottom, Inset: 72}}
}

// page builds the page of a route pushed over the tabs.
func (a *App) page(cx *core.Scope, r route) core.Node {
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
		return a.screen(titled("消息"), nil, a.messagesPage(cx))
	}
	return a.screen(titled(r.kind), nil)
}

// headlineTab is the home tab the app opens on, and back leads to.
const headlineTab = 2

// backFromRoot makes back leave the app from the headline tab only: from any
// other tab or category it first shows that one. A pushed page pops first.
func (a *App) backFromRoot() bool {
	if a.nav.Depth() > 0 {
		return a.nav.Pop()
	}
	if a.tab.GetUntracked() != 0 {
		a.tab.Set(0)
		a.home.selectTab(headlineTab)
		return true
	}
	if a.home.selected.GetUntracked() != headlineTab {
		a.home.selectTab(headlineTab)
		return true
	}
	return false
}

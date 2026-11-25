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
			images: newImageStore(filepath.Join(cx.CacheDir(), "img"), b.UserAgent()),
			seeds:  make(map[string]backend.Feed),
			local:  loadLocalState(cx.DataDir()),
			toasts: ui.NewToaster(cx),
		}
		a.nav = a.newPageStack(cx)
		a.viewing = core.NewSignalFunc(cx, (*viewer)(nil), nil)
		a.viewerShown = core.NewSignal(cx, false)
		a.searchHistory = core.NewSignalFunc(cx, a.local.Searches, nil)
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
	lists         []*pageList             // the server lists shown, which a post's page keeps current
	seeds         map[string]backend.Feed // posts a card shows, until the page loads the full post
	local         localState
	toasts        *ui.Toaster
	account       accountState
	messages      *messageCenter
	viewing       *core.Signal[*viewer] // the open viewer's pictures, nil when it is closed
	viewerShown   *core.Signal[bool]
	searchHistory *core.Signal[[]string]
	history       *core.Signal[[]historyEntry]
	topics        *topicHome
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
		{title: "首页", outline: &symbols.Home, filled: &symbols.HomeFill, content: a.homePage},
		{title: "数码", outline: &symbols.Devices, filled: &symbols.DevicesFill, content: func(cx *core.Scope) core.Node {
			return column(fill, "bg-background", a.rootTopBar("数码"), a.digital.view(cx, topics))
		}},
		{title: "发现", outline: &symbols.Explore, filled: &symbols.ExploreFill, content: func(cx *core.Scope) core.Node {
			return column(fill, "bg-background", a.rootTopBar("发现"), a.discovery.view(cx, topics))
		}},
		{title: "我的", outline: &symbols.Person, filled: &symbols.PersonFill, content: a.mePage,
			unread: func() int { return a.account.unread.Get().Total() }},
	}
	stack := a.nav.view(cx, func(cx *core.Scope) core.Node {
		return core.Group{a.rootBack(), a.shell(cx, dests)}
	}, a.page)
	return core.Group{stack, a.viewerLayer(), ui.Toasts{Toaster: a.toasts, Placement: ui.ToastBottom, Inset: 72}}
}

// page builds the page of a route pushed over the tabs.
func (a *App) page(cx *core.Scope, r route) core.Node {
	switch r.kind {
	case routeFeed:
		return a.detailPage(cx, r.id)
	case routeLogin:
		return a.loginPage(cx)
	case routeWeb:
		return a.webPage(cx, r.id)
	case routeUser:
		return a.userPage(cx, r.id)
	case routeFollows:
		return a.peoplePage(cx, r.id, backend.UserFollowing)
	case routeFans:
		return a.peoplePage(cx, r.id, backend.UserFans)
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
		return a.searchPage(cx, r.id)
	case routeChat:
		return a.chatPage(cx, r.id)
	case routeMessages:
		return a.screen(titled("通知中心"), nil, a.messagesPage(cx))
	case routeNotices:
		return a.noticesPage(cx, r.id)
	}
	return a.screen(titled(r.kind), nil)
}

// rootBack takes back while no page is pushed, and on Android for the pages
// too, which are the app's own layers there. iOS's stack pops its pages
// itself.
func (a *App) rootBack() core.Node {
	return core.Show{
		When: func() bool { return a.nav.android() || a.nav.Depth() == 0 },
		Then: func(cx *core.Scope) core.Node {
			cx.OnBackPress(a.backFromRoot)
			return core.Group{}
		},
	}
}

// headlineTab is the home tab the app opens on, and back leads to.
const headlineTab = 2

// backFromRoot makes back leave the app from the headline tab only: from any
// other tab or category it first shows that one.
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

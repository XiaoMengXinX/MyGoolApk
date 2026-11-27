package app

import (
	"path/filepath"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"

	"app/internal/backend"
)

// Root builds the app: its backend, from newBackend, and its pages.
func Root(newBackend func(dir string, profile backend.DeviceProfile) (backend.Backend, error)) func(cx *core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		cx.SetTheme(newTheme())
		hoverable = cx.Env().Desktop()
		local := loadLocalState(cx.DataDir())
		var profile backend.DeviceProfile
		if cx.Env().OS() == core.OSIOS && !local.RandomDeviceModel {
			profile = physicalDevice(cx.Env())
		}
		b, err := newBackend(cx.DataDir(), profile)
		if err != nil {
			panic(err)
		}
		a := &App{
			cx: cx, dir: cx.DataDir(), backend: b,
			images:        newImageStore(filepath.Join(cx.CacheDir(), "img"), b.UserAgent()),
			seeds:         make(map[string]backend.Feed),
			local:         local,
			toasts:        ui.NewToaster(cx),
			usesRealModel: profile.HardwareID != "",
		}
		a.nav = a.newPageStack(cx)
		a.size = a.newSize(cx)
		a.viewing = core.NewSignalFunc(cx, (*viewer)(nil), nil)
		a.viewerShown = core.NewSignal(cx, false)
		a.searchHistory = core.NewSignalFunc(cx, a.local.Searches, nil)
		a.history = core.NewSignalFunc(cx, a.local.History, nil)
		a.aiSummaryEnabled = core.NewSignal(cx, a.local.AISummaryEnabled)
		a.officialAISummary = core.NewSignal(cx, a.local.OfficialAISummary)
		a.officialAIRawMode = core.NewSignal(cx, a.local.OfficialAIRawMode)
		a.topics = a.newTopicHome(cx)
		a.applyAppearance()
		a.placeWindow(cx)
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
	cx                *core.Scope
	dir               string
	backend           backend.Backend
	images            *imageStore
	nav               *pageStack
	tab               *core.Signal[int]
	size              *core.Memo[sizeClass]
	lists             []*pageList             // the server lists shown, which a post's page keeps current
	seeds             map[string]backend.Feed // posts a card shows, until the page loads the full post
	local             localState
	toasts            *ui.Toaster
	account           accountState
	messages          *messageCenter
	viewing           *core.Signal[*viewer] // the open viewer's pictures, nil when it is closed
	viewerShown       *core.Signal[bool]
	searchHistory     *core.Signal[[]string]
	history           *core.Signal[[]historyEntry]
	aiSummaryEnabled  *core.Signal[bool]
	officialAISummary *core.Signal[bool]
	officialAIRawMode *core.Signal[bool]
	topics            *topicHome
	loginOpening      bool
	usesRealModel     bool
	// lastPress is the bar's last press on a destination, which a second
	// press soon after makes a double press.
	lastPress struct {
		dest int
		at   time.Time
	}

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
	// A window wider than a phone's has the destinations on a rail at its left.
	body := row(fill, "bg-background",
		core.Show{When: a.wide, Then: func(cx *core.Scope) core.Node { return a.rail(dests) }},
		// The stack keeps to its room: a layer shifting under a page goes off
		// its left edge, which in a medium window is the rail's.
		core.View{Style: fill.Merge(core.Style{Overflow: core.Hidden}), Children: []core.Node{stack}})
	return core.Group{body, a.viewerLayer(), ui.Toasts{Toaster: a.toasts, Placement: ui.ToastBottom, Inset: 72}}
}

// page builds the page of a route pushed over the tabs.
func (a *App) page(cx *core.Scope, r route) core.Node {
	scrolledUnder.Provide(cx, core.NewSignal(cx, false))
	switch r.kind {
	case routeFeed:
		return a.detailPage(cx, r.id)
	case routeEvent:
		return a.eventPage(cx, r.id)
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
	case routeTopic, routeTopicRatings:
		return a.topicPage(cx, r.id, r.kind == routeTopicRatings)
	case routeRatingData:
		return a.ratingDataPage(cx, r.topic)
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
		return a.messagesPage(cx)
	case routeNotices:
		return a.noticesPage(cx, r.id)
	}
	return a.screen(titled(r.kind), nil)
}

// rootBack takes back while no page is pushed, and for the pages too where
// they are the app's own layers. An iPhone's stack pops its pages itself.
func (a *App) rootBack() core.Node {
	return core.Show{
		When: func() bool { return !a.nav.native() || a.nav.Depth() == 0 },
		Then: func(cx *core.Scope) core.Node {
			// A predictive back gesture moves the page back pops; from the
			// headline tab back leaves the app, which Android previews.
			cx.OnBackPressWith(a.backFromRoot, core.BackOptions{Preview: a.nav.preview, Previewed: a.nav.dropTop, Takes: a.takesBack})
			return core.Group{}
		},
	}
}

// takesBack reports whether backFromRoot takes back now: a page to pop, or
// another tab or destination to go to.
func (a *App) takesBack() bool {
	return a.nav.Depth() > 0 || a.tab.Get() != 0 || a.home.selected.Get() != headlineTab
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

package app

import (
	"encoding/base64"
	"net/url"
	"strings"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// Routes are pages pushed over the tabs: kind names the page and id what it
// shows.
const (
	routeFeed     = "feed"
	routeEvent    = "event"
	routeUser     = "user"
	routeFollows  = "follows" // the accounts a user follows, by uid
	routeFans     = "fans"
	routeTopic    = "topic"
	routeLogin    = "login"
	routeWeb      = "web" // a page of the site shown in the app, by base64url of "title\x00url"
	routeSearch   = "search"
	routeMessages = "messages"
	routeNotices  = "notices" // a list of the message center, by its entry
	routeChat     = "chat"    // a private-message thread, by base64url of "ukey\x00uid\x00name"
	routeHistory  = "history"
	routeSettings = "settings"
)

func (a *App) push(kind, id string) { a.nav.Push(route{kind, id}) }

// openFeed shows a post, starting from what its card already shows.
func (a *App) openFeed(f backend.Feed) {
	if f.ID == "" {
		return
	}
	if f.Author.ID != "" && f.Kind != backend.PostComment {
		a.seeds[f.ID] = f
	}
	a.push(routeFeed, f.ID)
}

func (a *App) openUser(u backend.Author) {
	if u.ID != "" {
		a.push(routeUser, u.ID)
	}
}

func (a *App) openSearch() { a.push(routeSearch, "") }

// searchFor shows the results of a keyword.
func (a *App) searchFor(keyword string) { a.push(routeSearch, keyword) }

func (a *App) openHistory()  { a.push(routeHistory, "") }
func (a *App) openSettings() { a.push(routeSettings, "") }

// showsInApp reports whether raw is a page of the site itself, which the
// app shows in a web view as the official client does.
func showsInApp(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && backend.IsCoolapkHost(u.Hostname())
}

// openWebPage shows a page of the site in the app, under title, or under the
// page's own title without one.
func (a *App) openWebPage(title, raw string) {
	a.push(routeWeb, base64.RawURLEncoding.EncodeToString([]byte(title+"\x00"+raw)))
}

// webPage is a page of the site in a web view that identifies as the app.
func (a *App) webPage(cx *core.Scope, param string) core.Node {
	raw, _ := base64.RawURLEncoding.DecodeString(param)
	named, src, _ := strings.Cut(string(raw), "\x00")
	title := core.NewSignal(cx, named)
	body, back := a.siteContent(cx, src, func(value string) {
		if named == "" && value != "" {
			title.Set(value)
		}
	}, nil, nil)
	return a.screenBack(func() {
		if !back() {
			a.nav.Pop()
		}
	}, title.Get, nil, body)
}

// siteContent is shared by standalone site pages and web tabs inside a topic.
func (a *App) siteContent(cx *core.Scope, src string, title func(string), active func() bool,
	loaded func(*core.Ref)) (core.Node, func() bool) {
	loading := core.NewSignal(cx, true)
	failed := core.NewSignal(cx, "")
	var web core.Ref
	deep := false // the view has pages to go back to
	depth := a.nav.Depth()
	// Back leaves the pages the view went on to before it leaves the view,
	// unless a page it opened covers it: that page takes back.
	back := func() bool {
		if !deep || a.nav.Depth() != depth || active != nil && !active() {
			return false
		}
		web.GoBack()
		return true
	}
	cx.OnBackPress(back)
	// A link to a post, a product or another page the app has opens there,
	// as in the official client, and the view stays on its page.
	native := func(n core.WebNavigation) (backend.LinkTarget, bool) {
		t := backend.ResolveLink(n.URL)
		return t, n.URL != src && t.Kind != backend.LinkExternal && (t.Kind != backend.LinkDrawing || active != nil)
	}
	leave := func(n core.WebNavigation) {
		deep = n.CanGoBack
		failed.Set("")
		t, ok := native(n)
		loading.Set(!ok)
		if ok {
			web.StopLoading()
			if t.Kind == backend.LinkDrawing {
				// Android may clear the inline document as the drawing starts to load.
				web.Load(core.WebSource{Src: src})
			}
			a.openInApp(t)
		}
	}
	// The view may have gone to the link by the time it hears of it; it
	// returns to the page that held the link.
	settle := func(n core.WebNavigation) {
		deep = n.CanGoBack
		loading.Set(n.Loading)
		if _, ok := native(n); ok && n.CanGoBack {
			web.GoBack()
		} else if title != nil {
			title(n.Title)
		}
	}
	return column(fill.Merge(core.Style{PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}}), "",
		core.WebView{Ref: &web, Source: core.WebSource{Src: src}, UserAgent: a.backend.UserAgent(), InjectedJavaScript: secureImages, Style: fill,
			OnLoadStart: leave, OnLoad: func(n core.WebNavigation) {
				settle(n)
				if loaded != nil && n.URL == src {
					loaded(&web)
				}
			}, OnNavigationStateChange: settle,
			OnOpenWindow: func(req core.WebWindowRequest) { a.openURL(req.URL) },
			OnError: func(err error) {
				loading.Set(false)
				failed.Set(err.Error())
			}},
		core.Dynamic(func(cx *core.Scope) core.Node {
			if failed.Get() == "" {
				return core.Group{}
			}
			return core.View{Class: "bg-background", Style: fillAbs, Children: []core.Node{
				statusBlock("加载失败", failed.Get(), "重试", func() {
					failed.Set("")
					web.Load(core.WebSource{Src: src})
				}),
			}}
		}),
		// A page still on its way says so over its top edge.
		core.Dynamic(func(cx *core.Scope) core.Node {
			if !loading.Get() {
				return core.Group{}
			}
			return core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(16), Left: core.Pt(0), Right: core.Pt(0), AlignItems: core.AlignCenter},
				PointerEvents: core.PointerEventsNone, Children: []core.Node{spinner()}}
		})), back
}

// secureImages loads over https the pictures a page names by http, which a
// web view leaves out of an https page.
const secureImages = `(function(){
function fix(n){if(n.tagName==='IMG'&&n.src.indexOf('http://')===0){n.src='https://'+n.src.slice(7)}}
function all(r){if(r.querySelectorAll){fix(r);r.querySelectorAll('img').forEach(fix)}}
all(document);
new MutationObserver(function(ms){ms.forEach(function(m){m.addedNodes.forEach(all);if(m.type==='attributes'){fix(m.target)}})})
.observe(document,{subtree:true,childList:true,attributes:true,attributeFilter:['src']});
})();`

// openWeb opens a web page: the site's own in the app, others in the
// browser.
func (a *App) openWeb(raw string) {
	if showsInApp(raw) {
		a.openWebPage("", raw)
		return
	}
	a.cx.OpenURL(raw, nil)
}

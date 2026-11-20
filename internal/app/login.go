package app

import (
	"context"
	"net/url"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"

	coolapk "github.com/XiaoMengXinX/CoolapkApi-Go"
)

// Signing in works the way the Coolapk app does: its login page in a web
// view that identifies as the app, whose uid, username and token cookies
// become the session once the page sets them.
const loginPoll = time.Second

// openLogin clears the cookies of an earlier attempt, which would read as
// this one's success, and shows the login page.
func (a *App) openLogin() {
	if a.loginOpening {
		return
	}
	a.loginOpening = true
	a.cx.ClearWebsiteData(func(err error) {
		a.loginOpening = false
		if err != nil {
			a.notice("无法打开登录页：" + err.Error())
			return
		}
		a.push(routeLogin, "")
	})
}

// loginPage is the web view of the login page. It looks for the session's
// cookies after each page and every second, as some steps finish in the
// page's scripts without a navigation.
func (a *App) loginPage(cx *core.Scope) base.NavPage {
	importing := core.NewSignal(cx, false)
	failed := false
	errText := core.NewSignal(cx, "")
	closed := false
	cx.Cleanup(func() { closed = true })

	check := func() {
		if importing.GetUntracked() || failed || closed {
			return
		}
		importing.Set(true)
		cx.WebCookies(core.WebCookieFilter{URL: "https://account.coolapk.com/"}, func(cookies []core.WebCookie, err error) {
			values := make(map[string]string)
			for _, ck := range cookies {
				if v, uerr := url.QueryUnescape(ck.Value); uerr == nil {
					values[ck.Name] = v
				}
			}
			uid, token := values["uid"], values["token"]
			if err != nil || uid == "" || token == "" {
				importing.Set(false)
				return
			}
			go func() {
				err := a.backend.ImportSession(context.Background(), uid, values["username"], token)
				cx.Post(func() {
					importing.Set(false)
					if err != nil {
						failed = true
						errText.Set("登录失败：" + err.Error())
						return
					}
					a.resetAccount()
					a.loadAccount()
					a.home.reload(tabFollow, tabHeadline)
					a.nav.Pop(1)
				})
			}()
		})
	}
	var poll func()
	poll = func() {
		cx.AfterFunc(loginPoll, func() {
			if closed {
				return
			}
			check()
			poll()
		})
	}
	poll()

	banner := core.Dynamic(func(cx *core.Scope) core.Node {
		if errText.Get() == "" {
			return core.Group{}
		}
		return errorBanner(errText.Get(), func() { errText.Set(""); failed = false }, func() { errText.Set("") })
	})
	progress := core.Dynamic(func(cx *core.Scope) core.Node {
		if !importing.Get() {
			return core.Group{}
		}
		return core.View{Class: "bg-primary", Style: core.Style{Height: core.Pt(3), Width: core.Pct(100)}}
	})
	return base.NavPage{Title: "登录酷安", HeaderStyleFunc: a.headerStyle(cx), Class: "bg-background", Children: []core.Node{
		column(fill, "", progress, banner, core.WebView{
			Source:    core.WebSource{Src: coolapk.LoginURL},
			UserAgent: a.backend.UserAgent(),
			OnLoad:    func(core.WebNavigation) { check() },
			Style:     fill,
		}),
	}}
}

package app

import (
	"context"
	"net/url"
	"time"

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
func (a *App) loginPage(cx *core.Scope) core.Node {
	// importing is true while the session found in the cookies is verified.
	importing := core.NewSignal(cx, false)
	checking, failed := false, false
	errText := core.NewSignal(cx, "")
	closed := false
	cx.Cleanup(func() { closed = true })

	check := func() {
		if checking || failed || closed {
			return
		}
		checking = true
		cx.WebCookies(core.WebCookieFilter{URL: "https://account.coolapk.com/"}, func(cookies []core.WebCookie, err error) {
			values := make(map[string]string)
			for _, ck := range cookies {
				if v, uerr := url.QueryUnescape(ck.Value); uerr == nil {
					values[ck.Name] = v
				}
			}
			uid, token := values["uid"], values["token"]
			if err != nil || uid == "" || token == "" {
				checking = false
				return
			}
			importing.Set(true)
			go func() {
				err := a.backend.ImportSession(context.Background(), uid, values["username"], token)
				cx.Post(func() {
					importing.Set(false)
					if err != nil {
						checking, failed = false, true
						errText.Set("登录失败：" + err.Error())
						return
					}
					// The page is leaving: checking stays set, so the cookies
					// are not imported again while it slides out.
					a.resetAccount()
					a.loadAccount()
					a.home.reload(tabFollow, tabHeadline)
					a.nav.Pop()
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
		// Laid over the page: a row of its own would resize the web view.
		return core.View{Class: "bg-primary", Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Height: core.Pt(3)}}
	})
	return a.screen(titled("登录酷安"), nil,
		column(fill, "", banner, core.WebView{
			Source:    core.WebSource{Src: coolapk.LoginURL},
			UserAgent: a.backend.UserAgent(),
			OnLoad:    func(core.WebNavigation) { check() },
			Style:     fill,
		}, progress))
}

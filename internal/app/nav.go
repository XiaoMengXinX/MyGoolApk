package app

import "app/internal/backend"

// Routes are pages pushed over the tabs: kind names the page and id what it
// shows.
const (
	routeFeed     = "feed"
	routeUser     = "user"
	routeTopic    = "topic"
	routeLogin    = "login"
	routeSearch   = "search"
	routeMessages = "messages"
	routeHistory  = "history"
	routeSettings = "settings"
)

func (a *App) push(kind, id string) { a.nav.Push(route{kind, id}) }

// openFeed shows a post, starting from what its card already shows.
func (a *App) openFeed(f backend.Feed) {
	if f.ID == "" {
		return
	}
	if f.Author.ID != "" {
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

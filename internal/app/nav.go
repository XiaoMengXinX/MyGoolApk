package app

import "app/internal/backend"

// Routes are pages pushed over the tabs: kind names the page and id what it
// shows.
const (
	routeFeed     = "feed"
	routeUser     = "user"
	routeTopic    = "topic"
	routeLogin    = "login"
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

func (a *App) openTopic(name string) { a.push(routeTopic, name) }

// openTarget shows the feed of the topic or product a post is filed under.
func (a *App) openTarget(t *backend.Target) { a.openTopic(t.Title) }

func (a *App) openSearch()  {}
func (a *App) openCompose() {}

// openList shows a server list that a row opened, such as the products of a
// category.
func (a *App) openList(title, url string) {}

func (a *App) openHistory()  { a.push(routeHistory, "") }
func (a *App) openSettings() { a.push(routeSettings, "") }

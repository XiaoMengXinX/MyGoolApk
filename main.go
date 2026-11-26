package main

import (
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/i18n"

	"app/internal/app"
	"app/internal/backend/live"
)

// The app is written in Simplified Chinese: the framework's own words, as
// its updater's, follow it rather than English.
func init() { i18n.SetApp(i18n.NewCatalog("zh-Hans")) }

// A desktop's window opens wide enough for the pages beside the lists, and
// hidden: the app shows it once it is where it was last time.
func main() { nsgo.Run(app.Root(live.New), nsgo.WindowSize(1200, 800), nsgo.StartHidden()) }

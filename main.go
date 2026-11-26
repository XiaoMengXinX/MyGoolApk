package main

import (
	"github.com/nsgo-dev/nsgo"

	"app/internal/app"
	"app/internal/backend/live"
)

// A desktop's window opens wide enough for the pages beside the lists, and
// hidden: the app shows it once it is where it was last time.
func main() { nsgo.Run(app.Root(live.New), nsgo.WindowSize(1200, 800), nsgo.StartHidden()) }

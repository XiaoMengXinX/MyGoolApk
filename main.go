package main

import (
	"github.com/nsgo-dev/nsgo"

	"app/internal/app"
	"app/internal/backend/live"
)

func main() { nsgo.Run(app.Root(live.New)) }

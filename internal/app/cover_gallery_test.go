package app

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"

	"app/internal/backend"
)

func TestFeedPublishingLayoutRebinds(t *testing.T) {
	feed := backend.Feed{ID: "cover", BigCover: true, Content: "布局正文", Images: []backend.Image{{Width: 1132, Height: 1063}, {Width: 1536, Height: 1536}, {Width: 1463, Height: 1463}}}
	var value *core.Signal[*backend.Feed]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		initial := feed
		value = core.NewSignal(cx, &initial)
		get := func() backend.Feed { return *value.Get() }
		a := &App{cx: cx, images: newImageStore(t.TempDir(), "")}
		return core.View{TestID: "body", Children: []core.Node{
			dynamicOnShape(cx, func() feedShape { return shapeOfFeed(get(), 6) }, func(cx *core.Scope, s feedShape) core.Node { return a.feedBody(cx, get, s) })}}
	}, nsgotest.Size(400, 800))
	for _, big := range []bool{true, false, true} {
		next := feed
		next.BigCover = big
		app.Post(func() { value.Set(&next) })
		app.Settle()
		var imageY, textY core.Points
		var imageWidth core.Points
		var visit func(*nsgotest.View, core.Points)
		visit = func(v *nsgotest.View, y core.Points) {
			y += v.Frame.Origin.Y
			if v.Kind == nsgotest.KindImage && imageWidth == 0 {
				imageY, imageWidth = y, v.Frame.Size.Width
			}
			if v.Text == "布局正文" {
				textY = y
			}
			for _, child := range v.Children {
				visit(child, y)
			}
		}
		visit(app.Find("body"), 0)
		if imageWidth <= 0 || (imageY < textY) != big {
			t.Errorf("big cover=%v imageY=%g textY=%g width=%g", big, imageY, textY, imageWidth)
		}
		if big && imageWidth != 400 {
			t.Errorf("cover width=%g, want 400", imageWidth)
		}
		if !big && imageWidth >= 140 {
			t.Errorf("grid image width=%g", imageWidth)
		}
	}
}

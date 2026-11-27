package app

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"

	"app/internal/backend"
)

func TestFeedCardsKeepTextBeforePictures(t *testing.T) {
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
		if imageWidth <= 0 || imageY <= textY {
			t.Errorf("big cover=%v imageY=%g textY=%g width=%g", big, imageY, textY, imageWidth)
		}
		if imageWidth >= 140 {
			t.Errorf("grid image width=%g", imageWidth)
		}
	}
}

func TestBigCoverDetailsFitTheirPictures(t *testing.T) {
	var value *core.Signal[*backend.Feed]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		value = core.NewSignal(cx, &backend.Feed{ID: "cover", BigCover: true, Images: []backend.Image{{Width: 1600, Height: 600}}})
		get := func() backend.Feed { return *value.Get() }
		a := &App{cx: cx, images: newImageStore(t.TempDir(), "")}
		return core.View{TestID: "gallery", Style: core.Style{Padding: core.Symmetric(0, 16)}, Children: []core.Node{
			dynamicOnShape(cx, func() feedShape { return shapeOfFeed(get(), 6) },
				func(cx *core.Scope, s feedShape) core.Node { return a.feedMedia(cx, get, s, true, nil) })}}
	}, nsgotest.Size(360, 800))
	for _, tc := range []struct {
		pictures []backend.Image
		height   core.Points
	}{
		{[]backend.Image{{Width: 1600, Height: 600}}, 135},
		{[]backend.Image{{Width: 900, Height: 1050}}, 420},
		{[]backend.Image{{Width: 1080, Height: 4000}}, 480},
		{[]backend.Image{{Width: 1132, Height: 1063}, {Width: 1536, Height: 1536}}, 360},
		{[]backend.Image{{Width: 1600, Height: 600}, {Width: 1080, Height: 4000}}, 480},
	} {
		next := &backend.Feed{ID: "cover", BigCover: true, Images: tc.pictures}
		app.Post(func() { value.Set(next) })
		app.Settle()
		var visit func(*nsgotest.View)
		count := 0
		visit = func(v *nsgotest.View) {
			if v.Kind == nsgotest.KindImage {
				count++
				if size := v.Frame.Size; size.Width != 360 || size.Height != tc.height {
					t.Errorf("image %dx%d: frame = %+v, want 360x%g", tc.pictures[0].Width, tc.pictures[0].Height, size, tc.height)
				}
			}
			for _, child := range v.Children {
				visit(child)
			}
		}
		visit(app.Find("gallery"))
		if count != len(tc.pictures) {
			t.Errorf("gallery has %d image pages, want %d", count, len(tc.pictures))
		}
	}
}

func TestBigCoverDesktopPagination(t *testing.T) {
	old := hoverable
	hoverable = true
	t.Cleanup(func() { hoverable = old })
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.SetTheme(newTheme())
		a := &App{cx: cx, images: newImageStore(t.TempDir(), "")}
		return core.View{TestID: "gallery", Children: []core.Node{a.bigCoverImages(cx,
			func() []backend.Image { return make([]backend.Image, 3) }, 3)}}
	}, nsgotest.Size(360, 600))
	var findPager func(*nsgotest.View) *nsgotest.View
	findPager = func(v *nsgotest.View) *nsgotest.View {
		if v.Kind == nsgotest.KindScroll {
			return v
		}
		for _, child := range v.Children {
			if found := findPager(child); found != nil {
				return found
			}
		}
		return nil
	}
	const previous = "role:button 上一张图片"
	const next = "role:button 下一张图片"
	pager := findPager(app.Find("gallery"))
	app.Report(nsgotest.Event{Kind: nsgotest.EventScroll, ID: pager.ID, Point: core.Point{X: 240}})
	if app.Enabled(previous) {
		t.Fatal("previous button enabled before the gallery stopped scrolling")
	}
	app.Report(nsgotest.Event{Kind: nsgotest.EventScroll, ID: pager.ID})
	app.Wheel(next, core.Point{Y: -40}, core.Modifiers{})
	app.Settle()
	if got := findPager(app.Find("gallery")).Offset.X; got != 360 {
		t.Fatalf("wheel offset = %g, want 360", got)
	}
	if !app.Enabled(previous) {
		t.Fatal("previous button still disabled after scrolling settled on the second image")
	}
	app.Press(next)
	app.Settle()
	if got := findPager(app.Find("gallery")).Offset.X; got != 720 {
		t.Errorf("next button offset = %g, want 720", got)
	}
	if app.Enabled(next) {
		t.Fatal("next button enabled on the last image")
	}
	app.Press(previous)
	app.Settle()
	if got := findPager(app.Find("gallery")).Offset.X; got != 360 {
		t.Errorf("previous button offset = %g, want 360", got)
	}
	const first = "role:button 切换到第 1 张图片"
	const last = "role:button 切换到第 3 张图片"
	app.Press(last)
	app.Settle()
	if got := findPager(app.Find("gallery")).Offset.X; got != 720 {
		t.Fatalf("clicking the last dot scrolled to %g, want 720", got)
	}
	app.Resize(420, 600)
	if got := findPager(app.Find("gallery")).Offset.X; got != 840 || app.Enabled(next) {
		t.Fatalf("resizing should keep the last image selected and the next button disabled, offset = %g", got)
	}
	app.DragTo(last, first)
	app.Settle()
	if got := findPager(app.Find("gallery")).Offset.X; got != 0 {
		t.Errorf("dragging to the first dot scrolled to %g, want 0", got)
	}
	if app.Enabled(previous) {
		t.Error("previous button enabled after dragging to the first dot")
	}
}

func TestGalleryHoverPreservesBackground(t *testing.T) {
	old := hoverable
	hoverable = true
	t.Cleanup(func() { hoverable = old })
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.SetTheme(newTheme())
		a := &App{cx: cx, images: newImageStore(t.TempDir(), "")}
		return core.View{TestID: "gallery", Class: "bg-background", Children: []core.Node{
			a.bigCoverImages(cx, func() []backend.Image { return []backend.Image{{Width: 100, Height: 100}} }, 1)}}
	}, nsgotest.Size(360, 600))
	app.Hover("role:button 查看第 1 张图片")
	app.Settle()
	button := app.Find("role:button 查看第 1 张图片")
	if button.Style.Background != app.Find("gallery").Style.Background {
		t.Error("gallery margins have a different background from the page")
	}
	if len(button.Children) != 1 || button.Children[0].Kind != nsgotest.KindImage {
		t.Fatalf("gallery contains a visual state layer:\n%s", app.Dump())
	}
}

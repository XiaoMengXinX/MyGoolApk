package app

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"

	"app/internal/backend"
)

func TestViewerBackdropTapCloses(t *testing.T) {
	b := newFakeBackend()
	b.feed.Images = []backend.Image{{Width: 120, Height: 650}, {Width: 120, Height: 650}}
	app := start(t, b)
	app.Press("role:button 查看第 1 张图片 >> nth=1")
	app.FastForward(time.Second)
	view := app.Find("image-viewer")
	at := core.Point{X: 10, Y: view.Frame.Size.Height / 2}
	drag := app.Touch("image-viewer", at)
	drag.Move(core.Point{X: 42, Y: at.Y})
	drag.Move(at)
	drag.Lift()
	app.FastForward(time.Second)
	if !shows(app, "image-viewer") {
		t.Fatal("dragging in the margin dismissed the viewer")
	}
	app.Touch("image-viewer", at).Lift()
	app.FastForward(time.Second)
	if shows(app, "image-viewer") {
		t.Fatal("tapping the margin left the viewer open")
	}
}

func TestViewerShowsLongOriginalAtDifferentWidths(t *testing.T) {
	for _, width := range []int{798, 1206} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			im := backend.Image{URL: "https://image.coolapk.com/feed/long.jpg", Width: width, Height: 7500}
			store := newImageStore(t.TempDir(), "")
			path := store.pathOf(im.URL)
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				a := &App{cx: cx, images: store, size: core.NewMemo(cx, func() sizeClass { return compact })}
				return core.View{TestID: "viewer", Style: fill, Children: []core.Node{
					a.viewerPage(cx, &viewer{images: []backend.Image{im}})}}
			})
			var showsOriginal func(*nsgotest.View) bool
			showsOriginal = func(v *nsgotest.View) bool {
				if strings.Contains(v.Image.Source.URI, path) {
					return true
				}
				for _, child := range v.Children {
					if showsOriginal(child) {
						return true
					}
				}
				return false
			}
			if !showsOriginal(app.Find("viewer")) {
				t.Fatalf("the long picture has no original source:\n%s", app.Dump())
			}
			var checkBounds func(*nsgotest.View)
			checkBounds = func(v *nsgotest.View) {
				if v.Kind == nsgotest.KindScroll && v.Scroll.MaxZoomScale > 1 {
					if v.Content.Width >= v.Frame.Size.Width || v.Content.Height != v.Frame.Size.Height {
						t.Errorf("long image bounds include its side margins: content=%+v viewport=%+v", v.Content, v.Frame.Size)
					}
				}
				for _, child := range v.Children {
					checkBounds(child)
				}
			}
			checkBounds(app.Find("viewer"))
		})
	}
}

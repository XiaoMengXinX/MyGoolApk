package app

import (
	"strconv"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"

	"app/internal/backend"
)

func TestNewsPicturesHaveVisibleFrames(t *testing.T) {
	for _, count := range []int{1, 2, 3} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			entry := backend.PageEntry{Kind: backend.EntryNews, Title: "新闻标题", Feed: &backend.Feed{Images: make([]backend.Image, count)}}
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				a := &App{cx: cx, images: newImageStore(t.TempDir(), "")}
				return core.View{TestID: "news", Children: []core.Node{a.newsCard(cx, func() backend.PageEntry { return entry })}}
			}, nsgotest.Size(400, 800))
			var pictures []*nsgotest.View
			var visit func(*nsgotest.View)
			visit = func(v *nsgotest.View) {
				if v.Kind == nsgotest.KindImage {
					pictures = append(pictures, v)
				}
				for _, child := range v.Children {
					visit(child)
				}
			}
			visit(app.Find("news"))
			if len(pictures) != count {
				t.Fatalf("got %d pictures, want %d", len(pictures), count)
			}
			for _, picture := range pictures {
				if picture.Frame.Size.Width <= 0 || picture.Frame.Size.Height <= 0 {
					t.Errorf("picture has an empty frame: %+v", picture.Frame)
				}
				if w, h := picture.Frame.Size.Width, picture.Frame.Size.Height; w < h*1.3 || w > h*1.4 {
					t.Errorf("picture frame is not 4:3: %+v", picture.Frame)
				}
			}
		})
	}
}

package app

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"

	"app/internal/backend"
)

func TestBannerLoopsInBothDirections(t *testing.T) {
	old := hoverable
	t.Cleanup(func() { hoverable = old })
	for _, name := range []string{"Desktop", "Touch"} {
		t.Run(name, func(t *testing.T) {
			hoverable = name == "Desktop"
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				cx.SetTheme(newTheme())
				a := &App{images: newImageStore(t.TempDir(), "")}
				return core.View{TestID: "banner", Children: []core.Node{a.bannerCard(cx, &backend.Card{Items: []backend.CardItem{
					{Title: "第一张轮播图"}, {Title: "第二张轮播图"}, {Title: "第三张轮播图"},
				}})}}
			}, nsgotest.Size(360, 400))
			const banner = "id:banner"
			check := func(want core.Points) {
				t.Helper()
				app.Settle()
				if got := app.Find(banner).Children[0].Children[0].Offset.X; got != want {
					t.Fatalf("banner offset = %g, want %g", got, want)
				}
			}
			check(360)
			if hoverable {
				app.Wheel(banner, core.Point{Y: -40}, core.Modifiers{})
				check(720)
				app.TouchpadScroll(banner, core.Point{}, core.Point{X: -100})
				check(1080)
				app.TouchpadScroll(banner, core.Point{}, core.Point{X: -100})
				check(360)
				app.TouchpadScroll(banner, core.Point{}, core.Point{X: 100})
				check(1080)
			} else {
				app.Scroll(banner, 360)
				check(720)
				app.Scroll(banner, 360)
				check(1080)
				app.Scroll(banner, 360)
				check(360)
				app.Scroll(banner, -360)
				check(1080)
			}
			app.Resize(420, 400)
			check(1260)
		})
	}
}

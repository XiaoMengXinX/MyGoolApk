package app

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"

	"app/internal/backend"
)

func TestPictureHoverFollowsRoundedFrames(t *testing.T) {
	old := hoverable
	hoverable = true
	t.Cleanup(func() { hoverable = old })
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.SetTheme(newTheme())
		a := &App{cx: cx, images: newImageStore(t.TempDir(), "")}
		return core.View{Children: []core.Node{
			a.imageGrid(cx, func() []backend.Image { return make([]backend.Image, 3) }, 3, 3),
			core.View{Style: core.Style{Width: core.Pt(180), Height: core.Pt(180)}, Children: []core.Node{
				a.pictureButton(cx, func() backend.Image { return backend.Image{Width: 100, Height: 100} }, thumbSmall, func() {})}},
		}}
	}, nsgotest.Size(360, 600))
	for _, tc := range []struct {
		selector string
		radius   core.Points
	}{{"role:button 查看第 1 张图片", 8}, {"role:button 查看图片", 12}} {
		app.Hover(tc.selector)
		app.Settle()
		button := app.Find(tc.selector)
		layer := button.Children[len(button.Children)-1]
		if layer.Style.Opacity <= 0 || layer.Style.CornerRadius != tc.radius || layer.Frame.Size != button.Frame.Size {
			t.Errorf("picture hover: radius=%g opacity=%g size=%+v, picture=%+v", layer.Style.CornerRadius, layer.Style.Opacity, layer.Frame.Size, button.Frame.Size)
		}
	}
}

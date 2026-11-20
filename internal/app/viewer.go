package app

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"

	"app/internal/backend"
)

const routeImages = "images"

// tapWindow is how long a tap waits for a second one before it closes the
// viewer.
const tapWindow = 220 * time.Millisecond

// viewer is the pictures a viewer page shows.
type viewer struct {
	images []backend.Image
	index  int
}

// openImages shows the pictures of a post from the one at index.
func (a *App) openImages(images []backend.Image, index int) {
	if len(images) == 0 {
		return
	}
	a.viewerSeq++
	id := strconv.Itoa(a.viewerSeq)
	a.viewers[id] = &viewer{images: images, index: max(0, min(index, len(images)-1))}
	// Start the first picture's thumbnail before the page is built.
	go a.images.fetch(thumbURL(images[index], thumbSmall))
	a.push(routeImages, id)
}

// viewerPage is the full-screen picture viewer: black, a page per picture
// that pinches to zoom, a tap to close and a long press that offers to save.
func (a *App) viewerPage(cx *core.Scope, id string) core.Node {
	v := a.viewers[id]
	cx.Cleanup(func() { delete(a.viewers, id) })
	width := core.NewSignal(cx, core.Points(0))
	height := core.NewSignal(cx, core.Points(0))
	pos := core.NewSignal(cx, float32(v.index))
	saving := core.NewSignal(cx, false)
	held := core.NewSignal(cx, 0)
	var pager core.Ref
	opened := false

	pages := make([]core.Node, len(v.images))
	for i := range v.images {
		i := i
		im := v.images[i]
		pages[i] = core.Show{
			When: func() bool { return width.Get() > 0 && abs32(pos.Get()-float32(i)) < 1.6 },
			Then: func(cx *core.Scope) core.Node {
				zoomed := core.NewSignal(cx, false)
				var closing func() bool
				// A single tap closes the viewer once the double tap's window has
				// passed; a second tap zooms instead.
				press := func(e core.PressEvent) {
					if closing != nil {
						closing()
						closing = nil
					}
					if e.Clicks >= 2 {
						zoomed.Set(!zoomed.GetUntracked())
						return
					}
					closing = cx.AfterFunc(tapWindow, func() { a.nav.Pop() })
				}
				return core.Scroll{
					MinZoomScale: 1, MaxZoomScale: 4, CenterContent: true, HideIndicator: true,
					ZoomScaleFunc: func() float32 {
						if zoomed.Get() {
							return 2.5
						}
						return 1
					},
					StyleFunc: func() core.Style { return core.Style{Width: core.Pt(width.Get()), Height: core.Pt(height.Get())} },
					Children: []core.Node{core.View{
						StyleFunc: func() core.Style { return core.Style{Width: core.Pt(width.Get()), Height: core.Pt(height.Get())} },
						OnPress:   press,
						OnLongPress: func(core.PressEvent) {
							held.Set(i)
							saving.Set(true)
						},
						Children: []core.Node{
							a.picOf(cx, thumbURL(im, thumbSmall), core.Contain, "", fillAbs),
							a.picOf(cx, im.URL, core.Contain, "", fillAbs),
						},
					}},
				}
			},
		}
	}
	for i := range pages {
		i := i
		pages[i] = core.View{StyleFunc: func() core.Style {
			return core.Style{Width: core.Pt(width.Get()), Height: core.Pt(height.Get()), Overflow: core.Hidden}
		}, Children: []core.Node{pages[i]}}
		_ = i
	}
	paged := core.Scroll{
		Horizontal: true, HideIndicator: true, Ref: &pager,
		Behavior: core.ScrollBehavior{Paging: true},
		Style:    fill.Merge(core.Style{Direction: core.Row}),
		OnLayout: func(b core.Bounds) {
			width.Set(b.Size.Width)
			height.Set(b.Size.Height)
			if !opened && b.Size.Width > 0 {
				opened = true
				pager.ScrollTo(core.Point{X: core.Points(v.index) * b.Size.Width}, false)
			}
		},
		OnScroll: func(e core.ScrollEvent) {
			if !e.Stale && e.Size.Width > 0 {
				pos.Set(float32(e.Offset.X / e.Size.Width))
			}
		},
		Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, Height: core.Pct(100)}, Children: pages}},
	}
	counter := core.Dynamic(func(cx *core.Scope) core.Node {
		if len(v.images) < 2 {
			return core.Group{}
		}
		return core.Text{ContentFunc: func() string { return fmt.Sprintf("%d / %d", int(pos.Get()+0.5)+1, len(v.images)) },
			Class: "bg-black/60 text-white", Style: func() core.Style {
				s := textStyle(textSecondary, 500)
				s.Position, s.Bottom, s.Left, s.Right = core.Absolute, core.Pt(32), core.Pt(0), core.Pt(0)
				s.AlignSelf, s.Padding, s.CornerRadius = core.AlignCenter, core.Symmetric(4, 12), 12
				s.Width = core.Pt(72)
				s.TextAlign = core.TextAlignCenter
				s.Margin = core.Edges{Left: 0}
				return s
			}()}
	})
	save := ui.Sheet{Open: saving.Get, OnClose: func() { saving.Set(false) }, Children: []core.Node{
		listItem(&symbols.Download, "保存图片", nil, func() {
			saving.Set(false)
			a.saveImage(v.images[held.GetUntracked()])
		}),
	}}
	cx.SetStatusBar(core.StatusBarSetting{Foreground: core.StatusBarLight})
	cx.Cleanup(func() {
		if cx.Dark() {
			return
		}
		cx.SetStatusBar(core.StatusBarSetting{Foreground: core.StatusBarDark})
	})
	return column(fill, "bg-black", column(fill, "bg-black", paged), counter, save)
}

// saveImage keeps the original of a picture where the user chooses.
func (a *App) saveImage(im backend.Image) {
	name := path.Base(strings.SplitN(im.URL, "?", 2)[0])
	if !strings.Contains(name, ".") {
		name += ".jpg"
	}
	go func() {
		p, err := a.images.fetch(im.URL)
		a.cx.Post(func() {
			if err != nil {
				a.notice("保存失败：" + err.Error())
				return
			}
			a.cx.SaveFile(core.SaveOptions{Name: name, Path: p}, func(saved bool, _ string, err error) {
				switch {
				case err != nil:
					a.notice("保存失败：" + err.Error())
				case saved:
					a.notice("已保存")
				}
			})
		})
	}()
}

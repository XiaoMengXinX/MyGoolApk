package app

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"

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

// dismissDistance is how far a picture is dragged down before letting go
// closes the viewer.
const dismissDistance core.Points = 120

// viewerPage is the full-screen picture viewer: black, a page per picture
// that pinches and double taps to zoom, a tap to close, a drag down to close
// and a long press that offers to save.
func (a *App) viewerPage(cx *core.Scope, id string) core.Node {
	v := a.viewers[id]
	cx.Cleanup(func() { delete(a.viewers, id) })
	width := core.NewSignal(cx, core.Points(0))
	height := core.NewSignal(cx, core.Points(0))
	pos := core.NewSignal(cx, float32(v.index))
	saving := core.NewSignal(cx, false)
	held := core.NewSignal(cx, 0)
	// zoomed says a picture is magnified, whose drags pan it instead.
	zoomed := core.NewSignal(cx, false)
	dragY := core.NewSignal(cx, core.Points(0))
	dragging := core.NewSignal(cx, false)
	var pager core.Ref
	page := v.index // the picture shown, which a new width keeps in view
	var laidOut core.Points

	pages := make([]core.Node, len(v.images))
	for i := range v.images {
		i := i
		im := v.images[i]
		pages[i] = core.Show{
			When: func() bool { return width.Get() > 0 && abs32(pos.Get()-float32(i)) < 1.6 },
			Then: func(cx *core.Scope) core.Node {
				var zoom core.Ref
				scale := float32(1)
				var closing func() bool
				// A single tap closes the viewer once the double tap's window has
				// passed; a second tap zooms in on its point, or out again.
				press := func(e core.PressEvent) {
					if closing != nil {
						closing()
						closing = nil
						if scale > 1.05 {
							zoom.ZoomTo(1, nil, true)
						} else {
							zoom.ZoomTo(2.5, &e.Position, true)
						}
						return
					}
					closing = cx.AfterFunc(tapWindow, func() { closing = nil; a.nav.Pop() })
				}
				return core.Scroll{
					Ref: &zoom, MinZoomScale: 1, MaxZoomScale: 4, CenterContent: true, HideIndicator: true,
					OnScroll: func(e core.ScrollEvent) {
						scale = e.ZoomScale
						zoomed.Set(e.ZoomScale > 1.05)
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
	}
	paged := core.Scroll{
		Horizontal: true, HideIndicator: true, Ref: &pager,
		Behavior: core.ScrollBehavior{Paging: true},
		Style:    fill.Merge(core.Style{Direction: core.Row}),
		OnLayout: func(b core.Bounds) {
			if b.Size.Width == 0 {
				return // laid out under a page a back gesture previews
			}
			width.Set(b.Size.Width)
			height.Set(b.Size.Height)
			if b.Size.Width != laidOut {
				laidOut = b.Size.Width
				pager.ScrollTo(core.Point{X: core.Points(page) * b.Size.Width}, false)
			}
		},
		OnScroll: func(e core.ScrollEvent) {
			if !e.Stale && e.Size.Width > 0 {
				pos.Set(float32(e.Offset.X / e.Size.Width))
				if e.Size.Width == laidOut {
					page = int(e.Offset.X/e.Size.Width + 0.5)
				}
			}
		},
		Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, Height: core.Pct(100)}, Children: pages}},
	}

	// A drag down moves the pictures with the finger and fades the black
	// behind them, which closes the viewer past dismissDistance or on a fling,
	// and springs back before it. Horizontal drags are the pager's and
	// magnified pictures' own.
	onPan := func(e core.PanEvent) {
		switch e.Phase {
		case core.GestureBegan:
			if zoomed.GetUntracked() {
				return
			}
			dragging.Set(true)
		case core.GestureChanged:
			if dragging.GetUntracked() {
				dragY.Set(max(e.Translation.Y, 0))
			}
		case core.GestureEnded, core.GestureCancelled:
			if !dragging.GetUntracked() {
				return
			}
			dragging.Set(false)
			if dragY.GetUntracked() >= dismissDistance || e.Velocity.Y > 900 {
				a.nav.Pop()
				return
			}
			dragY.Set(0)
		}
	}
	fade := func() float32 { return 1 - min(float32(dragY.Get())/500, 0.7) }
	black := core.View{Class: "bg-black", Style: fillAbs, PointerEvents: core.PointerEventsNone,
		StyleFunc: func() core.Style {
			s := core.Style{Opacity: core.Alpha(fade())}
			if !dragging.Get() {
				s.Transition = core.Transition{Duration: 200 * time.Millisecond, Easing: emphasized, Properties: core.TransitionOpacity}
			}
			return s
		}}
	content := core.View{
		Style: fill, OnPan: onPan, PanActivation: core.PanActivation{ActiveOffsetY: 10, FailOffsetX: 10},
		StyleFunc: func() core.Style {
			s := core.Style{Transform: core.Transform{TranslateY: dragY.Get()}}
			if !dragging.Get() {
				s.Transition = core.Transition{Duration: 200 * time.Millisecond, Easing: emphasized, Properties: core.TransitionTransform}
			}
			return s
		},
		Children: []core.Node{paged},
	}
	counter := core.Dynamic(func(cx *core.Scope) core.Node {
		if len(v.images) < 2 {
			return core.Group{}
		}
		// The counter sits above the system's bottom bar, whichever the user's
		// navigation setting makes it.
		return core.View{
			PointerEvents: core.PointerEventsNone,
			Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), AlignItems: core.AlignCenter,
				Padding: core.Edges{Bottom: 16}, PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}},
			Children: []core.Node{core.Text{ContentFunc: func() string { return fmt.Sprintf("%d / %d", int(pos.Get()+0.5)+1, len(v.images)) },
				Class: "bg-black/60 text-white", Style: func() core.Style {
					s := textStyle(textSecondary, 500)
					s.Padding, s.CornerRadius = core.Symmetric(4, 12), 12
					return s
				}()}},
		}
	})
	save := a.bottomSheet(saving.Get, func() { saving.Set(false) },
		listItem(&symbols.Download, "保存图片", nil, func() {
			saving.Set(false)
			a.saveImage(v.images[held.GetUntracked()])
		}))
	cx.SetStatusBar(core.StatusBarSetting{Foreground: core.StatusBarLight})
	cx.Cleanup(func() {
		if cx.Dark() {
			return
		}
		cx.SetStatusBar(core.StatusBarSetting{Foreground: core.StatusBarDark})
	})
	return column(fill, "", black, content, counter, save)
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

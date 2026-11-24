package app

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// tapWindow is how long a tap waits for a second one before it closes the
// viewer.
const tapWindow = 220 * time.Millisecond

// viewer is the pictures the viewer shows.
type viewer struct {
	images  []backend.Image
	index   int
	closing bool
}

// viewerMotion is how long the viewer takes to fade in and out.
const viewerMotion = 180 * time.Millisecond

// openImages shows the pictures of a post from the one at index, over the
// page that shows them: the viewer is no page of the stack, so a picture
// dragged away shows that page behind it.
func (a *App) openImages(images []backend.Image, index int) {
	if len(images) == 0 || a.viewing.GetUntracked() != nil {
		return
	}
	index = max(0, min(index, len(images)-1))
	// Start the first picture's thumbnail before the viewer is built.
	go a.images.fetch(thumbURL(images[index], thumbSmall))
	a.viewerShown.Set(false)
	v := &viewer{images: images, index: index}
	a.viewing.Set(v)
	a.cx.AfterFunc(16*time.Millisecond, func() {
		if !v.closing {
			a.viewerShown.Set(true)
		}
	})
}

// closeViewer fades the viewer out, then takes it away.
func (a *App) closeViewer() {
	v := a.viewing.GetUntracked()
	if v == nil || v.closing {
		return
	}
	v.closing = true
	a.viewerShown.Set(false)
	a.cx.AfterFunc(viewerMotion, func() {
		if a.viewing.GetUntracked() == v {
			a.viewing.Set(nil)
		}
	})
}

// viewerLayer holds the viewer over everything else while it is open.
func (a *App) viewerLayer() core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		v := a.viewing.Get()
		if v == nil {
			return core.Group{}
		}
		cx.OnBackPress(func() bool { a.closeViewer(); return true })
		return core.View{
			Style: fillAbs.Merge(core.Style{Transition: core.Transition{Duration: viewerMotion, Easing: emphasized, Properties: core.TransitionOpacity | core.TransitionTransform}}),
			StyleFunc: func() core.Style {
				if a.viewerShown.Get() {
					return core.Style{Opacity: core.Alpha(1)}
				}
				return core.Style{Opacity: core.Alpha(0), Transform: core.Transform{Scale: 0.96}}
			},
			Children: []core.Node{a.viewerPage(cx, v)},
		}
	})
}

// dismissDistance is how far a picture is dragged down before letting go
// closes the viewer.
const dismissDistance core.Points = 120

// viewerPage is the full-screen picture viewer: black, a page per picture
// that pinches and double taps to zoom, a tap to close, a drag down to close
// and a long press that offers to save.
func (a *App) viewerPage(cx *core.Scope, v *viewer) core.Node {
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

	// current is the page the pager came to rest on. The pages next to it are
	// built, and the original of it alone loads, only once a swipe settles:
	// building or decoding during one made it stutter.
	current := core.NewSignal(cx, v.index)
	settle := func(e core.ScrollEvent) {
		if e.Size.Width > 0 && e.Size.Width == laidOut {
			page = int(e.Offset.X/e.Size.Width + 0.5)
			current.Set(page)
		}
	}
	pages := make([]core.Node, len(v.images))
	for i := range v.images {
		i := i
		im := v.images[i]
		pages[i] = core.Show{
			When: func() bool { return width.Get() > 0 && max(i-current.Get(), current.Get()-i) <= 1 },
			Then: func(cx *core.Scope) core.Node {
				var zoom core.Ref
				scale := float32(1)
				var closing func() bool
				// A picture left zoomed comes back whole.
				cx.Effect(func() {
					if current.Get() != i && scale > 1.05 {
						scale = 1
						zoom.ZoomTo(1, nil, false)
					}
				})
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
					closing = cx.AfterFunc(tapWindow, func() { closing = nil; a.closeViewer() })
				}
				// Over the small copy, the page that is current gets the
				// screen-wide copy, and a zoomed one the original: originals
				// run to 2494x3326 and more, which cost frames to decode and
				// draw at a size the screen does not show unzoomed. Each stays
				// once loaded.
				sharp, full := false, false
				zoomedHere := core.NewSignal(cx, false)
				medium := func() string {
					if current.Get() == i {
						sharp = true
					}
					if sharp {
						return thumbURL(im, thumbMedium)
					}
					return ""
				}
				original := func() string {
					if zoomedHere.Get() {
						full = true
					}
					if full && thumbURL(im, thumbMedium) != im.URL {
						return im.URL
					}
					return ""
				}
				return core.Scroll{
					Ref: &zoom, MinZoomScale: 1, MaxZoomScale: 4, CenterContent: true, HideIndicator: true,
					OnScroll: func(e core.ScrollEvent) {
						scale = e.ZoomScale
						zoomedHere.Set(e.ZoomScale > 1.05)
						if current.GetUntracked() == i {
							zoomed.Set(e.ZoomScale > 1.05)
						}
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
							a.pic(cx, medium, core.Contain, "", fillAbs),
							a.pic(cx, original, core.Contain, "", fillAbs),
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
		// A zoomed picture takes the drags, to move about in it.
		BehaviorFunc: func() core.ScrollBehavior { return core.ScrollBehavior{Paging: true, NoScroll: zoomed.Get()} },
		Style:        fill.Merge(core.Style{Direction: core.Row}),
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
			}
		},
		OnScrollEnd:   settle,
		OnMomentumEnd: settle,
		Children:      []core.Node{core.View{Style: core.Style{Direction: core.Row, Height: core.Pct(100)}, Children: pages}},
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
				a.closeViewer()
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

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
	// zooms are the pictures built, by page, for a drag on a zoomed one.
	zooms := map[int]*zoomPage{}
	// pinched is when a picture's zoom last changed: a pinch, whose fingers
	// the pans over the pictures must leave alone.
	var pinched time.Time
	pages := make([]core.Node, len(v.images))
	for i := range v.images {
		i := i
		im := v.images[i]
		pages[i] = core.Show{
			When: func() bool { return width.Get() > 0 && max(i-current.Get(), current.Get()-i) <= 1 },
			Then: func(cx *core.Scope) core.Node {
				var zoom core.Ref
				zp := &zoomPage{ref: &zoom}
				zooms[i] = zp
				cx.Cleanup(func() {
					if zooms[i] == zp {
						delete(zooms, i)
					}
				})
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
				// The CDN crops its copies of a long picture to the top, which
				// under the whole of it showed a slice of it a second time: a
				// long picture shows its original alone.
				long := im.Width > 0 && im.Height > 0 && float64(im.Width)/float64(im.Height) < longImageRatio
				medium := func() string {
					if long {
						return ""
					}
					if current.Get() == i {
						sharp = true
					}
					if sharp {
						return thumbURL(im, thumbMedium)
					}
					return ""
				}
				original := func() string {
					if long && current.Get() == i || zoomedHere.Get() {
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
						zp.off, zp.content, zp.size = e.Offset, e.ContentSize, e.Size
						if e.ZoomScale != scale {
							pinched = time.Now()
						}
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
							a.pic(cx, func() string {
								if long {
									return ""
								}
								return thumbURL(im, thumbSmall)
							}, core.Contain, "", fillAbs),
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
	row := core.View{Style: core.Style{Direction: core.Row, Height: core.Pct(100)}, Children: pages}
	// Android pages the pictures itself well: no lists are in them to slant into.
	byPan := cx.Env().OS() == core.OSIOS
	if byPan {
		sp := &swipePager{ref: &pager, width: width.GetUntracked, count: len(v.images), at: current.GetUntracked,
			goTo: func(i int) {
				page = i
				current.Set(i)
				pager.ScrollTo(core.Point{X: core.Points(i) * width.GetUntracked()}, true)
			}}
		// A zoomed picture takes the drag to move about in it, which this pan
		// over it would otherwise keep.
		var moving *zoomPage
		var from core.Point
		row.OnPan, row.PanActivation = func(e core.PanEvent) {
			if time.Since(pinched) < 150*time.Millisecond {
				moving = nil // a pinch's fingers move the picture's own zoom
				return
			}
			if e.Phase == core.GestureBegan && zoomed.GetUntracked() {
				if zp := zooms[current.GetUntracked()]; zp != nil {
					moving, from = zp, zp.off
				}
			}
			if moving == nil {
				sp.pan(e)
				return
			}
			to := moving.clamp(core.Point{X: from.X - e.Translation.X, Y: from.Y - e.Translation.Y})
			moving.ref.ScrollTo(to, false)
			if e.Phase == core.GestureEnded || e.Phase == core.GestureCancelled {
				moving = nil
			}
		}, swipeActivation
	}
	paged := core.Scroll{
		Horizontal: true, HideIndicator: true, Ref: &pager,
		// A zoomed picture takes the drags, to move about in it.
		BehaviorFunc: func() core.ScrollBehavior {
			return core.ScrollBehavior{Paging: true, NoScroll: byPan || zoomed.Get()}
		},
		Style: fill.Merge(core.Style{Direction: core.Row}),
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
				p := e.Offset.X / e.Size.Width
				pos.Set(float32(p))
				// A page reached whole has settled, whether or not the
				// platform reports the end of the scroll, as iOS may not.
				if d := p - core.Points(int(p+0.5)); d > -0.005 && d < 0.005 {
					settle(e)
				}
			}
		},
		OnScrollEnd:   settle,
		OnMomentumEnd: settle,
		Children:      []core.Node{row},
	}

	// A drag down moves the pictures with the finger and fades the black
	// behind them, which closes the viewer past dismissDistance or on a fling,
	// and springs back before it. Horizontal drags are the pager's and
	// magnified pictures' own.
	// On Android the drag down is an ancestor of the zoomed picture's scroll
	// view and takes every vertical drag from it, so it moves a zoomed
	// picture itself. iOS hands the drag to the scroll view first
	// (PanScrollHandoff).
	var panning *zoomPage
	var panFrom core.Point
	android := cx.Env().OS() == core.OSAndroid
	pinching := false
	// A finger that goes down or up during a pan moves its translation at
	// once, to the new middle of the fingers: such a step moves nothing.
	var lastT, skipped core.Point
	onPan := func(e core.PanEvent) {
		if e.Phase == core.GestureBegan {
			pinching = false
			lastT, skipped = core.Point{}, core.Point{}
		}
		if step := (core.Point{X: e.Translation.X - lastT.X, Y: e.Translation.Y - lastT.Y}); step.X*step.X+step.Y*step.Y > 40*40 {
			skipped = core.Point{X: skipped.X + step.X, Y: skipped.Y + step.Y}
		}
		lastT = e.Translation
		e.Translation = core.Point{X: e.Translation.X - skipped.X, Y: e.Translation.Y - skipped.Y}
		if !pinching && time.Since(pinched) < 150*time.Millisecond {
			// A pinch is under way: what this pan moved goes back at once.
			pinching = true
			panning = nil
			if dragging.GetUntracked() {
				dragging.Set(false)
				dragY.Set(0)
			}
		}
		if pinching {
			return
		}
		if panning != nil {
			to := panning.clamp(core.Point{X: panFrom.X - e.Translation.X, Y: panFrom.Y - e.Translation.Y})
			switch e.Phase {
			case core.GestureChanged:
				panning.ref.ScrollTo(to, false)
			case core.GestureEnded, core.GestureCancelled:
				// The picture stays where the finger left it: an animated scroll
				// on from there jumped there at once on Android.
				panning.ref.ScrollTo(to, false)
				panning = nil
			}
			return
		}
		switch e.Phase {
		case core.GestureBegan:
			if zoomed.GetUntracked() {
				if zp := zooms[current.GetUntracked()]; android && zp != nil {
					panning, panFrom = zp, zp.off
				}
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
		// A zoomed picture's own scroll view takes a drag first, and only
		// what goes on past its edge moves the pictures away.
		Style: fill, OnPan: onPan, PanActivation: core.PanActivation{ActiveOffsetY: 10, FailOffsetX: 10}, PanScrollHandoff: true,
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

// zoomPage is a picture of the viewer: its zoom scroll view and where it is.
type zoomPage struct {
	ref           *core.Ref
	off           core.Point
	content, size core.Size
}

// clamp keeps an offset of the zoomed content within its bounds.
func (z *zoomPage) clamp(p core.Point) core.Point {
	return core.Point{
		X: max(0, min(p.X, z.content.Width-z.size.Width)),
		Y: max(0, min(p.Y, z.content.Height-z.size.Height)),
	}
}

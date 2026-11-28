package app

import (
	"math"

	"github.com/nsgo-dev/nsgo/core"
)

// swipePager moves a paged horizontal Scroll by a pan on its content: on iOS
// a paged scroll view with scroll views inside its pages never gets a
// horizontal drag, which the inner ones keep waiting on, and on Android it
// takes any drag that goes sideways a little, a slanting scroll of a list
// included. The pan follows the finger, and letting go settles on the page
// nearest where a throw would carry it, at most one page on.
type swipePager struct {
	ref   *core.Ref
	width func() core.Points
	count int
	// at is the page shown; goTo is called with the page a throw settles on.
	at   func() int
	goTo func(i int)
	from core.Points
}

// swipesByPan reports whether pages swipe by the pan rather than by the scroll view.
func swipesByPan(cx *core.Scope) bool {
	os := cx.Env().OS()
	return os == core.OSIOS || os == core.OSAndroid
}

// swipeActivation starts a pan that goes clearly sideways, and leaves a drag
// that goes down or up first to the scroll views in the pages.
var swipeActivation = core.PanActivation{ActiveOffsetX: 24, FailOffsetY: 12}

func (p *swipePager) pan(e core.PanEvent) {
	w := p.width()
	if w <= 0 {
		return
	}
	switch e.Phase {
	case core.GestureBegan:
		p.from = core.Points(p.at()) * w
	case core.GestureChanged:
		x := max(0, min(p.from-e.Translation.X, core.Points(p.count-1)*w))
		p.ref.ScrollTo(core.Point{X: x}, false)
	case core.GestureEnded, core.GestureCancelled:
		start := int(p.from/w + 0.5)
		to := (p.from - e.Translation.X - e.Velocity.X*0.2) / w
		i := int(to + 0.5)
		if to < 0 {
			i = 0
		}
		i = max(start-1, min(i, start+1))
		i = max(0, min(i, p.count-1))
		p.goTo(i)
	}
}

// headed keeps a head and a bar over lists that swipe sideways: the head
// scrolls away with the shown list while the bar stays at the top. Each list
// leaves room for both above its rows and tells its scrolling (under).
type headed struct {
	shown *core.Signal[int]
	count int
	list  func(i int) *core.Ref
	// head is the head's height, top the head's with the bar's, and shift how
	// far the head is scrolled away.
	head, top *core.Signal[core.Points]
	viewport  *core.Signal[core.Points]
	shift     core.Points
	// ys follow the lists' offsets on the platform, which moves the head with
	// the shown list in the frame it scrolls; lifts turn them into the head's
	// translation.
	ys    []*core.AnimatedValue
	lifts *core.Memo[[]core.AnimatedNode]
	// offsets are where the lists are: where each said it scrolled to, or
	// where it was sent since, which it may not tell (a report from before
	// it got there is stale, and some platforms send no other).
	offsets map[int]core.Points
	// rests are the room under the lists' rows, which lets a short list
	// scroll the head away.
	rests []*core.Signal[core.Points]
	// ends are the room over the rows of each list and the room under them.
	ends [][2]core.Ref
	// held are the lists whose rows changed and are not laid out yet.
	held map[int]bool
}

// newHeaded is a head over count lists that list returns the refs of, nil for
// one not made yet, of which shown is in sight.
func newHeaded(cx *core.Scope, shown *core.Signal[int], count int, list func(i int) *core.Ref) *headed {
	h := &headed{shown: shown, count: count, list: list, head: core.NewSignal(cx, core.Points(0)), top: core.NewSignal(cx, core.Points(0)),
		viewport: core.NewSignal(cx, core.Points(0)), offsets: map[int]core.Points{}, held: map[int]bool{}}
	h.rests, h.ends = make([]*core.Signal[core.Points], count), make([][2]core.Ref, count)
	h.ys = make([]*core.AnimatedValue, count)
	for i := range h.rests {
		h.rests[i] = core.NewSignal(cx, core.Points(0))
		h.ys[i] = core.NewAnimatedValue(cx, 0)
	}
	// The nodes live as long as the head's height they are for.
	h.lifts = core.NewMemoFunc(cx, func() []core.AnimatedNode {
		head := float64(h.head.Get())
		if head <= 0 {
			return nil
		}
		lifts := make([]core.AnimatedNode, count)
		for i, y := range h.ys {
			lifts[i] = core.Interpolate(y, core.Interpolation{Input: []float64{0, head}, Output: []float64{0, -head}, Extrapolate: core.ExtrapolateClamp})
		}
		return lifts
	}, func(a, b []core.AnimatedNode) bool { return false })
	// The head goes where the list that comes in sight has scrolled it.
	cx.Effect(func() {
		i := shown.Get()
		h.shift = min(h.offsets[i], h.head.Get())
		h.follow(i - 1)
		h.follow(i + 1)
	})
	return h
}

// follow scrolls list i, when it shows more of the head than is in sight, to
// where the head is: its rows then start under the bar as the shown list's do.
func (h *headed) follow(i int) {
	if i < 0 || i >= h.count {
		return
	}
	ref := h.list(i)
	if ref == nil {
		return
	}
	// The list scrolls once the room under its rows is laid out.
	if h.fit(i, ref) {
		ref.Measure(func(_, _ core.Bounds, ok bool) {
			if ok {
				h.follow(i)
			}
		})
		return
	}
	shift := h.shift
	if h.offsets[i] < h.head.GetUntracked() && h.offsets[i] != shift {
		h.offsets[i] = shift
		h.scrollTo(i, ref, shift)
	}
}

// fit leaves under the rows of list i the room it lacks to scroll the head
// away, and reports whether that changed.
func (h *headed) fit(i int, ref *core.Ref) bool {
	frame, ok := ref.Frame()
	above, hasHeader := h.ends[i][0].Bounds()
	below, hasFooter := h.ends[i][1].Bounds()
	if !ok || !hasHeader || !hasFooter {
		return false
	}
	rest := h.rests[i].GetUntracked()
	want := max(0, frame.Size.Height+h.head.GetUntracked()-(below.Origin.Y-above.Origin.Y))
	if d := want - rest; d > -0.5 && d < 0.5 {
		return false
	}
	h.rests[i].Set(want)
	return true
}

// under makes spec the spec of list i under the head. rows reads what the
// list shows, in an effect: a list whose rows change stays where it was
// under the head, which a list that grew short would bring back.
func (h *headed) under(cx *core.Scope, i int, spec listSpec, rows func()) listSpec {
	spec.header = core.View{Ref: &h.ends[i][0], StyleFunc: func() core.Style { return core.Style{Height: core.Pt(h.top.Get())} }}
	spec.footer = core.View{
		Ref:       &h.ends[i][1],
		StyleFunc: func() core.Style { return core.Style{Height: core.Pt(h.rests[i].Get())} },
		// A list made beside the shown one starts where the head is.
		OnLayout: func(core.Bounds) {
			if i != h.shown.GetUntracked() {
				h.follow(i)
			} else {
				h.fit(i, h.list(i))
			}
		},
	}
	spec.onScroll = h.scrolled(i)
	spec.offset = core.AnimatedXY{Y: h.ys[i]}
	cx.Effect(func() {
		rows()
		h.rowsChanged(i)
	})
	return spec
}

// webPage gives the shared header a native scroll target while the site
// keeps its own viewport and handles scrolling within the document.
func (h *headed) webPage(i int, ref *core.Ref, web core.Node) core.Node {
	return core.Scroll{Ref: ref, Style: fill, HideIndicator: true,
		OnScroll: h.scrolled(i), OffsetValues: core.AnimatedXY{Y: h.ys[i]},
		Children: []core.Node{
			core.View{StyleFunc: func() core.Style { return core.Style{Height: core.Pt(h.top.Get()), FlexShrink: 0} }},
			core.View{StyleFunc: func() core.Style {
				return core.Style{Height: core.Pt(max(0, h.viewport.Get()-h.top.Get()+h.head.Get())), FlexShrink: 0}
			}, OnLayout: func(core.Bounds) {
				if i != h.shown.GetUntracked() {
					h.follow(i)
				}
			}, Children: []core.Node{web}},
		},
	}
}

// rowsChanged refits the space under the rows after layout, preserving the
// head's collapse if the list was already scrolled.
func (h *headed) rowsChanged(i int) {
	ref := h.list(i)
	if ref == nil || h.held[i] {
		return
	}
	at := min(h.offsets[i], h.head.GetUntracked())
	if at > 0 {
		h.held[i] = true
	}
	ref.Measure(func(_, _ core.Bounds, _ bool) {
		h.fit(i, ref)
		if at == 0 {
			return
		}
		ref.Measure(func(_, _ core.Bounds, _ bool) {
			delete(h.held, i)
			if h.offsets[i] < at {
				h.offsets[i] = at
				h.scrollTo(i, ref, at)
			}
		})
	})
}

// scrolled hears list i scroll.
func (h *headed) scrolled(i int) func(core.ScrollEvent) {
	return func(e core.ScrollEvent) {
		if e.Stale {
			return
		}
		h.offsets[i] = max(0, e.Offset.Y)
		if i == h.shown.GetUntracked() && !h.held[i] {
			h.shift = min(h.offsets[i], h.head.GetUntracked())
			h.follow(i - 1)
			h.follow(i + 1)
		}
	}
}

// scrollTo scrolls list i to y. A scroll the app asks for does not move the
// list's offset value on every platform, so it is set along.
func (h *headed) scrollTo(i int, ref *core.Ref, y core.Points) {
	ref.ScrollTo(core.Point{Y: y}, false)
	h.ys[i].Set(float64(y))
}

// lift moves the head up with the shown list, by as much as the list
// scrolled up to the head's height.
func (h *headed) lift() *core.AnimatedStyle {
	lifts, i := h.lifts.Get(), h.shown.Get()
	if i < 0 || i >= len(lifts) {
		return nil
	}
	return &core.AnimatedStyle{TranslateY: lifts[i]}
}

// view lays head and bar over pages.
func (h *headed) view(head, bar, pages core.Node) core.Node {
	return core.View{Style: fill.Merge(core.Style{Overflow: core.Hidden}),
		OnLayout: func(b core.Bounds) { h.viewport.Set(b.Size.Height) }, Children: []core.Node{
			column(fill, "", pages),
			core.View{
				Style:         core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0)},
				PointerEvents: core.PointerEventsBoxNone,
				StyleFunc:     func() core.Style { return core.Style{Animated: h.lift()} },
				OnLayout:      func(b core.Bounds) { h.top.Set(b.Size.Height) },
				// The head and the bar lie over the lists rather than in them:
				// the scrolls that start on them go to the list shown.
				ScrollTarget: func() *core.Ref { return h.list(h.shown.GetUntracked()) },
				Children: []core.Node{
					core.View{OnLayout: func(b core.Bounds) { h.head.Set(b.Size.Height) }, Children: []core.Node{head}},
					bar,
				},
			},
		}}
}

// pageSlide moves count pages that lie side by side in a row. On a touch
// screen a sideways drag moves the row on the platform's main thread
// (PanValues) and letting go springs it, from the finger's speed, to the page
// a throw carries it to, at most one page on; elsewhere the row is a paged
// scroll view. offset follows where the row is, in points from the first
// page, for what moves with it, such as the tabs' indicator.
type pageSlide struct {
	cx     *core.Scope
	byPan  bool
	width  *core.Signal[core.Points]
	count  int
	value  *core.AnimatedValue // the row's translation by pan, its scroll offset otherwise
	offset core.AnimatedNode
	shift  *core.Memo[core.AnimatedNode] // the row's translation, resisting past the ends
	run    *core.AnimationRun
	from   float64 // the value as a drag began
}

// pageSpring settles the row on a page, critically damped.
var pageSpring = core.SpringConfig{Stiffness: 320, Damping: 36}

func newPageSlide(cx *core.Scope, width *core.Signal[core.Points], count int) *pageSlide {
	s := &pageSlide{cx: cx, byPan: swipesByPan(cx), width: width, count: count, value: core.NewAnimatedValue(cx, 0)}
	s.offset = s.value
	if s.byPan {
		s.offset = core.Mul(s.value, core.Constant(-1))
	}
	s.shift = core.NewMemoFunc(cx, func() core.AnimatedNode {
		w := float64(width.Get())
		if !s.byPan || w <= 0 {
			return nil
		}
		return core.Resist(s.value, -float64(count-1)*w, 0, w)
	}, func(a, b core.AnimatedNode) bool { return false })
	return s
}

// row is the view that holds the pages; settle is told the page a drag
// settles on.
func (s *pageSlide) row(pages []core.Node, settle func(i int)) core.View {
	v := core.View{Style: core.Style{Direction: core.Row, Height: core.Pct(100)}, Children: pages}
	if s.byPan {
		v.StyleFunc = func() core.Style {
			if n := s.shift.Get(); n != nil {
				return core.Style{Animated: &core.AnimatedStyle{TranslateX: n}}
			}
			return core.Style{}
		}
		v.PanValues, v.OnPan, v.PanActivation = core.AnimatedXY{X: s.value}, s.pan(settle), swipeActivation
	}
	return v
}

func (s *pageSlide) pan(settle func(i int)) func(core.PanEvent) {
	return func(e core.PanEvent) {
		w := float64(s.width.GetUntracked())
		if w <= 0 {
			return
		}
		switch e.Phase {
		case core.GestureBegan:
			s.from = float64(e.Values.X)
		case core.GestureEnded, core.GestureCancelled:
			start := int(math.Round(-s.from / w))
			to := -(float64(e.Values.X) + float64(e.Velocity.X)*0.2) / w
			i := max(start-1, min(int(math.Round(to)), start+1))
			i = max(0, min(i, s.count-1))
			spring := pageSpring
			spring.Velocity = float64(e.Velocity.X)
			s.slide(i, &spring)
			settle(i)
		}
	}
}

// show moves the row to page i, springing there when animated. A scroll
// view's row is scrolled through ref.
func (s *pageSlide) show(ref *core.Ref, i int, animated bool) {
	w := s.width.GetUntracked()
	if !s.byPan {
		ref.ScrollTo(core.Point{X: core.Points(i) * w}, animated)
		return
	}
	if !animated {
		s.slide(i, nil)
		return
	}
	spring := pageSpring
	s.slide(i, &spring)
}

func (s *pageSlide) slide(i int, spring *core.SpringConfig) {
	if s.run != nil {
		s.run.Stop()
		s.run = nil
	}
	to := -float64(i) * float64(s.width.GetUntracked())
	if spring == nil {
		s.value.Set(to)
		return
	}
	s.run = s.cx.StartAnimation(core.Spring(s.value, to, *spring), nil)
}

// scroll is what a scroll view of the row follows: its offset, when it is
// the scroll view that moves the row.
func (s *pageSlide) scroll() core.AnimatedXY {
	if s.byPan {
		return core.AnimatedXY{}
	}
	return core.AnimatedXY{X: s.value}
}

// tabPages lays count pages side by side under the tabs that selected marks
// and slides between them (pageSlide): a change of selected glides to its
// page. A page is built when it is first shown or beside the shown one.
func tabPages(cx *core.Scope, selected *core.Signal[int], slide *pageSlide, page func(cx *core.Scope, i int) core.Node) core.Node {
	var ref core.Ref
	width := slide.width
	// shown is the page the scroll is at or on its way to; glide says it is
	// on its way, passing pages it does not choose.
	shown, glide := -1, false
	show := func(i int, animated bool) {
		shown, glide = i, animated
		slide.show(&ref, i, animated)
	}
	cx.Effect(func() {
		if i := selected.Get(); i != shown && width.GetUntracked() > 0 {
			show(i, true)
		}
	})
	pages := make([]core.Node, slide.count)
	for i := range pages {
		built := false
		pages[i] = core.View{
			StyleFunc: func() core.Style {
				return core.Style{Width: core.Pt(width.Get()), Height: core.Pct(100), Overflow: core.Hidden}
			},
			Children: []core.Node{core.Show{
				When: func() bool {
					d := selected.Get() - i
					built = built || width.Get() > 0 && d >= -1 && d <= 1
					return built
				},
				Then: func(cx *core.Scope) core.Node { return page(cx, i) },
			}},
		}
	}
	row := slide.row(pages, func(i int) {
		shown = i
		selected.Set(i)
	})
	return core.Scroll{
		Horizontal: true, HideIndicator: true, Ref: &ref,
		Behavior:     core.ScrollBehavior{Paging: true, NoScroll: slide.byPan},
		OffsetValues: slide.scroll(),
		Style:        fill.Merge(core.Style{Direction: core.Row}),
		OnLayout: func(b core.Bounds) {
			if w := b.Size.Width; w > 0 && w != width.GetUntracked() {
				width.Set(w)
				show(selected.GetUntracked(), false)
			}
		},
		OnScrollDragStart: func(core.ScrollEvent) { glide = false },
		OnScroll: func(e core.ScrollEvent) {
			if slide.byPan || glide || e.Stale || e.Size.Width == 0 {
				return
			}
			if i := int(e.Offset.X/e.Size.Width + 0.5); i != shown {
				shown = i
				selected.Set(i)
			}
		},
		Children: []core.Node{row},
	}
}

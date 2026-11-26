package app

import "github.com/nsgo-dev/nsgo/core"

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
	cx    *core.Scope
	shown *core.Signal[int]
	count int
	list  func(i int) *core.Ref
	// head is the head's height, top the head's with the bar's, and shift how
	// far the head is scrolled away.
	head, top, shift *core.Signal[core.Points]
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
	// from is where the shown list was as a drag on the head began, and
	// glide stops the glide a throw of it set off.
	from  core.Points
	glide func()
}

// newHeaded is a head over count lists that list returns the refs of, nil for
// one not made yet, of which shown is in sight.
func newHeaded(cx *core.Scope, shown *core.Signal[int], count int, list func(i int) *core.Ref) *headed {
	h := &headed{cx: cx, shown: shown, count: count, list: list, head: core.NewSignal(cx, core.Points(0)), top: core.NewSignal(cx, core.Points(0)),
		shift: core.NewSignal(cx, core.Points(0)), offsets: map[int]core.Points{}, held: map[int]bool{}}
	h.rests, h.ends = make([]*core.Signal[core.Points], count), make([][2]core.Ref, count)
	for i := range h.rests {
		h.rests[i] = core.NewSignal(cx, core.Points(0))
	}
	// The head goes where the list that comes in sight has scrolled it.
	cx.Effect(func() {
		i := shown.Get()
		h.shift.Set(min(h.offsets[i], h.head.Get()))
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
	shift := h.shift.GetUntracked()
	if h.offsets[i] < h.head.GetUntracked() && h.offsets[i] != shift {
		h.offsets[i] = shift
		ref.ScrollTo(core.Point{Y: shift}, false)
	}
}

// fit leaves under the rows of list i the room it lacks to scroll the head
// away, and reports whether that changed.
func (h *headed) fit(i int, ref *core.Ref) bool {
	frame, ok := ref.Frame()
	above, _ := h.ends[i][0].Bounds()
	below, _ := h.ends[i][1].Bounds()
	if !ok {
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
			}
		},
	}
	spec.onScroll = h.scrolled(i)
	cx.Effect(func() {
		rows()
		h.keep(i)
	})
	return spec
}

// keep holds list i where it is under the head while its changed rows are
// laid out.
func (h *headed) keep(i int) {
	ref := h.list(i)
	at := min(h.offsets[i], h.head.GetUntracked())
	if ref == nil || at == 0 || h.held[i] {
		return
	}
	h.held[i] = true
	ref.Measure(func(_, _ core.Bounds, _ bool) {
		h.fit(i, ref)
		ref.Measure(func(_, _ core.Bounds, _ bool) {
			delete(h.held, i)
			if h.offsets[i] < at {
				h.offsets[i] = at
				ref.ScrollTo(core.Point{Y: at}, false)
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
			h.shift.Set(min(h.offsets[i], h.head.GetUntracked()))
			h.follow(i - 1)
			h.follow(i + 1)
		}
	}
}

// drag scrolls the shown list by a vertical drag that began on the head or
// the bar on iOS, and glides it on as the finger lets go. A sideways drag
// stays the head's own rows'. Elsewhere the head hands its scrolls to the
// list natively (View.ScrollTarget).
func (h *headed) drag(e core.PanEvent) {
	i := h.shown.GetUntracked()
	ref := h.list(i)
	if ref == nil {
		return
	}
	switch e.Phase {
	case core.GestureBegan:
		if h.glide != nil {
			h.glide()
		}
		h.from = h.offsets[i]
	case core.GestureChanged:
		ref.ScrollTo(core.Point{Y: max(h.from-e.Translation.Y, 0)}, false)
	case core.GestureEnded:
		// An ease-out cubic starts at three times its mean speed, so this
		// glide leaves at the finger's.
		y := max(h.from-e.Translation.Y, 0)
		distance := -e.Velocity.Y * core.Points(glideTime.Seconds()/3)
		h.glide = h.cx.Animate(glideTime, easeOutCubic, func(k float64) {
			ref.ScrollTo(core.Point{Y: max(y+distance*core.Points(k), 0)}, false)
		})
	}
}

// iosDrag is drag on iOS, whose backend hands no touches to a ScrollTarget,
// nil elsewhere.
func (h *headed) iosDrag() func(core.PanEvent) {
	if h.cx.Env().OS() != core.OSIOS {
		return nil
	}
	return h.drag
}

// view lays head and bar over pages.
func (h *headed) view(head, bar, pages core.Node) core.Node {
	return core.View{Style: fill.Merge(core.Style{Overflow: core.Hidden}), Children: []core.Node{
		column(fill, "", pages),
		core.View{
			Style:         core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0)},
			PointerEvents: core.PointerEventsBoxNone,
			StyleFunc:     func() core.Style { return core.Style{Transform: core.Transform{TranslateY: -h.shift.Get()}} },
			OnLayout:      func(b core.Bounds) { h.top.Set(b.Size.Height) },
			// The head and the bar lie over the lists rather than in them:
			// the scrolls that start on them go to the list shown.
			ScrollTarget:  func() *core.Ref { return h.list(h.shown.GetUntracked()) },
			OnPan:         h.iosDrag(),
			PanActivation: core.PanActivation{ActiveOffsetY: 10, FailOffsetX: 10},
			Children: []core.Node{
				core.View{OnLayout: func(b core.Bounds) { h.head.Set(b.Size.Height) }, Children: []core.Node{head}},
				bar,
			},
		},
	}}
}

// tabPages lays count pages side by side under the tabs that selected marks
// and slides between them: a sideways drag follows the finger and settles on
// the page beside, and a change of selected glides to its page. at tells the
// tabs where between the pages the scroll is. A page is built when it is
// first shown or beside the shown one.
func tabPages(cx *core.Scope, selected *core.Signal[int], at *core.Signal[float32], count int, page func(cx *core.Scope, i int) core.Node) core.Node {
	var ref core.Ref
	width := core.NewSignal(cx, core.Points(0))
	// shown is the page the scroll is at or on its way to; glide says it is
	// on its way, passing pages it does not choose.
	shown, glide := -1, false
	show := func(i int, animated bool) {
		shown, glide = i, animated
		ref.ScrollTo(core.Point{X: core.Points(i) * width.GetUntracked()}, animated)
	}
	cx.Effect(func() {
		if i := selected.Get(); i != shown && width.GetUntracked() > 0 {
			show(i, true)
		}
	})
	pages := make([]core.Node, count)
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
	row := core.View{Style: core.Style{Direction: core.Row, Height: core.Pct(100)}, Children: pages}
	byPan := swipesByPan(cx)
	if byPan {
		sp := &swipePager{ref: &ref, width: width.GetUntracked, count: count, at: selected.GetUntracked,
			goTo: func(i int) {
				show(i, true)
				selected.Set(i)
			}}
		row.OnPan, row.PanActivation = sp.pan, swipeActivation
	}
	return core.Scroll{
		Horizontal: true, HideIndicator: true, Ref: &ref,
		Behavior: core.ScrollBehavior{Paging: true, NoScroll: byPan},
		Style:    fill.Merge(core.Style{Direction: core.Row}),
		OnLayout: func(b core.Bounds) {
			if w := b.Size.Width; w > 0 && w != width.GetUntracked() {
				width.Set(w)
				show(selected.GetUntracked(), false)
			}
		},
		OnScrollDragStart: func(core.ScrollEvent) { glide = false },
		OnScroll: func(e core.ScrollEvent) {
			if e.Size.Width > 0 {
				at.Set(float32(e.Offset.X / e.Size.Width))
			}
			if byPan || glide || e.Stale || e.Size.Width == 0 {
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

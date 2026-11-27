package app

import (
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// spinner is an indeterminate progress ring in the primary color.
func spinner() core.Node { return ui.Spinner{Size: 28, LineWidth: 3} }

// textButton is a Material 3 text button.
func textButton(text string, fn func()) core.Node {
	return tap(text, fn, core.Style{Height: core.Pt(40), Padding: core.Symmetric(0, 12), CornerRadius: 20, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, "",
		label(text, textSecondary, 500, "text-primary"))
}

// tonalButton is a Material 3 filled tonal button.
func tonalButton(text string, fn func()) core.Node {
	return tap(text, fn, core.Style{Padding: core.Symmetric(10, 24), CornerRadius: 20, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, "bg-secondary-container",
		label(text, textSecondary, 500, "text-on-secondary-container"))
}

// statusBlock is the centered title, detail and action of an empty or
// failed list.
func statusBlock(title, detail, action string, fn func()) core.Node {
	kids := []core.Node{core.Text{Content: title, Class: "text-on-surface", Style: func() core.Style {
		s := textStyle(textBody, 500)
		s.TextAlign = core.TextAlignCenter
		return s
	}()}}
	if detail != "" {
		kids = append(kids, core.Text{Content: detail, Class: "text-on-surface-variant", MaxLines: 4, Style: func() core.Style {
			s := textStyle(textSecondary, 400)
			s.TextAlign = core.TextAlignCenter
			return s
		}()})
	}
	if action != "" && fn != nil {
		kids = append(kids, core.View{Style: core.Style{Height: core.Pt(8)}}, tonalButton(action, fn))
	}
	return column(core.Style{Padding: core.Symmetric(24, 32), Gap: 8, AlignItems: core.AlignCenter}, "", kids...)
}

// errorBanner is a dismissible error-container strip above loaded content.
func errorBanner(text string, retry, dismiss func()) core.Node {
	kids := []core.Node{core.Text{Content: text, Class: "text-on-error-container", MaxLines: 2, Style: func() core.Style {
		s := textStyle(textSecondary, 400)
		s.FlexGrow, s.FlexShrink, s.FlexBasis = 1, 1, core.Pt(0)
		return s
	}()}}
	if retry != nil {
		kids = append(kids, textButtonOn("重试", retry, "text-on-error-container"))
	}
	if dismiss != nil {
		kids = append(kids, textButtonOn("关闭", dismiss, "text-on-error-container"))
	}
	return row(core.Style{Padding: core.Edges{Top: 4, Right: 8, Bottom: 4, Left: 16}, Gap: 4, AlignItems: core.AlignCenter}, "bg-error-container", kids...)
}

func textButtonOn(text string, fn func(), class string) core.Node {
	return tap(text, fn, core.Style{Height: core.Pt(40), Padding: core.Symmetric(0, 12), CornerRadius: 20, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}, "",
		label(text, textSecondary, 500, class))
}

// listItem is a Material 3 one-line list item: a leading icon, a title and
// a trailing node.
func listItem(glyph *symbol, title string, trailing core.Node, fn func()) core.Node {
	kids := []core.Node{icon(glyph, 24, "text-on-surface-variant"),
		core.Text{Content: title, MaxLines: 1, Class: "text-on-surface", Style: fill.Merge(textStyle(textBody, 400))}}
	if trailing != nil {
		kids = append(kids, trailing)
	}
	return tap(title, fn, core.Style{Direction: core.Row, Height: core.Pt(56), Padding: core.Symmetric(0, 16), Gap: 16, AlignItems: core.AlignCenter}, "", kids...)
}

// badge is a small count in the error color.
func badge(n func() int) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		if n() <= 0 {
			return core.Group{}
		}
		return core.Text{ContentFunc: func() string { return formatCount(n()) }, Class: "bg-error text-on-error", Style: func() core.Style {
			s := textStyle(textCaption, 500)
			s.Padding, s.CornerRadius, s.MinWidth = core.Symmetric(0, 6), 8, core.Pt(16)
			s.TextAlign = core.TextAlignCenter
			return s
		}()}
	})
}

// toggleRow is a row with a Material 3 switch.
func toggleRow(title string, on func() bool, set func(bool)) core.Node {
	return row(core.Style{Height: core.Pt(56), Padding: core.Symmetric(0, 16), Gap: 16, AlignItems: core.AlignCenter}, "",
		core.Text{Content: title, Class: "text-on-surface", Style: fill.Merge(textStyle(textBody, 400))},
		m3Switch(on, set, title),
	)
}

// toggleOptionRow is a switch row with supporting text and an optional enabled
// predicate that dims and disables the control when false.
func toggleOptionRow(title, subtitle string, on, enabled func() bool, set func(bool)) core.Node {
	isEnabled := func() bool { return enabled == nil || enabled() }
	sw := m3Switch(on, func(v bool) {
		if isEnabled() {
			set(v)
		}
	}, title)
	return core.View{
		Style: core.Style{Direction: core.Row, MinHeight: core.Pt(64), Padding: core.Symmetric(10, 16), Gap: 16, AlignItems: core.AlignCenter},
		StyleFunc: func() core.Style {
			if isEnabled() {
				return core.Style{}
			}
			return core.Style{Opacity: core.Alpha(0.38)}
		},
		Children: []core.Node{
			column(fill.Merge(core.Style{Gap: 2}), "",
				core.Text{Content: title, Class: "text-on-surface", Style: textStyle(textBody, 400)},
				core.Text{Content: subtitle, Class: "text-on-surface-variant", MaxLines: 3, Style: textStyle(textSecondary, 400)}),
			sw,
		},
	}
}

// simpleTabs are Material 3 primary tabs: equal-width labels, the selected
// one in primary, over a rule. The indicator moves with the pages slide
// shows, on the platform's main thread.
func simpleTabs(cx *core.Scope, selected *core.Signal[int], slide *pageSlide, titles ...string) core.Node {
	tabs := make([]core.Node, len(titles))
	for i, title := range titles {
		i := i
		tabs[i] = pressable(core.View{
			Role: core.RoleTab, Label: title,
			Style:   fill.Merge(core.Style{Height: core.Pct(100), JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter, Overflow: core.Hidden}),
			OnPress: func(core.PressEvent) { selected.Set(i) },
			Children: []core.Node{core.Text{Content: title, Style: textStyle(textSecondary, 500), Class: "text-on-surface-variant", ClassFunc: func() string {
				if selected.Get() == i {
					return "text-primary"
				}
				return "text-on-surface-variant"
			}}},
		})
	}
	n := len(titles)
	strip := core.NewSignal(cx, core.Points(0))
	move := core.NewMemoFunc(cx, func() core.AnimatedNode {
		w, tab := float64(slide.width.Get()), float64(strip.Get())/float64(n)
		if n < 2 || w <= 0 || tab <= 0 {
			return nil
		}
		return core.Interpolate(slide.offset, core.Interpolation{Input: []float64{0, float64(n-1) * w},
			Output: []float64{0, float64(n-1) * tab}, Extrapolate: core.ExtrapolateClamp})
	}, func(a, b core.AnimatedNode) bool { return false })
	indicator := core.View{
		PointerEvents: core.PointerEventsNone,
		Style:         core.Style{Position: core.Absolute, Bottom: core.Pt(0), Left: core.Pt(0), Width: core.Pct(100 / float32(n)), AlignItems: core.AlignCenter},
		StyleFunc: func() core.Style {
			if m := move.Get(); m != nil {
				return core.Style{Animated: &core.AnimatedStyle{TranslateX: m}}
			}
			return core.Style{}
		},
		Children: []core.Node{core.View{Class: "bg-primary",
			Style: core.Style{Width: core.Pt(32), Height: core.Pt(3), CornerRadii: core.Corners{TopLeft: 3, TopRight: 3}}}},
	}
	return column(core.Style{FlexShrink: 0}, "bg-background",
		core.View{Style: core.Style{Height: core.Pt(48), Padding: core.Symmetric(0, 8)}, Children: []core.Node{
			core.View{Style: fill.Merge(core.Style{Direction: core.Row}), OnLayout: func(b core.Bounds) { strip.Set(b.Size.Width) },
				Children: append(tabs, indicator)}}},
		core.View{Style: core.Style{Height: core.Pt(1)}, Class: "bg-outline-variant"})
}

// filterChip is a Material 3 filter chip. Its border stays, transparent
// while selected, so selecting it moves nothing but the check it leads with.
func filterChip(name string, on func() bool, fn func()) core.View { return chip(name, on, fn, true) }

// compactChip is a filter chip without the check, for a row of choices whose
// places must not move as one is chosen, such as a search's sort.
func compactChip(name string, on func() bool, fn func()) core.View { return chip(name, on, fn, false) }

func chip(name string, on func() bool, fn func(), check bool) core.View {
	leads := func() bool { return check && on() }
	return pressable(core.View{
		Role: core.RoleButton, Label: name, Selected: on, OnPress: func(core.PressEvent) { fn() },
		ClassFunc: func() string {
			if on() {
				return "bg-secondary-container border-transparent"
			}
			return "border-outline-variant"
		},
		// Selected, the chip leads with a check and keeps 8dp before it.
		StyleFunc: func() core.Style {
			if leads() {
				return core.Style{Padding: core.Edges{Left: 8, Right: 12}}
			}
			return core.Style{}
		},
		Style: core.Style{Direction: core.Row, Height: core.Pt(32), Padding: core.Symmetric(0, 12), Gap: 8, BorderWidth: 1, CornerRadius: 8, AlignItems: core.AlignCenter,
			JustifyContent: core.JustifyCenter, Overflow: core.Hidden},
		// The chip takes presses over 48dp, as Material 3 asks of a target.
		HitSlop: core.Edges{Top: 8, Bottom: 8},
		Children: []core.Node{
			core.Show{When: leads, Then: func(*core.Scope) core.Node { return icon(&symbols.Check, 18, "text-on-secondary-container") }},
			core.Text{Content: name, Style: textStyle(textSecondary, 500), ClassFunc: func() string {
				if on() {
					return "text-on-secondary-container"
				}
				return "text-on-surface-variant"
			}}},
	})
}

// filledButton is a Material 3 filled button, dimmed while enabled says no.
func filledButton(text string, enabled func() bool, fn func()) core.Node {
	return pressable(core.View{
		Role: core.RoleButton, Label: text,
		OnPress: func(core.PressEvent) {
			if enabled() {
				fn()
			}
		},
		Class: "bg-primary",
		StyleFunc: func() core.Style {
			if enabled() {
				return core.Style{}
			}
			return core.Style{Opacity: core.Alpha(0.4)}
		},
		Style:    core.Style{Padding: core.Symmetric(10, 24), CornerRadius: 20, Overflow: core.Hidden, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		Children: []core.Node{label(text, textSecondary, 500, "text-on-primary")},
	})
}

// m3Switch is a Material 3 switch: a 52x32 track whose thumb grows from 16 to
// 24dp and slides across as it turns on, the track filling with the primary
// color; off, it is outlined.
func m3Switch(on func() bool, set func(bool), name string) core.Node {
	move := core.Transition{Duration: 200 * time.Millisecond, Easing: emphasized,
		Properties: core.TransitionTransform | core.TransitionBackground | core.TransitionOpacity}
	track := core.View{
		Role: core.RoleSwitch, Label: name, Checked: on,
		OnPress: func(core.PressEvent) { set(!on()) },
		ClassFunc: func() string {
			if on() {
				return "bg-primary"
			}
			return "bg-outline"
		},
		Style: core.Style{Width: core.Pt(52), Height: core.Pt(32), CornerRadius: 16, FlexShrink: 0, Overflow: core.Hidden,
			Transition: move},
		Children: []core.Node{
			core.View{Class: "bg-surface-container-highest", Style: insetAbs(2).Merge(core.Style{CornerRadius: 14, Transition: move}),
				StyleFunc: func() core.Style {
					if on() {
						return core.Style{Opacity: core.Alpha(0)}
					}
					return core.Style{Opacity: core.Alpha(1)}
				}},
			core.View{
				ClassFunc: func() string {
					if on() {
						return "bg-on-primary"
					}
					return "bg-outline"
				},
				Style: core.Style{Position: core.Absolute, Top: core.Pt(4), Left: core.Pt(4), Width: core.Pt(24), Height: core.Pt(24), CornerRadius: 12, Transition: move},
				StyleFunc: func() core.Style {
					if on() {
						return core.Style{Transform: core.Transform{TranslateX: 20, Scale: 1}}
					}
					return core.Style{Transform: core.Transform{TranslateX: 0, Scale: 16.0 / 24}}
				}},
		},
	}
	return track
}

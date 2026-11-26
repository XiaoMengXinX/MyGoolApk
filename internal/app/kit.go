package app

import (
	"github.com/nsgo-dev/nsgo/core"
)

// tap makes a view that runs fn when pressed, with Android's ripple.
func tap(label string, fn func(), s core.Style, class string, children ...core.Node) core.View {
	// A pressable that clips its content takes touches only within its bounds:
	// Android hands a touch outside every child to the last child that does not
	// clip, which would take it from the pressables before it.
	if s.Overflow == 0 {
		s.Overflow = core.Hidden
	}
	return pressable(core.View{
		Class: class, Style: s, Role: core.RoleButton, Label: label,
		OnPress:  func(core.PressEvent) { fn() },
		Children: children,
	})
}

// hoverable says the app runs where a pointer hovers: a desktop's, set once
// as the app starts.
var hoverable bool

// pressable gives a view that takes presses the feedback of Material 3:
// Android's ripple, and hoverFeedback.
func pressable(v core.View) core.View {
	v.Android.Ripple = &core.Ripple{}
	return hoverFeedback(v)
}

// pointing gives a view that takes presses the pointing hand, where a
// pointer hovers.
func pointing(v core.View) core.View {
	if hoverable {
		v.Style.Cursor = core.CursorPointer
	}
	return v
}

// hoverFeedback is pointing with an on-surface state layer at 8% over the
// view while hovered.
func hoverFeedback(v core.View) core.View {
	if !hoverable {
		return v
	}
	v = pointing(v)
	return hoverGroup(v, "press", fillAbs)
}

// hoverGroup makes v the group named group, and lays over it at place a
// state layer shown while the pointer is over v or anything in it: a press
// area under others that take presses shows its feedback through them.
func hoverGroup(v core.View, group string, place core.Style) core.View {
	if !hoverable {
		return v
	}
	v.Style.Group = group
	layer := core.View{Class: "bg-on-surface", PointerEvents: core.PointerEventsNone,
		Style: place.Merge(core.Style{Opacity: core.Alpha(0), GroupStyle: &core.GroupStyle{Name: group, Hover: &core.Style{Opacity: core.Alpha(0.08)}}})}
	v.Children = append(v.Children[:len(v.Children):len(v.Children)], layer)
	return v
}

// row and column are flex containers.
func row(s core.Style, class string, children ...core.Node) core.Node {
	s.Direction = core.Row
	return core.View{Class: class, Style: s, Children: children}
}

func column(s core.Style, class string, children ...core.Node) core.Node {
	return core.View{Class: class, Style: s, Children: children}
}

// label is a run of text in one of the type scale's sizes.
func label(content string, size core.Points, weight core.FontWeight, class string) core.Node {
	return core.Text{Content: content, Class: class, Style: textStyle(size, weight)}
}

func labelFunc(content func() string, size core.Points, weight core.FontWeight, class string) core.Node {
	return core.Text{ContentFunc: content, Class: class, Style: textStyle(size, weight)}
}

// fill is the style of a view that takes the rest of its parent.
var fill = core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinHeight: core.Pt(0), MinWidth: core.Pt(0)}

// spacer pushes its siblings apart.
var spacer core.Node = core.View{Style: core.Style{FlexGrow: 1}}

// fillAbs fills the padding box of its parent whatever size the parent has
// got from, which a percentage of an aspect-ratio box does not always: a
// picture sized so came out 0 by 0 in some rows.
var fillAbs = core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}

// insetAbs is fillAbs inset by d on every side.
func insetAbs(d core.Points) core.Style {
	return core.Style{Position: core.Absolute, Top: core.Pt(d), Left: core.Pt(d), Right: core.Pt(d), Bottom: core.Pt(d)}
}

// bottomSpace is room below the last row of a page. A pushed page draws under
// the system's bottom bar, so it adds the bar's room too; a destination of the
// shell ends above its own bar.
func bottomSpace(h core.Points, pushed bool) core.Node {
	if !pushed {
		return core.View{Style: core.Style{Height: core.Pt(h), FlexShrink: 0}}
	}
	return column(core.Style{FlexShrink: 0}, "",
		core.View{Style: core.Style{Height: core.Pt(h)}},
		core.View{Style: core.Style{PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}}})
}

// through lets a press on the room of a container that none of its children
// takes go to what lies under it, as the press area under a card's parts.
func through(n core.Node) core.Node {
	v := n.(core.View)
	v.PointerEvents = core.PointerEventsBoxNone
	return v
}

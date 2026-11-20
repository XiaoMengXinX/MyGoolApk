package app

import (
	"github.com/nsgo-dev/nsgo/core"
)

// tap makes a view that runs fn when pressed, with Android's ripple.
func tap(label string, fn func(), s core.Style, class string, children ...core.Node) core.Node {
	return core.View{
		Class: class, Style: s, Role: core.RoleButton, Label: label,
		OnPress:  func(core.PressEvent) { fn() },
		Android:  core.AndroidViewProps{Ripple: &core.Ripple{}},
		Children: children,
	}
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

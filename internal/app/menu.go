package app

import (
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// optionMenu is a tonal button that shows the selected one of labels and
// opens a Material 3 menu of them, the selected one checked.
func optionMenu(name string, labels []string, selected func() int, pick func(int)) core.Node {
	items := make([]core.Node, len(labels))
	for i, text := range labels {
		on := func() bool { return selected() == i }
		items[i] = base.MenuItem{Label: text, Role: core.RoleMenuItemRadio, OnSelect: func() { pick(i) }, Checked: on,
			Style: core.Style{Height: core.Pt(48), Padding: core.Symmetric(0, 16), Direction: core.Row, Gap: 12, AlignItems: core.AlignCenter, MinWidth: core.Pt(156)},
			Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(18), Height: core.Pt(18)}, Children: []core.Node{
					core.Show{When: on, Then: func(*core.Scope) core.Node { return icon(&symbols.Check, 18, "text-primary") }},
				}},
				label(text, textSecondary, 500, "text-on-surface"),
			}}
	}
	current := func() string { return labels[selected()] }
	rest := core.Style{Opacity: core.Alpha(0), Transform: core.Transform{TranslateY: -6}}
	return base.Menu{Align: core.AlignEnd, Offset: 4, Margin: 8,
		LabelFunc: func() string { return name + "，" + current() },
		Style:     core.Style{FlexShrink: 0, Height: core.Pt(40), Padding: core.Edges{Left: 16, Right: 8}, CornerRadius: 20, JustifyContent: core.JustifyCenter},
		Class:     "bg-surface-container", PopupClass: "bg-surface-container-high",
		PopupStyle: core.Style{CornerRadius: 16, Padding: core.Symmetric(6, 0),
			Transition: core.Transition{Duration: 180 * time.Millisecond, Easing: emphasized, Properties: core.TransitionOpacity | core.TransitionTransform}},
		StartingStyle: rest, EndingStyle: rest,
		Trigger: row(core.Style{Gap: 4, AlignItems: core.AlignCenter}, "",
			core.Text{ContentFunc: current, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textSecondary, 500)},
			core.View{Style: core.Style{Width: core.Pt(18), Height: core.Pt(18), Transform: core.Transform{Rotate: 90}},
				Children: []core.Node{icon(&symbols.ChevronRight, 18, "text-on-surface-variant")}}),
		Children: items}
}

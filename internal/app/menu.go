package app

import (
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// option is a choice of a menu or a row of chips, by its value.
type option struct {
	label, value string
}

// optionMenu is a Material 3 menu of options behind an outlined button that
// shows the selected one: a 32dp button in a 48dp press area.
func optionMenu(cx *core.Scope, name string, align core.Align, options []option, selected func() string, apply func(string)) core.Node {
	items := make([]core.Node, len(options))
	for i, opt := range options {
		active := func() bool { return selected() == opt.value }
		items[i] = base.MenuItem{Label: opt.label, Role: core.RoleMenuItemRadio, Checked: active,
			OnSelect: func() { apply(opt.value) },
			Style:    core.Style{Height: core.Pt(48), Padding: core.Symmetric(0, 12), Direction: core.Row, Gap: 12, AlignItems: core.AlignCenter, MinWidth: core.Pt(168)},
			Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(20), Height: core.Pt(20)}, Children: []core.Node{
					core.Show{When: active, Then: func(*core.Scope) core.Node { return icon(&symbols.Check, 20, "text-primary") }},
				}},
				label(opt.label, textBody, 400, "text-on-surface"),
			}}
	}
	value := func() string { return optionLabel(options, selected()) }
	popup := core.Style{CornerRadius: 8, Padding: core.Symmetric(8, 0),
		Transition: core.Transition{Duration: 200 * time.Millisecond, Easing: emphasized, Properties: core.TransitionOpacity | core.TransitionTransform}}
	if cx.Env().OS() == core.OSAndroid {
		// Android maps ShadowRadius to native elevation and composites it with the popup.
		popup.ShadowRadius, popup.ShadowColor = 3, core.RGB(0)
	} else {
		popup.Shadows = []core.BoxShadow{
			{Offset: core.Point{Y: 1}, Blur: 2, Color: core.RGBA(0, 0.30)},
			{Offset: core.Point{Y: 2}, Blur: 6, Spread: 2, Color: core.RGBA(0, 0.15)},
		}
	}
	rest := core.Style{Opacity: core.Alpha(0), Transform: core.Transform{TranslateY: -4}}
	return base.Menu{Placement: core.PlaceBottom, Align: align, Offset: 8, Margin: 8,
		LabelFunc:     func() string { return name + "，" + value() },
		Style:         core.Style{FlexShrink: 0, Height: core.Pt(48)},
		PopupClass:    "bg-surface-container",
		PopupStyle:    popup,
		StartingStyle: rest, EndingStyle: rest,
		// Menu uses the bounds of its trigger content. Give the content the full touch height.
		Trigger: core.View{Style: core.Style{Height: core.Pt(48), JustifyContent: core.JustifyCenter},
			Children: []core.Node{row(core.Style{Height: core.Pt(32), Padding: core.Symmetric(0, 8), Gap: 4, AlignItems: core.AlignCenter, BorderWidth: 1, CornerRadius: 8}, "border-outline-variant",
				core.Text{ContentFunc: value, MaxLines: 1, Class: "text-on-surface-variant", Style: textStyle(textSecondary, 500)},
				core.View{Style: core.Style{Width: core.Pt(18), Height: core.Pt(18), Transform: core.Transform{Rotate: 90}},
					Children: []core.Node{icon(&symbols.ChevronRight, 18, "text-on-surface-variant")}})}},
		Children: items}
}

func optionLabel(options []option, value string) string {
	for _, opt := range options {
		if opt.value == value {
			return opt.label
		}
	}
	return options[0].label
}

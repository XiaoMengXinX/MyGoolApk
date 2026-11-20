package app

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// bottomSheet is a Material 3 modal bottom sheet: a 28dp rounded surface with
// a drag handle over a scrim. It rises above the software keyboard and the
// system's bottom bar, so a field in it stays in sight.
func (a *App) bottomSheet(open func() bool, onClose func(), children ...core.Node) core.Node {
	env := a.cx.Env()
	handle := core.View{Class: "bg-on-surface-variant/40", Style: core.Style{Width: core.Pt(32), Height: core.Pt(4), CornerRadius: 2, AlignSelf: core.AlignCenter, FlexShrink: 0}}
	return base.Sheet{
		Open: open,
		OnOpenChange: func(isOpen bool, _ base.Reason) {
			if !isOpen {
				onClose()
			}
		},
		CloseOnSwipe:  true,
		BackdropStyle: core.Style{Background: core.RGBA(0x000000, 0.32)},
		PopupClass:    "bg-surface-container-low",
		PopupStyleFunc: func() core.Style {
			bottom := max(env.SafeArea().Bottom, env.KeyboardHeight())
			return core.Style{Padding: core.Edges{Top: 12, Right: 24, Bottom: bottom + 16, Left: 24}, Gap: 12,
				CornerRadii: core.Corners{TopLeft: 28, TopRight: 28}}
		},
		Children: append([]core.Node{handle}, children...),
	}
}

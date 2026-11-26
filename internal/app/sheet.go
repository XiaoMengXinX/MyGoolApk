package app

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// bottomSheet is a Material 3 modal bottom sheet: a 28dp rounded surface with
// a drag handle over a scrim. It rises above the software keyboard and the
// system's bottom bar, so a field in it stays in sight.
func (a *App) bottomSheet(open func() bool, onClose func(), children ...core.Node) core.Node {
	return a.sheet(open, onClose, "bg-surface-container-low", 24, children...)
}

// sheet is a bottom sheet of class whose content keeps side from its edges.
// A window wider than a phone's shows it as a dialog in its middle, as
// Material 3 does on larger screens.
func (a *App) sheet(open func() bool, onClose func(), class string, side core.Points, children ...core.Node) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		if a.wide() {
			return a.sheetDialog(open, onClose, class, side, children...)
		}
		return a.bottomSheetOf(open, onClose, class, side, children...)
	})
}

func (a *App) sheetDialog(open func() bool, onClose func(), class string, side core.Points, children ...core.Node) core.Node {
	return base.Dialog{
		Open: open,
		OnOpenChange: func(isOpen bool, _ base.Reason) {
			if !isOpen {
				onClose()
			}
		},
		// A dialog fades in where it is, scrim and all, as Material 3's do.
		Animation:     core.ModalFade,
		BackdropStyle: core.Style{Background: core.RGBA(0x000000, 0.32)},
		PopupClass:    class,
		PopupStyle: core.Style{Width: core.Pt(dialogWidth), MaxWidth: core.Pct(90), MaxHeight: core.Pct(85), CornerRadius: 28, Overflow: core.Hidden,
			Padding: core.Edges{Top: 24, Right: side, Bottom: 24, Left: side}, Gap: 12},
		Children: children,
	}
}

func (a *App) bottomSheetOf(open func() bool, onClose func(), class string, side core.Points, children ...core.Node) core.Node {
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
		PopupClass:    class,
		PopupStyleFunc: func() core.Style {
			bottom := max(env.SafeArea().Bottom, env.KeyboardHeight())
			return core.Style{Padding: core.Edges{Top: 12, Right: side, Bottom: bottom + 16, Left: side}, Gap: 12,
				CornerRadii: core.Corners{TopLeft: 28, TopRight: 28}}
		},
		Children: append([]core.Node{handle}, children...),
	}
}

package app

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// spinner is an indeterminate progress ring in the primary color.
func spinner() core.Node { return ui.Spinner{Size: 28, LineWidth: 3} }

// textButton is a Material 3 text button.
func textButton(text string, fn func()) core.Node {
	return tap(text, fn, core.Style{Padding: core.Symmetric(8, 12), CornerRadius: 20, Overflow: core.Hidden}, "",
		label(text, textSecondary, 500, "text-primary"))
}

// tonalButton is a Material 3 filled tonal button.
func tonalButton(text string, fn func()) core.Node {
	return tap(text, fn, core.Style{Padding: core.Symmetric(10, 24), CornerRadius: 20, Overflow: core.Hidden}, "bg-secondary-container",
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
	return tap(text, fn, core.Style{Padding: core.Symmetric(8, 12), CornerRadius: 20, Overflow: core.Hidden}, "",
		label(text, textSecondary, 500, class))
}

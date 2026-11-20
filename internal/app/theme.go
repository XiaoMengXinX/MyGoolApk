package app

import "github.com/nsgo-dev/nsgo/core"

// The type scale keeps four sizes; weight and color carry the rest of the
// hierarchy. They are the Material 3 title-large, body-large, body-medium
// and body-small roles.
const (
	textTitle     core.Points = 22
	textBody      core.Points = 16
	textSecondary core.Points = 14
	textCaption   core.Points = 12
)

// Line heights follow the Material 3 type scale for those sizes.
func lineHeight(size core.Points) core.Points {
	switch size {
	case textTitle:
		return 28
	case textBody:
		return 24
	case textSecondary:
		return 20
	default:
		return 16
	}
}

// text styles a run of text of size and weight.
func textStyle(size core.Points, weight core.FontWeight) core.Style {
	return core.Style{FontSize: size, LineHeight: lineHeight(size), FontWeight: weight}
}

// newTheme is the Material 3 theme of the app: its color roles are named
// colors of the palette, so classes such as "bg-surface-container-high" and
// "text-on-surface-variant" follow the light and dark appearance.
func newTheme() *core.Theme {
	t := core.DefaultTheme().Clone()
	for _, role := range schemeRoles {
		t.Light.SetColor(role.name, role.light)
		t.Dark.SetColor(role.name, role.dark)
	}
	for _, p := range []*core.Palette{&t.Light, &t.Dark} {
		role := func(name string) core.Color { c, _ := p.Color(name); return c }
		p.Background = role("surface")
		p.Surface = role("surface-container") // the native navigation bar's
		p.SurfaceHover = role("surface-container-low")
		p.SurfacePressed = role("surface-container")
		p.Border = role("outline-variant")
		p.Text = role("on-surface")
		p.TextMuted = role("on-surface-variant")
		p.Accent = role("primary")
		p.AccentText = role("on-secondary-container") // the navigation bar's selected label
		p.SetColor("accent-container", role("secondary-container"))
		p.Danger = role("error")
	}
	return t
}

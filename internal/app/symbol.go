package app

import (
	"fmt"
	"strings"
	"sync"

	"github.com/nsgo-dev/nsgo/core"
)

// symbol is one Material Symbols glyph, parsed on first use in each color.
type symbol struct {
	path string
	mu   sync.Mutex
	svgs map[core.Color]*core.SVG
}

// in returns the glyph filled with c.
func (s *symbol) in(c core.Color) *core.SVG {
	s.mu.Lock()
	defer s.mu.Unlock()
	if svg := s.svgs[c]; svg != nil {
		return svg
	}
	fill := fmt.Sprintf(`fill="#%06x" fill-opacity="%.3f"`, uint32(c)>>8, float64(uint32(c)&0xff)/255)
	svg, err := core.ParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 -960 960 960"><path ` + fill + ` d="` + s.path + `"/></svg>`))
	if err != nil {
		panic("symbol: " + err.Error())
	}
	if s.svgs == nil {
		s.svgs = make(map[core.Color]*core.SVG)
	}
	s.svgs[c] = svg
	return svg
}

// icon draws a symbol size points wide in the color class names, a "text-"
// role of the palette. The glyph carries its color itself: an icon that took
// its color from the text style around it came out black in parts of the
// app on Android, whatever the role or the appearance.
func icon(s *symbol, size core.Points, class string) core.Node {
	role, _ := strings.CutPrefix(class, "text-")
	return core.Dynamic(func(cx *core.Scope) core.Node {
		p := cx.Palette()
		c, ok := p.Color(role)
		switch {
		case role == "white":
			c = core.RGB(0xFFFFFF)
		case !ok:
			c = p.Text
		}
		return core.SVGView{SVG: s.in(c), Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), FlexShrink: 0}}
	})
}

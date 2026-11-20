package app

import (
	"sync"

	"github.com/nsgo-dev/nsgo/core"
)

// symbol is one Material Symbols glyph, parsed on first use.
type symbol struct {
	path string
	once sync.Once
	svg  *core.SVG
}

func (s *symbol) SVG() *core.SVG {
	s.once.Do(func() {
		var err error
		s.svg, err = core.ParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 -960 960 960"><path d="` + s.path + `"/></svg>`))
		if err != nil {
			panic("symbol: " + err.Error())
		}
	})
	return s.svg
}

// icon draws a symbol size points wide in the text color class names.
func icon(s *symbol, size core.Points, class string) core.Node {
	return core.Icon{SVG: s.SVG(), Class: class, Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), FlexShrink: 0}}
}

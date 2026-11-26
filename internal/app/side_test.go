package app

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A row wider than the window shows an arrow towards what is out of sight
// while the mouse is over it.
func TestSideScrollArrowsOnHover(t *testing.T) {
	chips := make([]core.Node, 20)
	for i := range chips {
		chips[i] = core.View{TestID: "chip", Style: core.Style{Width: core.Pt(100), Height: core.Pt(32)}}
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return column(fill, "", sideScroll{Children: []core.Node{row(core.Style{Gap: 8}, "", chips...)}})
	}, nsgotest.Idiom(core.IdiomDesktop))
	app.Settle()
	if shows(app, "role:button 向右滚动") {
		t.Fatal("the arrow shows before the mouse is over the row")
	}
	app.Hover("id:chip >> nth=0")
	app.Settle()
	if !shows(app, "role:button 向右滚动") || shows(app, "role:button 向左滚动") {
		t.Fatalf("hovered at its start, the row shows no arrow to the right alone:\n%s", app.Dump())
	}
}

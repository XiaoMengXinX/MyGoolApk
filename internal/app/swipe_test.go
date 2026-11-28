package app

import (
	"context"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestHeadedShortListCanCollapseHeader(t *testing.T) {
	const headHeight = 160
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		p := newPager(cx, func(context.Context, int) ([]string, bool, error) {
			return []string{"一条讨论"}, false, nil
		}, func(s string) string { return s })
		h := newHeaded(cx, core.NewSignal(cx, 0), 1, func(int) *core.Ref { return &p.list })
		spec := h.under(cx, 0, listSpec{}, func() { p.items.Get(); p.loaded.Get() })
		list := pagerList(cx, p, spec, func(_ *core.Scope, get func() string, _ core.ListRow) core.Node {
			return core.Text{ContentFunc: get}
		})
		return h.view(core.View{Style: core.Style{Height: core.Pt(headHeight)}},
			core.View{Style: core.Style{Height: core.Pt(48)}}, list)
	}, nsgotest.Size(360, 640))
	settle(app)
	app.Find("text:一条讨论")
	app.Scroll("id:list", headHeight)
	if y := app.Find("id:list").Offset.Y; y < headHeight-1 {
		t.Errorf("list stopped at %v before its 160pt header collapsed", y)
	}
}

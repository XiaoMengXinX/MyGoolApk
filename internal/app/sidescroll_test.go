package app

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"

	"app/internal/backend"
)

func TestTopicChipsScrollHorizontallyWithWheel(t *testing.T) {
	old := hoverable
	hoverable = true
	t.Cleanup(func() { hoverable = old })
	parentWheels := 0
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.SetTheme(newTheme())
		a := &App{}
		return core.View{TestID: "topics", OnWheel: func(core.WheelEvent) { parentWheels++ }, Children: []core.Node{a.chipsCard(&backend.Card{Items: []backend.CardItem{
			{Title: "iPhone 15"}, {Title: "壁纸"}, {Title: "KernelSU模块"}, {Title: "数码日常"}, {Title: "AI兴趣小组"},
		}})}}
	}, nsgotest.Size(240, 400))
	app.Wheel("role:button iPhone 15", core.Point{Y: -40}, core.Modifiers{})
	app.Settle()
	if got := app.Find("topics").Children[0].Offset.X; got != 40 {
		t.Errorf("horizontal topic offset = %g, want 40", got)
	}
	app.Resize(650, 400)
	app.Wheel("role:button iPhone 15", core.Point{Y: -40}, core.Modifiers{})
	if parentWheels != 1 {
		t.Errorf("parent received %d wheel events, want only the event when the row fits", parentWheels)
	}
}

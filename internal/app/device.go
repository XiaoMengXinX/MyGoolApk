package app

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// devicePage edits the identity requests carry: the model fields and the
// two-step confirmation of a new device ID.
func (a *App) devicePage(cx *core.Scope) base.NavPage {
	// info is the identity as of the last load: X-App-Token is made from the
	// clock, so reading it on every frame would have it change at the frame
	// rate.
	info := core.NewSignalFunc(cx, a.backend.Device(), nil)
	model, hardware := core.NewSignal(cx, info.GetUntracked().Model), core.NewSignal(cx, info.GetUntracked().HardwareID)
	sales, system := core.NewSignal(cx, info.GetUntracked().SalesModel), core.NewSignal(cx, info.GetUntracked().SystemVersion)
	confirm := core.NewSignal(cx, false)
	resetting := core.NewSignal(cx, false)
	reload := func() {
		d := a.backend.Device()
		info.Set(d)
		model.Set(d.Model)
		hardware.Set(d.HardwareID)
		sales.Set(d.SalesModel)
		system.Set(d.SystemVersion)
	}
	save := func() {
		if err := a.backend.SetDeviceProfile(model.GetUntracked(), hardware.GetUntracked(), sales.GetUntracked(), system.GetUntracked()); err != nil {
			a.notice("保存失败：" + err.Error())
			return
		}
		reload()
		a.notice("机型已保存")
	}
	regenerate := func() {
		if err := a.backend.RegenerateDeviceProfile(); err != nil {
			a.notice("保存失败：" + err.Error())
			return
		}
		reload()
		a.notice("已换一个机型，设备 ID 不变")
	}
	resetID := func() {
		if !confirm.GetUntracked() {
			confirm.Set(true)
			return
		}
		confirm.Set(false)
		resetting.Set(true)
		backgroundErr(a, a.backend.ResetDeviceID, func(err error) {
			resetting.Set(false)
			reload()
			if err != nil {
				a.notice("设备 ID 已更换，CDID 申请失败：" + err.Error())
			} else {
				a.notice("设备 ID 已更换")
			}
		})
	}
	value := func(name string, get func(backend.DeviceInfo) string) core.Node {
		return column(core.Style{Padding: core.Symmetric(12, 16), Gap: 2}, "",
			core.Text{Content: name, Class: "text-on-surface-variant", Style: textStyle(textSecondary, 400)},
			core.Text{ContentFunc: func() string { return get(info.Get()) }, Selectable: core.SelectableOn, Class: "text-on-surface", Style: textStyle(textBody, 400)})
	}
	field := func(name string, v *core.Signal[string]) core.Node {
		return column(core.Style{Gap: 4}, "",
			label(name, textCaption, 400, "text-on-surface-variant"),
			core.Input{Value: v.Get, OnChange: v.Set, Label: name, Disabled: resetting.Get, Class: "text-on-surface border-outline",
				Style: func() core.Style {
					s := textStyle(textBody, 400)
					s.BorderWidth, s.CornerRadius, s.Padding, s.Height = 1, 8, core.Symmetric(0, 16), core.Pt(48)
					return s
				}()})
	}
	wide := func(n core.Node) core.Node { return column(core.Style{}, "", n) }
	outlined := func(text string, fn func()) core.Node {
		return tap(text, fn, core.Style{Height: core.Pt(40), CornerRadius: 20, BorderWidth: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Overflow: core.Hidden}, "border-outline",
			label(text, textSecondary, 500, "text-primary"))
	}
	idKind := func() string {
		d := info.Get()
		switch {
		case d.HasCDID:
			return "设备 ID（CDID）"
		case a.backend.SessionUID() != "":
			return "设备 ID（随机 UUID，尚未换取 CDID）"
		}
		return "设备 ID（随机 UUID）"
	}
	_ = idKind
	return base.NavPage{Title: "设备信息", HeaderStyleFunc: a.headerStyle(cx), Class: "bg-background", Children: []core.Node{
		core.Scroll{Style: fill, Children: []core.Node{column(core.Style{Padding: core.Edges{Top: 8, Right: 16, Bottom: 24, Left: 16}, Gap: 8}, "",
			settingsLabel("设备标识"),
			listGroup(
				value("设备 ID", func(d backend.DeviceInfo) string { return d.ID }),
				value("X-App-Device", func(d backend.DeviceInfo) string { return d.AppDevice }),
				value("X-App-Token", func(d backend.DeviceInfo) string { return d.AppToken }),
			),
			settingsLabel("机型"),
			field("机型", model), field("硬件标识", hardware), field("销售型号", sales), field("系统版本", system),
			wide(filledButton("保存机型", func() bool { return !resetting.Get() }, save)),
			settingsLabel("重置"),
			outlined("重新生成机型", regenerate),
			core.Dynamic(func(cx *core.Scope) core.Node {
				text := "重新生成设备 ID"
				if confirm.Get() {
					text = "再点一次，确认换成新的设备"
				}
				return outlined(text, resetID)
			}),
			core.Text{Content: "登录状态下，新设备 ID 会向酷安设备服务申请 CDID；申请失败时设备保持随机 ID，之后的写操作可能被拒绝。",
				Class: "text-on-surface-variant", Style: func() core.Style {
					s := textStyle(textSecondary, 400)
					s.Padding = core.Symmetric(4, 8)
					return s
				}()},
		)}},
	}}
}

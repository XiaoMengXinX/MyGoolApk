package app

import (
	"os"
	"runtime/debug"
	"time"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

const routeCompose = "compose"
const routeDevice = "device"
const routeList = "list"

func (a *App) openDevice() { a.push(routeDevice, "") }

// historyPage lists the posts opened on this device.
func (a *App) historyPage(cx *core.Scope) core.Node {
	clear := core.Dynamic(func(cx *core.Scope) core.Node {
		if len(a.history.Get()) == 0 {
			return core.Group{}
		}
		return textButton("清空", func() {
			a.local.History = nil
			a.history.Set(nil)
			a.saveLocalState()
		})
	})
	body := core.Dynamic(func(cx *core.Scope) core.Node {
		if len(a.history.Get()) == 0 {
			return statusBlock("暂无浏览记录", "看过的帖子会保存在这台设备上。", "", nil)
		}
		return core.List{
			Style: fill, Count: func() int { return len(a.history.Get()) }, EstimatedSize: 72,
			Key: func(i int) any {
				if h := a.history.Get(); i < len(h) {
					return h[i].FeedID
				}
				return i
			},
			Footer: bottomSpace(24, true),
			Row: func(cx *core.Scope, r core.ListRow) core.Node {
				entry := func() historyEntry {
					if h := a.history.Get(); r.Index() < len(h) {
						return h[r.Index()]
					}
					return historyEntry{}
				}
				return tap("历史", func() { a.openFeed(backend.Feed{ID: entry().FeedID}) },
					core.Style{Direction: core.Row, Height: core.Pt(72), Padding: core.Symmetric(0, 16), Gap: 16, AlignItems: core.AlignCenter}, "bg-background",
					a.avatar(cx, func() backend.Author { e := entry(); return backend.Author{Name: e.Author, AvatarURL: e.AvatarURL} }, 40),
					column(fill.Merge(core.Style{Gap: 2}), "",
						core.Text{ContentFunc: func() string { return entry().Title }, MaxLines: 1, Class: "text-on-surface", Style: textStyle(textBody, 400)},
						core.Text{ContentFunc: func() string { e := entry(); return e.Author + "  " + formatTimeNow(e.ViewedAt) }, MaxLines: 1,
							Class: "text-on-surface-variant", Style: textStyle(textSecondary, 400)}))
			},
		}
	})
	return a.screen(titled("浏览历史"), clear, body)
}

func settingsLabel(text string) core.Node {
	return core.Text{Content: text, Class: "text-primary", Style: func() core.Style {
		s := textStyle(textSecondary, 500)
		s.Padding = core.Edges{Top: 16, Right: 16, Bottom: 4, Left: 16}
		return s
	}()}
}

func listGroup(items ...core.Node) core.Node {
	return column(core.Style{CornerRadius: 28, Overflow: core.Hidden}, "bg-surface-container-low", items...)
}

// settingsPage holds the appearance, the history and cache switches, and
// the way to the device identity.
func (a *App) settingsPage(cx *core.Scope) core.Node {
	mode := core.NewSignal(cx, a.local.ThemeMode)
	history := core.NewSignal(cx, a.local.HistoryEnabled)
	setMode := func(m int) {
		mode.Set(m)
		a.local.ThemeMode = m
		a.applyAppearance()
		a.saveLocalState()
	}
	modes := row(core.Style{Gap: 8, Padding: core.Symmetric(0, 16)}, "",
		filterChip("跟随系统", func() bool { return mode.Get() == 0 }, func() { setMode(0) }),
		filterChip("浅色", func() bool { return mode.Get() == 1 }, func() { setMode(1) }),
		filterChip("深色", func() bool { return mode.Get() == 2 }, func() { setMode(2) }))
	return a.screen(titled("设置"), nil,
		core.Scroll{Style: fill, Children: []core.Node{column(core.Style{Padding: core.Edges{Top: 8, Right: 16, Left: 16}, Gap: 8}, "",
			settingsLabel("外观"), modes,
			settingsLabel("隐私与存储"),
			listGroup(
				toggleRow("保存浏览历史", history.Get, func(on bool) {
					history.Set(on)
					a.local.HistoryEnabled = on
					if !on {
						a.local.History = nil
						a.history.Set(nil)
					}
					a.saveLocalState()
				}),
				listItem(&symbols.Delete, "清除图片缓存", nil, func() {
					_ = os.RemoveAll(a.images.dir)
					a.notice("图片缓存已清除")
				}),
			),
			settingsLabel("设备"),
			listGroup(listItem(&symbols.Settings, "设备信息", nil, a.openDevice)),
			settingsLabel("关于"),
			listGroup(column(core.Style{Padding: core.Symmetric(12, 16), Gap: 2}, "",
				label("酷安 MyGO", textBody, 400, "text-on-surface"),
				label(buildLabel(), textSecondary, 400, "text-on-surface-variant"))),
			bottomSpace(24, true),
		)}})
}

// applyAppearance follows the system's light or dark, or forces one.
func (a *App) applyAppearance() {
	switch a.local.ThemeMode {
	case 1:
		a.cx.SetAppearance(core.AppearanceLight)
	case 2:
		a.cx.SetAppearance(core.AppearanceDark)
	default:
		a.cx.SetAppearance(core.AppearanceSystem)
	}
}

// buildLabel tells the app's version and the commit and time it was built
// from, so that an installed build can be told from another.
func buildLabel() string {
	label := "版本 " + appVersion
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return label
	}
	var revision, built, dirty string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value[:min(len(s.Value), 7)]
		case "vcs.time":
			if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
				built = t.Local().Format("01-02 15:04")
			}
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "+"
			}
		}
	}
	if revision == "" {
		return label
	}
	return label + "（" + revision + dirty + "，" + built + "）"
}

// appVersion is the version of nsgo.yaml.
const appVersion = "1.0.0"

package app

import (
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
)

// minWindow is the least a desktop window goes to: a phone's width, and
// room for a list under the bars.
var minWindow = core.Size{Width: 360, Height: 560}

// placeWindow opens a desktop's main window where it was when the app last
// ran, and keeps where it goes. main.go starts the window hidden, so it shows
// once placed.
func (a *App) placeWindow(cx *core.Scope) {
	if !cx.Env().Desktop() {
		return
	}
	desktop.SetWindowConstraints(cx, desktop.WindowConstraints{MinWidth: minWindow.Width, MinHeight: minWindow.Height})
	if f := a.local.Window; f.Width >= minWindow.Width && f.Height >= minWindow.Height {
		f.Move = true
		desktop.SetWindowFrame(cx, f)
	}
	a.setMenu(cx)
	desktop.MainWindow(cx).Show()
	// A frame is kept once the window has rested a moment, not on each step
	// of a drag.
	var stop func() bool
	cx.Effect(func() {
		f := desktop.Frame(cx)
		if f.Width == 0 || desktop.FullScreen(cx) {
			return
		}
		if stop != nil {
			stop()
		}
		stop = cx.AfterFunc(500*time.Millisecond, func() {
			f.Move, f.Animated = false, false
			if f != a.local.Window {
				a.local.Window = f
				a.saveLocalState()
			}
		})
	})
}

// setMenu sets the application menu, macOS's menu bar, whose shortcuts are
// the keyboard's way around the app.
func (a *App) setMenu(cx *core.Scope) {
	// What the keyboard opens takes the place of the pages shown, as what is
	// pressed in the list does.
	open := func(fn func()) func() {
		return func() {
			a.nav.fromList = true
			fn()
		}
	}
	destination := func(i int, title, key string) desktop.MenuItem {
		return desktop.MenuItem{Label: title, Shortcut: key, When: core.MenuNotEditing, OnSelect: func() { a.selectDestination(i) }}
	}
	desktop.SetApplicationMenu(cx, []desktop.MenuItem{
		{Label: "MyGoolApk", Items: []desktop.MenuItem{
			{Role: desktop.MenuAbout},
			{Separator: true},
			{Label: "设置…", Shortcut: "mod+,", OnSelect: open(a.openSettings)},
			{Separator: true},
			{Role: desktop.MenuHide},
			{Role: desktop.MenuHideOthers},
			{Role: desktop.MenuShowAll},
			{Separator: true},
			{Role: desktop.MenuQuit},
		}},
		{Label: "编辑", Items: []desktop.MenuItem{
			{Role: desktop.MenuUndo},
			{Role: desktop.MenuRedo},
			{Separator: true},
			{Role: desktop.MenuCut},
			{Role: desktop.MenuCopy},
			{Role: desktop.MenuPaste},
			{Role: desktop.MenuSelectAll},
		}},
		{Label: "显示", Items: []desktop.MenuItem{
			destination(0, "首页", "mod+1"),
			destination(1, "数码", "mod+2"),
			destination(2, "发现", "mod+3"),
			destination(3, "我的", "mod+4"),
			{Separator: true},
			{Label: "搜索", Shortcut: "mod+f", OnSelect: open(a.openSearch)},
			{Label: "刷新", Shortcut: "mod+r", When: core.MenuNotEditing, OnSelect: a.refreshDestination},
			// Command-[ goes back, as in Safari and Finder.
			{Label: "返回", Shortcut: "mod+[", When: core.MenuNotEditing, OnSelect: func() { a.nav.Pop() }},
			{Separator: true},
			{Role: desktop.MenuToggleFullScreen},
		}},
		{Role: desktop.MenuWindowMenu, Label: "窗口", Items: []desktop.MenuItem{
			{Role: desktop.MenuMinimize},
			{Role: desktop.MenuZoom},
			{Separator: true},
			{Role: desktop.MenuBringAllToFront},
		}},
	})
}

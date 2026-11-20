package app

import (
	"context"
	"strings"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"

	"app/internal/backend"
)

func (a *App) openCompose() {
	if a.requireLogin() {
		a.push(routeCompose, "")
	}
}

// composePage writes a text post.
func (a *App) composePage(cx *core.Scope) base.NavPage {
	text := core.NewSignal(cx, "")
	private := core.NewSignal(cx, false)
	publishing := core.NewSignal(cx, false)
	errText := core.NewSignal(cx, "")
	publish := func() {
		publishing.Set(true)
		errText.Set("")
		draft := backend.FeedDraft{Message: strings.TrimSpace(text.GetUntracked()), Private: private.GetUntracked()}
		background(a, func(ctx context.Context) (string, error) { return a.backend.CreateFeed(ctx, draft) },
			func(id string, err error) {
				publishing.Set(false)
				if err != nil {
					errText.Set("发布失败：" + err.Error())
					return
				}
				a.nav.Pop(1)
				a.openFeed(backend.Feed{ID: id})
			})
	}
	canPublish := func() bool { return !publishing.Get() && strings.TrimSpace(text.Get()) != "" }
	return base.NavPage{
		Title: "发帖", HeaderStyleFunc: a.headerStyle(cx), Class: "bg-background",
		HeaderRight: filledButton("发布", canPublish, publish),
		Children: []core.Node{column(fill, "",
			core.Input{
				Multiline: true, AutoFocus: true, Placeholder: "分享新鲜事…", Label: "正文",
				Value: text.Get, OnChange: func(s string) { text.Set(s); errText.Set("") }, Disabled: publishing.Get,
				Class: "text-on-surface",
				Style: func() core.Style {
					s := textStyle(textBody, 400)
					s.FlexGrow, s.FlexShrink, s.FlexBasis, s.Padding = 1, 1, core.Pt(0), core.Symmetric(8, 16)
					return s
				}(),
			},
			core.Dynamic(func(cx *core.Scope) core.Node {
				if errText.Get() == "" {
					return core.Group{}
				}
				return errorBanner(errText.Get(), nil, func() { errText.Set("") })
			}),
			row(core.Style{Padding: core.Symmetric(8, 16), Gap: 8, AlignItems: core.AlignCenter, FlexShrink: 0,
				PaddingEnv: core.EnvEdges{Bottom: core.EnvSafeArea}}, "bg-surface-container",
				core.Dynamic(func(cx *core.Scope) core.Node {
					if publishing.Get() {
						return spinner()
					}
					return core.Group{}
				}),
				spacer,
				core.Text{Content: "仅自己可见", Class: "text-on-surface", Style: textStyle(textSecondary, 400)},
				ui.Switch{Checked: private.Get, OnChange: private.Set, Label: "仅自己可见", Disabled: publishing.Get},
			),
		)},
	}
}

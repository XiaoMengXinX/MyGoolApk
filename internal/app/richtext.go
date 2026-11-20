package app

import (
	"strings"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/core"

	"app/internal/backend"
)

// richOptions styles one rich paragraph.
type richOptions struct {
	size       core.Points
	class      string // text color
	maxLines   int
	selectable bool
	press      func() // runs on a press outside the links, which the text would otherwise take
}

// richText renders what get reads as one paragraph, falling back to the
// plain content when a feed carried no markup. Topics, mentions and links
// take the primary color and open in the app or the system browser; emoji
// are pictures from the app's resources, a little taller than the text.
func (a *App) richText(cx *core.Scope, get func() ([]backend.RichSpan, string), o richOptions) core.Node {
	style := textStyle(o.size, 400)
	style.LineHeight = lineHeight(o.size)
	// The text takes touches only within its own box: it does not clip its
	// content, and Android would hand it touches meant for what is before it.
	return core.View{Style: core.Style{Overflow: core.Hidden}, Children: []core.Node{a.richTextNode(cx, get, o, style)}}
}

func (a *App) richTextNode(cx *core.Scope, get func() ([]backend.RichSpan, string), o richOptions, style core.Style) core.Node {
	emoji := map[string][]*core.Image{}
	return core.Text{
		Class:      o.class,
		Style:      style,
		MaxLines:   o.maxLines,
		Selectable: selectable(o.selectable),
		OnPress: func(core.PressEvent) {
			if o.press != nil {
				o.press()
			}
		},
		SpansFunc: func() []core.Span {
			used := map[string]int{}
			spans, fallback := get()
			if len(spans) == 0 {
				return []core.Span{{Content: fallback}}
			}
			out := make([]core.Span, 0, len(spans))
			for _, sp := range spans {
				sp := sp
				switch sp.Kind {
				case backend.SpanEmoji:
					if sp.Name != "" {
						size := o.size * 1.25
						// The picture is the same view each time the spans are made
						// again, as it is the same pointer: new ones are laid out in
						// the corner of the text until its next layout.
						n := used[sp.Name]
						used[sp.Name]++
						if n >= len(emoji[sp.Name]) {
							emoji[sp.Name] = append(emoji[sp.Name], &core.Image{
								Source: core.ImageSource{URI: "asset:emoji/" + sp.Name + ".png"},
								Style:  core.Style{Width: core.Pt(size), Height: core.Pt(size)},
							})
						}
						out = append(out, core.Span{Content: sp.Text, Attachment: emoji[sp.Name][n],
							Style: core.Style{VerticalAlign: core.VerticalAlignMiddle}})
						continue
					}
					out = append(out, core.Span{Content: sp.Text})
				case backend.SpanTopic, backend.SpanUser, backend.SpanLink:
					if sp.Text == "" {
						continue
					}
					span := core.Span{Content: sp.Text, Class: "text-primary"}
					if sp.URL != "" {
						span.OnPress = func(core.PressEvent) { a.openRichSpan(cx, sp) }
					}
					out = append(out, span)
				default:
					if sp.Text != "" {
						out = append(out, core.Span{Content: sp.Text})
					}
				}
			}
			return out
		},
	}
}

// plainRich is richText for a post's own text.
func (a *App) feedRich(cx *core.Scope, get func() backend.Feed, o richOptions) core.Node {
	return a.richText(cx, func() ([]backend.RichSpan, string) { f := get(); return f.Rich, f.Content }, o)
}

// mentionSpan is the clickable name of an account at the start of a
// nested reply.
func mentionSpan(u backend.Author) backend.RichSpan {
	if u.ID == "" {
		return backend.RichSpan{Kind: backend.SpanText, Text: displayName(u.Name)}
	}
	return backend.RichSpan{Kind: backend.SpanUser, Text: displayName(u.Name), URL: "/u/" + u.ID}
}

// openLink opens a Coolapk link that came from outside the app: a feed, a
// user or a topic. Others, which have no page here, are ignored.
func (a *App) openLink(raw string) {
	a.openInApp(backend.ResolveLink(raw))
}

// openRichSpan follows a topic, mention or link: in the app where it has a
// page, else in the browser.
func (a *App) openRichSpan(cx *core.Scope, sp backend.RichSpan) {
	target := backend.ResolveLink(sp.URL)
	if !a.openInApp(target) && target.URL != "" {
		cx.OpenURL(target.URL, nil)
	}
}

// openInApp shows the page of a link that has one, and reports whether it did.
func (a *App) openInApp(target backend.LinkTarget) bool {
	switch target.Kind {
	case backend.LinkFeed:
		a.openFeed(backend.Feed{ID: target.Param})
	case backend.LinkUser:
		a.openUser(backend.Author{ID: target.Param})
	case backend.LinkTopic:
		a.openTopic(target.Param)
	default:
		return false
	}
	return true
}

// feedTextNeedsMore reports whether a card clamps a message that goes on,
// counting a generous line of the body text.
func feedTextNeedsMore(content string, lines int) bool {
	return lines > 0 && utf8.RuneCountInString(content) > lines*24
}

func joinNonEmpty(parts []string, sep string) string {
	kept := parts[:0:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

func selectable(on bool) core.Selectable {
	if on {
		return core.SelectableOn
	}
	return core.SelectableAuto
}

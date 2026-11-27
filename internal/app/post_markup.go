package app

import (
	"bytes"
	"html"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/base/textview/highlight"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"app/internal/backend"
)

var (
	// Asterisks are valid in URL paths. Goldmark's default Linkify pattern
	// stops before them, leaving the path to be parsed as emphasis.
	postLinkify = extension.NewLinkify(
		extension.WithLinkifyURLRegexp(regexp.MustCompile(`^(?:https?|ftp)://`+postWebAddress)),
		extension.WithLinkifyWWWRegexp(regexp.MustCompile(`^www\.`+postWebAddress)),
	)
	// Goldmark passes Coolapk HTML through to TextView's native parser.
	postMarkdown = goldmark.New(
		goldmark.WithExtensions(extension.Table, postStrikethrough{}, extension.TaskList, postLinkify, extension.Typographer),
		goldmark.WithRendererOptions(gmhtml.WithUnsafe(), gmhtml.WithHardWraps(), gmhtml.WithWriter(postMarkdownWriter{gmhtml.NewWriter()})),
	)
	postCodeLink   = regexp.MustCompile(`<a class="feed-link-(?:tag|uname)" href="[^"]*">([^<]*)</a>`)
	postTypography = strings.NewReplacer("(c)", "©", "(C)", "©", "(r)", "®", "(R)", "®", "(tm)", "™", "(TM)", "™", "+-", "±")
	escapedQuote   = regexp.MustCompile(`(?m)^ {0,3}(?:&gt;[ \t]*)+`)
	postEmoji      = textview.Extension{Name: "coolapk-emoji", Pattern: regexp.MustCompile(`\[[^\[\]\r\n]+\]|#\([^()\r\n]+\)`)}
)

const postWebAddress = `[-a-zA-Z0-9@:%._+~#=]+\.[a-zA-Z]+(?::[0-9]+)?(?:[/#?][-a-zA-Z0-9@:%_+.~#$!*?&/=();,'">\^{}\[\]` + "`" + `]*)?`

type postStrikethrough struct{}

func (postStrikethrough) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(
		util.Prioritized(postStrikeParser{extension.NewStrikethroughParser()}, 500),
	))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(extension.NewStrikethroughHTMLRenderer(), 500),
	))
}

type postStrikeParser struct{ parser.InlineParser }

func (p postStrikeParser) Parse(parent ast.Node, block text.Reader, context parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if !bytes.HasPrefix(line, []byte("~~")) {
		return nil
	}
	return p.InlineParser.Parse(parent, block, context)
}

type postMarkdownWriter struct{ gmhtml.Writer }

func (w postMarkdownWriter) Write(out util.BufWriter, source []byte) {
	w.Writer.Write(out, []byte(postTypography.Replace(string(source))))
}

// Coolapk HTML-escapes the message before sending it, including code that
// a Markdown renderer would otherwise treat as literal entity spellings.
func (w postMarkdownWriter) RawWrite(out util.BufWriter, source []byte) {
	code := postCodeLink.ReplaceAllString(string(source), "$1")
	w.Writer.RawWrite(out, []byte(html.UnescapeString(code)))
}

type postDocument struct {
	native, markdown textview.Document
}

type postDocuments struct {
	body        postDocument
	parts       map[int]postDocument
	hasMarkdown bool
}

func preparePostDocuments(f backend.Feed) postDocuments {
	var docs postDocuments
	read := func(raw string) postDocument {
		doc := parsePostDocument(raw)
		docs.hasMarkdown = docs.hasMarkdown || len(doc.markdown.Blocks) != 0
		return doc
	}
	switch {
	case f.Rating != nil:
		docs.parts = make(map[int]postDocument, len(f.Rating.Sections))
		for i, section := range f.Rating.Sections {
			docs.parts[i] = read(section.Markup)
		}
	case len(f.Blocks) > 0:
		docs.parts = make(map[int]postDocument, len(f.Blocks))
		for i, block := range f.Blocks {
			if block.Kind == backend.BlockText {
				docs.parts[i] = read(block.Markup)
			}
		}
	default:
		docs.body = read(f.Markup)
	}
	return docs
}

func parsePostDocument(raw string) postDocument {
	if raw == "" {
		return postDocument{}
	}
	native, markdown := backend.PostMarkupSources(raw)
	doc := postDocument{native: textview.ParseHTML(native, postEmoji)}

	source := []byte(escapedQuote.ReplaceAllStringFunc(markdown, func(prefix string) string {
		return strings.ReplaceAll(prefix, "&gt;", ">")
	}))
	root := postMarkdown.Parser().Parse(text.NewReader(source))
	if hasMarkdownSyntax(root, source) {
		var rendered bytes.Buffer
		if err := postMarkdown.Renderer().Render(&rendered, source, root); err == nil {
			doc.markdown = textview.ParseHTML(rendered.String(), postEmoji)
			keepPostImagesInFlow(&doc.markdown)
		}
	}
	return doc
}

// parseMarkdownDocument parses plain Markdown (such as Coolapk's AI assistant
// response) into a textview.Document for native Markdown rendering.
func parseMarkdownDocument(raw string) textview.Document {
	if strings.TrimSpace(raw) == "" {
		return textview.Document{}
	}
	var rendered bytes.Buffer
	if err := postMarkdown.Convert([]byte(raw), &rendered); err != nil {
		return textview.ParseHTML(html.EscapeString(raw))
	}
	return textview.ParseHTML(rendered.String())
}

// markdownView renders reactive Markdown text using the post document styles.
func (a *App) markdownView(cx *core.Scope, source func() string) core.Node {
	styles := core.NewMemoFunc(cx, func() textview.Styles { return postDocumentStyles(cx) }, nil)
	doc := core.NewMemoFunc(cx, func() textview.Document { return parseMarkdownDocument(source()) }, nil)
	return textview.TextView{
		DocumentFunc: doc.Get,
		Selectable:   true,
		Highlighter:  highlight.New(cx),
		Classes:      postDocumentClasses,
		StyleOf:      func(part textview.Part) core.Style { return styles.Get()[part] },
		OnLinkPress:  func(e textview.LinkPressEvent) { a.openDocumentLink(cx, e) },
	}
}

func (a *App) openDocumentLink(cx *core.Scope, e textview.LinkPressEvent) {
	u, err := url.Parse(e.URL)
	if err != nil {
		return
	}
	switch u.Scheme {
	case "mailto":
		cx.OpenURL(e.URL, nil)
	case "", "https", "http", "coolmarket", "coolapk", "searchtab":
		if u.Path != "" || u.Host != "" {
			a.openURL(e.URL)
		}
	}
}

// A paragraph made of images on consecutive lines is a block gallery.
// TextView's image-only layout expects just the images and their links.
func keepPostImagesInFlow(doc *textview.Document) {
	for i, block := range doc.Blocks {
		p, ok := block.(textview.Paragraph)
		if !ok || len(p.Content) < 2 {
			continue
		}
		if slices.ContainsFunc(p.Content, postOtherInline) {
			continue
		}
		p.Content = slices.DeleteFunc(p.Content, func(in textview.Inline) bool {
			_, isBreak := in.(textview.Break)
			return isBreak
		})
		doc.Blocks[i] = p
	}
}

func postOtherInline(in textview.Inline) bool {
	switch in := in.(type) {
	case textview.Break, textview.Image:
		return false
	case textview.Link:
		if len(in.Content) == 1 {
			_, isImage := in.Content[0].(textview.Image)
			return !isImage
		}
	}
	return true
}

func hasMarkdownSyntax(root ast.Node, source []byte) bool {
	found := false
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.(type) {
		case *ast.AutoLink:
			// Goldmark represents both URL recognition and explicit <URL>
			// syntax as AutoLink nodes. The explicit form starts at '<'.
			pos := n.Pos()
			if pos < 0 || pos >= len(source) || source[pos] != '<' {
				return ast.WalkContinue, nil
			}
		case *ast.Heading, *ast.Emphasis, *ast.CodeSpan, *ast.FencedCodeBlock, *ast.CodeBlock,
			*ast.List, *ast.Blockquote, *ast.ThematicBreak, *ast.Link, *ast.Image,
			*east.Table, *east.Strikethrough:
		default:
			return ast.WalkContinue, nil
		}
		found = true
		return ast.WalkStop, nil
	})
	return found
}

var postDocumentClasses = textview.Classes{
	textview.PartRoot:              "text-on-surface",
	textview.PartLink:              "text-primary",
	textview.PartBlockquote:        "text-on-surface-variant",
	textview.PartCodeBlock:         "bg-surface-container-low",
	textview.PartCode:              "bg-surface-container-high",
	textview.PartTableHead:         "bg-surface-container-high",
	textview.PartListMarker:        "text-on-surface-variant",
	textview.PartRule:              "bg-outline-variant",
	textview.PartTaskMarker:        "bg-surface-container-high",
	textview.PartTaskMarkerChecked: "bg-primary text-on-primary",
}

func postDocumentStyles(cx *core.Scope) textview.Styles {
	palette := cx.Palette()
	outline, _ := palette.Color("outline-variant")
	primary, _ := palette.Color("primary")
	body := textStyle(textBody, 400)
	body.LineHeight = feedLineHeight
	heading := textStyle(18, 600)
	heading.LineHeight = 28
	code := textStyle(textSecondary, 400)
	code.FontFamily, code.LineHeight = "monospace", 22
	strong := core.Style{FontWeight: 600}
	return textview.Styles{
		textview.PartRoot:            body.Merge(core.Style{Gap: 10, Overflow: core.Hidden}),
		textview.PartHeading:         heading,
		textview.PartHeading1:        {FontSize: 20, LineHeight: 30},
		textview.PartHeading4:        body.Merge(strong),
		textview.PartHeading5:        body.Merge(strong),
		textview.PartHeading6:        body.Merge(strong),
		textview.PartStrong:          strong,
		textview.PartEmphasis:        {FontStyle: core.FontStyleItalic},
		textview.PartUnderline:       {TextDecoration: core.TextDecorationUnderline},
		textview.PartStrikethrough:   {TextDecoration: core.TextDecorationLineThrough},
		textview.PartMark:            {Background: palette.Selection},
		textview.PartSmall:           {FontSize: textCaption},
		textview.PartSuperscript:     {FontSize: textCaption, VerticalAlign: core.VerticalAlignTextTop},
		textview.PartSubscript:       {FontSize: textCaption, VerticalAlign: core.VerticalAlignTextBottom},
		textview.PartRule:            {Height: core.Pt(1), Margin: core.Symmetric(4, 0)},
		textview.PartCode:            code,
		textview.PartCodeBlock:       {CornerRadius: 12, Overflow: core.Hidden},
		textview.PartCodeBlockCode:   code,
		textview.PartCodeBlockScroll: {Padding: core.All(12)},
		textview.PartBlockquote: {Gap: 6, Padding: core.Edges{Left: 12}, Borders: core.Edges{Left: 2},
			BorderColors: core.EdgeColors{Left: outline}},
		textview.PartList:            {Gap: 4},
		textview.PartListItem:        {Direction: core.Row, AlignItems: core.AlignStart, Gap: 8},
		textview.PartListItemContent: {Gap: 4, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)},
		textview.PartTaskMarker: {Width: core.Pt(16), Height: core.Pt(16), Margin: core.Edges{Top: 5},
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, BorderWidth: 1, BorderColor: outline,
			CornerRadius: 4, FontSize: textCaption, LineHeight: 14},
		textview.PartTaskMarkerChecked: {BorderColor: primary},
		textview.PartTable:             {CornerRadius: 12, Overflow: core.Hidden, BorderWidth: 1, BorderColor: outline},
		textview.PartTableRow:          {Borders: core.Edges{Top: 1}, BorderColors: core.EdgeColors{Top: outline}},
		textview.PartTableCell:         {Padding: core.Symmetric(6, 12)},
		textview.PartTableHeaderCell:   strong,
	}
}

func (a *App) postDocument(cx *core.Scope, doc postDocument, markdown func() bool, fallback func() ([]backend.RichSpan, string)) core.Node {
	if len(doc.native.Blocks) == 0 && len(doc.markdown.Blocks) == 0 {
		return a.richText(cx, fallback, richOptions{size: textBody, lineHeight: feedLineHeight, class: "text-on-surface", selectable: true})
	}
	styles := core.NewMemoFunc(cx, func() textview.Styles { return postDocumentStyles(cx) }, nil)
	return textview.TextView{
		DocumentFunc: func() textview.Document {
			if markdown() && len(doc.markdown.Blocks) > 0 {
				return doc.markdown
			}
			return doc.native
		},
		Selectable:  true,
		Highlighter: highlight.New(cx),
		Classes:     postDocumentClasses,
		StyleOf:     func(part textview.Part) core.Style { return styles.Get()[part] },
		OnLinkPress: func(e textview.LinkPressEvent) { a.openDocumentLink(cx, e) },
		CustomSpan:  postEmojiSpan,
		RenderImage: func(cx *core.Scope, im textview.Image) core.Node {
			if src := postImageURL(im.Src); src != "" {
				if im.Width.IsAuto() && im.Height.IsAuto() {
					return column(core.Style{Width: core.Pct(100), AspectRatio: 4.0 / 3, CornerRadius: 12, Overflow: core.Hidden}, "bg-surface-container-low",
						a.picOf(cx, src, core.Contain, "", fillAbs))
				}
				return a.picOf(cx, src, core.Contain, "", core.Style{Width: im.Width, Height: im.Height, AspectRatio: 1})
			}
			return label(im.Alt, textSecondary, 400, "text-on-surface-variant")
		},
		OnImagePress: func(im textview.Image) {
			if src := postImageURL(im.Src); src != "" {
				a.openImages([]backend.Image{{URL: src}}, 0)
			}
		},
	}
}

func postImageURL(source string) string {
	if strings.HasPrefix(source, "//") {
		source = "https:" + source
	}
	if u, err := url.Parse(source); err == nil && u.Host != "" && (u.Scheme == "https" || u.Scheme == "http") {
		return source
	}
	return ""
}

func postEmojiSpan(in textview.Custom) core.Span {
	_, spans := backend.RichTextFromAPI(in.Text)
	if len(spans) == 1 && spans[0].Kind == backend.SpanEmoji {
		size := core.Pt(textBody * 1.25)
		return core.Span{Content: in.Text, Style: core.Style{VerticalAlign: core.VerticalAlignMiddle},
			Attachment: core.Image{Source: core.Src("asset:emoji/" + spans[0].Name + ".png"), Style: core.Style{Width: size, Height: size}}}
	}
	return core.Span{Content: in.Text}
}

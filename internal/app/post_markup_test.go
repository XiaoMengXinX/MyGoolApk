package app

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/textview"

	"app/internal/backend"
)

func TestPostMarkdownDetection(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         bool
	}{
		{"plain", "普通文字、A | B、https://example.com 与 #话题#", false},
		{"native URL", `<a class="feed-link-url" href="https://example.com">查看链接</a>`, false},
		{"underscores in a bare URL", "https://example.com/__docs__", false},
		{"underscores in a native URL", `<a class="feed-link-url" href="https://example.com/__docs__">查看链接</a>`, false},
		{"asterisks in a bare URL", "www.example.com/**docs**", false},
		{"asterisks in a native URL", `<a class="feed-link-url" href="https://example.com/**docs**">查看链接</a>`, false},
		{"explicit autolink", "<https://example.com/__docs__>", true},
		{"emphasis next to a bare URL", "**文档** https://example.com/__docs__", true},
		{"single tilde", "H~2~O", false},
		{"double tilde", "~~删除线~~", true},
		{"escaped", `\*\*文字\*\* 和 &ast;斜体&ast;`, false},
		{"native markup", `<h2>标题</h2><p><b>加粗</b><code>**代码**</code><u>下划线</u></p>`, false},
		{"native tags and links", `<a class="feed-link-tag">#我**的话题#</a> <a href="/u/42">@x_y_z</a>`, false},
		{"heading", "## 标题", true},
		{"inline emphasis", "一个**粗体**，一个*斜体*。", true},
		{"thematic break", "---", true},
		{"table", "| A | B |\n| --- | --- |\n| 1 | 2 |", true},
		{"task list", "- [x] 任务", true},
		{"link", "[上一帖](/feed/42)", true},
		{"escaped quote", "&gt; 引用", true},
		{"mixed", `<p><u>原生格式</u>和 **Markdown** <a href="/u/42">@用户</a>[doge]</p>`, true},
		{"adjacent native block", "<h2>原生标题</h2>\n**Markdown正文**", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := parsePostDocument(tc.source)
			if got := len(doc.markdown.Blocks) != 0; got != tc.want {
				t.Errorf("detected Markdown = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPostMixedFormattingPreservesNativeStyles(t *testing.T) {
	doc := parsePostDocument(`<p><u>原生格式</u> **Markdown**</p>`)
	for _, rendered := range []textview.Document{doc.native, doc.markdown} {
		if len(rendered.Blocks) != 1 {
			t.Fatalf("got %d blocks, want one paragraph", len(rendered.Blocks))
		}
		p, ok := rendered.Blocks[0].(textview.Paragraph)
		if !ok || len(p.Content) == 0 {
			t.Fatalf("got %#v, want a paragraph with formatting", rendered.Blocks[0])
		}
		if u, ok := p.Content[0].(textview.Underline); !ok || textview.PlainText(u.Content) != "原生格式" {
			t.Errorf("got %#v, want underlined native text", p.Content[0])
		}
	}
	if got := doc.markdown.PlainText(); strings.Contains(got, "**") {
		t.Errorf("rendered Markdown contains delimiters: %q", got)
	}
	if got := doc.native.PlainText(); !strings.Contains(got, "**Markdown**") {
		t.Errorf("source view dropped Markdown delimiters: %q", got)
	}
}

func TestPostMarkdownPreservesCode(t *testing.T) {
	doc := parsePostDocument("```go\n    fmt.Println(&quot;x &lt; y&quot;)\n\n    // [doge]\n```")
	if got := len(doc.markdown.Blocks); got != 1 {
		t.Fatalf("got %d Markdown blocks, want 1 fenced code block", got)
	}
	code, ok := doc.markdown.Blocks[0].(textview.CodeBlock)
	want := "    fmt.Println(\"x < y\")\n\n    // [doge]"
	if !ok || code.Language != "go" || strings.TrimSuffix(code.Code, "\n") != want {
		t.Errorf("got %#v, want Go code %q", doc.markdown.Blocks[0], want)
	}
}

func TestPostMarkdownRestoresCoolapkLinksInCode(t *testing.T) {
	const code = `&lt;resources xmlns:android=&quot;<a class="feed-link-url" href="http://schemas.android.com/apk/res/android&quot;&gt;">查看链接</a>
  &lt;item&gt;<a class="feed-link-uname" href="/u/drawable">@drawable</a>/icon&lt;/item&gt;
  &lt;a href=&quot;/literal&quot;&gt;example&lt;/a&gt;
&lt;/resources&gt;`
	want := "<resources xmlns:android=\"http://schemas.android.com/apk/res/android\">\n  <item>@drawable/icon</item>\n  <a href=\"/literal\">example</a>\n</resources>"
	doc := parsePostDocument("```xml\n" + code + "\n```")
	if got := len(doc.markdown.Blocks); got != 1 {
		t.Fatalf("got %d blocks, want one fenced code block", got)
	}
	block, ok := doc.markdown.Blocks[0].(textview.CodeBlock)
	if !ok || block.Language != "xml" || block.Code != want {
		t.Errorf("got %#v, want XML code %q", doc.markdown.Blocks[0], want)
	}
	inline := parsePostDocument("`" + `<a class="feed-link-tag" href="/t/测试">#测试#</a> <a class="feed-link-url" href="https://example.com">查看链接</a>` + "`")
	if got := inline.markdown.PlainText(); got != "#测试# https://example.com" {
		t.Errorf("restored inline code = %q", got)
	}
}

func TestPostMarkdownRestoresCoolapkDestinations(t *testing.T) {
	source := `__[链接](<a class="feed-link-url" href="https://example.com/a)__">查看链接</a>

[带标题](<a class="feed-link-url" href="https://example.com/b">查看链接</a> &quot;标题&quot;)

![图片](<a class="feed-link-url" href="https://example.com/a.png)">查看链接</a>
![引用图][id]

[id]: <a class="feed-link-url" href="https://example.com/b.png">查看链接</a> &quot;图片标题&quot;

自动链接 <a class="feed-link-url" href="https://example.com/?a=1&amp;b=2">查看链接</a>`
	doc := parsePostDocument(source)
	if len(doc.markdown.Blocks) != 4 {
		t.Fatalf("unexpected Markdown blocks: %#v", doc.markdown.Blocks)
	}
	item := func(block, inline int) textview.Inline {
		return doc.markdown.Blocks[block].(textview.Paragraph).Content[inline]
	}
	strong, ok := item(0, 0).(textview.Strong)
	if !ok || len(strong.Content) != 1 {
		t.Fatalf("expected bold link, got %#v", item(0, 0))
	}
	if link, ok := strong.Content[0].(textview.Link); !ok || link.URL != "https://example.com/a" {
		t.Errorf("bold link = %#v", strong.Content[0])
	}
	if link, ok := item(1, 0).(textview.Link); !ok || link.URL != "https://example.com/b" || link.Title != "标题" {
		t.Errorf("titled link = %#v", item(1, 0))
	}
	if content := doc.markdown.Blocks[2].(textview.Paragraph).Content; len(content) != 2 {
		t.Fatalf("expected two block images, got %#v", content)
	}
	if image, ok := item(2, 0).(textview.Image); !ok || image.Src != "https://example.com/a.png" {
		t.Errorf("direct image = %#v", item(2, 0))
	}
	if image, ok := item(2, 1).(textview.Image); !ok || image.Src != "https://example.com/b.png" || image.Title != "图片标题" {
		t.Errorf("reference image = %#v", item(2, 1))
	}
	if link, ok := item(3, 1).(textview.Link); !ok || link.URL != "https://example.com/?a=1&b=2" {
		t.Errorf("autolink = %#v", item(3, 1))
	}
	if got := doc.native.PlainText(); !strings.Contains(got, "查看链接") {
		t.Errorf("native view lost its link labels: %q", got)
	}
}

func TestPostDetailMarkdownButton(t *testing.T) {
	for _, markdown := range []bool{false, true} {
		name := "native"
		if markdown {
			name = "mixed"
		}
		t.Run(name, func(t *testing.T) {
			b := &delayedDetailBackend{fakeBackend: newFakeBackend(), ready: make(chan struct{})}
			close(b.ready)
			b.feed.Kind = backend.PostArticle
			b.detail = b.feed
			markup := `<strong>原生格式</strong> <a class="feed-link-uname" href="/u/42">@原生用户</a>`
			if markdown {
				markup += " **Markdown正文**"
			}
			plain, rich := backend.RichTextFromAPI(markup)
			b.detail.Blocks = []backend.Block{
				{Kind: backend.BlockText, Text: plain, Rich: rich, Markup: markup},
				{Kind: backend.BlockImage, Image: backend.Image{Width: 4, Height: 3}},
			}
			app := start(t, b)
			openCard(app)
			body := app.WaitFor("text:原生格式", time.Second)
			const toggle = "role:button Markdown"
			if !markdown {
				if shows(app, toggle) {
					t.Fatal("native formatting showed a Markdown button")
				}
				return
			}
			button := app.Find(toggle)
			if !button.Props.Selected {
				t.Fatal("Markdown is off on first open")
			}
			bounds := button.Bounds()
			const buttonText = toggle + " >> text:\"Markdown\""
			textBounds := app.Find(buttonText).Bounds()
			if bounds.Origin.Y+bounds.Size.Height > detailList(t, body).Bounds().Origin.Y {
				t.Fatal("Markdown button is inside the scrolling body")
			}
			if shows(app, "text:**Markdown正文**") {
				t.Fatal("Markdown source visible while on")
			}
			const image = "role:button 查看图片"
			picture := app.FindMounted(image).ID
			app.Press(toggle)
			if app.Find(toggle).Props.Selected {
				t.Fatal("Markdown button stayed selected after turning Markdown off")
			}
			if off := app.Find(toggle).Bounds(); off != bounds {
				t.Errorf("Markdown button moved from %v to %v", bounds, off)
			}
			if off := app.Find(buttonText).Bounds(); off != textBounds {
				t.Errorf("Markdown label moved from %v to %v", textBounds, off)
			}
			app.Find("text:**Markdown正文**")
			app.Find("text:@原生用户")
			if after := app.FindMounted(image).ID; after != picture {
				t.Errorf("toggling Markdown remounted article picture %d as %d", picture, after)
			}
			app.Press(toggle)
			if shows(app, "text:**Markdown正文**") {
				t.Fatal("Markdown source visible after turning Markdown back on")
			}
		})
	}
}

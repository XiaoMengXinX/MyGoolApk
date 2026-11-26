package backend

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

var (
	postIndent           = regexp.MustCompile(`(?m)^[ \t]+`)
	postMarkdownEntities = strings.NewReplacer(
		"&quot;", `"`, "&#34;", `"`, "&#039;", "'", "&#39;", "'",
		`\&lt;`, `\<`, `\&gt;`, `\>`,
	)
	postMarkdownURL = strings.NewReplacer("<", "&lt;", ">", "&gt;", `"`, "&quot;")
)

// PostMarkupSources adapts Coolapk's message fragment to the two readers.
// The Markdown reader gets line boundaries for the HTML paragraph wrappers
// and breaks, with native links and code kept opaque to Markdown syntax.
func PostMarkupSources(raw string) (native, markdown string) {
	var h, m strings.Builder
	lexer := html.NewTokenizer(strings.NewReader(raw))
	var anchor *richAnchor
	pre, code, link := false, false, false
	blockDepth := 0
	for {
		kind := lexer.Next()
		token := string(lexer.Raw())
		if kind == html.ErrorToken {
			if anchor != nil {
				writePostAnchor(&h, &m, *anchor)
			}
			h.WriteString(token)
			m.WriteString(token)
			return h.String(), m.String()
		}
		if anchor != nil {
			switch {
			case kind == html.TextToken:
				anchor.text.Write(lexer.Text())
			case kind == html.EndTagToken && lexer.Token().Data == "a":
				writePostAnchor(&h, &m, *anchor)
				anchor = nil
			}
			continue
		}
		switch kind {
		case html.TextToken:
			if pre {
				h.WriteString(token)
			} else {
				token = strings.ReplaceAll(token, "\r\n", "\n")
				visible := postIndent.ReplaceAllStringFunc(token, func(indent string) string {
					indent = strings.ReplaceAll(indent, "\t", "    ")
					return strings.Repeat("&nbsp;", len(indent))
				})
				h.WriteString(strings.ReplaceAll(visible, "\n", "<br>"))
			}
			if code || link {
				m.WriteString(markdownLiteral(string(lexer.Text())))
			} else {
				m.WriteString(postMarkdownEntities.Replace(token))
			}
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			tag := lexer.Token()
			if strings.ContainsAny(tag.Data, ":@") {
				h.WriteString(html.EscapeString(token))
				m.WriteString(token)
				continue
			}
			if tag.Data == "a" && kind == html.StartTagToken && !pre && !code {
				var a richAnchor
				for _, attr := range tag.Attr {
					switch attr.Key {
					case "class":
						a.class = attr.Val
					case "href":
						a.href = attr.Val
					case "title":
						a.title = attr.Val
					}
				}
				for class := range strings.FieldsSeq(a.class) {
					if class == "feed-link-tag" || class == "feed-link-uname" || class == "feed-link-url" {
						a.class = class
						anchor = &a
						break
					}
				}
				if anchor != nil {
					continue
				}
			}
			h.WriteString(token)
			switch {
			case tag.Data == "pre":
				pre = kind == html.StartTagToken
			case !pre && tag.Data == "code":
				code = kind == html.StartTagToken
			case !pre && !code && tag.Data == "a":
				link = kind == html.StartTagToken
			case blockDepth == 0 && !pre && !code && !link && (tag.Data == "br" || tag.Data == "p" || tag.Data == "div"):
				if tag.Data == "br" {
					m.WriteByte('\n')
				} else {
					m.WriteString("\n\n")
				}
				continue
			}
			switch tag.Data {
			case "pre", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "ul", "ol", "table", "details":
				if kind == html.StartTagToken {
					if blockDepth == 0 {
						m.WriteString("\n\n")
					}
					blockDepth++
				}
				m.WriteString(token)
				if kind == html.EndTagToken && blockDepth > 0 {
					blockDepth--
					if blockDepth == 0 {
						m.WriteString("\n\n")
					}
				}
				continue
			}
			m.WriteString(token)
		case html.CommentToken:
			// Article excerpts may contain a <!--break--> marker.
		default:
			h.WriteString(token)
			m.WriteString(token)
		}
	}
}

func writePostAnchor(native, markdown *strings.Builder, anchor richAnchor) {
	span := anchorSpan(anchor)
	if label := strings.TrimSpace(anchor.text.String()); label != "" {
		span.Text = label
	}
	open, close := "", ""
	if span.URL != "" {
		open, close = `<a href="`+html.EscapeString(span.URL)+`">`, "</a>"
	}
	native.WriteString(open + html.EscapeString(span.Text) + close)
	// Coolapk's URL wrapper can absorb trailing Markdown or code syntax.
	// Restore the original text so the Markdown parser sees those characters.
	if anchor.class == "feed-link-url" && anchor.href != "" {
		markdown.WriteString(postMarkdownURL.Replace(anchor.href))
		return
	}
	if span.URL != "" {
		// The renderer uses this class to unwrap generated mentions and
		// topics inside code while keeping literal HTML examples intact.
		open = `<a class="` + html.EscapeString(anchor.class) + `" href="` + html.EscapeString(span.URL) + `">`
	}
	markdown.WriteString(open + markdownLiteral(span.Text) + close)
}

// Character references keep punctuation in a native link label or code span
// from becoming Markdown delimiters. The native reader decodes them again.
func markdownLiteral(text string) string {
	var out strings.Builder
	for _, r := range text {
		if r <= unicode.MaxASCII && (unicode.IsPunct(r) || unicode.IsSymbol(r)) {
			fmt.Fprintf(&out, "&#%d;", r)
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

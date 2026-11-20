package backend

import (
	_ "embed"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

// emojiCatalogText maps every bracket token of the official app to the
// picture it shows, one "token<TAB>name" per line. It is generated from
// Payload/CoolMarket.app/CoolApkEmotion.json of the official IPA, with the
// pictures themselves in the app's resources/emoji.
//
//go:embed emoji.txt
var emojiCatalogText string

// emojiNames returns the token to picture map, decoded once.
var emojiNames = sync.OnceValue(func() map[string]string {
	names := make(map[string]string, 512)
	for _, line := range strings.Split(emojiCatalogText, "\n") {
		token, name, ok := strings.Cut(line, "\t")
		if ok && token != "" && name != "" {
			names[token] = name
		}
	}
	return names
})

// LinkKind is where a link opens.
type LinkKind int

const (
	LinkExternal LinkKind = iota
	LinkFeed
	LinkUser
	LinkTopic
)

// LinkTarget is the destination of a topic, mention or link span.
type LinkTarget struct {
	Kind  LinkKind
	Param string
	URL   string
}

// ResolveLink normalizes a Coolapk href, as "/feed/1",
// "coolmarket://feed/1", a coolapk.com URL or a "#/feed/1" route, into an
// in-app page. Everything else opens externally: paths of pages the
// native client does not have, such as a product, apk, game, event or
// custom page, go to the website.
func ResolveLink(raw string) LinkTarget {
	s := strings.TrimSpace(raw)
	if s == "" {
		return LinkTarget{}
	}
	if strings.HasPrefix(s, "#/") {
		s = s[1:]
	}
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "coolmarket://"):
		s = "/" + strings.TrimPrefix(s[len("coolmarket://"):], "/")
	case strings.HasPrefix(lower, "coolapk://"):
		s = "/" + strings.TrimPrefix(s[len("coolapk://"):], "/")
	case strings.HasPrefix(s, "//"):
		s = "https:" + s
	}
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		if !isCoolapkHost(u.Hostname()) {
			return LinkTarget{URL: s}
		}
		s = u.RequestURI()
	} else if !strings.HasPrefix(s, "/") {
		if strings.Contains(s, "://") {
			return LinkTarget{URL: s}
		}
		if !strings.Contains(s, "/") {
			return LinkTarget{URL: "https://" + s}
		}
		s = "/" + s
	}
	path := s
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	switch {
	case strings.HasPrefix(path, "/feed/"):
		if id := trimPathID(path, "/feed/"); id != "" {
			return LinkTarget{Kind: LinkFeed, Param: id}
		}
	case strings.HasPrefix(path, "/u/"):
		if uid := trimPathID(path, "/u/"); uid != "" {
			return LinkTarget{Kind: LinkUser, Param: uid}
		}
	case strings.HasPrefix(path, "/t/"):
		if name := strings.Trim(strings.TrimPrefix(path, "/t/"), "/"); name != "" {
			if decoded, err := url.PathUnescape(name); err == nil {
				name = decoded
			}
			if name != "" {
				return LinkTarget{Kind: LinkTopic, Param: name}
			}
		}
	}
	return LinkTarget{URL: coolapkWebURL(s)}
}

// isCoolapkHost reports whether host belongs to coolapk.com.
func isCoolapkHost(host string) bool {
	host = strings.ToLower(host)
	return host == "coolapk.com" || strings.HasSuffix(host, ".coolapk.com")
}

// trimPathID returns the positive numeric id after prefix.
func trimPathID(path, prefix string) string {
	id := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if n, err := strconv.Atoi(id); err == nil && n > 0 {
		return id
	}
	return ""
}

// coolapkWebURL makes a Coolapk path openable by the system browser.
func coolapkWebURL(s string) string {
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	if strings.HasPrefix(s, "//") {
		return "https:" + s
	}
	if strings.HasPrefix(s, "/") {
		return "https://www.coolapk.com" + s
	}
	return s
}

// parseRichText turns the message HTML of Coolapk into spans. Every
// visible character is kept: text, topic tags, mentions, links, line
// breaks and emoji tokens; other tags only contribute their text.
func parseRichText(raw string) []RichSpan {
	if raw == "" {
		return nil
	}
	var spans []RichSpan
	var text strings.Builder
	flush := func() {
		if text.Len() == 0 {
			return
		}
		spans = append(spans, splitEmojiSpans(text.String())...)
		text.Reset()
	}
	var anchor *richAnchor
	z := html.NewTokenizer(strings.NewReader(raw))
	for {
		switch z.Next() {
		case html.ErrorToken:
			// Malformed markup: the open anchor becomes plain text.
			if anchor != nil {
				text.WriteString(anchor.text.String())
				anchor = nil
			}
			flush()
			return spans
		case html.TextToken:
			if anchor != nil {
				anchor.text.WriteString(string(z.Text()))
			} else {
				text.WriteString(string(z.Text()))
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) {
			case "a":
				attrs := tagAttrs(z, hasAttr)
				if anchor == nil {
					flush()
					anchor = &richAnchor{class: attrs["class"], href: attrs["href"], title: attrs["title"]}
				}
			case "br", "p", "div", "li", "section", "blockquote":
				if anchor != nil {
					anchor.text.WriteString("\n")
				} else {
					text.WriteString("\n")
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "a":
				if anchor != nil {
					spans = append(spans, anchorSpan(*anchor))
					anchor = nil
				}
			case "br", "p", "div", "li", "section", "blockquote":
				if anchor != nil {
					anchor.text.WriteString("\n")
				} else {
					text.WriteString("\n")
				}
			}
		}
	}
}

// richAnchor is the <a> being read.
type richAnchor struct {
	class string
	href  string
	title string
	text  strings.Builder
}

// tagAttrs reads a start tag's attributes.
func tagAttrs(z *html.Tokenizer, hasAttr bool) map[string]string {
	attrs := make(map[string]string, 3)
	if !hasAttr {
		return attrs
	}
	for {
		key, value, more := z.TagAttr()
		if len(key) > 0 {
			attrs[string(key)] = string(value)
		}
		if !more {
			return attrs
		}
	}
}

// anchorSpan classifies one completed <a>.
func anchorSpan(a richAnchor) RichSpan {
	label := strings.TrimSpace(a.text.String())
	classes := " " + a.class + " "
	switch {
	case strings.Contains(classes, " feed-link-tag "):
		return topicSpan(label, a.href)
	case strings.Contains(classes, " feed-link-uname "):
		return userSpan(label, a.href)
	case strings.Contains(classes, " feed-link-url "):
		text := strings.TrimSpace(a.title)
		if text == "" {
			text = label
		}
		if text == "" {
			text = a.href
		}
		if a.href == "" {
			return RichSpan{Kind: SpanText, Text: text}
		}
		return RichSpan{Kind: SpanLink, Text: text, URL: a.href}
	}
	if a.href == "" {
		return RichSpan{Kind: SpanText, Text: label}
	}
	text := label
	if text == "" {
		text = a.href
	}
	switch ResolveLink(a.href).Kind {
	case LinkUser:
		return RichSpan{Kind: SpanUser, Text: text, URL: a.href}
	case LinkTopic:
		return RichSpan{Kind: SpanTopic, Text: text, URL: a.href}
	}
	return RichSpan{Kind: SpanLink, Text: text, URL: a.href}
}

// topicSpan is a "#topic#" tag, with a target even without an href.
func topicSpan(label, href string) RichSpan {
	label = strings.TrimSpace(label)
	if label == "" {
		if target := ResolveLink(href); target.Kind == LinkTopic {
			label = "#" + target.Param + "#"
		}
	}
	if href == "" {
		if name := strings.Trim(label, "#"); name != "" {
			href = "/t/" + url.PathEscape(name)
		}
	}
	if label == "" {
		return RichSpan{Kind: SpanText, Text: href}
	}
	return RichSpan{Kind: SpanTopic, Text: label, URL: href}
}

// userSpan is a mention of an account.
func userSpan(label, href string) RichSpan {
	label = strings.TrimSpace(label)
	if label == "" {
		label = href
	}
	if href == "" {
		return RichSpan{Kind: SpanText, Text: label}
	}
	return RichSpan{Kind: SpanUser, Text: label, URL: href}
}

// splitEmojiSpans turns the emoji tokens of plain text into emoji spans.
// Unmatched brackets stay text, so nothing is lost.
func splitEmojiSpans(text string) []RichSpan {
	if text == "" {
		return nil
	}
	var spans []RichSpan
	var plain strings.Builder
	flush := func() {
		if plain.Len() > 0 {
			spans = append(spans, RichSpan{Kind: SpanText, Text: plain.String()})
			plain.Reset()
		}
	}
	runes := []rune(text)
	for i := 0; i < len(runes); {
		if name, next, ok := emojiTokenAt(runes, i); ok {
			flush()
			spans = append(spans, RichSpan{Kind: SpanEmoji, Text: string(runes[i:next]), Name: name})
			i = next
			continue
		}
		plain.WriteRune(runes[i])
		i++
	}
	flush()
	return spans
}

// emojiTokenAt matches "[name]" or "#(name)" at runes[i], returning the
// picture name and the rune after the token.
func emojiTokenAt(runes []rune, i int) (string, int, bool) {
	var close rune
	start := 0
	switch {
	case runes[i] == '[':
		close, start = ']', i+1
	case runes[i] == '#' && i+1 < len(runes) && runes[i+1] == '(':
		close, start = ')', i+2
	default:
		return "", 0, false
	}
	limit := min(i+16, len(runes))
	for j := start; j < limit; j++ {
		if runes[j] != close {
			continue
		}
		if name, ok := emojiNames()[string(runes[i:j+1])]; ok {
			return name, j + 1, true
		}
		return "", 0, false
	}
	return "", 0, false
}

// RichTextFromAPI parses one message into its plain text, which keeps the
// native client's line handling, and its spans.
func RichTextFromAPI(raw string) (string, []RichSpan) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	if !strings.Contains(raw, "<") {
		plain := strings.TrimSpace(raw)
		return plain, splitEmojiSpans(plain)
	}
	spans := normalizeRichSpans(parseRichText(raw))
	return normalizeAPILines(richTextPlain(spans)), spans
}

// richTextPlain joins the visible text of spans.
func richTextPlain(spans []RichSpan) string {
	var text strings.Builder
	for _, sp := range spans {
		text.WriteString(sp.Text)
	}
	return text.String()
}

// normalizeRichSpans trims the edges of a message and keeps at most one
// line break between two lines of text, as normalizeAPILines does.
func normalizeRichSpans(spans []RichSpan) []RichSpan {
	out := make([]RichSpan, 0, len(spans))
	var text []rune
	lastBreak := true // trims the start of the message
	for _, sp := range spans {
		if sp.Kind != SpanText {
			if len(text) > 0 {
				out = append(out, RichSpan{Kind: SpanText, Text: string(text)})
				text = text[:0]
			}
			out = append(out, sp)
			lastBreak = false
			continue
		}
		for _, r := range sp.Text {
			switch r {
			case '\r':
				continue
			case '\n':
				text = trimRightRich(text, true)
				if len(text) == 0 && len(out) == 0 {
					continue
				}
				if lastBreak {
					continue
				}
				text = append(text, '\n')
				lastBreak = true
			case ' ', '\t':
				if len(text) == 0 && len(out) == 0 {
					continue
				}
				text = append(text, ' ')
				lastBreak = false
			default:
				text = append(text, r)
				lastBreak = false
			}
		}
	}
	text = trimRightRich(text, true)
	if len(text) > 0 {
		out = append(out, RichSpan{Kind: SpanText, Text: string(text)})
	}
	return out
}

// trimRightRich drops trailing blanks, and the line breaks before them
// when breaks is true.
func trimRightRich(text []rune, breaks bool) []rune {
	for len(text) > 0 {
		switch text[len(text)-1] {
		case ' ', '\t':
			text = text[:len(text)-1]
		case '\n':
			if !breaks {
				return text
			}
			text = text[:len(text)-1]
		default:
			return text
		}
	}
	return text
}

func normalizeAPILines(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n")
	var text strings.Builder
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			blank = text.Len() > 0
			continue
		}
		if blank {
			text.WriteByte('\n')
			blank = false
		}
		text.WriteString(line)
		text.WriteByte('\n')
	}
	return strings.TrimSpace(text.String())
}

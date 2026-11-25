package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Coolapk's message markup becomes spans whose text still reads the same.
func TestRichTextParsesMarkup(t *testing.T) {
	raw := `前 <a class="feed-link-tag" href="/t/mygo">#MyGo#</a> 由 ` +
		`<a class="feed-link-uname" href="/u/9">作者</a> 发布<br>` +
		`<a class="feed-link-url" href="https://example.com/a" title="示例站点">example.com</a> 结束`
	const want = "前 #MyGo# 由 作者 发布\n示例站点 结束"
	plain, spans := RichTextFromAPI(raw)
	if plain != want {
		t.Fatalf("plain = %q, want %q", plain, want)
	}
	if joined := richTextPlain(spans); joined != want {
		t.Fatalf("spans read %q, want %q", joined, want)
	}
	wants := []struct {
		kind RichSpanKind
		text string
		url  string
	}{
		{SpanTopic, "#MyGo#", "/t/mygo"},
		{SpanUser, "作者", "/u/9"},
		{SpanLink, "示例站点", "https://example.com/a"},
	}
	next := 0
	for _, sp := range spans {
		if next < len(wants) && sp.Kind == wants[next].kind {
			if sp.Text != wants[next].text || sp.URL != wants[next].url {
				t.Fatalf("%v span = %+v, want %+v", sp.Kind, sp, wants[next])
			}
			next++
		}
	}
	if next != len(wants) {
		t.Fatalf("found %d of %d wanted spans in %+v", next, len(wants), spans)
	}
}

// A classless anchor to a user or topic is still a mention or tag.
func TestRichTextAnchorWithoutClass(t *testing.T) {
	_, spans := RichTextFromAPI(`<a href="/u/7">七号</a> 和 <a href="/product/x">产品</a>`)
	if spans[0].Kind != SpanUser || spans[0].URL != "/u/7" {
		t.Fatalf("user anchor = %+v", spans[0])
	}
	var link *RichSpan
	for i := range spans {
		if spans[i].Kind == SpanLink {
			link = &spans[i]
		}
	}
	if link == nil || link.Text != "产品" || link.URL != "/product/x" {
		t.Fatalf("product anchor = %+v", spans)
	}
}

// Emoji tokens become pictures, and anything else stays text.
func TestEmojiTokenParsing(t *testing.T) {
	const text = "你好[哈哈哈]世界[不存在]结束#(滑稽)"
	spans := splitEmojiSpans(text)
	if got := richTextPlain(spans); got != text {
		t.Fatalf("spans read %q, want %q", got, text)
	}
	var emoji []RichSpan
	for _, sp := range spans {
		if sp.Kind == SpanEmoji {
			emoji = append(emoji, sp)
		}
	}
	if len(emoji) != 2 {
		t.Fatalf("emoji spans = %+v", emoji)
	}
	if emoji[0].Text != "[哈哈哈]" || emoji[0].Name != "coolapk_emotion_1_hahaha" {
		t.Errorf("first emoji = %+v", emoji[0])
	}
	if emoji[1].Text != "#(滑稽)" || emoji[1].Name != "coolapk_emotion_62_huaji" {
		t.Errorf("second emoji = %+v", emoji[1])
	}
	if !strings.Contains(richTextPlain(spans), "[不存在]") {
		t.Error("unknown bracket text was dropped")
	}
	if got := richTextPlain(splitEmojiSpans("[未闭合")); got != "[未闭合" {
		t.Fatalf("unmatched bracket read %q", got)
	}
}

// Messages without markup keep their plain text and still expose spans.
func TestRichTextPlainFallback(t *testing.T) {
	plain, spans := RichTextFromAPI("纯文本")
	if plain != "纯文本" || len(spans) != 1 || spans[0].Kind != SpanText || spans[0].Text != "纯文本" {
		t.Fatalf("plain message = %q %+v", plain, spans)
	}
	if plain, spans := RichTextFromAPI(""); plain != "" || spans != nil {
		t.Fatalf("empty message = %q %+v", plain, spans)
	}
}

// Links route inside the app only where the native client has a page.
func TestResolveRichLink(t *testing.T) {
	cases := []struct {
		in    string
		kind  LinkKind
		param string
		url   string
	}{
		{"/feed/123", LinkFeed, "123", ""},
		{"coolmarket://feed/123?rid=5", LinkFeed, "123", ""},
		{"https://www.coolapk.com/feed/123", LinkFeed, "123", ""},
		{"http://m.coolapk.com/u/9", LinkUser, "9", ""},
		{"#/feed/42", LinkFeed, "42", ""},
		{"//www.coolapk.com/feed/5", LinkFeed, "5", ""},
		{"/u/9", LinkUser, "9", ""},
		{"/t/mygo", LinkTopic, "mygo", ""},
		{"coolmarket://t/%E8%AF%9D%E9%A2%98", LinkTopic, "话题", ""},
		{"/product/123", LinkProduct, "123", ""},
		{"/page?url=V9_HOME_TAB_RANKING", LinkPage, "/page?url=V9_HOME_TAB_RANKING", ""},
		{"#/feed/statList?statType=day", LinkPage, "#/feed/statList?statType=day", ""},
		{"/product/categoryList?id=1000", LinkPage, "/page?url=%2Fproduct%2FcategoryDetailList%3Ftype%3Dcategory%26id%3D1000%26showMode%3D0", ""},
		{"searchTab://all?keyword=酷安", LinkSearch, "酷安", ""},
		{"/apk/com.example", LinkExternal, "", "https://www.coolapk.com/apk/com.example"},
		{"/event/9", LinkExternal, "", "https://www.coolapk.com/event/9"},
		{"https://m.coolapk.com/mp/productSelector/configSearch?a=1", LinkExternal, "", "https://m.coolapk.com/mp/productSelector/configSearch?a=1"},
		{"https://example.com/a?b=1", LinkExternal, "", "https://example.com/a?b=1"},
		{"/feed/abc", LinkExternal, "", "https://www.coolapk.com/feed/abc"},
		{"", LinkExternal, "", ""},
	}
	for _, c := range cases {
		got := ResolveLink(c.in)
		if got.Kind != c.kind || got.Param != c.param || got.URL != c.url {
			t.Errorf("ResolveLink(%q) = %+v, want kind %v param %q url %q", c.in, got, c.kind, c.param, c.url)
		}
	}
}

// Every emoji the catalog names ships with the app.
func TestEmojiAssetsExist(t *testing.T) {
	dir := filepath.Join("..", "..", "resources")
	names := emojiNames()
	if len(names) < 155 {
		t.Fatalf("emoji catalog has %d tokens", len(names))
	}
	checked := make(map[string]bool, len(names))
	for token, name := range names {
		if checked[name] {
			continue
		}
		checked[name] = true
		if _, err := os.Stat(filepath.Join(dir, "emoji", name+".png")); err != nil {
			t.Errorf("emoji %s (%s): %v", token, name, err)
		}
	}
}

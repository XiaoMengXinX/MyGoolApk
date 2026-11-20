package live

import (
	"encoding/json"
	"testing"
	"time"

	"app/internal/backend"
)

func TestMixedCardsAndFeedFields(t *testing.T) {
	data := json.RawMessage(`[
		{"entityType":"card","entities":[{"entityType":"feed","id":"41","feedType":"feedArticle","message_title":"Nested article","pic":"http://image.coolapk.com/feed/a@1200x600.jpg"}]},
		{"entityType":"feed","id":42,"username":"Coolapk user","userAvatar":"http://avatar.coolapk.com/a.jpg","message":"Hello <a href=\"/t/mygo\">#MyGo#</a><br>native UI","dateline":1700000000,"likenum":"12","commentnum":7,"picArr":["http://image.coolapk.com/feed/b@800x800.jpg"]}
	]`)
	objects, err := decodeAPIObjects(data)
	if err != nil {
		t.Fatal(err)
	}
	feeds := feedsFromObjects(objects)
	if len(feeds) != 2 {
		t.Fatalf("got %d feeds, want 2", len(feeds))
	}
	if feeds[0].ID != "41" || feeds[0].Title != "Nested article" {
		t.Fatalf("nested feed was not preserved: %+v", feeds[0])
	}
	if got := feeds[1].Content; got != "Hello #MyGo#\nnative UI" {
		t.Errorf("content = %q", got)
	}
	if got := feeds[1].Images[0]; got.URL != "https://image.coolapk.com/feed/b@800x800.jpg" || got.Width != 800 || got.Height != 800 {
		t.Errorf("image = %+v", got)
	}
	if feeds[1].Stats.Likes != 12 || feeds[1].Stats.Comments != 7 {
		t.Errorf("stats = %+v", feeds[1].Stats)
	}
}

func TestCleanAPITextWithoutMarkup(t *testing.T) {
	const text = "1 < 2, and this is plain text"
	if got := cleanAPIText(text); got != text {
		t.Fatalf("cleanAPIText changed plain text to %q", got)
	}
}

// Posts are read by their feedType: an article has a cover and no pictures,
// a video its cover and length, a question its counts.
func TestFeedKinds(t *testing.T) {
	data := json.RawMessage(`[
		{"entityType":"feed","id":1,"feedType":"feedArticle","message_title":"文章","message":"摘要","message_cover":"http://image.coolapk.com/feed_cover/a@1600x719.jpg","pic":"http://image.coolapk.com/feed_cover/a@1600x719.jpg","picArr":[]},
		{"entityType":"feed","id":2,"feedType":"video","message":"视频","media_url":"https://weibo.com/1","media_info":"{\"name\":\"片名\",\"cover\":\"https://wx4.sinaimg.cn/c.jpg\",\"duration\":\"34400\"}"},
		{"entityType":"feed","id":3,"feedType":"question","message_title":"问题","question_answer_num":4,"question_follow_num":2},
		{"entityType":"feed","id":4,"feedType":"answer","message_title":"问题","message":"回答"},
		{"entityType":"feed","id":5,"feedType":"picture","pic":"http://image.coolapk.com/picture/b@800x1200.jpg","picArr":["http://image.coolapk.com/picture/b@800x1200.jpg"]},
		{"entityType":"feed","id":6,"feedType":"feed","message":"文字","picArr":["http://image.coolapk.com/feed/c@100x100.jpg"]}
	]`)
	objects, err := decodeAPIObjects(data)
	if err != nil {
		t.Fatal(err)
	}
	feeds := feedsFromObjects(objects)
	if len(feeds) != 6 {
		t.Fatalf("got %d feeds", len(feeds))
	}
	if f := feeds[0]; f.Kind != backend.PostArticle || f.Cover == nil || f.Cover.Width != 1600 || len(f.Images) != 0 {
		t.Errorf("article = %+v", f)
	}
	if f := feeds[1]; f.Kind != backend.PostVideo || f.Video == nil || f.Video.Duration != 34400*time.Millisecond || f.Video.CoverURL != "https://wx4.sinaimg.cn/c.jpg" || f.Video.URL != "https://weibo.com/1" || len(f.Images) != 0 {
		t.Errorf("video = %+v %+v", f, f.Video)
	}
	if f := feeds[2]; f.Kind != backend.PostQuestion || f.Answers != 4 || f.Followers != 2 {
		t.Errorf("question = %+v", f)
	}
	if f := feeds[3]; f.Kind != backend.PostAnswer || f.Title != "问题" || f.Content != "回答" {
		t.Errorf("answer = %+v", f)
	}
	if f := feeds[4]; f.Kind != backend.PostPicture || len(f.Images) != 1 || f.Images[0].Height != 1200 {
		t.Errorf("picture = %+v", f)
	}
	if f := feeds[5]; f.Kind != backend.PostText || len(f.Images) != 1 {
		t.Errorf("text = %+v", f)
	}
}

// An article's body is its message_raw_output blocks, in order.
func TestArticleBlocks(t *testing.T) {
	raw := `[{"type":"text","message":"开头<!--break-->第一段[欢呼]"},{"type":"image","url":"http://image.coolapk.com/feed/a@1206x6869.jpg","description":"说明"},{"type":"shareUrl","url":"https://example.com","title":"标题","subTitle":"副标题","logo":"http://image.coolapk.com/l.png"},{"type":"text","message":" "}]`
	data, _ := json.Marshal([]map[string]any{{"entityType": "feed", "id": 1, "feedType": "feedArticle", "message_raw_output": raw}})
	objects, err := decodeAPIObjects(data)
	if err != nil {
		t.Fatal(err)
	}
	blocks := feedsFromObjects(objects)[0].Blocks
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want text, image and link: %+v", len(blocks), blocks)
	}
	if blocks[0].Kind != backend.BlockText || blocks[0].Text != "开头第一段[欢呼]" {
		t.Errorf("text block = %+v", blocks[0])
	}
	if b := blocks[1]; b.Kind != backend.BlockImage || b.Image.Width != 1206 || b.Image.Height != 6869 || b.Caption != "说明" {
		t.Errorf("image block = %+v", b)
	}
	if b := blocks[2]; b.Kind != backend.BlockLink || b.Link.URL != "https://example.com" || b.Link.Title != "标题" {
		t.Errorf("link block = %+v", b)
	}
}

// The follow tab asks for the entry its selector remembers, or the first.
func TestSelectorEntry(t *testing.T) {
	var objects []map[string]any
	data := `[{"entityType":"card","entityTemplate":"iconScrollCard"},
	{"entityType":"card","entityTemplate":"selectorLinkCard","entities":[
		{"title":"全部关注","url":"/page?url=A"},
		{"title":"话题关注","url":"/page?url=B","extraData":{"rememberSelected":1}}]}]`
	if err := json.Unmarshal([]byte(data), &objects); err != nil {
		t.Fatal(err)
	}
	if u, title := selectorEntry(objects); u != "/page?url=B" || title != "话题关注" {
		t.Errorf("remembered entry = %q %q", u, title)
	}
	objects[1]["entities"].([]any)[1].(map[string]any)["extraData"] = nil
	if u, title := selectorEntry(objects); u != "/page?url=A" || title != "全部关注" {
		t.Errorf("first entry = %q %q", u, title)
	}
	if u, _ := selectorEntry(objects[:1]); u != "" {
		t.Errorf("no selector gave %q", u)
	}
}

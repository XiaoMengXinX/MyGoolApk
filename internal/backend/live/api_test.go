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
		{"entityType":"feed","id":42,"username":"Coolapk user","userAvatar":"http://avatar.coolapk.com/a.jpg","message":"Hello <a href=\"/t/mygo\">#MyGo#</a><br>native UI","dateline":1700000000,"likenum":"12","commentnum":7,"favnum":3,"targetRow":{"title":"OPPO Reno17","targetType":"product","id":86},"relationRows":[{"title":"酷安图赏与评测","url":"/page?url=channel"}],"picArr":["http://image.coolapk.com/feed/b@800x800.jpg"]}
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
	if feeds[1].Stats.Likes != 12 || feeds[1].Stats.Comments != 7 || feeds[1].Stats.Favorites != 3 {
		t.Errorf("stats = %+v", feeds[1].Stats)
	}
	if got := feeds[1]; got.Target == nil || got.Target.ID != "86" || got.Relation == nil || got.Relation.URL != "/page?url=channel" {
		t.Errorf("feed links = target %+v, relation %+v", got.Target, got.Relation)
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
		{"entityType":"feed","id":6,"feedType":"feed","message":"文字","picArr":["http://image.coolapk.com/feed/c@100x100.jpg"]},
		{"entityType":"feed","id":7,"feedType":"ershou","title":"二手手机","ershou_info":{"product_price":"价格面议"},"picArr":["http://image.coolapk.com/feed/a.jpg","http://image.coolapk.com/feed/b.jpg"]}
	]`)
	objects, err := decodeAPIObjects(data)
	if err != nil {
		t.Fatal(err)
	}
	feeds := feedsFromObjects(objects)
	if len(feeds) != 7 {
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
	if f := feeds[6]; f.Kind != backend.PostTrade || f.Price != "价格面议" || len(f.Images) != 2 {
		t.Errorf("trade = %+v", f)
	}
}

func TestSearchKeepsCommunityResults(t *testing.T) {
	objects, err := decodeAPIObjects(json.RawMessage(`[
		{"entityType":"card","entityTemplate":"listCard","entityId":"card-app","title":"应用","url":"tab://app","entities":[{"entityType":"apk","id":10,"title":"An app"}]},
		{"entityType":"card","entityTemplate":"listCard","entityId":"card-product","title":"数码","url":"tab://product","description":"更多相关数码","entities":[{"entityType":"product","id":21,"title":"手机","hot_num_txt":"1.7万","feed_comment_num_txt":"2.4万","owner_star_average_score":"8.6","star_average_score":"7.6"},{"entityType":"apk","id":11,"title":"Another app"}]},
		{"entityType":"card","entityTemplate":"listCard","entityId":"card-topic","title":"话题","url":"tab://feedTopic","description":"更多相关话题","entities":[{"entityType":"topic","title":"iOS16","hot_num_txt":"7295","commentnum_txt":"10.8万","star_average_score":"6.6"}]},
		{"entityType":"dyh","entityId":"31","title":"看看号"},
		{"entityType":"goods","entityId":"41","title":"商品"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	entries := entriesFromObjects(searchObjects(objects))
	if len(entries) != 3 {
		t.Fatalf("got %d community results, want 3", len(entries))
	}
	if c := entries[0].Card; c == nil || c.URL != "tab://product" || c.More != "更多相关数码" || len(c.Results) != 1 || c.Results[0].ID != "21" || c.Results[0].Hot != "1.7万" || c.Results[0].Discussions != "2.4万" || c.Results[0].Score != "8.6" {
		t.Errorf("product preview = %+v", c)
	}
	if c := entries[1].Card; c == nil || len(c.Results) != 1 || c.Results[0].URL != "/t/iOS16" || c.Results[0].Hot != "7295" || c.Results[0].Discussions != "10.8万" || c.Results[0].Score != "6.6" {
		t.Errorf("topic preview = %+v", c)
	}
	if c := entries[2]; c.Title != "看看号" || c.URL != backend.ChannelURLPrefix+"31" {
		t.Errorf("channel = %+v", c)
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

// A page that ends with its tabs lists them as the card that chooses the rows
// below, and the people and events a tab's list holds as links.
func TestTabPagesAndLinkRows(t *testing.T) {
	data := json.RawMessage(`[
		{"entityType":"card","entityTemplate":"messageCard","description":"读 <a href=\"t/今日热点\">#今日热点#</a>"},
		{"entityType":"card","entityTemplate":"iconTabLinkGridCard","entities":[
			{"entityType":"iconTabLink","title":"众测","url":"/page?url=V8_A"},
			{"entityType":"iconTabLink","title":"线下","url":"V8_B"},
			{"entityType":"iconTabLink","title":"投票","url":"#/feed/digestList?type=16"}]},
		{"entityType":"card","entityTemplate":"configCard","title":"默认配置"},
		{"entityType":"user","uid":12202,"username":"酷安小编","fans":140466},
		{"entityType":"pear_goods","id":7,"goods_title":"摄影活动","goods_promo_title":"2026年8月8日","goods_tags":"北京,8人","goods_url":"https://m.coolapk.com/activity/x"}
	]`)
	objects, err := decodeAPIObjects(data)
	if err != nil {
		t.Fatal(err)
	}
	entries := entriesFromObjects(objects)
	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4: %+v", len(entries), entries)
	}
	if c := entries[0].Card; c == nil || c.Kind != backend.CardNote || len(c.Note) == 0 {
		t.Errorf("note = %+v", entries[0].Card)
	}
	tabs := entries[1].Card
	if tabs == nil || !tabs.Chooses() || len(tabs.Items) != 3 {
		t.Fatalf("tabs = %+v", tabs)
	}
	for i, want := range []string{"/page?url=V8_A", "/page?url=V8_B", "#/feed/digestList?type=16"} {
		if got := tabs.Items[i].URL; got != want {
			t.Errorf("tab %d url = %q, want %q", i, got, want)
		}
	}
	if e := entries[2]; e.Kind != backend.EntryLink || e.URL != "/u/12202" || e.Title != "酷安小编" || !e.Round || e.Subtitle != "140466粉丝" {
		t.Errorf("user = %+v", e)
	}
	if e := entries[3]; e.Kind != backend.EntryLink || e.URL != "https://m.coolapk.com/activity/x" || e.Subtitle != "2026年8月8日  北京 8人" {
		t.Errorf("event = %+v", e)
	}
}

func TestVoteCounts(t *testing.T) {
	parse := func(src string) *backend.Vote {
		var o map[string]any
		if err := json.Unmarshal([]byte(src), &o); err != nil {
			t.Fatal(err)
		}
		return voteFromObject(o)
	}
	open := parse(`{"id":1,"type":0,"total_vote_num":6,"max_select_num":1,"options":[
		{"id":6337,"title":"A","total_select_num":-999,"color":"#F44336"},{"id":6338,"title":"B","total_select_num":-999,"color":"#2196F3"}]}`)
	if open.Counted || !open.Versus || open.Options[0].Votes != 0 || open.Options[0].Color != 0xF44336 || open.Voters != 6 {
		t.Errorf("open versus vote = %+v", open)
	}
	if listed := parse(`{"id":2,"type":1,"options":[{"id":1,"title":"A"},{"id":2,"title":"B"}]}`); listed.Counted || listed.Versus {
		t.Errorf("listed vote = %+v", listed)
	}
	done := parse(`{"id":3,"type":1,"max_select_num":2,"user_selected_option":1,"options":[{"id":1,"title":"A","total_select_num":3},{"id":2,"title":"B","total_select_num":0}]}`)
	if !done.Counted || done.Options[0].Votes != 3 || done.MaxSelect != 2 || !done.Options[0].Chosen || done.Options[1].Chosen {
		t.Errorf("counted vote = %+v", done)
	}
}

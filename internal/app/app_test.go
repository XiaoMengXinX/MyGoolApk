package app

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"

	"app/internal/backend"
)

// fakeBackend serves fixed content and counts what the pages ask for.
type fakeBackend struct {
	mu      sync.Mutex
	sorts   []backend.ReplySort
	details map[string]int
	uid     string
	feed    backend.Feed
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{details: map[string]int{}, feed: backend.Feed{
		ID: "1", Author: backend.Author{ID: "9", Name: "作者"}, Content: "第一条",
		Target: &backend.Target{Title: "一加15"}, Stats: backend.Stats{Likes: 3, Comments: 2},
	}}
}

func (b *fakeBackend) Feed(context.Context, backend.FeedQuery) (backend.FeedPage, error) {
	return backend.FeedPage{Feeds: []backend.Feed{b.feed}}, nil
}
func (b *fakeBackend) FeedDetail(_ context.Context, id string) (backend.Feed, error) {
	b.mu.Lock()
	b.details[id]++
	b.mu.Unlock()
	f := b.feed
	f.ID, f.Content = id, "帖子 "+id
	return f, nil
}
func (b *fakeBackend) Replies(_ context.Context, q backend.ReplyQuery) (backend.ReplyPage, error) {
	b.mu.Lock()
	b.sorts = append(b.sorts, q.Sort)
	b.mu.Unlock()
	return backend.ReplyPage{Replies: []backend.Reply{{ID: "r1", Author: backend.Author{ID: "8", Name: "评论者"}, Content: "热评内容"}}}, nil
}
func (b *fakeBackend) Search(_ context.Context, q backend.SearchQuery) (backend.SearchPage, error) {
	if q.Keyword == "" {
		return backend.SearchPage{HotSearches: []string{"新机资讯"}}, nil
	}
	switch q.Kind {
	case backend.SearchAll:
		return backend.SearchPage{Entries: []backend.PageEntry{
			{Kind: backend.EntryCard, ID: "products", Card: &backend.Card{Kind: backend.CardList, Title: "数码", URL: "tab://product", More: "更多相关数码", Results: []backend.PageEntry{{Kind: backend.EntryProduct, ID: "1", Title: "预览产品", Hot: "1.7万", Discussions: "2400", Score: "8.6"}}}},
			{Kind: backend.EntryFeed, ID: b.feed.ID, Feed: &b.feed},
		}}, nil
	case backend.SearchProducts:
		return backend.SearchPage{Entries: []backend.PageEntry{{Kind: backend.EntryProduct, ID: "1", Title: "搜索到的手机", Hot: "1.7万", Discussions: "2400", Score: "8.6"}}}, nil
	case backend.SearchQuestions:
		question := backend.Feed{ID: "301", Kind: backend.PostQuestion, Title: "相机该怎么选"}
		answer := backend.Feed{ID: "302", Kind: backend.PostAnswer, Title: "相机该怎么选", Content: "先看拍摄需求"}
		entries := []backend.PageEntry{}
		if q.FeedType != "answer" {
			entries = append(entries, backend.PageEntry{Kind: backend.EntryFeed, ID: question.ID, Feed: &question})
		}
		if q.FeedType != "question" {
			entries = append(entries, backend.PageEntry{Kind: backend.EntryFeed, ID: answer.ID, Feed: &answer})
		}
		return backend.SearchPage{Entries: entries}, nil
	}
	return backend.SearchPage{}, nil
}
func (b *fakeBackend) SearchSuggestions(context.Context, string) ([]string, error) { return nil, nil }
func (b *fakeBackend) User(_ context.Context, uid string) (backend.UserProfile, error) {
	return backend.UserProfile{Author: backend.Author{ID: uid, Name: "用户 " + uid}, FeedCount: 5}, nil
}
func (b *fakeBackend) CastVote(context.Context, string, []string) error { return nil }
func (b *fakeBackend) DeleteReply(context.Context, string) error        { return nil }
func (b *fakeBackend) UserFeeds(context.Context, string, backend.UserFeedKind, int) (backend.FeedPage, error) {
	return backend.FeedPage{Feeds: []backend.Feed{b.feed}}, nil
}
func (b *fakeBackend) UserConnections(context.Context, string, backend.UserConnectionKind, int) (backend.UserConnectionPage, error) {
	return backend.UserConnectionPage{}, nil
}
func (b *fakeBackend) PageEntries(ctx context.Context, q backend.PageQuery) (backend.PageEntriesResult, error) {
	if q.URL == backend.ProductCategoriesURL {
		return backend.PageEntriesResult{Entries: []backend.PageEntry{{Kind: backend.EntryCategory, ID: "category:1000", Title: "手机分类", URL: "/page?url=cat"}}}, nil
	}
	f := b.feed
	switch q.URL {
	case "/page?url=V9_HOME_TAB_RANKING":
		selector := &backend.Card{Kind: backend.CardSelector, Items: []backend.CardItem{
			{Title: "今日热门", URL: "#/day"}, {Title: "手机榜", URL: "#/phone"}}}
		return backend.PageEntriesResult{Entries: []backend.PageEntry{
			{Kind: backend.EntryCard, ID: "card:selector", Card: selector},
			{Kind: backend.EntryFeed, ID: f.ID, Feed: &f}}}, nil
	case "#/phone":
		f = backend.Feed{ID: "2", Author: backend.Author{ID: "9", Name: "作者"}, Content: "手机的帖子"}
	}
	return backend.PageEntriesResult{Entries: []backend.PageEntry{{Kind: backend.EntryFeed, ID: f.ID, Feed: &f}}}, nil
}
func (b *fakeBackend) TopicCategories(context.Context) ([]backend.TopicCategory, error) {
	return []backend.TopicCategory{{Title: "推荐", URL: "u1"}, {Title: "摄影", URL: "u2"}}, nil
}
func (b *fakeBackend) TopicList(_ context.Context, c backend.TopicCategory, _ int) (backend.TopicPage, error) {
	return backend.TopicPage{Items: []backend.TopicItem{{ID: "1", Title: c.Title + "话题", Subtitle: "1万热度"}}}, nil
}
func (b *fakeBackend) SessionUID() string { return b.uid }
func (b *fakeBackend) UserAgent() string  { return "" }
func (b *fakeBackend) ImportSession(context.Context, string, string, string) error {
	return nil
}
func (b *fakeBackend) Logout() error { return nil }
func (b *fakeBackend) Device() backend.DeviceInfo {
	return backend.DeviceInfo{ID: "ID-1", Model: "iPhone 16", HardwareID: "iPhone17,3", SystemVersion: "26.7", AppDevice: "dev", AppToken: "tok"}
}
func (b *fakeBackend) SetDeviceProfile(model, hardwareID, salesModel, systemVersion string) error {
	return nil
}
func (b *fakeBackend) RegenerateDeviceProfile() error          { return nil }
func (b *fakeBackend) ResetDeviceID(ctx context.Context) error { return nil }
func (b *fakeBackend) UnreadCounts(context.Context) (backend.UnreadCounts, error) {
	return backend.UnreadCounts{}, nil
}
func (b *fakeBackend) Notifications(context.Context, backend.NoticeKind, int) (backend.NotificationPage, error) {
	return backend.NotificationPage{}, nil
}
func (b *fakeBackend) MarkNotificationsRead(context.Context) error { return nil }
func (b *fakeBackend) ClearNotifications(context.Context) error    { return nil }
func (b *fakeBackend) NoticeSenders(context.Context) ([]backend.NoticeSender, error) {
	return nil, nil
}
func (b *fakeBackend) ServiceNotices(context.Context, string, int) (backend.NotificationPage, error) {
	return backend.NotificationPage{}, nil
}
func (b *fakeBackend) Chat(context.Context, string, int) (backend.ChatPage, error) {
	return backend.ChatPage{}, nil
}
func (b *fakeBackend) SendMessage(context.Context, string, string) error { return nil }
func (b *fakeBackend) Conversations(context.Context, int) (backend.ConversationPage, error) {
	return backend.ConversationPage{}, nil
}
func (b *fakeBackend) SetFeedLike(_ context.Context, _ string, liked bool) (int, error) {
	if liked {
		return 4, nil
	}
	return 3, nil
}
func (b *fakeBackend) SetReplyLike(context.Context, string, bool) (int, error) { return 0, nil }
func (b *fakeBackend) SetUserFollow(context.Context, string, bool) error       { return nil }
func (b *fakeBackend) Topic(context.Context, string, string) (backend.TopicDetail, error) {
	return backend.TopicDetail{}, nil
}
func (b *fakeBackend) SetTopicFollow(context.Context, string, string, bool) error { return nil }
func (b *fakeBackend) Collections(context.Context, string, int) (backend.CollectionPage, error) {
	return backend.CollectionPage{}, nil
}
func (b *fakeBackend) SetCollection(context.Context, string, string, bool) error { return nil }
func (b *fakeBackend) CreateReply(context.Context, backend.ReplyDraft) (backend.Reply, error) {
	return backend.Reply{}, nil
}
func (b *fakeBackend) CreateFeed(context.Context, backend.FeedDraft) (string, error) { return "", nil }
func (b *fakeBackend) FeedSummary(context.Context, string, func(string)) (backend.FeedSummary, error) {
	return backend.FeedSummary{}, backend.ErrNoSummary
}

// start opens the app on b, in a phone-sized window, once what it asked the
// backend for has arrived.
func start(t *testing.T, b backend.Backend) *nsgotest.App {
	t.Helper()
	app := nsgotest.Start(t, Root(func(string) (backend.Backend, error) { return b, nil }))
	settle(app)
	return app
}

// settle waits for the background requests of the page shown.
func settle(app *nsgotest.App) {
	for range 5 {
		time.Sleep(10 * time.Millisecond)
		app.Settle()
	}
}

// openCard presses the card of the headline tab; the neighbouring tabs the
// home page has built hold the same card.
func openCard(app *nsgotest.App) {
	app.Press("role:button 查看帖子 >> nth=1")
	settle(app)
}

// shows reports whether a view matches sel.
func shows(app *nsgotest.App, sel string) bool { return len(app.Query(sel)) > 0 }

func TestHomeShowsTheHeadlineFeed(t *testing.T) {
	app := start(t, newFakeBackend())
	if !shows(app, "text:第一条") {
		t.Fatal("the headline feed is not shown")
	}
}

func TestPressingACardOpensThePostAndItsComments(t *testing.T) {
	b := newFakeBackend()
	app := start(t, b)
	openCard(app)
	settle(app)
	if !shows(app, "text:帖子 1") || !shows(app, "text:热评内容") {
		t.Fatalf("the post page shows no post or comment:\n%s", app.Dump())
	}
	if b.details["1"] != 1 {
		t.Errorf("the post was asked for %d times, want 1", b.details["1"])
	}
}

func TestCommentSortSwitches(t *testing.T) {
	b := newFakeBackend()
	app := start(t, b)
	openCard(app)
	settle(app)
	app.Press("role:button 评论排序，默认")
	settle(app)
	app.Press("role:menuitemradio 最新")
	settle(app)
	b.mu.Lock()
	defer b.mu.Unlock()
	if n := len(b.sorts); n < 2 || b.sorts[0] != backend.RepliesRecent || b.sorts[n-1] != backend.RepliesLatest {
		t.Errorf("reply sorts asked for: %v, want recent first and latest last", b.sorts)
	}
}

func TestTopicChipOpensTheTopicPage(t *testing.T) {
	app := start(t, newFakeBackend())
	app.Press("role:button 话题 >> nth=1")
	settle(app)
	if !shows(app, "text:一加15") {
		t.Fatalf("the topic page is not shown:\n%s", app.Dump())
	}
	app.Back()
	app.FastForward(time.Second)
	settle(app)
	if !shows(app, "text:第一条") {
		t.Error("back does not return to the home page")
	}
}

func TestSearchShowsResults(t *testing.T) {
	app := start(t, newFakeBackend())
	app.Press("role:button 搜索")
	settle(app)
	if !shows(app, "text:新机资讯") {
		t.Fatalf("the hot searches are not shown:\n%s", app.Dump())
	}
	app.Type("label:搜索", "iphone")
	app.Submit("label:搜索")
	settle(app)
	if !shows(app, "text:第一条") {
		t.Fatalf("the results are not shown:\n%s", app.Dump())
	}
	if !shows(app, "text:1.7万热度  2400讨论") || !shows(app, "text:8.6") {
		t.Fatalf("combined search preview metrics are missing:\n%s", app.Dump())
	}
	app.Press("role:button 更多相关数码")
	settle(app)
	if !shows(app, "text:搜索到的手机") {
		t.Fatalf("the product group did not switch to its tab:\n%s", app.Dump())
	}
	if !shows(app, "text:1.7万热度  2400讨论") || !shows(app, "text:8.6") {
		t.Fatalf("product search metrics are missing:\n%s", app.Dump())
	}
}

func TestSearchQuestionFiltersIncludeAnswers(t *testing.T) {
	app := start(t, newFakeBackend())
	app.Press("role:button 搜索")
	app.Type("label:搜索", "相机")
	app.Submit("label:搜索")
	settle(app)
	app.Press("role:tab 问答 >> nth=1")
	settle(app)
	if !shows(app, "text:先看拍摄需求") {
		t.Fatalf("all Q&A did not show an answer:\n%s", app.Dump())
	}
	app.Press("role:button 只看提问")
	settle(app)
	if shows(app, "text:先看拍摄需求") {
		t.Fatal("question-only results still show an answer")
	}
	app.Press("role:button 只看回答")
	settle(app)
	if !shows(app, "text:先看拍摄需求") {
		t.Fatalf("answer-only results did not show an answer:\n%s", app.Dump())
	}
}

func TestArticleShareCardOpensReferencedPost(t *testing.T) {
	b := newFakeBackend()
	b.feed.Kind = backend.PostArticle
	b.feed.Blocks = []backend.Block{{Kind: backend.BlockLink, Link: &backend.Link{
		Title: "引用的图文", URL: "https://www.coolapk.com/feed/62425872?s=share",
	}}}
	app := start(t, b)
	openCard(app)
	settle(app)
	app.Press("role:button 链接")
	settle(app)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.details["62425872"] == 0 {
		t.Fatal("the referenced post detail was not requested")
	}
}

func TestTheDigitalTabListsCategories(t *testing.T) {
	app := start(t, newFakeBackend())
	app.Press("role:tab 数码")
	settle(app)
	if !shows(app, "text:手机分类") {
		t.Fatalf("the product categories are not shown:\n%s", app.Dump())
	}
}

func TestTheDeviceIdentityShows(t *testing.T) {
	app := start(t, newFakeBackend())
	app.Press("role:tab 我的")
	app.Press("role:button 设置")
	settle(app)
	app.Press("role:button 设备信息")
	settle(app)
	if !shows(app, "text:ID-1") {
		t.Fatalf("the device ID is not shown:\n%s", app.Dump())
	}
}

func TestAGuestIsSentToLoginToLike(t *testing.T) {
	b := newFakeBackend()
	app := start(t, b)
	openCard(app)
	settle(app)
	app.Press("role:button 点赞")
	settle(app)
	if shows(app, "text:4") {
		t.Error("a guest liked the post")
	}
}

func TestLikingAPostCountsIt(t *testing.T) {
	b := newFakeBackend()
	b.uid = "1"
	app := start(t, b)
	openCard(app)
	settle(app)
	app.Press("role:button 点赞")
	settle(app)
	if !shows(app, "text:4") {
		t.Fatalf("the like count did not change:\n%s", app.Dump())
	}
}

func TestPicturesOpenInTheViewer(t *testing.T) {
	b := newFakeBackend()
	b.feed.Images = []backend.Image{{URL: "http://127.0.0.1:1/a.jpg", Width: 100, Height: 100}, {URL: "http://127.0.0.1:1/b.jpg", Width: 100, Height: 100}}
	app := start(t, b)
	app.Press("role:button 查看第 2 张图片 >> nth=1")
	settle(app)
	if !shows(app, "text:2 / 2") {
		t.Fatalf("the viewer is not shown:\n%s", app.Dump())
	}
	app.Back()
	app.FastForward(time.Second)
	settle(app)
	if shows(app, "text:2 / 2") {
		t.Error("back does not close the viewer")
	}
}

func TestPressingAnAuthorOpensTheirPage(t *testing.T) {
	app := start(t, newFakeBackend())
	app.Press("role:button 作者主页 >> nth=1")
	app.FastForward(time.Second)
	settle(app)
	if !shows(app, "text:用户 9") {
		t.Fatalf("the author's page is not shown:\n%s", app.Dump())
	}
}

func TestPressingACommenterOpensTheirPage(t *testing.T) {
	app := start(t, newFakeBackend())
	openCard(app)
	// The commenter's avatar is the last of the page's.
	app.Press(fmt.Sprintf("role:button 作者主页 >> nth=%d", len(app.Query("role:button 作者主页"))-1))
	app.FastForward(time.Second)
	settle(app)
	if !shows(app, "text:用户 8") {
		t.Fatalf("the commenter's page is not shown:\n%s", app.Dump())
	}
}

// A selector's chip lists its own posts under the cards above it.
func TestASelectorChipSwapsTheListBelowIt(t *testing.T) {
	app := start(t, newFakeBackend())
	app.Press("role:tab 热榜")
	settle(app)
	app.Press("role:button 手机榜")
	settle(app)
	if !shows(app, "text:手机的帖子") {
		t.Fatalf("the chosen list is not shown:\n%s", app.Dump())
	}
	if !shows(app, "role:button 今日热门") {
		t.Error("the selector went with the list it replaced")
	}
}

func TestWideWindowShowsThePostBesideTheList(t *testing.T) {
	app := start(t, newFakeBackend())
	app.Resize(1200, 800)
	settle(app)
	openCard(app)
	app.FastForward(time.Second)
	settle(app)
	if !shows(app, "text:帖子 1") || !shows(app, "text:第一条") {
		t.Fatalf("the post is not beside the list:\n%s", app.Dump())
	}
}

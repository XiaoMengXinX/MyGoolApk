package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"

	"app/internal/backend"
)

type delayedDetailBackend struct {
	*fakeBackend
	detail backend.Feed
	ready  chan struct{}
}

func (b *delayedDetailBackend) FeedDetail(context.Context, string) (backend.Feed, error) {
	<-b.ready
	return b.detail, nil
}

func TestSeededPostShowsPinnedCommentsWithoutMovingThePost(t *testing.T) {
	b := &delayedDetailBackend{fakeBackend: newFakeBackend(), ready: make(chan struct{}, 1)}
	t.Cleanup(func() { close(b.ready) })
	b.detail = b.feed
	b.detail.Pinned = []backend.Reply{{ID: "pinned", Content: "置顶评论内容", Pinned: true}}
	app := start(t, b)
	openCard(app)
	list := detailList(t, app.Find("text:默认"))
	selector := fmt.Sprintf("view:%d", list.ID)
	app.Find(selector + " >> text:第一条")
	if shows(app, selector+" >> text:热评内容") {
		t.Error("the regular comment was shown before the pinned comments arrived")
	}
	b.ready <- struct{}{}
	app.WaitFor(selector+" >> text:置顶评论内容", time.Second)
	app.WaitFor(selector+" >> text:热评内容", time.Second)
	if offset := app.Find(selector).Offset.Y; offset != 0 {
		t.Errorf("the post moved to offset %v, want 0", offset)
	}
}

func detailList(t *testing.T, post *nsgotest.View) *nsgotest.View {
	t.Helper()
	for view := post.Parent; view != nil; view = view.Parent {
		if view.Props.TestID == "list" {
			return view
		}
	}
	t.Fatal("the post is outside its list")
	return nil
}

func TestArticleWaitsForItsDetailBeforeShowingThePost(t *testing.T) {
	for _, distinctCover := range []bool{true, false} {
		name := "separate cover"
		if !distinctCover {
			name = "cover is a body image"
		}
		t.Run(name, func(t *testing.T) {
			b := &delayedDetailBackend{fakeBackend: newFakeBackend(), ready: make(chan struct{}, 1)}
			t.Cleanup(func() { close(b.ready) })
			cover := backend.Image{URL: "cover.jpg", Width: 1600, Height: 900}
			image := backend.Image{URL: "body.jpg", Width: 1206, Height: 6869}
			if !distinctCover {
				cover.URL = image.URL
			}
			b.feed.Kind, b.feed.Title, b.feed.Cover = backend.PostArticle, "图文标题", &cover
			b.feed.Content = "列表正文" + strings.Repeat("摘要", 170)
			b.detail = b.feed
			b.detail.Content = "引言\n正文第二段"
			b.detail.Blocks = []backend.Block{
				{Kind: backend.BlockText, Text: "引言"},
				{Kind: backend.BlockImage, Image: image},
				{Kind: backend.BlockText, Text: "正文第二段"},
			}
			b.detail.Pinned = []backend.Reply{{ID: "pinned", Content: "置顶评论内容", Pinned: true}}
			if !distinctCover {
				b.detail.Cover = nil
			}

			app := start(t, b)
			openCard(app)
			const coverButton = "role:button 查看封面"
			if len(app.QueryMounted(coverButton)) != 0 {
				t.Error("the article mounted its provisional cover before the detail arrived")
			}

			b.ready <- struct{}{}
			intro := app.WaitFor("text:引言", time.Second)
			settle(app)
			list := detailList(t, intro)
			if list.Offset.Y != 0 {
				t.Errorf("the article opened at offset %v, want 0", list.Offset.Y)
			}
			if got := len(app.QueryMounted("role:button 查看图片")); got != 1 {
				t.Errorf("mounted %d body pictures, want 1", got)
			}
			wantCovers := 0
			if distinctCover {
				wantCovers = 1
			}
			if got := len(app.QueryMounted(coverButton)); got != wantCovers {
				t.Errorf("mounted %d covers, want %d", got, wantCovers)
			}
			selector := fmt.Sprintf("view:%d", list.ID)
			app.ScrollTo(selector, core.Point{Y: list.Content.Height})
			position := func(view *nsgotest.View) core.Points {
				var top core.Points
				for ; view != list; view = view.Parent {
					top += view.Frame.Origin.Y
				}
				return top
			}
			pinned := app.Find(selector + " >> text:置顶评论内容")
			regular := app.Find(selector + " >> text:热评内容")
			if position(pinned) >= position(regular) {
				t.Error("the pinned comment is not above the regular comment")
			}
		})
	}
}

type delayedSummaryBackend struct {
	*delayedDetailBackend
	summaryReady chan struct{}
}

func (b *delayedSummaryBackend) FeedSummary(context.Context, backend.SummaryRequest, func(string)) (backend.FeedSummary, error) {
	<-b.summaryReady
	return backend.FeedSummary{}, backend.ErrNoSummary
}

func TestArticleKeepsItsImagesWhenSummaryIsUnavailable(t *testing.T) {
	article := &delayedDetailBackend{fakeBackend: newFakeBackend(), ready: make(chan struct{})}
	close(article.ready)
	article.feed.Kind = backend.PostArticle
	article.detail = article.feed
	article.detail.Blocks = []backend.Block{
		{Kind: backend.BlockText, Text: "引言"},
		{Kind: backend.BlockImage, Image: backend.Image{Width: 1200, Height: 800}},
		{Kind: backend.BlockText, Text: strings.Repeat("正文", 400)},
	}
	b := &delayedSummaryBackend{delayedDetailBackend: article, summaryReady: make(chan struct{}, 1)}
	t.Cleanup(func() { close(b.summaryReady) })
	app := start(t, b)
	openCard(app)
	app.WaitFor("text:AI 总结", time.Second)
	const picture = "role:button 查看图片"
	before := app.FindMounted(picture).ID
	b.summaryReady <- struct{}{}
	app.WaitForGone("text:AI 总结", time.Second)
	if after := app.FindMounted(picture).ID; before != after {
		t.Errorf("article picture was replaced when the summary went away: %d -> %d", before, after)
	}
}

type streamingSummaryBackend struct {
	*delayedDetailBackend
	pieces chan string
	reqs   []backend.SummaryRequest
	model  string
}

func (b *streamingSummaryBackend) FeedSummary(ctx context.Context, req backend.SummaryRequest, delta func(string)) (backend.FeedSummary, error) {
	b.mu.Lock()
	b.reqs = append(b.reqs, req)
	model := b.model
	if model == "" {
		model = "test model"
	}
	b.mu.Unlock()
	var text string
	for {
		select {
		case piece, ok := <-b.pieces:
			if !ok {
				return backend.FeedSummary{Text: text, Model: model}, nil
			}
			text += piece
			delta(piece)
		case <-ctx.Done():
			return backend.FeedSummary{}, ctx.Err()
		}
	}
}

func TestSummaryStreamsInTheSameCard(t *testing.T) {
	detail := &delayedDetailBackend{fakeBackend: newFakeBackend(), ready: make(chan struct{})}
	close(detail.ready)
	detail.feed.Kind = backend.PostArticle
	detail.detail = detail.feed
	detail.detail.Blocks = []backend.Block{{Kind: backend.BlockText, Text: strings.Repeat("正文", 400)}}
	b := &streamingSummaryBackend{delayedDetailBackend: detail, pieces: make(chan string, 2)}
	app := start(t, b)
	openCard(app)
	heading := app.WaitFor("text:AI 总结", time.Second).ID
	body := app.Find("text:生成中…").ID
	b.pieces <- "摘要第一段"
	first := app.WaitFor("text:摘要第一段", time.Second)
	b.pieces <- "，以及第二段。"
	close(b.pieces)
	last := app.WaitFor("text:摘要第一段，以及第二段。", time.Second)
	app.WaitFor("text:test model", time.Second)
	if first.ID != body || last.ID != body || app.Find("text:AI 总结").ID != heading {
		t.Error("streaming replaced the summary card's views")
	}
}

type collectionBackend struct {
	*fakeBackend
	removed bool
}

func (b *collectionBackend) Collections(_ context.Context, _ string, page int) (backend.CollectionPage, error) {
	return backend.CollectionPage{Items: []backend.Collection{
		{ID: "folder", Name: "收藏夹示例", Count: 3, Contains: true},
	}, HasMore: page == 1}, nil
}

func (b *collectionBackend) SetCollection(_ context.Context, _, _ string, add bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.removed = !add
	return nil
}

func (b *collectionBackend) FeedDetail(ctx context.Context, id string) (backend.Feed, error) {
	f, err := b.fakeBackend.FeedDetail(ctx, id)
	b.mu.Lock()
	defer b.mu.Unlock()
	f.Favorited = true
	f.Stats.Favorites = 23
	if b.removed {
		f.Stats.Favorites = 21
	}
	return f, err
}

func TestPostUsesServerFavoriteCountAfterRemovingFromFolder(t *testing.T) {
	b := &collectionBackend{fakeBackend: newFakeBackend()}
	b.uid = "42"
	app := start(t, b)
	openCard(app)
	app.WaitFor("role:button 收藏 >> text:23", time.Second)
	app.Press("role:button 收藏")
	app.WaitFor("text:收藏夹示例", time.Second)
	app.Press("role:button 收藏夹 >> text:收藏夹示例")
	app.WaitFor("text:2 个内容", time.Second)
	app.Back()
	app.WaitFor("role:button 收藏 >> text:21", time.Second)
}

func newSummaryTestBackend(uid string, longPost, showAI bool) *streamingSummaryBackend {
	detail := &delayedDetailBackend{fakeBackend: newFakeBackend(), ready: make(chan struct{})}
	close(detail.ready)
	detail.uid = uid
	detail.feed.Kind = backend.PostArticle
	detail.detail = detail.feed
	detail.detail.Title = "测试文章"
	detail.detail.ShowAIAssistant = showAI
	if showAI {
		detail.detail.AIAccessKey = "key-test"
	}
	if longPost {
		detail.detail.Blocks = []backend.Block{{Kind: backend.BlockText, Text: strings.Repeat("正文", 400)}}
	} else {
		detail.detail.Blocks = []backend.Block{{Kind: backend.BlockText, Text: "短文内容"}}
	}
	return &streamingSummaryBackend{delayedDetailBackend: detail, pieces: make(chan string, 4), model: "DeepSeek-R1"}
}

func TestLoggedInSummaryRoutingAndManualTrigger(t *testing.T) {
	t.Run("logged in long post with assistant uses official concise", func(t *testing.T) {
		b := newSummaryTestBackend("42", true, true)
		b.pieces <- "精简摘要"
		close(b.pieces)
		app := start(t, b)
		openCard(app)
		app.WaitFor("text:精简摘要", time.Second)
		app.WaitFor("text:DeepSeek-R1", time.Second)
		if len(b.reqs) != 1 || b.reqs[0].Mode != backend.SummaryOfficialConcise || !b.reqs[0].HasTitle || b.reqs[0].AIAccessKey != "key-test" {
			t.Fatalf("unexpected summary requests: %+v", b.reqs)
		}
	})

	t.Run("logged in long post without assistant falls back to coolapk1s", func(t *testing.T) {
		b := newSummaryTestBackend("42", true, false)
		b.model = "Gemma 4 26B"
		b.pieces <- "镜像摘要"
		close(b.pieces)
		app := start(t, b)
		openCard(app)
		app.WaitFor("text:镜像摘要", time.Second)
		if len(b.reqs) != 1 || b.reqs[0].Mode != backend.SummaryCoolapk1s {
			t.Fatalf("unexpected summary requests: %+v", b.reqs)
		}
	})

	t.Run("logged in short post with assistant waits for tap", func(t *testing.T) {
		b := newSummaryTestBackend("42", false, true)
		app := start(t, b)
		openCard(app)
		app.WaitFor("text:点击获取AI总结", time.Second)
		if len(b.reqs) != 0 {
			t.Fatalf("short post should not auto-request summary, got %+v", b.reqs)
		}
		b.pieces <- "手动触发的摘要"
		close(b.pieces)
		app.Press("role:button 获取 AI 总结")
		app.WaitFor("text:手动触发的摘要", time.Second)
		if len(b.reqs) != 1 || b.reqs[0].Mode != backend.SummaryOfficialConcise {
			t.Fatalf("unexpected summary requests: %+v", b.reqs)
		}
	})
}

func TestAISummarySettingsMasterRawModeAndBigCoverOrder(t *testing.T) {
	t.Run("master switch disables sub-switches and suppresses summary", func(t *testing.T) {
		b := newSummaryTestBackend("42", true, true)
		app := start(t, b)

		app.Press("role:tab 我的")
		settle(app)
		app.Press("role:button 设置")
		settle(app)
		app.WaitFor("role:switch 启用 AI 总结", time.Second)

		// Turning off master switch disables both dependent switches.
		app.Press("role:switch 启用 AI 总结")
		settle(app)
		app.Press("role:switch 使用酷安 AI 助手生成总结")
		settle(app)
		if !app.Find("role:switch 使用酷安 AI 助手生成总结").Props.Checked {
			t.Fatal("official AI switch should stay locked while master switch is off")
		}
		app.Press("role:switch 弹窗查看酷安 AI 完整总结")
		settle(app)
		if app.Find("role:switch 弹窗查看酷安 AI 完整总结").Props.Checked {
			t.Fatal("raw mode switch should stay off while master switch is off")
		}

		// Opening a long post with master switch off neither requests nor shows summary.
		app.Back()
		settle(app)
		app.Press("role:tab 首页")
		settle(app)
		openCard(app)
		if len(b.reqs) != 0 {
			t.Fatalf("master switch off should not request summary, got %+v", b.reqs)
		}
		if shows(app, "text:AI 总结") {
			t.Fatal("summary card should not appear when master switch is off")
		}
	})

	t.Run("raw mode opens markdown sheet and official toggle gates it", func(t *testing.T) {
		b := newSummaryTestBackend("42", false, true)
		app := start(t, b)

		app.Press("role:tab 我的")
		settle(app)
		app.Press("role:button 设置")
		settle(app)
		app.WaitFor("role:switch 启用 AI 总结", time.Second)

		// Turning off official AI disables the dependent raw-mode switch.
		app.Press("role:switch 使用酷安 AI 助手生成总结")
		settle(app)
		app.Press("role:switch 弹窗查看酷安 AI 完整总结")
		settle(app)
		if app.Find("role:switch 弹窗查看酷安 AI 完整总结").Props.Checked {
			t.Fatal("raw mode switch should stay disabled when official AI is off")
		}

		// Re-enable official AI and enable raw mode.
		app.Press("role:switch 使用酷安 AI 助手生成总结")
		settle(app)
		app.Press("role:switch 弹窗查看酷安 AI 完整总结")
		settle(app)
		if !app.Find("role:switch 弹窗查看酷安 AI 完整总结").Props.Checked {
			t.Fatal("raw mode switch did not turn on")
		}

		// Go back to headline and open the short post: raw mode shows tap card and opens Markdown sheet.
		app.Back()
		settle(app)
		app.Press("role:tab 首页")
		settle(app)
		openCard(app)
		app.WaitFor("text:点击获取AI总结", time.Second)
		if len(b.reqs) != 0 {
			t.Fatalf("raw mode should wait for tap, got %+v", b.reqs)
		}

		app.Press("role:button 获取 AI 总结")
		app.WaitFor("text:生成中…", time.Second)
		b.pieces <- "## 核心结论\n\n第一点内容"
		app.WaitFor("role:dialog >> role:header 核心结论", time.Second)
		b.pieces <- "，以及补充说明。"
		close(b.pieces)
		app.WaitFor("role:dialog >> text:第一点内容，以及补充说明。", time.Second)
		app.WaitFor("role:dialog >> text:DeepSeek-R1", time.Second)
		if len(b.reqs) != 1 || b.reqs[0].Mode != backend.SummaryOfficialRaw {
			t.Fatalf("unexpected summary requests in raw mode: %+v", b.reqs)
		}
	})

	t.Run("big cover post places summary below cover images and above body text", func(t *testing.T) {
		detail := &delayedDetailBackend{fakeBackend: newFakeBackend(), ready: make(chan struct{})}
		close(detail.ready)
		detail.uid = "42"
		detail.feed.BigCover = true
		detail.feed.Images = []backend.Image{{URL: "big.jpg", Width: 1200, Height: 1600}}
		detail.feed.Content = "灯箱正文内容" + strings.Repeat("字", 800)
		detail.detail = detail.feed
		detail.detail.ShowAIAssistant = true
		detail.detail.AIAccessKey = "key-big"
		b := &streamingSummaryBackend{delayedDetailBackend: detail, pieces: make(chan string, 1), model: "DeepSeek-R1"}
		b.pieces <- "灯箱摘要"
		close(b.pieces)

		app := start(t, b)
		openCard(app)
		summaryView := app.WaitFor("text:灯箱摘要", time.Second)
		list := detailList(t, summaryView)
		scope := fmt.Sprintf("view:%d >> ", list.ID)
		picView := app.Find(scope + "role:button 查看第 1 张图片")
		bodyView := app.Find(scope + "text:灯箱正文内容")
		topInList := func(v *nsgotest.View) core.Points {
			var y core.Points
			for ; v != nil && v != list; v = v.Parent {
				y += v.Frame.Origin.Y
			}
			return y
		}
		if !(topInList(picView) < topInList(summaryView) && topInList(summaryView) < topInList(bodyView)) {
			t.Fatalf("want image (%v) < summary (%v) < body (%v)", topInList(picView), topInList(summaryView), topInList(bodyView))
		}
	})
}

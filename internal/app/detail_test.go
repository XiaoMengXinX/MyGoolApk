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

func (b *delayedSummaryBackend) FeedSummary(context.Context, string, func(string)) (backend.FeedSummary, error) {
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
}

func (b *streamingSummaryBackend) FeedSummary(ctx context.Context, _ string, delta func(string)) (backend.FeedSummary, error) {
	var text string
	for {
		select {
		case piece, ok := <-b.pieces:
			if !ok {
				return backend.FeedSummary{Text: text, Model: "test model"}, nil
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

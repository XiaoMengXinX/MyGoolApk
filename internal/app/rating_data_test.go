package app

import (
	"context"
	"testing"

	"app/internal/backend"
)

type topicRatingBackend struct {
	*fakeBackend
	ratings func(backend.NodeRatingQuery) backend.NodeRatingPage
}

func (b topicRatingBackend) TopicNodeRatings(_ context.Context, q backend.NodeRatingQuery) (backend.NodeRatingPage, error) {
	if b.ratings == nil {
		return backend.NodeRatingPage{}, nil
	}
	return b.ratings(q), nil
}

func (topicRatingBackend) TopicRatingTrends(context.Context, string) (backend.RatingTrends, error) {
	return backend.RatingTrends{Day: backend.RatingTrend{
		ScoreCeiling: 10, CountCeiling: 10,
		Periods: []backend.RatingPeriod{{Date: "11-28", Score: 9.2, Count: 10}},
	}}, nil
}

func (topicRatingBackend) Topic(context.Context, string, string) (backend.TopicDetail, error) {
	return backend.TopicDetail{ID: "125029", Title: "HyperOS4", Tabs: []backend.TopicTab{
		{Title: "讨论", URL: "#/discussion"},
		{Title: "机型区", URL: "#/models"},
		{Title: "点评", URL: "#/reviews"},
	}, Rating: &backend.Rating{
		Score: "9.2", Users: 1280, Detailed: 1204,
		Stats: &backend.TopicRatingStats{Stars: [5]int{47, 12, 38, 163, 1020}, RecentScore: "9.2", RecentCount: 708, RecentPositive: "91"},
	}}, nil
}

func (b topicRatingBackend) PageEntries(ctx context.Context, q backend.PageQuery) (backend.PageEntriesResult, error) {
	if q.URL == "#/reviews" {
		f := backend.Feed{ID: "review", Content: "话题点评正文"}
		return backend.PageEntriesResult{Entries: []backend.PageEntry{{Kind: backend.EntryFeed, ID: f.ID, Feed: &f}}}, nil
	}
	return b.fakeBackend.PageEntries(ctx, q)
}

func TestTopicRatingCardShowsDistributionAndSwitchesTab(t *testing.T) {
	app := start(t, topicRatingBackend{fakeBackend: newFakeBackend()})
	app.Press("role:button 一加15 >> nth=1")
	settle(app)
	for _, text := range []string{"5星", "1020", "1星", "47"} {
		if !shows(app, "text:"+text) {
			t.Errorf("missing %q in the rating card", text)
		}
	}
	app.Press("role:button 查看点评")
	settle(app)
	if !shows(app, "role:tab 点评") || !shows(app, "text:话题点评正文") {
		t.Fatalf("the topic should switch to its reviews tab:\n%s", app.Dump())
	}
}

func TestTopicRatingDataOpensFromTopic(t *testing.T) {
	app := start(t, topicRatingBackend{fakeBackend: newFakeBackend()})
	app.Press("role:button 一加15 >> nth=1")
	settle(app)
	app.Press("role:button 详细数据")
	settle(app)
	for _, text := range []string{"评分详情", "全部用户评分", "1280名用户评分", "1020", "91%", "点评人数"} {
		if !shows(app, "text:"+text) {
			t.Errorf("missing %q in rating details", text)
		}
	}
}

func TestTopicRatingFiltersReloadTheList(t *testing.T) {
	requests := make(chan backend.NodeRatingQuery, 8)
	b := topicRatingBackend{fakeBackend: newFakeBackend(), ratings: func(q backend.NodeRatingQuery) backend.NodeRatingPage {
		requests <- q
		if q.Filter.Comment == backend.RatingWithComment {
			return backend.NodeRatingPage{}
		}
		id, name := "3-125029-1", "全部列表酷友"
		if q.Filter.Star == 5 {
			id, name = "3-125029-2", "五星列表酷友"
		}
		return backend.NodeRatingPage{
			Items:  []backend.NodeRating{{ID: id, Author: backend.Author{Name: name}, Stars: 5}},
			Cursor: backend.ListCursor{FirstItem: id, LastItem: id},
		}
	}}
	app := start(t, b)
	app.Press("role:button 一加15 >> nth=1")
	settle(app)
	app.Press("role:button 详细数据")
	settle(app)
	check := func(filter backend.NodeRatingFilter, from string) {
		t.Helper()
		select {
		case q := <-requests:
			if q.TopicID != "125029" || q.Page != 1 || q.Filter != filter ||
				q.Cursor != (backend.ListCursor{FirstItem: from, LastItem: from}) {
				t.Fatalf("unexpected ratings request: %+v", q)
			}
		default:
			t.Fatal("rating list did not load")
		}
	}
	check(backend.NodeRatingFilter{}, "")
	app.Press("role:button 5星")
	settle(app)
	check(backend.NodeRatingFilter{Star: 5}, "3-125029-1")
	if !shows(app, "text:五星列表酷友") || shows(app, "text:全部列表酷友") {
		t.Fatalf("previous rows were not replaced:\n%s", app.Dump())
	}
	app.Press("role:button 只看详细打分")
	settle(app)
	check(backend.NodeRatingFilter{Star: 5, Detail: backend.RatingDetailOnly}, "3-125029-2")
	app.Press("role:button 只看详细打分")
	settle(app)
	check(backend.NodeRatingFilter{Star: 5, Detail: backend.RatingDetailOff}, "3-125029-2")
	app.Press("role:button 只看有评语")
	settle(app)
	check(backend.NodeRatingFilter{Star: 5, Detail: backend.RatingDetailOff, Comment: backend.RatingWithComment}, "3-125029-2")
	app.Press("role:button 只看无评语")
	settle(app)
	check(backend.NodeRatingFilter{Star: 5, Detail: backend.RatingDetailOff, Comment: backend.RatingWithoutComment}, "")
	app.Press("role:button 只看无评语")
	settle(app)
	check(backend.NodeRatingFilter{Star: 5, Detail: backend.RatingDetailOff, Comment: backend.RatingCommentAll}, "3-125029-2")
}

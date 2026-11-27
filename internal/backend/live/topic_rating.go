package live

import (
	"context"
	"strconv"

	"app/internal/backend"
)

func (b *coolapkBackend) TopicRatingTrends(ctx context.Context, id string) (backend.RatingTrends, error) {
	r, err := b.client.GetRawWithCtx(ctx, "/topic/ratingChart", map[string]any{"id": id})
	if err != nil {
		return backend.RatingTrends{}, err
	}
	o, err := decodeAPIObject(r.Data)
	if err != nil {
		return backend.RatingTrends{}, err
	}
	return backend.RatingTrends{
		Day:   ratingTrend(apiObject(apiObject(o, "day"), "ratingChart")),
		Week:  ratingTrend(apiObject(apiObject(o, "week"), "ratingChart")),
		Month: ratingTrend(apiObject(apiObject(o, "month"), "ratingChart")),
	}, nil
}

func ratingTrend(chart map[string]any) backend.RatingTrend {
	t := backend.RatingTrend{ScoreCeiling: 10}
	if axes := apiObjects(chart, "y"); len(axes) == 2 {
		floor, _ := strconv.ParseFloat(firstAPIString(axes[0], "min"), 32)
		ceil, _ := strconv.ParseFloat(firstAPIString(axes[0], "max"), 32)
		if floor >= 0 && floor < ceil && ceil <= 10 {
			t.ScoreFloor, t.ScoreCeiling = float32(floor), float32(ceil)
		}
		t.CountCeiling = apiInt(axes[1], "max")
	}
	for _, o := range apiObjects(chart, "x") {
		date := firstAPIString(o, "datelineStr")
		if date == "" {
			continue
		}
		score, _ := strconv.ParseFloat(searchScore(o, "score"), 32)
		count := apiInt(o, "count")
		t.CountCeiling = max(t.CountCeiling, count)
		t.Periods = append(t.Periods, backend.RatingPeriod{Date: date, Score: float32(score), Count: count})
	}
	return t
}

func (b *coolapkBackend) TopicNodeRatings(ctx context.Context, q backend.NodeRatingQuery) (backend.NodeRatingPage, error) {
	params := map[string]any{"url": "/feed/nodeRatingList", "targetType": 3, "targetId": q.TopicID, "isOwner": 0, "page": q.Page}
	if q.Cursor.FirstItem != "" {
		params["firstItem"], params["lastItem"] = q.Cursor.FirstItem, q.Cursor.LastItem
	}
	if q.Filter.Star != 0 {
		params["star"] = q.Filter.Star
	}
	// An untouched filter is omitted. The server gives explicit zero its own meaning.
	switch q.Filter.Detail {
	case backend.RatingDetailOnly:
		params["v4RatingItem"] = 1
	case backend.RatingDetailOff:
		params["v4RatingItem"] = 0
	}
	switch q.Filter.Comment {
	case backend.RatingCommentAll:
		params["ratingType"] = -1
	case backend.RatingWithComment:
		params["ratingType"] = 1
	case backend.RatingWithoutComment:
		params["ratingType"] = 0
	}
	objects, err := listObjects(b.client.GetRawWithCtx(ctx, "/page/dataList", params))
	if err != nil {
		return backend.NodeRatingPage{}, err
	}
	return nodeRatingPage(objects, q.Cursor, q.Page), nil
}

func nodeRatingPage(objects []map[string]any, cursor backend.ListCursor, page int) backend.NodeRatingPage {
	var items []backend.NodeRating
	for _, o := range objects {
		if firstAPIString(o, "entityType") != "nodeRating" {
			continue
		}
		id := firstAPIString(o, "entityId")
		if id == "" {
			continue
		}
		feed := firstAPIString(o, "feed_id")
		if feed == "0" {
			feed = ""
		}
		items = append(items, backend.NodeRating{ID: id, FeedID: feed, Stars: apiInt(o, "star"),
			Author: authorFromObject(o), Detailed: apiInt(o, "v4_has_rating_score") == 1})
	}
	if page == 1 {
		cursor = backend.ListCursor{}
	}
	if len(items) > 0 {
		if cursor.FirstItem == "" {
			cursor.FirstItem = items[0].ID
		}
		cursor.LastItem = items[len(items)-1].ID
	}
	return backend.NodeRatingPage{Items: items, HasMore: len(items) > 0, Cursor: cursor}
}

package live

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"app/internal/backend"
	coolapk "github.com/XiaoMengXinX/CoolapkApi-Go"
)

func TestTopicRatingsUseTheCapturedPaginationContract(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v6/page/dataList", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("url") != "/feed/nodeRatingList" || q.Get("isOwner") != "0" ||
			q.Get("targetId") != "125029" || q.Get("targetType") != "3" {
			t.Errorf("wrong node rating request: %s", q.Encode())
		}
		switch q.Get("page") {
		case "1":
			if q.Has("firstItem") || q.Has("lastItem") {
				t.Errorf("first page has cursors: %s", q.Encode())
			}
			fmt.Fprint(w, `{"data":[
				{"entityType":"nodeRating","entityId":"3-125029-10","star":5,"v4_has_rating_score":1,"feed_id":123,"userInfo":{"uid":"10","username":"酷友"}},
				{"entityType":"nodeRating","entityId":"3-125029-11","star":4,"feed_id":0}]}`)
		case "2":
			if q.Get("firstItem") != "3-125029-10" || q.Get("lastItem") != "3-125029-11" {
				t.Errorf("wrong page 2 cursor: %s", q.Encode())
			}
			fmt.Fprint(w, `{"data":[{"entityType":"nodeRating","entityId":"3-125029-12","star":4}]}`)
		case "3":
			if q.Get("firstItem") != "3-125029-10" || q.Get("lastItem") != "3-125029-12" {
				t.Errorf("wrong page 3 cursor: %s", q.Encode())
			}
			fmt.Fprint(w, `{"data":[]}`)
		default:
			t.Errorf("unexpected page: %s", q.Encode())
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	mux.HandleFunc("GET /v6/topic/ratingChart", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") != "125029" {
			t.Errorf("wrong chart id: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"data":{
			"day":{"ratingChart":{"x":[{"datelineStr":"11.28","score":"10.0","count":2}]}},
			"week":{"ratingChart":{"y":[{"min":7.5,"max":10},{"min":0,"max":500}],"x":[{"datelineStr":"11.23","score":"9.0","count":81}]}},
			"month":{"ratingChart":{"x":[{"datelineStr":"2026.11","score":"9.3","count":679}]}}
		}}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client := coolapk.New()
	client.APIEndpoint = server.URL + "/v6"
	b := &coolapkBackend{client: client}
	var cursor backend.ListCursor
	for i, more := range []bool{true, true, false} {
		r, err := b.TopicNodeRatings(t.Context(), backend.NodeRatingQuery{TopicID: "125029", Page: i + 1, Cursor: cursor})
		if err != nil || r.HasMore != more {
			t.Fatalf("rating page %d = %+v, %v", i+1, r, err)
		}
		if i == 0 {
			if len(r.Items) != 2 || r.Items[0].Stars != 5 || !r.Items[0].Detailed ||
				r.Items[0].Author.Name != "酷友" || r.Items[0].FeedID != "123" || r.Items[1].FeedID != "" {
				t.Errorf("ratings = %+v", r.Items)
			}
		}
		cursor = r.Cursor
	}
	trends, err := b.TopicRatingTrends(t.Context(), "125029")
	if err != nil || len(trends.Day.Periods) != 1 || len(trends.Week.Periods) != 1 || len(trends.Month.Periods) != 1 {
		t.Fatalf("rating trends = %+v, %v", trends, err)
	}
	if trends.Week.ScoreFloor != 7.5 || trends.Week.CountCeiling != 500 || trends.Week.Periods[0].Count != 81 {
		t.Errorf("weekly trend = %+v", trends.Week)
	}
}

func TestTopicRatingFiltersUseTheCapturedParameters(t *testing.T) {
	steps := []struct {
		filter backend.NodeRatingFilter
		page   int
		query  string
	}{
		{backend.NodeRatingFilter{Star: 4}, 1, "star=4"},
		{backend.NodeRatingFilter{Star: 4, Detail: backend.RatingDetailOnly}, 1, "star=4&v4RatingItem=1"},
		{backend.NodeRatingFilter{Star: 4, Detail: backend.RatingDetailOff, Comment: backend.RatingWithComment}, 1, "ratingType=1&star=4&v4RatingItem=0"},
		{backend.NodeRatingFilter{Star: 4, Detail: backend.RatingDetailOff, Comment: backend.RatingWithoutComment}, 1, "ratingType=0&star=4&v4RatingItem=0"},
		{backend.NodeRatingFilter{Star: 4, Detail: backend.RatingDetailOff, Comment: backend.RatingWithoutComment}, 2, "ratingType=0&star=4&v4RatingItem=0"},
		{backend.NodeRatingFilter{Star: 4, Detail: backend.RatingDetailOff, Comment: backend.RatingCommentAll}, 1, "ratingType=-1&star=4&v4RatingItem=0"},
	}
	requests := make(chan string, len(steps))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for _, key := range []string{"targetId", "targetType", "isOwner", "url"} {
			q.Del(key)
		}
		requests <- q.Encode()
		fmt.Fprint(w, `{"data":[{"entityType":"nodeRating","entityId":"3-125029-20","star":4}]}`)
	}))
	defer server.Close()
	client := coolapk.New()
	client.APIEndpoint = server.URL + "/v6"
	b := &coolapkBackend{client: client}
	for _, step := range steps {
		cursor := backend.ListCursor{FirstItem: "3-125029-10", LastItem: "3-125029-15"}
		wantFirst := "3-125029-20"
		if step.page > 1 {
			wantFirst = "3-125029-10"
		}
		resp, err := b.TopicNodeRatings(t.Context(), backend.NodeRatingQuery{TopicID: "125029", Page: step.page, Cursor: cursor, Filter: step.filter})
		if err != nil || resp.Cursor != (backend.ListCursor{FirstItem: wantFirst, LastItem: "3-125029-20"}) {
			t.Fatalf("filtered page %d = %+v, %v", step.page, resp, err)
		}
		want := fmt.Sprintf("firstItem=3-125029-10&lastItem=3-125029-15&page=%d&%s", step.page, step.query)
		if got := <-requests; got != want {
			t.Errorf("filtered page %d query = %q, want %q", step.page, got, want)
		}
	}
}

package live

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"app/internal/backend"
	coolapk "github.com/XiaoMengXinX/CoolapkApi-Go"
)

func TestAccountListRequests(t *testing.T) {
	cases := []struct {
		query  backend.PageQuery
		path   string
		args   url.Values
		body   string
		id     string
		cursor backend.ListCursor
	}{
		{backend.PageQuery{URL: backend.MyCollectionsURL, Page: 1}, "/collection/list",
			url.Values{"showDefault": {"0"}, "page": {"1"}},
			`[{"entityType":"collection","id":101,"title":"默认收藏单","item_num":0,"is_open":0}]`, "collection:101", backend.ListCursor{FirstItem: "101", LastItem: "101"}},
		{backend.PageQuery{URL: backend.MyCollectionsURL, Page: 2, Cursor: backend.ListCursor{FirstItem: "101", LastItem: "101"}}, "/collection/list",
			url.Values{"showDefault": {"0"}, "page": {"2"}, "firstItem": {"101"}, "lastItem": {"101"}},
			`[]`, "", backend.ListCursor{FirstItem: "101", LastItem: "101"}},
		{backend.PageQuery{URL: backend.FollowedCollectionsURL, Title: "关注", Page: 2, Cursor: backend.ListCursor{FirstItem: "102", LastItem: "102"}}, "/page/dataList",
			url.Values{"url": {"#/collection/followList"}, "title": {"我关注的收藏单"}, "page": {"2"}, "firstItem": {"102"}, "lastItem": {"102"}},
			`[]`, "", backend.ListCursor{FirstItem: "102", LastItem: "102"}},
		{backend.PageQuery{URL: backend.MyRepliesURL, Page: 1}, "/user/replyList",
			url.Values{"uid": {"10"}, "page": {"1"}},
			`[{"entityType":"feed_reply","id":103,"fid":201,"message":"我的回复"}]`, "reply:103", backend.ListCursor{FirstItem: "103", LastItem: "103"}},
		{backend.PageQuery{URL: backend.RepliesToMeURL, Page: 2, Cursor: backend.ListCursor{FirstItem: "104", LastItem: "105"}}, "/user/replyToMeList",
			url.Values{"uid": {"10"}, "page": {"2"}, "firstItem": {"104"}, "lastItem": {"105"}},
			`[{"entityType":"feed_reply","id":106,"fid":202,"message":"收到的回复"}]`, "reply:106", backend.ListCursor{FirstItem: "104", LastItem: "106"}},
	}
	for _, tc := range cases {
		t.Run(tc.query.URL, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v6"+tc.path || r.URL.Query().Encode() != tc.args.Encode() {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				fmt.Fprintf(w, `{"data":%s}`, tc.body)
			}))
			defer server.Close()
			c := coolapk.New()
			c.APIEndpoint = server.URL + "/v6"
			c.SetSession(coolapk.Session{UID: "10", Token: "test"})
			b := &coolapkBackend{client: c}
			got, err := b.PageEntries(t.Context(), tc.query)
			if err != nil {
				t.Fatal(err)
			}
			if got.Cursor != tc.cursor {
				t.Fatalf("cursor = %+v, want %+v", got.Cursor, tc.cursor)
			}
			if tc.id == "" {
				if got.HasMore || len(got.Entries) != 0 {
					t.Fatalf("expected the end of the list, got %+v", got)
				}
				return
			}
			if len(got.Entries) != 1 || got.Entries[0].ID != tc.id {
				t.Fatalf("wrong entries: %+v", got.Entries)
			}
		})
	}
}

func TestPageTitleWithSeparateFeeds(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint("feed-error=", fail), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Query().Get("url") {
				case "/page?url=daily":
					fmt.Fprint(w, `{"data":[{"entityType":"card","entityTemplate":"configCard","extraDataArr":{"pageTitle":"今日酷安:2026年11月27日","flexList":"feed","url":"#/feed/list"}}]}`)
				case "#/feed/list":
					if fail {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					fmt.Fprint(w, `{"data":[]}`)
				default:
					t.Errorf("unexpected page request: %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			c := coolapk.New()
			c.APIEndpoint = server.URL + "/v6"
			b := &coolapkBackend{client: c, cursors: make(map[string]apiCursor)}
			page, err := b.PageEntries(t.Context(), backend.PageQuery{URL: "/page?url=daily", Page: 1})
			if (err != nil) != fail || page.Title != "今日酷安:2026年11月27日" {
				t.Fatalf("page title = %q, error = %v", page.Title, err)
			}
		})
	}
}

func TestServerPageKeepsTheFirstCursorAcrossPages(t *testing.T) {
	steps := []struct {
		page, first, last, body string
	}{
		{"1", "", "", `[{"entityType":"card","entityTemplate":"selectorLinkCard","entityId":15033,"entityFixed":1,"extraDataArr":{"selectedTab":"手机榜"},"entities":[{"title":"热度榜","url":"#/hot"},{"title":"手机榜","url":"#/phone"}]},{"entityType":"product","id":5134},{"entityType":"product","id":4350},{"entityType":"card","entityTemplate":"configCard","entityId":37371,"extraDataArr":{"withRanking":1}}]`},
		{"2", "5134", "4350", `[{"entityType":"product","id":5673},{"entityType":"product","id":5554}]`},
		{"3", "5134", "5554", `[{"entityType":"product","id":5799}]`},
		{"1", "5134", "5799", `[]`},
	}
	i := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step := steps[i]
		q := r.URL.Query()
		if q.Get("url") != backend.DigitalRankURL || q.Get("page") != step.page ||
			q.Get("firstItem") != step.first || q.Get("lastItem") != step.last {
			t.Errorf("request %d: %v", i, q)
		}
		fmt.Fprintf(w, `{"data":%s}`, step.body)
		i++
	}))
	defer server.Close()
	c := coolapk.New()
	c.APIEndpoint = server.URL + "/v6"
	b := &coolapkBackend{client: c}
	q := backend.PageQuery{URL: backend.DigitalRankURL, Title: "排行榜", Page: 1}
	for _, page := range []int{1, 2, 3, 1} {
		q.Page = page
		result, err := b.PageEntries(t.Context(), q)
		if err != nil {
			t.Fatal(err)
		}
		q.Cursor = result.Cursor
		if page > 1 && (len(result.Entries) == 0 || !result.Entries[0].Ranked) {
			t.Fatalf("rank presentation for page %d: %+v", page, result.Entries)
		}
		if len(result.Entries) > 0 && page == 1 && result.Entries[0].Card.DefaultTab != 1 {
			t.Fatalf("default group: %+v", result.Entries[0].Card)
		}
	}
}

func TestSeparateContentPaginatesWithoutRefetchingHead(t *testing.T) {
	const parent = "/page?url=IOS_FIND_ERSHOU"
	const content = "#/feed/ershouList?dataListType=staggered"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		q := r.URL.Query()
		if q.Get("url") == parent && requests == 1 {
			fmt.Fprintf(w, `{"data":[{"entityType":"card","entityTemplate":"configCard","extraDataArr":{"flexList":"1","url":%q}}]}`, content)
			return
		}
		if q.Get("url") != content || requests == 3 && (q.Get("page") != "2" || q.Get("firstItem") != "101" || q.Get("lastItem") != "99") {
			t.Errorf("request %d: %v", requests, q)
		}
		fmt.Fprint(w, `{"data":[{"entityType":"feed","id":101},{"entityType":"feed","id":99}]}`)
	}))
	defer server.Close()
	c := coolapk.New()
	c.APIEndpoint = server.URL + "/v6"
	b := &coolapkBackend{client: c}
	first, err := b.PageEntries(t.Context(), backend.PageQuery{URL: parent, Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.PageEntries(t.Context(), backend.PageQuery{URL: parent, Page: 2, ContentURL: first.ContentURL, Cursor: first.Cursor})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 3 {
		t.Fatalf("got %d requests, want the parent and two content pages", requests)
	}
}

package live

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"app/internal/backend"
	coolapk "github.com/XiaoMengXinX/CoolapkApi-Go"
)

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

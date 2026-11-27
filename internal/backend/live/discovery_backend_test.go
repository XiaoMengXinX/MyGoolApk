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
			b := &coolapkBackend{client: c, cursors: make(map[string]apiCursor), flexURLs: make(map[string]string)}
			page, err := b.PageEntries(t.Context(), backend.PageQuery{URL: "/page?url=daily", Page: 1})
			if err != nil || page.Title != "今日酷安:2026年11月27日" {
				t.Fatalf("page title = %q, error = %v", page.Title, err)
			}
		})
	}
}

package live

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"app/internal/backend"
	coolapk "github.com/XiaoMengXinX/CoolapkApi-Go"
)

func TestServiceNotificationsUseTheirOwnGroupsAndCursor(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v6/notificationV18/list", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[]}`)
	})
	mux.HandleFunc("GET /v6/notificationV18/subGroupList", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"note_type":"notify_xms_v18","from_username":"酷安小秘书","subitem_notify":1}]}`)
	})
	mux.HandleFunc("GET /v6/notificationV18/subList", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("noteType") != "notify_xms_v18" {
			t.Errorf("wrong service group: %q", q.Get("noteType"))
		}
		switch q.Get("page") {
		case "1":
			fmt.Fprint(w, `{"data":[{"entityId":"41","note":"服务通知","url_btn_title":"立即查看"},{"entityId":"39"}]}`)
		case "2":
			if q.Get("firstItem") != "41" || q.Get("lastItem") != "39" {
				t.Errorf("wrong page 2 cursor: %s", q.Encode())
			}
			fmt.Fprint(w, `{"data":[{"entityId":"38"}]}`)
		case "3":
			if q.Get("firstItem") != "41" || q.Get("lastItem") != "38" {
				t.Errorf("wrong page 3 cursor: %s", q.Encode())
			}
			fmt.Fprint(w, `{"data":[]}`)
		default:
			t.Errorf("unexpected page: %s", q.Encode())
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	c := coolapk.New()
	c.APIEndpoint = server.URL + "/v6"
	b := &coolapkBackend{client: c, cursors: make(map[string]apiCursor)}
	if main, err := b.Notifications(t.Context(), backend.NoticesAll, 1); err != nil || len(main.Items) != 0 {
		t.Fatalf("main notifications = %+v, %v", main, err)
	}
	groups, err := b.NoticeSenders(t.Context())
	if err != nil || len(groups) != 1 || groups[0] != (backend.NoticeSender{Kind: "notify_xms_v18", Name: "酷安小秘书"}) {
		t.Fatalf("groups = %+v, %v", groups, err)
	}
	first, err := b.ServiceNotices(t.Context(), groups[0].Kind, 1)
	if err != nil || len(first.Items) != 2 || first.Items[0].Summary != "服务通知" || first.Items[0].Action != "立即查看" {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	if _, err := b.ServiceNotices(t.Context(), groups[0].Kind, 2); err != nil {
		t.Fatal(err)
	}
	if last, err := b.ServiceNotices(t.Context(), groups[0].Kind, 3); err != nil || last.HasMore {
		t.Fatalf("last page = %+v, %v", last, err)
	}
}

func TestBulkNotificationRequests(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v6/notification/clearCount", func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query(); q.Get("type") != "all" {
			t.Errorf("wrong clearCount query: %s", q.Encode())
		}
		fmt.Fprint(w, `{"data":"清除成功"}`)
	})
	mux.HandleFunc("POST /v6/notificationV18/deleteAll", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.PostForm.Get("type") != "notification" || r.URL.RawQuery != "" {
			t.Errorf("wrong deleteAll form: %v, %v", r.PostForm, err)
		}
		fmt.Fprint(w, `{"data":"删除成功"}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	c := coolapk.New()
	c.APIEndpoint = server.URL + "/v6"
	b := &coolapkBackend{client: c}
	if err := b.MarkNotificationsRead(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := b.ClearNotifications(t.Context()); err != nil {
		t.Fatal(err)
	}
}

package live

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	coolapk "github.com/XiaoMengXinX/CoolapkApi-Go"

	"app/internal/backend"
)

func TestOfficialSummaryTwoTurnAndCache(t *testing.T) {
	if n := utf8.RuneCountInString(officialTitledPrompt); n > 140 {
		t.Fatalf("officialTitledPrompt is %d runes, Coolapk limits prompt to 140", n)
	}
	if n := utf8.RuneCountInString(officialUntitledPrompt); n > 140 {
		t.Fatalf("officialUntitledPrompt is %d runes, Coolapk limits prompt to 140", n)
	}
	var asks []struct {
		prompt string
		stream string
	}
	historyBody := `{"data":[]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v6/ai/feedAskHistoryList":
			w.Header().Set("Content-Type", "application/json; charset=UTF-8")
			fmt.Fprint(w, historyBody)
		case "/v6/ai/feedAsk":
			_ = r.ParseForm()
			prompt := r.PostForm.Get("prompt")
			asks = append(asks, struct {
				prompt string
				stream string
			}{prompt: prompt, stream: r.URL.Query().Get("stream_output")})
			w.Header().Set("Content-Type", "text/event-stream;charset=UTF-8")
			if prompt == "" {
				fmt.Fprintln(w, `{"data":{"entityId":"10","entityType":"ai_prompt","thoughts":"","finish_reason":"null","text":"","is_answer":false,"recommendPrompt":[]}}`)
				fmt.Fprintln(w, `{"data":{"entityId":"10","entityType":"ai_prompt_answer","thoughts":"","finish_reason":"finish","text":"# 长总结\n内容","is_answer":true,"recommendPrompt":[]}}`)
				return
			}
			fmt.Fprintln(w, `{"data":{"entityId":"11","entityType":"ai_prompt","thoughts":"","finish_reason":"null","text":"","is_answer":false,"recommendPrompt":[]}}`)
			fmt.Fprintln(w, `{"data":{"entityId":"11","entityType":"ai_prompt_answer_stream","thoughts":"","finish_reason":"null","text":"精简","is_answer":true,"recommendPrompt":[]}}`)
			fmt.Fprintln(w, `{"data":{"entityId":"11","entityType":"ai_prompt_answer_stream","thoughts":"","finish_reason":"stop","text":"摘要。","is_answer":true,"recommendPrompt":[]}}`)
			fmt.Fprintln(w, `{"data":{"entityId":"11","entityType":"ai_prompt_answer","thoughts":"","finish_reason":"finish","text":"精简摘要。","is_answer":true,"recommendPrompt":[]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := coolapk.New()
	c.APIEndpoint = srv.URL + "/v6"
	c.HTTPClient = srv.Client()
	b := &coolapkBackend{client: c}

	var streamed strings.Builder
	got, err := b.FeedSummary(context.Background(), backend.SummaryRequest{
		FeedID:      "75215310",
		Mode:        backend.SummaryOfficialConcise,
		HasTitle:    true,
		AIAccessKey: "k1",
	}, func(piece string) { streamed.WriteString(piece) })
	if err != nil {
		t.Fatalf("FeedSummary concise: %v", err)
	}
	if got.Text != "精简摘要。" || got.Model != officialSummaryModelName || streamed.String() != "精简摘要。" {
		t.Fatalf("got=%+v streamed=%q", got, streamed.String())
	}
	if len(asks) != 2 || asks[0].prompt != "" || asks[0].stream != "0" || asks[1].prompt != officialTitledPrompt || asks[1].stream != "1" {
		t.Fatalf("unexpected asks: %+v", asks)
	}

	// Second call with cached history should skip feedAsk entirely.
	asks = nil
	historyBody = fmt.Sprintf(`{"data":[{"entityId":11,"entityType":"ai_prompt_answer","role":"assistant","content":"精简摘要。"},{"entityId":11,"entityType":"ai_prompt","role":"user","content":%q},{"entityId":10,"entityType":"ai_prompt_answer","role":"assistant","content":"# 长总结\n内容"}]}`, officialTitledPrompt)
	streamed.Reset()
	got, err = b.FeedSummary(context.Background(), backend.SummaryRequest{
		FeedID:      "75215310",
		Mode:        backend.SummaryOfficialConcise,
		HasTitle:    true,
		AIAccessKey: "k1",
	}, func(piece string) { streamed.WriteString(piece) })
	if err != nil {
		t.Fatalf("FeedSummary cached concise: %v", err)
	}
	if len(asks) != 0 || got.Text != "精简摘要。" || streamed.String() != "精简摘要。" {
		t.Fatalf("asks=%v got=%+v streamed=%q", asks, got, streamed.String())
	}

	// Raw mode with cached turn-1 history returns turn-1 Markdown without calling feedAsk.
	streamed.Reset()
	got, err = b.FeedSummary(context.Background(), backend.SummaryRequest{
		FeedID:      "75215310",
		Mode:        backend.SummaryOfficialRaw,
		HasTitle:    true,
		AIAccessKey: "k1",
	}, func(piece string) { streamed.WriteString(piece) })
	if err != nil {
		t.Fatalf("FeedSummary cached raw: %v", err)
	}
	if len(asks) != 0 || got.Text != "# 长总结\n内容" {
		t.Fatalf("raw asks=%v got=%+v", asks, got)
	}
}

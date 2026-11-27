package live

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	coolapk "github.com/XiaoMengXinX/CoolapkApi-Go"

	"app/internal/backend"
)

// summaryHost is the coolapk1s.com mirror, whose AI summary endpoint the
// post pages use when signed out or when Coolapk's assistant is unavailable.
// It answers 404 for posts too short to summarize, and accepts requests only
// with a same-site referer.
const summaryHost = "https://www.coolapk1s.com"

const (
	officialSummaryModelName = "DeepSeek-R1"
	officialSummaryTimeout   = 60 * time.Second

	// Coolapk's /v6/ai/feedAsk enforces a 140-character limit on prompt and
	// ignores prompt on the first turn. These follow-up prompts condense the
	// first-turn summary into 1-2 plain-text sentences without Markdown.
	officialTitledPrompt   = "请将上述文章浓缩为1到2句中文纯文本摘要，控制在60到80字，不超过100字。标题已在上方，勿复述标题，只写核心做法、结论或关键数据。直接以产品或主体开头，禁以本文/作者/用户开头，不罗列条目，禁用换行、引号和任何Markdown符号。"
	officialUntitledPrompt = "请将上述文章浓缩为1到2句中文纯文本摘要，控制在60到80字，不超过100字。必须点明产品或事件主体，并写核心做法、结论或关键数据。直接以主体开头，禁以本文/作者/用户开头，不罗列条目，禁用换行、引号和任何Markdown符号。"
)

// The service streams a fresh summary in pieces and answers a cached one at
// once; generation may take tens of seconds while the providers are tried.
var summaryHTTP = &http.Client{Timeout: officialSummaryTimeout}

// FeedSummary implements backend.Backend.
func (b *coolapkBackend) FeedSummary(ctx context.Context, req backend.SummaryRequest, delta func(string)) (backend.FeedSummary, error) {
	switch req.Mode {
	case backend.SummaryOfficialConcise:
		return b.officialFeedSummary(ctx, req, false, delta)
	case backend.SummaryOfficialRaw:
		return b.officialFeedSummary(ctx, req, true, delta)
	default:
		return b.coolapk1sSummary(ctx, req.FeedID, delta)
	}
}

func (b *coolapkBackend) coolapk1sSummary(ctx context.Context, feedID string, delta func(string)) (backend.FeedSummary, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", summaryHost+"/summary/"+feedID, nil)
	if err != nil {
		return backend.FeedSummary{}, err
	}
	req.Header.Set("Referer", summaryHost+"/feed/"+feedID)
	resp, err := summaryHTTP.Do(req)
	if err != nil {
		return backend.FeedSummary{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return backend.FeedSummary{}, backend.ErrNoSummary
	}
	if resp.StatusCode != http.StatusOK {
		return backend.FeedSummary{}, fmt.Errorf("summary: HTTP %d", resp.StatusCode)
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		var s struct {
			Text  string `json:"text"`
			Model string `json:"model"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil || s.Text == "" {
			return backend.FeedSummary{}, backend.ErrNoSummary
		}
		if delta != nil {
			delta(s.Text)
		}
		return backend.FeedSummary{Text: s.Text, Model: s.Model}, nil
	}
	return streamSummary(resp.Body, delta)
}

func (b *coolapkBackend) officialFeedSummary(ctx context.Context, req backend.SummaryRequest, rawOnly bool, delta func(string)) (backend.FeedSummary, error) {
	feedID, err := strconv.Atoi(req.FeedID)
	if err != nil || feedID <= 0 || req.AIAccessKey == "" {
		return backend.FeedSummary{}, backend.ErrNoSummary
	}
	ctx, cancel := context.WithTimeout(ctx, officialSummaryTimeout)
	defer cancel()

	history, err := b.client.GetFeedAskHistoryListWithCtx(ctx, feedID)
	if err != nil {
		return backend.FeedSummary{}, err
	}

	if rawOnly {
		if initial := findInitialAIAnswer(history); initial != "" {
			if delta != nil {
				delta(initial)
			}
			return backend.FeedSummary{Text: initial, Model: officialSummaryModelName}, nil
		}
		return b.streamOfficialAsk(ctx, feedID, req.AIAccessKey, "", delta)
	}

	prompt := officialUntitledPrompt
	if req.HasTitle {
		prompt = officialTitledPrompt
	}
	if cached := findPromptAIAnswer(history, prompt); cached != "" {
		if delta != nil {
			delta(cached)
		}
		return backend.FeedSummary{Text: cached, Model: officialSummaryModelName}, nil
	}

	if !hasCompletedAIAnswer(history) {
		noStream := false
		if _, err := b.client.FeedAskWithCtx(ctx, coolapk.FeedAskRequest{
			FeedID:       feedID,
			Model:        coolapk.DefaultAIModel,
			Prompt:       "",
			AIAccessKey:  req.AIAccessKey,
			StreamOutput: &noStream,
		}, nil); err != nil {
			return backend.FeedSummary{}, err
		}
	}

	return b.streamOfficialAsk(ctx, feedID, req.AIAccessKey, prompt, delta)
}

func (b *coolapkBackend) streamOfficialAsk(ctx context.Context, feedID int, aiAccessKey, prompt string, delta func(string)) (backend.FeedSummary, error) {
	final, err := b.client.FeedAskWithCtx(ctx, coolapk.FeedAskRequest{
		FeedID:      feedID,
		Model:       coolapk.DefaultAIModel,
		Prompt:      prompt,
		AIAccessKey: aiAccessKey,
	}, func(ev coolapk.FeedAskEvent) {
		if ev.EntityType == "ai_prompt_answer_stream" && ev.Text != "" && delta != nil {
			delta(ev.Text)
		}
	})
	if err != nil {
		return backend.FeedSummary{}, err
	}
	text := strings.TrimSpace(final.Text)
	if text == "" {
		return backend.FeedSummary{}, backend.ErrNoSummary
	}
	return backend.FeedSummary{Text: text, Model: officialSummaryModelName}, nil
}

func hasCompletedAIAnswer(history []coolapk.FeedAskHistoryItem) bool {
	for _, item := range history {
		if item.EntityType == "ai_prompt_answer" && strings.TrimSpace(item.Content) != "" {
			return true
		}
	}
	return false
}

// findInitialAIAnswer returns the first-turn automatic summary from history
// (an ai_prompt_answer without a sibling user ai_prompt of the same entityId,
// or the oldest assistant answer if all have prompts).
func findInitialAIAnswer(history []coolapk.FeedAskHistoryItem) string {
	userPrompts := make(map[string]bool)
	for _, item := range history {
		if item.EntityType == "ai_prompt" {
			userPrompts[item.EntityID.String()] = true
		}
	}
	fallback := ""
	for i := len(history) - 1; i >= 0; i-- {
		item := history[i]
		if item.EntityType != "ai_prompt_answer" {
			continue
		}
		text := strings.TrimSpace(item.Content)
		if text == "" {
			continue
		}
		if !userPrompts[item.EntityID.String()] {
			return text
		}
		if fallback == "" {
			fallback = text
		}
	}
	return fallback
}

// findPromptAIAnswer returns the newest assistant answer paired with a user
// turn matching prompt.
func findPromptAIAnswer(history []coolapk.FeedAskHistoryItem, prompt string) string {
	want := strings.TrimSpace(prompt)
	answers := make(map[string]string)
	for _, item := range history {
		if item.EntityType == "ai_prompt_answer" && strings.TrimSpace(item.Content) != "" {
			answers[item.EntityID.String()] = strings.TrimSpace(item.Content)
		}
	}
	for _, item := range history {
		if item.EntityType == "ai_prompt" && strings.TrimSpace(item.Content) == want {
			if ans := answers[item.EntityID.String()]; ans != "" {
				return ans
			}
		}
	}
	return ""
}

// streamSummary reads the server-sent events of a fresh summary: the model
// in use and the pieces of the text, until done or a failure.
func streamSummary(body io.Reader, delta func(string)) (backend.FeedSummary, error) {
	var out backend.FeedSummary
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event struct {
			Model string `json:"model"`
			Delta string `json:"delta"`
			Done  bool   `json:"done"`
			Error bool   `json:"error"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(line[len("data:"):])), &event); err != nil {
			continue
		}
		if event.Model != "" {
			out.Model = event.Model
		}
		if event.Delta != "" {
			out.Text += event.Delta
			if delta != nil {
				delta(event.Delta)
			}
		}
		if event.Done {
			return out, nil
		}
		if event.Error {
			return backend.FeedSummary{}, backend.ErrNoSummary
		}
	}
	if err := scanner.Err(); err != nil {
		return backend.FeedSummary{}, err
	}
	// The stream ended without a verdict; what arrived is kept.
	if out.Text != "" {
		return out, nil
	}
	return backend.FeedSummary{}, backend.ErrNoSummary
}

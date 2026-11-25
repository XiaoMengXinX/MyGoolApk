package live

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"app/internal/backend"
)

// summaryHost is the coolapk1s.com mirror, whose AI summary endpoint the
// post pages use. It answers 404 for posts too short to summarize, and
// accepts requests only with a same-site referer.
const summaryHost = "https://www.coolapk1s.com"

// The service streams a fresh summary in pieces and answers a cached one at
// once; generation may take tens of seconds while the providers are tried.
var summaryHTTP = &http.Client{Timeout: 60 * time.Second}

// FeedSummary implements backend.Backend. A cached summary arrives as one
// JSON document; a fresh one streams as server-sent events, one JSON object
// a line, and delta reports its pieces as they arrive.
func (b *coolapkBackend) FeedSummary(ctx context.Context, feedID string, delta func(string)) (backend.FeedSummary, error) {
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

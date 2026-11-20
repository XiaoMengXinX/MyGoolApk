package live

import (
	"encoding/json"
	"testing"

	"app/internal/backend"
)

// The backend fills the plain and rich fields together.
func TestFeedRichFields(t *testing.T) {
	data := json.RawMessage(`[{"entityType":"feed","id":"7","message":"看 <a class=\"feed-link-tag\" href=\"/t/mygo\">#MyGo#</a>[哈哈哈]"}]`)
	objects, err := decodeAPIObjects(data)
	if err != nil {
		t.Fatal(err)
	}
	feeds := feedsFromObjects(objects)
	if len(feeds) != 1 {
		t.Fatalf("got %d feeds", len(feeds))
	}
	feed := feeds[0]
	if feed.Content != "看 #MyGo#[哈哈哈]" {
		t.Fatalf("content = %q", feed.Content)
	}
	if len(feed.Rich) != 3 {
		t.Fatalf("rich = %+v", feed.Rich)
	}
	if feed.Rich[1].Kind != backend.SpanTopic || feed.Rich[2].Kind != backend.SpanEmoji || feed.Rich[2].Name == "" {
		t.Fatalf("rich = %+v", feed.Rich)
	}
}

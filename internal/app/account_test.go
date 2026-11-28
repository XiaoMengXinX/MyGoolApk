package app

import (
	"context"
	"testing"

	"app/internal/backend"
)

type accountRepliesBackend struct{ *fakeBackend }

func (b accountRepliesBackend) PageEntries(ctx context.Context, q backend.PageQuery) (backend.PageEntriesResult, error) {
	if q.URL == backend.RepliesToMeURL {
		f := backend.Feed{ID: "201", Kind: backend.PostComment, Content: "这是收到的回复"}
		return backend.PageEntriesResult{Entries: []backend.PageEntry{{Kind: backend.EntryFeed, ID: "reply:101", Feed: &f}}}, nil
	}
	return b.fakeBackend.PageEntries(ctx, q)
}

func TestMyRepliesOpensReceivedRepliesTab(t *testing.T) {
	b := newFakeBackend()
	b.uid = "10"
	app := start(t, accountRepliesBackend{b})
	app.Press("role:tab 我的")
	settle(app)
	app.Press("role:button 我的回复")
	settle(app)
	app.Press("role:tab 我收到的回复")
	settle(app)
	if !shows(app, "text:这是收到的回复") {
		t.Fatalf("received replies are missing:\n%s", app.Dump())
	}
}

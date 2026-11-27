package app

import (
	"context"
	"errors"
	"slices"
	"testing"
)

type authorFollowCall struct {
	uid    string
	follow bool
}

type authorFollowBackend struct {
	*fakeBackend
	calls   []authorFollowCall
	failure error
}

func (b *authorFollowBackend) SetUserFollow(_ context.Context, uid string, follow bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, authorFollowCall{uid, follow})
	return b.failure
}

func TestDetailAuthorFollow(t *testing.T) {
	b := &authorFollowBackend{fakeBackend: newFakeBackend()}
	b.uid = "1"
	app := start(t, b)
	openCard(app)
	settle(app)
	app.Press("role:button 关注")
	settle(app)
	if !shows(app, "role:button 取消关注") || !shows(app, "text:已关注") {
		t.Fatalf("the author is not shown as followed:\n%s", app.Dump())
	}
	app.Press("role:button 取消关注")
	settle(app)
	if !shows(app, "role:button 关注") {
		t.Fatal("the author is still shown as followed")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if want := []authorFollowCall{{"9", true}, {"9", false}}; !slices.Equal(b.calls, want) {
		t.Errorf("follow requests = %+v, want %+v", b.calls, want)
	}
}

func TestDetailAuthorFollowFailure(t *testing.T) {
	b := &authorFollowBackend{fakeBackend: newFakeBackend(), failure: errors.New("request failed")}
	b.uid = "1"
	app := start(t, b)
	openCard(app)
	settle(app)
	app.Press("role:button 关注")
	settle(app)
	if !shows(app, "role:button 关注") || shows(app, "role:button 取消关注") {
		t.Fatal("a failed follow changed the button state")
	}
	if !shows(app, "text:关注失败：request failed") {
		t.Fatalf("the follow error is not shown:\n%s", app.Dump())
	}
}

func TestOwnPostAuthorHeader(t *testing.T) {
	b := newFakeBackend()
	b.uid = b.feed.Author.ID
	app := start(t, b)
	openCard(app)
	settle(app)
	if shows(app, "role:button 关注") || shows(app, "role:button 取消关注") {
		t.Fatal("own post offers an author follow action")
	}
}

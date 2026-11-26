package app

import (
	"testing"
	"time"

	"app/internal/backend"
)

func TestFormatCount(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 12345: "1.2万", 20000: "2万", 123456789: "1.2亿"} {
		if got := formatCount(n); got != want {
			t.Errorf("formatCount(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestFormatTime(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		at   time.Time
		want string
	}{
		{now.Add(-10 * time.Second), "刚刚"},
		{now.Add(-5 * time.Minute), "5分钟前"},
		{now.Add(-3 * time.Hour), "3小时前"},
		{time.Date(2026, 11, 20, 18, 17, 0, 0, time.Local), "昨天 18:17"},
		{time.Date(2026, 11, 3, 8, 0, 0, 0, time.Local), "11月3日"},
		{time.Date(2025, 11, 3, 8, 0, 0, 0, time.Local), "2025年11月3日"},
	} {
		if got := formatTime(now, tc.at); got != tc.want {
			t.Errorf("formatTime(%v) = %q, want %q", tc.at, got, tc.want)
		}
	}
}

func TestThumbURLPicksTheVariant(t *testing.T) {
	cdn := backend.Image{URL: "https://image.coolapk.com/feed/a.jpg", Width: 2000}
	if got, want := thumbURL(cdn, thumbSquare), cdn.URL+".xs.jpg"; got != want {
		t.Errorf("square = %q, want %q", got, want)
	}
	cover := backend.Image{URL: "https://image.coolapk.com/feed_cover/a@1555x699.jpeg.xs.jpg"}
	if got := thumbURL(cover, thumbSquare); got != cover.URL {
		t.Errorf("article link cover = %q, want %q", got, cover.URL)
	}
	goods := backend.Image{URL: "https://image.coolapk.com/goods_cover/2024/0927/16/0_4501_6382_117@800x800.jpg"}
	if got := thumbURL(goods, thumbSquare); got != goods.URL {
		t.Errorf("goods link cover = %q, want %q", got, goods.URL)
	}
	narrow := backend.Image{URL: "https://image.coolapk.com/feed/b.jpg", Width: 600}
	if got := thumbURL(narrow, thumbSmall); got != narrow.URL {
		t.Errorf("an original narrower than the variant should be used as it is, got %q", got)
	}
	other := backend.Image{URL: "https://example.com/c.jpg", Width: 4000}
	if got := thumbURL(other, thumbMedium); got != other.URL {
		t.Errorf("a picture off the CDN should be used as it is, got %q", got)
	}
}

func TestMergeByIDDropsRepeats(t *testing.T) {
	id := func(s string) string { return s }
	got := mergeByID([]string{"a", "b"}, []string{"b", "c", ""}, id)
	want := []string{"a", "b", "c", ""}
	if len(got) != len(want) {
		t.Fatalf("mergeByID = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("mergeByID = %v, want %v", got, want)
		}
	}
}

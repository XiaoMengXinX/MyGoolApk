package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"app/internal/backend"
)

func displayName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "酷安用户"
	}
	return name
}

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

func formatCount(n int) string {
	switch {
	case n <= 0:
		return "0"
	case n < 10000:
		return strconv.Itoa(n)
	case n < 100000000:
		return trimZero(float64(n)/10000) + "万"
	default:
		return trimZero(float64(n)/100000000) + "亿"
	}
}

func trimZero(v float64) string {
	return strings.TrimSuffix(strconv.FormatFloat(v, 'f', 1, 64), ".0")
}

func formatTime(now, value time.Time) string {
	if value.IsZero() {
		return ""
	}
	d := max(now.Sub(value), 0)
	if d < time.Minute {
		return "刚刚"
	}
	local, today := value.Local(), now.Local()
	if sameDate(local, today) {
		if d < time.Hour {
			return strconv.Itoa(int(d/time.Minute)) + "分钟前"
		}
		return strconv.Itoa(int(d/time.Hour)) + "小时前"
	}
	if sameDate(local, today.AddDate(0, 0, -1)) {
		return fmt.Sprintf("昨天 %02d:%02d", local.Hour(), local.Minute())
	}
	if local.Year() == today.Year() {
		return fmt.Sprintf("%d月%d日", int(local.Month()), local.Day())
	}
	return fmt.Sprintf("%d年%d月%d日", local.Year(), int(local.Month()), local.Day())
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// mediaRatio is an image's width/height clamped to [lo, hi], or fallback
// when the URL carried no size.
func mediaRatio(im backend.Image, fallback, lo, hi float32) float32 {
	if im.Width <= 0 || im.Height <= 0 {
		return fallback
	}
	return max(lo, min(float32(im.Width)/float32(im.Height), hi))
}

// Coolapk image sizes: image.coolapk.com serves a JPEG variant for a URL
// with a suffix: ".xs.jpg" crops 480px squares, ".s.jpg" and ".m.jpg" keep
// the ratio at 720px and 1080px wide.
const (
	thumbSquare = "xs"
	thumbSmall  = "s"
	thumbMedium = "m"
)

var thumbWidths = map[string]int{thumbSquare: 480, thumbSmall: 720, thumbMedium: 1080}

// thumbURL returns the variant of a Coolapk image at size, or the original
// when it is no wider or is not a Coolapk upload. Variants are upscaled
// past the original's width, which only adds bytes.
func thumbURL(im backend.Image, size string) string {
	if !strings.HasPrefix(im.URL, "https://image.coolapk.com/") || strings.Contains(im.URL, "?") {
		return im.URL
	}
	if size != thumbSquare && im.Width > 0 && im.Width <= thumbWidths[size] {
		return im.URL
	}
	return im.URL + "." + size + ".jpg"
}

// formatTimeNow is formatTime relative to the present.
func formatTimeNow(t time.Time) string { return formatTime(time.Now(), t) }

package live

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"app/internal/backend"
)

// cardKinds maps the entityTemplate of a server card to how it shows. Cards
// not listed either hold rows the list shows as they are (feedListCard), or
// nothing the app shows (unLoginCard, sponsorCard, configCard).
var cardKinds = map[string]backend.CardKind{
	"imageCarouselCard_1":     backend.CardBanner,
	"imageCarouselCard":       backend.CardBanner,
	"imageScaleCard":          backend.CardBanner,
	"iconLinkGridCard":        backend.CardIcons,
	"iconGridCard":            backend.CardIcons,
	"iconLongTitleGridCard":   backend.CardIcons,
	"iconMiniGridCard":        backend.CardList,
	"iconMiniScrollCard":      backend.CardChips,
	"iconMiniLinkGridCard":    backend.CardChips,
	"capsuleListCard":         backend.CardChips,
	"iconListCard":            backend.CardList,
	"productTimelineListCard": backend.CardList,
	"imageTextScrollCard":     backend.CardScroll,
	"imageSquareScrollCard":   backend.CardScroll,
	"feedScrollCard":          backend.CardScroll,
	"titleCard":               backend.CardTitle,
	"selectorLinkCard":        backend.CardSelector,
}

// cardFromObject reads a server card, or nil when the app does not show it
// as a card.
func cardFromObject(o map[string]any) *backend.Card {
	kind, ok := cardKinds[firstAPIString(o, "entityTemplate")]
	if !ok {
		return nil
	}
	c := &backend.Card{Kind: kind, Title: firstAPIString(o, "title")}
	if kind == backend.CardTitle {
		// The entities of a title card configure the server's own layout.
		if c.Title == "" {
			return nil
		}
		return c
	}
	for _, e := range apiObjects(o, "entities") {
		if item, ok := cardItem(e); ok {
			c.Items = append(c.Items, item)
		}
	}
	if len(c.Items) == 0 {
		return nil
	}
	return c
}

// picSize reads the "@WxH" the CDN names a picture's size with.
var picSize = regexp.MustCompile(`@(\d+)x(\d+)\.`)

func picRatio(pic string) float32 {
	m := picSize.FindStringSubmatch(pic)
	if m == nil {
		return 0
	}
	w, _ := strconv.Atoi(m[1])
	h, _ := strconv.Atoi(m[2])
	if w <= 0 || h <= 0 {
		return 0
	}
	return float32(w) / float32(h)
}

// cardItem reads one entity of a card: a link, a topic, a product, a user or
// a post.
func cardItem(e map[string]any) (backend.CardItem, bool) {
	var it backend.CardItem
	switch firstAPIString(e, "entityType") {
	case "topic":
		it.Title = firstAPIString(e, "title")
		it.Pic = firstAPIString(e, "logo")
		it.URL = "/t/" + url.PathEscape(strings.Trim(it.Title, "#"))
		if n := firstAPIString(e, "hot_num_txt"); n != "" && n != "0" {
			it.Subtitle = n + "热度"
		} else if n := firstAPIString(e, "follownum_txt"); n != "" && n != "0" {
			it.Subtitle = n + "关注"
		}
	case "product":
		it.Title = firstAPIString(e, "title")
		it.Pic = firstAPIString(e, "logo")
		if id := apiID(e); id != "" {
			it.URL = "/product/" + id
		}
		// One figure fits under a product in a grid: its score, else its
		// followers.
		if score := firstAPIString(e, "star_average_score"); score != "" && score != "0" && score != "0.0" {
			it.Subtitle = score + "分"
		} else if n := firstAPIString(e, "follow_num_txt"); n != "" && n != "0" {
			it.Subtitle = n + "人关注"
		}
	case "user":
		it.Title = firstAPIString(e, "displayUsername", "username")
		it.Pic = firstAPIString(e, "userAvatar", "avatar")
		it.Round = true
		if uid := firstAPIString(e, "uid"); uid != "" {
			it.URL = "/u/" + uid
		}
		if n := firstAPIString(e, "fans"); n != "" && n != "0" {
			it.Subtitle = n + "粉丝"
		}
	case "feed":
		f := feedFromObject(e)
		it.Title = f.Title
		if it.Title == "" {
			it.Title = f.Content
		}
		if f.Cover != nil && f.Cover.URL != "" {
			it.Pic = f.Cover.URL
		} else if len(f.Images) > 0 {
			it.Pic = f.Images[0].URL
		}
		it.Subtitle = f.Author.Name
		if f.ID != "" {
			it.URL = "/feed/" + f.ID
		}
	default:
		it.Title = firstAPIString(e, "title")
		it.Subtitle = firstAPIString(e, "subTitle", "description")
		it.Pic = firstAPIString(e, "pic", "logo")
		it.URL = firstAPIString(e, "url")
	}
	it.Title = strings.TrimSpace(it.Title)
	it.Pic = normalizeCoolapkImage(it.Pic)
	it.Ratio = picRatio(it.Pic)
	return it, it.Title != "" || it.Pic != ""
}

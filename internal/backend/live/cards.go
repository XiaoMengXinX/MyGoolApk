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
	"iconMiniGridCard":        backend.CardPairs,
	"iconScrollCard":          backend.CardAvatars,
	"iconMiniScrollCard":      backend.CardChips,
	"iconMiniLinkGridCard":    backend.CardChips,
	"capsuleListCard":         backend.CardChips,
	"iconListCard":            backend.CardList,
	"productTimelineListCard": backend.CardList,
	"imageTextScrollCard":     backend.CardEditorial,
	"imageSquareScrollCard":   backend.CardTiles,
	"feedScrollCard":          backend.CardScroll,
	"titleCard":               backend.CardTitle,
	"selectorLinkCard":        backend.CardSelector,
	"sortSelectCard":          backend.CardSelector,
	"iconGridHorizonCard":     backend.CardLogos,
	"iconTabLinkGridCard":     backend.CardTabs,
	"imageTextGridCard":       backend.CardImageGrid,
	"iconButtonGridCard":      backend.CardButtons,
	"messageCard":             backend.CardNote,
	"headCard":                backend.CardNote,
	"refreshCard":             backend.CardTitle,
	"listCard":                backend.CardList,
	"productConfigList":       backend.CardConfigs,
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
	if kind == backend.CardNote {
		c.Subtitle = firstAPIString(o, "subTitle")
		if _, c.Note = backend.RichTextFromAPI(firstAPIString(o, "description")); len(c.Note) == 0 {
			return nil
		}
		return c
	}
	if m := dimensionRatio.FindStringSubmatch(firstAPIString(o, "extraData")); m != nil {
		w, _ := strconv.Atoi(m[1])
		h, _ := strconv.Atoi(m[2])
		if w > 0 && h > 0 {
			c.Ratio = float32(w) / float32(h)
		}
	}
	// A grid of products the server ranks shows their ranks.
	if kind == backend.CardIcons && strings.Contains(firstAPIString(o, "extraData"), `"withRanking":"1"`) {
		kind, c.Kind = backend.CardRanking, backend.CardRanking
	}
	switch kind {
	case backend.CardList, backend.CardConfigs, backend.CardAvatars, backend.CardPairs, backend.CardRanking, backend.CardEditorial:
		c.URL, c.More = firstAPIString(o, "url"), firstAPIString(o, "subTitle")
		if strings.HasPrefix(c.URL, "tab://") {
			c.More = firstAPIString(o, "description", "subTitle")
		}
	}
	entities := apiObjects(o, "entities")
	if kind == backend.CardList && (c.URL == "tab://product" || c.URL == "tab://feedTopic") {
		c.Results = entriesFromObjects(entities)
		if len(c.Results) == 0 {
			return nil
		}
		return c
	}
	for _, e := range entities {
		switch item, ok := cardItem(e); {
		case kind == backend.CardConfigs:
			// A version has no entity type: its name, price and page.
			c.Items = append(c.Items, backend.CardItem{Title: firstAPIString(e, "title"), Subtitle: price(e, "price"), URL: firstAPIString(e, "url")})
		case ok && kind == backend.CardList && firstAPIString(e, "entityType") == "product":
			// A product in a list has a line for its price and what it is made of.
			specs, _ := e["product_specs"].([]any)
			parts := []string{price(e, "price_min")}
			for _, s := range specs {
				if s, ok := s.(string); ok {
					parts = append(parts, s)
				}
			}
			if line := joinNonEmpty("  ", parts...); line != "" {
				item.Subtitle = line
			}
			c.Items = append(c.Items, item)
		case ok && kind == backend.CardSelector && item.Title == "应用关注":
			// The apps an account follows belong to the market, which the app leaves out.
		case ok:
			c.Items = append(c.Items, item)
		}
	}
	if len(c.Items) == 0 {
		return nil
	}
	if kind == backend.CardImageGrid && len(c.Items) == 1 {
		c.Kind = backend.CardBanner
		if c.Title == "" {
			c.Title = c.Items[0].Title
		}
	}
	return c
}

// dimensionRatio reads the shape a card's extraData asks its pictures to
// show at.
var dimensionRatio = regexp.MustCompile(`"dimensionRatio":"(\d+):(\d+)"`)

// price is the price o holds under key, "" when it has none.
func price(o map[string]any, key string) string {
	if n := apiInt(o, key); n > 0 {
		return "¥" + strconv.Itoa(n)
	}
	return ""
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
	case "user", "contacts":
		it.Title = firstAPIString(e, "displayUsername", "username")
		it.Pic = firstAPIString(e, "userAvatar", "avatar")
		it.Round = true
		if uid := firstAPIString(e, "uid"); uid != "" {
			it.URL = "/u/" + uid
		}
		if n := firstAPIString(e, "fans"); n != "" && n != "0" {
			it.Subtitle = n + "粉丝"
		}
	case "dyh":
		it.Title = firstAPIString(e, "title")
		it.Pic = firstAPIString(e, "logo")
		it.Subtitle = firstAPIString(e, "description")
		if id := apiID(e); id != "" {
			it.URL = "/dyh/" + id
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
		// A tab of an iconTabLinkGridCard may name a server page alone.
		if firstAPIString(e, "entityType") == "iconTabLink" && it.URL != "" &&
			!strings.HasPrefix(it.URL, "#") && !strings.HasPrefix(it.URL, "/") && !strings.Contains(it.URL, "://") {
			it.URL = "/page?url=" + it.URL
		}
	}
	it.Title = strings.TrimSpace(it.Title)
	it.Pic = normalizeCoolapkImage(it.Pic)
	it.Ratio = picRatio(it.Pic)
	return it, it.Title != "" || it.Pic != ""
}

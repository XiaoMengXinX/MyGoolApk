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
	"productTimelineListCard": backend.CardTimeline,
	"colorfulScrollCard":      backend.CardProductHighlights,
	"ratingNodeUserEnv":       backend.CardDeviceEnvironment,
	"imageTextScrollCard":     backend.CardEditorial,
	"imageSquareScrollCard":   backend.CardTiles,
	"imageScrollCard":         backend.CardTiles,
	"feedScrollCard":          backend.CardScroll,
	"titleCard":               backend.CardTitle,
	"selectorLinkCard":        backend.CardSelector,
	"sortSelectCard":          backend.CardSelector,
	"iconGridHorizonCard":     backend.CardLogos,
	"iconTabLinkGridCard":     backend.CardTabs,
	"imageTextGridCard":       backend.CardImageGrid,
	"iconButtonGridCard":      backend.CardButtons,
	"messageCard":             backend.CardNote,
	"textCard":                backend.CardNote,
	"headCard":                backend.CardNote,
	"refreshCard":             backend.CardTitle,
	"listCard":                backend.CardList,
	"productConfigList":       backend.CardConfigs,
}

// cardFromObject reads a server card, or nil when the app does not show it
// as a card.
func cardFromObject(o map[string]any) *backend.Card {
	template := firstAPIString(o, "entityTemplate")
	kind, ok := cardKinds[template]
	entities := apiObjects(o, "entities")
	if template == "feedListCard" && len(entities) > 0 && firstAPIString(o, "title") != "" {
		kind, ok = backend.CardEntries, true
	}
	if !ok {
		return nil
	}
	c := &backend.Card{Kind: kind, Title: firstAPIString(o, "title")}
	extra := cardExtra(o)
	page := firstAPIString(extra, "cardPageName")
	c.Ranked = apiInt(extra, "withRanking") == 1
	c.Continues = firstAPIString(extra, "viewBackgroundStyle") == "roundTop"
	isCategoryGrid := template == "iconLinkGridCard" && firstAPIString(o, "url") == "#/product/categoryList?listType=grid"
	if isCategoryGrid {
		c.Columns = 5
	} else if template == "iconLinkGridCard" && (page == "V9_HOME_TAB_WENDA" || page == "IOS_find_ERSHOU") {
		c.Columns, c.Inset = 4, true
	}
	if kind == backend.CardList && len(entities) > 0 {
		switch firstAPIString(entities[0], "entityType") {
		case "product":
			kind = backend.CardProductList
		case "topic":
			if template == "listCard" || strings.Contains(firstAPIString(o, "url"), "ratingUI=1") {
				kind = backend.CardProductList
			}
		}
	}
	if template == "iconMiniLinkGridCard" && page == "V9_HOME_TAB_SHIPIN" {
		kind, c.Inset = backend.CardMiniLinks, true
	} else if template == "iconMiniLinkGridCard" && firstAPIString(extra, "viewBackgroundStyle") == "roundBottom" {
		kind = backend.CardMiniLinks
	}
	if template == "iconScrollCard" && len(entities) > 0 && firstAPIString(entities[0], "entityType") == "topic" {
		kind, c.Inset = backend.CardTopicStrip, true
	}
	if page == "V15_YOUXI" && template == "imageScrollCard" {
		c.Inset = true
	}
	if kind == backend.CardIcons && c.Ranked {
		kind = backend.CardRanking
	}
	c.Kind = kind
	switch kind {
	case backend.CardList, backend.CardConfigs, backend.CardAvatars, backend.CardPairs, backend.CardRanking,
		backend.CardEditorial, backend.CardTimeline, backend.CardProductList, backend.CardProductHighlights,
		backend.CardEntries, backend.CardDeviceEnvironment, backend.CardTitle, backend.CardTiles, backend.CardTopicStrip:
		c.URL, c.More = firstAPIString(o, "url"), firstAPIString(o, "subTitle")
	case backend.CardIcons:
		if c.Title != "" && len(entities) > 0 && (firstAPIString(entities[0], "entityType") == "topic" ||
			firstAPIString(entities[0], "entityType") == "product") {
			c.URL, c.More = firstAPIString(o, "url"), firstAPIString(o, "subTitle")
		}
	}
	if kind == backend.CardTitle {
		// The entities of a title card configure the server's own layout.
		if c.Title == "" || apiInt(extra, "showToast") != 0 || apiInt(extra, "breakLine") != 0 {
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
	if template == "imageScrollCard" && c.Ratio == 0 {
		c.Ratio = 5.0 / 3
	}
	if c.URL == "tab://product" || c.URL == "tab://feedTopic" {
		c.Kind = backend.CardList
		c.More = firstAPIString(o, "description", "subTitle")
	}
	if kind == backend.CardEntries || c.URL == "tab://product" || c.URL == "tab://feedTopic" {
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
		case ok && kind == backend.CardSelector && item.Title == "应用关注":
			// The apps an account follows belong to the market, which the app leaves out.
		case ok && page == "V9_HOME_TAB_SHIPIN" && item.URL == "/feed/writer?type=share":
			// The video channel only links to browsing pages.
		case ok:
			item.Round = item.Round || isCategoryGrid
			if c.Chooses() && item.Title == firstAPIString(extra, "selectedTab") {
				c.DefaultTab = len(c.Items)
			}
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
		it.Hot = firstAPIString(e, "hot_num_txt")
		it.Discussions = firstAPIString(e, "commentnum_txt", "commentnum")
		it.Rating = itemRating(e, "star_average_score", "rating_average_score")
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
		it.Hot = firstAPIString(e, "hot_num_txt")
		it.Discussions = firstAPIString(e, "feed_comment_num_txt", "feed_comment_num")
		it.Rating = productRating(e)
		it.Subtitle = firstAPIString(e, "subTitle")
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
		if it.Title == "" {
			it.Title = firstAPIString(e, "title")
		}
		if f.Cover != nil && f.Cover.URL != "" {
			it.Pic = f.Cover.URL
		} else if len(f.Images) > 0 {
			it.Pic = f.Images[0].URL
		} else if f.Rating != nil && f.Target != nil {
			it.Pic = f.Target.LogoURL
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

func cardExtra(o map[string]any) map[string]any {
	if extra := apiObject(o, "extraDataArr"); extra != nil {
		return extra
	}
	if raw := firstAPIString(o, "extraData"); raw != "" {
		extra, _ := decodeAPIObject([]byte(raw))
		return extra
	}
	return nil
}

func productRating(o map[string]any) backend.ItemRating {
	rating := itemRating(o, "owner_star_average_score", "star_average_score")
	rating.OwnerCount = firstAPIString(o, "owner_star_total_count")
	return rating
}

func itemRating(o map[string]any, scoreFields ...string) backend.ItemRating {
	r := backend.ItemRating{
		Score:       searchScore(o, scoreFields...),
		Count:       firstAPIString(o, "rating_total_num_txt", "rating_total_num", "star_total_count"),
		ReleaseDate: firstAPIString(o, "release_time"),
		Unreleased:  firstAPIString(o, "release_status") == "0",
	}
	if extra := cardExtra(o); firstAPIString(extra, "rightStyle") == "text" {
		r.Score = firstAPIString(extra, "rightTopText")
		r.Label = firstAPIString(extra, "rightBottomText")
	}
	return r
}

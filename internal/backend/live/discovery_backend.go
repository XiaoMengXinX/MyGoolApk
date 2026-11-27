package live

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"app/internal/backend"
	coolapk "github.com/XiaoMengXinX/CoolapkApi-Go"
)

func (b *coolapkBackend) Search(ctx context.Context, q backend.SearchQuery) (backend.SearchPage, error) {
	q.Keyword = strings.TrimSpace(q.Keyword)
	if q.Keyword == "" {
		objects, err := listObjects(b.client.GetHotSearchWithCtx(ctx))
		if err != nil {
			return backend.SearchPage{}, err
		}
		seen := make(map[string]bool)
		var hot []string
		var visit func(map[string]any)
		visit = func(object map[string]any) {
			if firstAPIString(object, "entityType") == "hotSearch" {
				title := strings.TrimSpace(firstAPIString(object, "title"))
				if title != "" && !seen[title] {
					seen[title] = true
					hot = append(hot, title)
				}
			}
			for _, child := range apiObjects(object, "entities") {
				visit(child)
			}
		}
		for _, object := range objects {
			visit(object)
		}
		return backend.SearchPage{HotSearches: hot}, nil
	}
	searchType := searchTypes[q.Kind]
	feedType, sort := q.FeedType, q.Sort
	if feedType == "" {
		feedType = "all"
	}
	if sort == "" {
		sort = "default"
	}
	key := "search:" + searchType + ":" + feedType + ":" + sort + ":" + q.City + ":" + q.Keyword
	cursor := b.cursor(key, q.Page)
	negotiable := q.Kind == backend.SearchTrades && sort == backend.SearchTradeNegotiable
	if negotiable {
		sort = "default"
	}
	params := map[string]any{
		"type": searchType, "feedType": feedType, "sort": sort, "searchValue": q.Keyword,
		"page": q.Page, "showAnonymous": -1, "isStrict": 0,
	}
	if sort == "none" {
		params["isStrict"] = 1
	}
	if q.City != "" {
		params["city_code"] = q.City
	}
	if cursor.first != "" {
		params["firstItem"] = cursor.first
	}
	if cursor.last != "" {
		params["lastItem"] = cursor.last
	}
	objects, err := listObjects(b.client.GetRawWithCtx(ctx, "/search", params))
	if err != nil {
		return backend.SearchPage{}, err
	}
	objects = searchObjects(objects)
	cursor = feedCursor(objects)
	if q.Kind == backend.SearchAll {
		cursor = feedEntityCursor(objects)
	}
	b.storeCursor(key, q.Page, cursor)
	more := len(objects) > 0
	if negotiable {
		// Imported listings can say 价格面议 even when the server's price filter omits them.
		objects = slices.DeleteFunc(objects, func(object map[string]any) bool {
			return firstAPIString(apiObject(object, "ershou_info"), "product_price") != "价格面议"
		})
	}
	entries := entriesFromObjects(objects)
	return backend.SearchPage{Entries: entries, HasMore: len(entries) > 0 || negotiable && more}, nil
}

var searchTypes = [...]string{
	backend.SearchAll:       "all",
	backend.SearchProducts:  "product",
	backend.SearchFeeds:     "feed",
	backend.SearchTopics:    "feedTopic",
	backend.SearchUsers:     "user",
	backend.SearchTrades:    "ershou",
	backend.SearchQuestions: "ask",
	backend.SearchChannels:  "dyh",
}

// searchObjects keeps community results, including the previews in 综合.
func searchObjects(objects []map[string]any) []map[string]any {
	var results []map[string]any
	for _, object := range objects {
		switch firstAPIString(object, "entityType") {
		case "feed", "feed_reply", "product", "topic", "user", "contacts", "dyh":
			results = append(results, object)
		case "card":
			switch firstAPIString(object, "entityTemplate") {
			case "listCard":
				switch firstAPIString(object, "url") {
				case "tab://product", "tab://feedTopic", "tab://user", "tab://feed", "tab://ershou", "tab://dyh":
					var items []any
					for _, child := range searchObjects(apiObjects(object, "entities")) {
						items = append(items, child)
					}
					object["entities"] = items
					results = append(results, object)
				}
			case "feedListCard":
				results = append(results, searchObjects(apiObjects(object, "entities"))...)
			}
		}
	}
	return results
}

func (b *coolapkBackend) SearchSuggestions(ctx context.Context, keyword string) ([]string, error) {
	objects, err := listObjects(b.client.GetSearchSuggestionsWithCtx(ctx, strings.TrimSpace(keyword)))
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(objects))
	for _, object := range objects {
		value := strings.TrimSpace(firstAPIString(object, "title"))
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result, nil
}

func (b *coolapkBackend) User(ctx context.Context, uid string) (backend.UserProfile, error) {
	response, err := b.client.GetUserSpaceWithCtx(ctx, uid)
	if err != nil {
		return backend.UserProfile{}, err
	}
	object, err := decodeAPIObject(response.Data)
	if err != nil {
		return backend.UserProfile{}, err
	}
	gender := -1
	if firstAPIString(object, "gender") != "" {
		gender = apiInt(object, "gender")
	}
	return backend.UserProfile{
		Author:      authorFromObject(object),
		Bio:         firstAPIString(object, "bio"),
		CoverURL:    normalizeCoolapkImage(firstAPIString(object, "cover")),
		Level:       apiInt(object, "level"),
		FeedCount:   apiInt(object, "feed"),
		FollowCount: apiInt(object, "follow"),
		FansCount:   apiInt(object, "fans"),
		IsFollowed:  apiInt(object, "isFollow") != 0,
		VerifyTitle: firstAPIString(object, "verify_title"),
		LikeCount:   apiInt(object, "be_like_num"),
		Gender:      gender,
		Region:      joinNonEmpty(" ", firstAPIString(object, "province"), firstAPIString(object, "city")),
		LastActive:  apiTime(object, "logintime"),
	}, nil
}

func (b *coolapkBackend) UserFeeds(ctx context.Context, uid string, kind backend.UserFeedKind, page int) (backend.FeedPage, error) {
	key := fmt.Sprintf("user-feed:%s:%d", uid, kind)
	cursor := b.cursor(key, page)
	request := coolapk.ListRequest{Page: page, LastItem: cursor.last}
	list := b.client.GetUserFeedWithCtx
	switch kind {
	case backend.UserFeedArticles:
		list = b.client.GetUserArticleFeedWithCtx
	case backend.UserFeedQA:
		list = b.client.GetUserQuestionFeedWithCtx
	}
	objects, err := listObjects(list(ctx, uid, request))
	if err != nil {
		return backend.FeedPage{}, err
	}
	b.storeCursor(key, page, feedCursor(objects))
	return backend.FeedPage{Feeds: feedsFromObjects(objects), HasMore: len(objects) > 0}, nil
}

func (b *coolapkBackend) UserConnections(ctx context.Context, uid string, kind backend.UserConnectionKind, page int) (backend.UserConnectionPage, error) {
	key := fmt.Sprintf("user-connections:%s:%d", uid, kind)
	cursor := b.cursor(key, page)
	request := coolapk.ListRequest{Page: page, LastItem: cursor.last}
	var response *coolapk.RawResponse
	var err error
	if kind == backend.UserFans {
		response, err = b.client.GetFansListWithCtx(ctx, uid, request)
	} else {
		response, err = b.client.GetFollowListWithCtx(ctx, uid, request)
	}
	objects, err := listObjects(response, err)
	if err != nil {
		return backend.UserConnectionPage{}, err
	}
	b.storeCursor(key, page, feedCursor(objects))
	users := make([]backend.Author, 0, len(objects))
	for _, object := range objects {
		if user := apiObject(object, "userInfo"); user != nil {
			object = user
		}
		author := authorFromObject(object)
		if author.ID != "" {
			users = append(users, author)
		}
	}
	return backend.UserConnectionPage{Users: users, HasMore: len(objects) > 0}, nil
}

// topicHomeURL is the page of the topic tab: its card lists the categories.
const topicHomeURL = "V11_VERTICAL_TOPIC"

func (b *coolapkBackend) TopicCategories(ctx context.Context) ([]backend.TopicCategory, error) {
	objects, err := listObjects(b.client.GetPageDataListWithCtx(ctx, coolapk.PageDataListRequest{URL: topicHomeURL, Title: "话题", Page: 1}))
	if err != nil {
		return nil, err
	}
	var out []backend.TopicCategory
	for _, card := range objects {
		selected := firstAPIString(cardExtra(card), "selectedTab")
		for _, e := range apiObjects(card, "entities") {
			title, url := firstAPIString(e, "title"), firstAPIString(e, "url")
			if title != "" && url != "" {
				out = append(out, backend.TopicCategory{Title: title, URL: url, Default: title == selected})
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("没有话题分类")
	}
	return out, nil
}

func (b *coolapkBackend) TopicList(ctx context.Context, category backend.TopicCategory, page int, cursor backend.ListCursor) (backend.TopicPage, error) {
	objects, err := listObjects(b.client.GetPageDataListWithCtx(ctx, coolapk.PageDataListRequest{
		URL: category.URL, Title: category.Title, Page: page, FirstItem: cursor.FirstItem, LastItem: cursor.LastItem,
	}))
	if err != nil {
		return backend.TopicPage{}, err
	}
	var items []backend.TopicItem
	for _, o := range objects {
		// The first entry is a configuration card, not a topic.
		if firstAPIString(o, "entityType") != "topic" {
			continue
		}
		items = append(items, backend.TopicItem{
			ID:       apiID(o),
			Title:    firstAPIString(o, "title"),
			LogoURL:  normalizeCoolapkImage(firstAPIString(o, "logo")),
			Subtitle: firstAPIString(o, "subTitle"),
		})
	}
	return backend.TopicPage{Items: items, HasMore: len(items) > 0, Cursor: pageCursor(objects, cursor, page)}, nil
}

func (b *coolapkBackend) PageEntries(ctx context.Context, q backend.PageQuery) (backend.PageEntriesResult, error) {
	switch q.URL {
	case backend.HeadlineURL:
		objects, err := listObjects(b.client.GetHomeFeedWithCtx(ctx, coolapk.HomeFeedRequest{
			Page: q.Page, FirstItem: q.Cursor.FirstItem, LastItem: q.Cursor.LastItem,
		}))
		if err != nil {
			return backend.PageEntriesResult{}, err
		}
		return backend.PageEntriesResult{
			Entries: entriesFromObjects(objects), HasMore: len(objects) > 0, Cursor: pageCursor(objects, q.Cursor, q.Page),
		}, nil
	case backend.MyLikesURL, backend.MyRepliesURL, backend.MyFootprintsURL, backend.MyBlocksURL, backend.MyCollectionsURL:
		return b.myList(ctx, q)
	}
	if strings.HasPrefix(q.URL, backend.CollectionURLPrefix) {
		return b.myList(ctx, q)
	}
	if id, ok := strings.CutPrefix(q.URL, backend.ChannelURLPrefix); ok {
		key := "channel:" + id
		cursor := b.cursor(key, q.Page)
		params := map[string]any{"dyhId": id, "page": q.Page}
		if cursor.last != "" {
			params["lastItem"] = cursor.last
		}
		objects, err := listObjects(b.client.GetRawWithCtx(ctx, "/dyhArticle/list", params))
		if err != nil {
			return backend.PageEntriesResult{}, err
		}
		b.storeCursor(key, q.Page, feedCursor(objects))
		return backend.PageEntriesResult{Entries: entriesFromObjects(objects), HasMore: len(objects) > 0}, nil
	}
	if q.URL == backend.TradeBrandsURL {
		return b.tradeBrands(ctx, q.Page)
	}
	listURL := q.URL
	if q.Page > 1 && q.ContentURL != "" {
		listURL = q.ContentURL
	}
	fetch := func(url string, cursor backend.ListCursor) ([]map[string]any, error) {
		return listObjects(b.client.GetPageDataListWithCtx(ctx, coolapk.PageDataListRequest{
			URL: url, Title: q.Title, Page: q.Page, FirstItem: cursor.FirstItem, LastItem: cursor.LastItem,
		}))
	}
	var objects []map[string]any
	var err error
	cursor := q.Cursor
	if q.URL == backend.ProductCategoriesURL {
		objects, err = listObjects(b.client.GetProductCategoriesWithCtx(ctx))
	} else {
		if listURL != q.ContentURL && q.ContentURL != "" {
			cursor = backend.ListCursor{}
		}
		objects, err = fetch(listURL, cursor)
	}
	if err != nil {
		return backend.PageEntriesResult{}, err
	}
	contentURL := q.ContentURL
	title := pageTitle(objects)
	var head []backend.PageEntry
	if q.Page == 1 {
		contentURL = flexListURL(objects)
		if contentURL != "" && !slices.ContainsFunc(objects, func(o map[string]any) bool {
			return firstAPIString(o, "entityType") == "feed"
		}) {
			head = entriesFromObjects(objects)
			cursor = backend.ListCursor{}
			if contentURL == q.ContentURL {
				cursor = q.Cursor
			}
			objects, err = fetch(contentURL, cursor)
			if err != nil {
				return backend.PageEntriesResult{Title: title}, err
			}
			if len(objects) == 0 && cursor.FirstItem != "" {
				return backend.PageEntriesResult{}, nil
			}
		} else {
			contentURL = ""
		}
	}
	// The card of a product's versions comes empty: the product's page told them.
	for _, o := range objects {
		if firstAPIString(o, "entityTemplate") == "productConfigList" && len(apiObjects(o, "entities")) == 0 {
			if u, err := url.Parse(firstAPIString(o, "url")); err == nil {
				b.mu.Lock()
				rows := b.configs[u.Query().Get("productId")]
				b.mu.Unlock()
				entities := make([]any, len(rows))
				for i, r := range rows {
					entities[i] = r
				}
				o["entities"] = entities
			}
		}
	}
	entries := entriesFromObjects(objects)
	if q.URL == backend.DigitalHomeURL && q.Page == 1 {
		entries = b.digitalFollowedProducts(ctx, entries)
	}
	ranked := q.URL == backend.DigitalRankURL
	for _, o := range objects {
		if firstAPIString(o, "entityTemplate") == "configCard" {
			ranked = apiInt(cardExtra(o), "withRanking") == 1
			break
		}
	}
	// The request keeps the rank style even when a later page has no config card.
	if link, err := url.Parse(strings.TrimPrefix(q.URL, "#")); err == nil {
		query := link.Query()
		extra, _ := decodeAPIObject([]byte(query.Get("configCardExtraData")))
		ranked = ranked || apiInt(extra, "withRanking") == 1
		for i := range entries {
			e := &entries[i]
			if e.Kind == backend.EntryProduct || (query.Get("ratingUI") == "1" && strings.HasPrefix(e.ID, "topic:")) {
				e.RatingRow, e.Ranked = true, ranked
			}
		}
	}
	return backend.PageEntriesResult{
		Entries: append(head, entries...), HasMore: len(objects) > 0 && q.URL != backend.ProductCategoriesURL,
		Title: title, ContentURL: contentURL, Cursor: pageCursor(objects, cursor, q.Page),
	}, nil
}

// Pinned cards and client instructions are outside the scrollable list.
func pageCursor(objects []map[string]any, previous backend.ListCursor, page int) backend.ListCursor {
	var cursor backend.ListCursor
	for _, o := range objects {
		switch firstAPIString(o, "entityTemplate") {
		case "configCard", "refreshCard":
			continue
		}
		if apiInt(o, "entityFixed") == 1 {
			continue
		}
		if id := apiID(o); id != "" {
			if cursor.FirstItem == "" {
				cursor.FirstItem = id
			}
			cursor.LastItem = id
		}
	}
	if cursor.FirstItem == "" {
		return previous
	}
	if page > 1 && previous.FirstItem != "" {
		cursor.FirstItem = previous.FirstItem
	}
	return cursor
}

func isDeviceEnvironmentCard(e backend.PageEntry) bool {
	return e.Card != nil && e.Card.Kind == backend.CardDeviceEnvironment
}

// digitalFollowedProducts replaces the environment recommendations with the signed-in account's products.
func (b *coolapkBackend) digitalFollowedProducts(ctx context.Context, entries []backend.PageEntry) []backend.PageEntry {
	i := slices.IndexFunc(entries, isDeviceEnvironmentCard)
	if i < 0 {
		return entries
	}
	if b.SessionUID() != "" {
		objects, err := listObjects(b.client.GetPageDataListWithCtx(ctx, coolapk.PageDataListRequest{
			URL: backend.MyProductsURL, Title: "关注的数码", Page: 1,
		}))
		if err == nil {
			c := &backend.Card{Kind: backend.CardFollowedProducts, Title: "我的关注", URL: backend.MyProductsURL, More: "更多"}
			for _, o := range objects {
				if firstAPIString(o, "entityType") == "product" {
					if item, ok := cardItem(o); ok {
						c.Items = append(c.Items, item)
					}
				}
				if len(c.Items) == 4 {
					break
				}
			}
			if len(c.Items) > 0 {
				entries[i].Card = c
				return entries
			}
		}
	}
	return slices.Delete(entries, i, i+1)
}

func pageTitle(objects []map[string]any) string {
	for _, o := range objects {
		if firstAPIString(o, "entityTemplate") == "configCard" {
			return firstAPIString(cardExtra(o), "pageTitle")
		}
	}
	return ""
}

// entriesFromObjects reads the rows of a server page: posts, products,
// categories, and the cards the app shows; the rows other cards hold show as
// rows of their own.
func entriesFromObjects(objects []map[string]any) []backend.PageEntry {
	var entries []backend.PageEntry
	seen := map[string]bool{}
	var visit func(map[string]any)
	visit = func(o map[string]any) {
		switch firstAPIString(o, "entityType") {
		case "feed":
			if f := feedFromObject(o); f.ID != "" && !seen["feed:"+f.ID] {
				seen["feed:"+f.ID] = true
				e := backend.PageEntry{Kind: backend.EntryFeed, ID: f.ID, Feed: &f}
				if firstAPIString(o, "entityTemplate") == "articleNews" {
					e.Kind, e.Title = backend.EntryNews, firstAPIString(o, "editor_title", "message_title")
					if e.Title == "" {
						e.Title, _, _ = strings.Cut(f.Content, "\n")
					}
				}
				entries = append(entries, e)
			}
			return
		case "product":
			if id := apiID(o); id != "" && !seen["product:"+id] {
				seen["product:"+id] = true
				entries = append(entries, backend.PageEntry{
					Kind: backend.EntryProduct, ID: id, Title: firstAPIString(o, "title"),
					LogoURL:  normalizeCoolapkImage(firstAPIString(o, "logo")),
					Subtitle: productSubtitle(o), Hot: firstAPIString(o, "hot_num_txt"),
					Discussions: firstAPIString(o, "feed_comment_num_txt", "feed_comment_num"), Score: searchScore(o, "owner_star_average_score", "star_average_score"),
					Rating:   productRating(o),
					Followed: apiInt(apiObject(o, "userAction"), "follow") == 1,
					Timeline: strings.HasPrefix(firstAPIString(o, "entityTemplate"), "productTimeline"),
				})
			}
			return
		case "productBrand":
			if url := firstAPIString(o, "url"); url != "" {
				entries = append(entries, backend.PageEntry{
					Kind: backend.EntryCategory, ID: "category:" + apiID(o), Title: firstAPIString(o, "title"),
					LogoURL:  normalizeCoolapkImage(firstAPIString(o, "logo")),
					Subtitle: productCount(apiInt(o, "product_num")), URL: url,
				})
			}
			return
		case "user", "contacts":
			// A person of a ranking (#/member/rankList, #/user/statList).
			if u := authorFromObject(o); u.ID != "" && !seen["user:"+u.ID] {
				seen["user:"+u.ID] = true
				entries = append(entries, backend.PageEntry{
					Kind: backend.EntryLink, ID: "user:" + u.ID, Title: u.Name, LogoURL: u.AvatarURL, Round: true,
					Subtitle: userSubtitle(o), URL: "/u/" + u.ID,
				})
			}
			return
		case "dyh":
			if id := apiID(o); id != "" && !seen["dyh:"+id] {
				seen["dyh:"+id] = true
				entries = append(entries, backend.PageEntry{
					Kind: backend.EntryCategory, ID: "dyh:" + id, Title: firstAPIString(o, "title"),
					LogoURL: normalizeCoolapkImage(firstAPIString(o, "logo")), Subtitle: firstAPIString(o, "description"),
					URL: backend.ChannelURLPrefix + id,
				})
			}
			return
		case "topic":
			if title := firstAPIString(o, "title"); title != "" && !seen["topic:"+title] {
				seen["topic:"+title] = true
				hot := firstAPIString(o, "hot_num_txt")
				discussions := firstAPIString(o, "commentnum_txt", "commentnum")
				subtitle := ""
				if hot != "" {
					subtitle = hot + "热度"
				}
				if discussions != "" {
					subtitle = joinNonEmpty("  ", subtitle, discussions+"讨论")
				}
				entries = append(entries, backend.PageEntry{
					Kind: backend.EntryLink, ID: "topic:" + title, Title: title, LogoURL: normalizeCoolapkImage(firstAPIString(o, "logo")),
					Subtitle: subtitle,
					Hot:      hot, Discussions: discussions, Score: searchScore(o, "star_average_score"),
					Rating: itemRating(o, "star_average_score", "rating_average_score"),
					URL:    "/t/" + title,
				})
			}
			return
		case "collection":
			if id := apiID(o); id != "" && !seen["collection:"+id] {
				seen["collection:"+id] = true
				entries = append(entries, backend.PageEntry{
					Kind: backend.EntryCategory, ID: "collection:" + id, Title: firstAPIString(o, "title"),
					LogoURL:  normalizeCoolapkImage(firstAPIString(o, "cover_pic", "logo")),
					Subtitle: joinNonEmpty("  ", countText(firstAPIString(o, "item_num"), "个内容"), firstAPIString(o, "description")),
					URL:      backend.CollectionURLPrefix + id,
				})
			}
			return
		case "pear_goods":
			// An offline event: its date and place, and the page it opens.
			if link := firstAPIString(o, "goods_url", "goods_buy_url"); link != "" {
				entries = append(entries, backend.PageEntry{
					Kind: backend.EntryLink, ID: "goods:" + apiID(o), Title: firstAPIString(o, "goods_title", "title"),
					LogoURL:  normalizeCoolapkImage(firstAPIString(o, "goods_pic")),
					Subtitle: joinNonEmpty("  ", firstAPIString(o, "goods_promo_title"), strings.ReplaceAll(firstAPIString(o, "goods_tags"), ",", " ")),
					URL:      link,
				})
			}
			return
		case "feed_reply":
			// A comment of the account's own lists shows as the post it is on,
			// which it opens.
			if f := replyAsFeed(o); f.ID != "" && !seen["reply:"+apiID(o)] {
				seen["reply:"+apiID(o)] = true
				entries = append(entries, backend.PageEntry{Kind: backend.EntryFeed, ID: "reply:" + apiID(o), Feed: &f})
			}
			return
		case "card":
			if c := cardFromObject(o); c != nil {
				id := "card:" + firstAPIString(o, "entityTemplate") + ":" + c.Title + ":" + apiID(o)
				if !seen[id] {
					seen[id] = true
					entries = append(entries, backend.PageEntry{Kind: backend.EntryCard, ID: id, Card: c})
				}
				return
			}
		}
		for _, child := range apiObjects(o, "entities") {
			visit(child)
		}
	}
	for _, o := range objects {
		visit(o)
	}
	return entries
}

// userSubtitle is the followers of a person's row, and their bio.
func userSubtitle(o map[string]any) string {
	fans := ""
	if n := apiInt(o, "fans"); n > 0 {
		fans = strconv.Itoa(n) + "粉丝"
	}
	return joinNonEmpty("  ", fans, strings.TrimSpace(firstAPIString(o, "bio")))
}

func joinNonEmpty(sep string, parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

// productSubtitle is the score and the followers of a product row.
func productSubtitle(o map[string]any) string {
	var parts []string
	if score := firstAPIString(o, "star_average_score"); score != "" && score != "0" && score != "0.0" {
		parts = append(parts, score+"分")
	}
	if follows := firstAPIString(o, "follow_num_txt"); follows != "" && follows != "0" {
		parts = append(parts, follows+"人关注")
	}
	return strings.Join(parts, "  ")
}

func searchScore(o map[string]any, keys ...string) string {
	for _, key := range keys {
		value := firstAPIString(o, key)
		if score, err := strconv.ParseFloat(value, 64); err == nil && score > 0 {
			if score == 10 {
				return "10"
			}
			return value
		}
	}
	return ""
}

// productCount is the size of a product category, "" when it is not known.
func productCount(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n) + " 款"
}

// myList lists one of the signed-in account's own lists.
func (b *coolapkBackend) myList(ctx context.Context, q backend.PageQuery) (backend.PageEntriesResult, error) {
	uid := b.SessionUID()
	if uid == "" {
		return backend.PageEntriesResult{}, fmt.Errorf("请先登录")
	}
	key := "my:" + q.URL
	cursor := b.cursor(key, q.Page)
	r := coolapk.ListRequest{Page: q.Page, FirstItem: cursor.first, LastItem: cursor.last}
	var response *coolapk.RawResponse
	var err error
	switch {
	case strings.HasPrefix(q.URL, backend.CollectionURLPrefix):
		response, err = b.client.GetCollectionItemListWithCtx(ctx, strings.TrimPrefix(q.URL, backend.CollectionURLPrefix), r)
	case q.URL == backend.MyCollectionsURL:
		response, err = b.client.GetCollectionsWithCtx(ctx, coolapk.CollectionListRequest{UID: uid, Page: q.Page, FirstItem: cursor.first, LastItem: cursor.last})
	case q.URL == backend.MyBlocksURL:
		response, err = b.client.GetBlackListWithCtx(ctx, r)
	case q.URL == backend.MyLikesURL:
		response, err = b.client.GetUserLikeListWithCtx(ctx, uid, r)
	case q.URL == backend.MyRepliesURL:
		response, err = b.client.GetUserReplyListWithCtx(ctx, uid, r)
	default:
		response, err = b.client.GetHitHistoryWithCtx(ctx, coolapk.ListRequest{Page: q.Page, FirstItem: cursor.first, LastItem: cursor.last})
	}
	objects, err := listObjects(response, err)
	if err != nil {
		return backend.PageEntriesResult{}, err
	}
	b.storeCursor(key, q.Page, feedCursor(objects))
	return backend.PageEntriesResult{Entries: entriesFromObjects(objects), HasMore: len(objects) > 0}, nil
}

// replyAsFeed reads a comment as a card of the post it is on: its author,
// time and text, and the post's id.
func replyAsFeed(o map[string]any) backend.Feed {
	f := backend.Feed{ID: firstAPIString(o, "fid", "feedid"), Kind: backend.PostComment}
	f.Author = backend.Author{
		ID:        firstAPIString(o, "uid"),
		Name:      firstAPIString(o, "username"),
		AvatarURL: normalizeCoolapkImage(firstAPIString(o, "userAvatar")),
	}
	f.PublishedAt = apiTime(o, "dateline")
	f.Content, f.Rich = backend.RichTextFromAPI(firstAPIString(o, "message"))
	return f
}

// countText is a count the server gives, with its unit, or nothing for none.
func countText(n, unit string) string {
	if n == "" || n == "0" {
		return ""
	}
	return n + unit
}

// tradeBrands lists the kinds of second-hand goods, each leading to its
// posts, in the order the server sends them.
func (b *coolapkBackend) tradeBrands(ctx context.Context, page int) (backend.PageEntriesResult, error) {
	if page > 1 {
		return backend.PageEntriesResult{}, nil
	}
	response, err := b.client.GetErshouBrandsWithCtx(ctx)
	if err != nil {
		return backend.PageEntriesResult{}, err
	}
	// The kinds are an object keyed by their ids, whose order is the
	// server's: it is read from the JSON, as a map would lose it.
	var groups []struct {
		Type     string          `json:"type"`
		Entities json.RawMessage `json:"entities"`
	}
	if err := json.Unmarshal(response.Data, &groups); err != nil {
		return backend.PageEntriesResult{}, fmt.Errorf("解析二手分类: %w", err)
	}
	kinds := &backend.Card{Kind: backend.CardIcons}
	for _, g := range groups {
		if g.Type != "ershouType" {
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(g.Entities))
		if _, err := decoder.Token(); err != nil {
			return backend.PageEntriesResult{}, fmt.Errorf("解析二手分类: %w", err)
		}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return backend.PageEntriesResult{}, fmt.Errorf("解析二手分类: %w", err)
			}
			var kind struct{ Title, Logo string }
			if err := decoder.Decode(&kind); err != nil {
				return backend.PageEntriesResult{}, fmt.Errorf("解析二手分类: %w", err)
			}
			kinds.Items = append(kinds.Items, backend.CardItem{Title: kind.Title, Pic: normalizeCoolapkImage(kind.Logo),
				URL: "#/feed/ershouList?ershouType=" + fmt.Sprint(key) + "&dataListType=staggered"})
		}
	}
	if len(kinds.Items) == 0 {
		return backend.PageEntriesResult{}, nil
	}
	return backend.PageEntriesResult{Entries: []backend.PageEntry{{Kind: backend.EntryCard, ID: "card:trade-kinds", Card: kinds}}}, nil
}

// flexListURL reads the url a page's configCard names for its content list,
// which the clients fetch apart from the page's own cards.
func flexListURL(objects []map[string]any) string {
	for _, o := range objects {
		if firstAPIString(o, "entityTemplate") != "configCard" {
			continue
		}
		extra := apiObject(o, "extraDataArr")
		if extra == nil {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(firstAPIString(o, "extraData")), &parsed); err == nil {
				extra = parsed
			}
		}
		if extra == nil {
			continue
		}
		if firstAPIString(extra, "flexList") != "" {
			if u := firstAPIString(extra, "url"); u != "" {
				return u
			}
		}
	}
	return ""
}

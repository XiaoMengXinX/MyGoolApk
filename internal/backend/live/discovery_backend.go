package live

import (
	"context"
	"fmt"
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
	searchType := "user"
	if q.Kind == backend.SearchFeeds {
		searchType = "feed"
	}
	key := "search:" + searchType + ":" + q.Keyword
	cursor := b.cursor(key, q.Page)
	objects, err := listObjects(b.client.SearchWithCtx(ctx, coolapk.SearchRequest{
		SearchValue: q.Keyword,
		Type:        searchType,
		FeedType:    "all",
		Sort:        "default",
		Page:        q.Page,
		LastItem:    cursor.last,
	}))
	if err != nil {
		return backend.SearchPage{}, err
	}
	b.storeCursor(key, q.Page, feedCursor(objects))
	if q.Kind == backend.SearchFeeds {
		return backend.SearchPage{Feeds: feedsFromObjects(objects), HasMore: len(objects) > 0}, nil
	}
	users := make([]backend.Author, 0, len(objects))
	seen := make(map[string]bool)
	for _, object := range objects {
		if firstAPIString(object, "entityType") != "user" && firstAPIString(object, "entityType") != "contacts" {
			continue
		}
		author := authorFromObject(object)
		if author.ID != "" && !seen[author.ID] {
			seen[author.ID] = true
			users = append(users, author)
		}
	}
	return backend.SearchPage{Users: users, HasMore: len(objects) > 0}, nil
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
	}, nil
}

func (b *coolapkBackend) UserFeeds(ctx context.Context, uid string, page int) (backend.FeedPage, error) {
	key := "user-feed:" + uid
	cursor := b.cursor(key, page)
	objects, err := listObjects(b.client.GetUserFeedWithCtx(ctx, coolapk.UserListRequest{UID: uid, Page: page, LastItem: cursor.last}))
	if err != nil {
		return backend.FeedPage{}, err
	}
	b.storeCursor(key, page, feedCursor(objects))
	return backend.FeedPage{Feeds: feedsFromObjects(objects), HasMore: len(objects) > 0}, nil
}

func (b *coolapkBackend) UserConnections(ctx context.Context, uid string, kind backend.UserConnectionKind, page int) (backend.UserConnectionPage, error) {
	key := fmt.Sprintf("user-connections:%s:%d", uid, kind)
	cursor := b.cursor(key, page)
	request := coolapk.UserListRequest{UID: uid, Page: page, LastItem: cursor.last}
	var response *coolapk.RawResponse
	var err error
	if kind == backend.UserFans {
		response, err = b.client.GetFansListWithCtx(ctx, request)
	} else {
		response, err = b.client.GetFollowListWithCtx(ctx, request)
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
const topicHomeURL = "/page?url=V11_VERTICAL_TOPIC"

func (b *coolapkBackend) TopicCategories(ctx context.Context) ([]backend.TopicCategory, error) {
	objects, err := listObjects(b.client.GetPageDataListWithCtx(ctx, coolapk.PageDataListRequest{URL: topicHomeURL, Title: "话题", Page: 1}))
	if err != nil {
		return nil, err
	}
	var out []backend.TopicCategory
	for _, card := range objects {
		for _, e := range apiObjects(card, "entities") {
			title, url := firstAPIString(e, "title"), firstAPIString(e, "url")
			if title != "" && url != "" {
				out = append(out, backend.TopicCategory{Title: title, URL: url})
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("没有话题分类")
	}
	return out, nil
}

func (b *coolapkBackend) TopicList(ctx context.Context, category backend.TopicCategory, page int) (backend.TopicPage, error) {
	key := "topics:" + category.URL
	cursor := b.cursor(key, page)
	objects, err := listObjects(b.client.GetPageDataListWithCtx(ctx, coolapk.PageDataListRequest{
		URL: category.URL, Title: category.Title, Page: page, LastItem: cursor.last,
	}))
	if err != nil {
		return backend.TopicPage{}, err
	}
	var items []backend.TopicItem
	last := ""
	for _, o := range objects {
		// The first entry is a configuration card, not a topic.
		if firstAPIString(o, "entityType") != "topic" {
			continue
		}
		last = apiID(o)
		items = append(items, backend.TopicItem{
			ID:       last,
			Title:    firstAPIString(o, "title"),
			LogoURL:  normalizeCoolapkImage(firstAPIString(o, "logo")),
			Subtitle: firstAPIString(o, "subTitle"),
		})
	}
	b.storeCursor(key, page, apiCursor{last: last})
	return backend.TopicPage{Items: items, HasMore: len(items) > 0}, nil
}

func (b *coolapkBackend) PageEntries(ctx context.Context, q backend.PageQuery) (backend.PageEntriesResult, error) {
	switch q.URL {
	case backend.HeadlineURL, backend.FollowURL:
		kind := backend.FeedRecommend
		if q.URL == backend.FollowURL {
			kind = backend.FeedFollow
		}
		objects, err := b.feedObjects(ctx, backend.FeedQuery{Kind: kind, Page: q.Page})
		if err != nil {
			return backend.PageEntriesResult{}, err
		}
		return backend.PageEntriesResult{Entries: entriesFromObjects(objects), HasMore: len(objects) > 0}, nil
	case backend.MyLikesURL, backend.MyRepliesURL, backend.MyFootprintsURL:
		return b.myList(ctx, q)
	}
	key := "pageEntries:" + q.URL
	cursor := b.cursor(key, q.Page)
	var response *coolapk.RawResponse
	var err error
	if q.URL == backend.ProductCategoriesURL {
		response, err = b.client.GetProductCategoriesWithCtx(ctx)
	} else {
		response, err = b.client.GetPageDataListWithCtx(ctx, coolapk.PageDataListRequest{
			URL: q.URL, Title: q.Title, Page: q.Page, LastItem: cursor.last,
		})
	}
	objects, err := listObjects(response, err)
	if err != nil {
		return backend.PageEntriesResult{}, err
	}
	if q.URL != backend.ProductCategoriesURL {
		b.storeCursor(key, q.Page, feedCursor(objects))
	}
	return backend.PageEntriesResult{Entries: entriesFromObjects(objects), HasMore: len(objects) > 0 && q.URL != backend.ProductCategoriesURL}, nil
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
				entries = append(entries, backend.PageEntry{Kind: backend.EntryFeed, ID: f.ID, Feed: &f})
			}
			return
		case "product":
			if id := apiID(o); id != "" && !seen["product:"+id] {
				seen["product:"+id] = true
				entries = append(entries, backend.PageEntry{
					Kind: backend.EntryProduct, ID: id, Title: firstAPIString(o, "title"),
					LogoURL:  normalizeCoolapkImage(firstAPIString(o, "logo")),
					Subtitle: productSubtitle(o),
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
	switch q.URL {
	case backend.MyLikesURL:
		response, err = b.client.GetUserLikeListWithCtx(ctx, uid, r)
	case backend.MyRepliesURL:
		response, err = b.client.GetUserReplyListWithCtx(ctx, uid, r)
	default:
		response, err = b.client.GetHitHistoryWithCtx(ctx, q.Page, cursor.first, cursor.last)
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

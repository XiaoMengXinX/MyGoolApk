package live

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"app/internal/backend"
	coolapk "github.com/XiaoMengXinX/CoolapkApi-Go"
)

type coolapkBackend struct {
	client      *coolapk.Coolapk
	dir         string // where identity.json lives
	installTime int64

	mu          sync.Mutex
	cursors     map[string]apiCursor
	productTags map[string]string // product id -> the tag its feed is listed under
	// configs are the versions of the products shown, by product id: the
	// product's page tells them, and the card of its list shows them.
	configs map[string][]map[string]any
	// flexURLs are the content-list urls of the pages whose configCard
	// names one, by the page's url.
	flexURLs    map[string]string
	followURL   string
	followTitle string
}

// followFeedURL is the page of the follow tab's posts. The newer
// V15_HOME_TAB_FOLLOW page answers with its header cards only (friends with
// updates, the tab selector) and no posts.
const followFeedURL = coolapk.HomeTabFollowFeedURL

// followSource returns the url and title the follow tab's posts are asked
// for. As the iOS client does, the first page asks the V15 page for its
// tab selector (selectorLinkCard) and takes the entry the server marks as
// remembered, or the first, whose url and title the later pages repeat. A
// page without a selector, as a guest gets, lists the V9 page, which holds
// the posts of followed accounts.
func (b *coolapkBackend) followSource(ctx context.Context, page int) (url, title string) {
	b.mu.Lock()
	url, title = b.followURL, b.followTitle
	b.mu.Unlock()
	if page > 1 && url != "" {
		return url, title
	}
	url, title = followFeedURL, "关注"
	if response, err := b.client.GetPageDataListWithCtx(ctx, coolapk.PageDataListRequest{
		URL: coolapk.HomeTabFollowURL, Title: "关注", Page: 1,
	}); err == nil {
		if objects, err := decodeAPIObjects(response.Data); err == nil {
			if u, t := selectorEntry(objects); u != "" {
				url, title = u, t
			}
		}
	}
	b.mu.Lock()
	b.followURL, b.followTitle = url, title
	b.mu.Unlock()
	return url, title
}

// selectorEntry returns the url and title of the entry a selectorLinkCard
// marks as remembered, or of its first.
func selectorEntry(objects []map[string]any) (url, title string) {
	for _, card := range objects {
		if firstAPIString(card, "entityTemplate") != "selectorLinkCard" {
			continue
		}
		for i, entry := range apiObjects(card, "entities") {
			u := firstAPIString(entry, "url")
			remembered := firstAPIString(apiObject(entry, "extraData"), "rememberSelected") == "1"
			if u != "" && (i == 0 || remembered) {
				url, title = u, firstAPIString(entry, "title")
				if remembered {
					return url, title
				}
			}
		}
	}
	return url, title
}

type apiCursor struct {
	first string
	last  string
}

// New returns the backend that talks to the Coolapk API with the device and
// account saved in dir.
func New(dir string) (backend.Backend, error) {
	identity, err := loadCoolapkIdentity(dir)
	if err != nil {
		return nil, fmt.Errorf("初始化设备身份失败: %w", err)
	}
	client := coolapk.New()
	// Spaced like a person's taps; see MinRequestInterval.
	client.MinRequestInterval = 80 * time.Millisecond
	client.Device = identity.Device
	client.SetSession(identity.Session)
	return &coolapkBackend{
		client:      client,
		dir:         dir,
		installTime: identity.InstallTime,
		cursors:     make(map[string]apiCursor),
		productTags: make(map[string]string),
		configs:     make(map[string][]map[string]any),
		flexURLs:    make(map[string]string),
	}, nil
}

func (b *coolapkBackend) Feed(ctx context.Context, q backend.FeedQuery) (backend.FeedPage, error) {
	objects, err := b.feedObjects(ctx, q)
	if err != nil {
		return backend.FeedPage{}, err
	}
	return backend.FeedPage{Feeds: feedsFromObjects(objects), HasMore: len(objects) > 0}, nil
}

// feedObjects fetches the rows of a feed page and keeps its cursor.
func (b *coolapkBackend) feedObjects(ctx context.Context, q backend.FeedQuery) ([]map[string]any, error) {
	key := fmt.Sprintf("feed:%d:%s", q.Kind, q.Topic)
	cursor := b.cursor(key, q.Page)
	if q.Kind == backend.FeedRecommend {
		// A refresh also names the first and last post of the list it
		// replaces, and the server answers with posts it has not sent.
		cursor = b.lastCursor(key)
	}

	var response *coolapk.RawResponse
	var err error
	switch q.Kind {
	case backend.FeedRecommend:
		response, err = b.client.GetHomeFeedWithCtx(ctx, coolapk.HomeFeedRequest{
			Page: q.Page, FirstItem: cursor.first, LastItem: cursor.last,
		})
	case backend.FeedFollow:
		url, title := b.followSource(ctx, q.Page)
		response, err = b.client.GetPageDataListWithCtx(ctx, coolapk.PageDataListRequest{
			URL: url, Title: title, Page: q.Page, LastItem: cursor.last,
		})
	case backend.FeedHot:
		response, err = b.client.GetPageDataListWithCtx(ctx, coolapk.PageDataListRequest{
			URL: coolapk.HomeTabRankURL, Title: "热榜", Page: q.Page, LastItem: cursor.last,
		})
	case backend.FeedTopic:
		if strings.TrimSpace(q.Topic) == "" {
			return nil, fmt.Errorf("话题不能为空")
		}
		tag := q.Topic
		if q.ProductID != "" {
			if tag, err = b.productTag(ctx, q.ProductID, q.Topic); err != nil {
				return nil, err
			}
		}
		response, err = b.client.GetTopicFeedWithCtx(ctx, coolapk.TopicFeedRequest{
			Tag: tag, Page: q.Page, FirstItem: cursor.first, LastItem: cursor.last,
		})
	default:
		return nil, fmt.Errorf("未知帖子列表")
	}
	objects, err := listObjects(response, err)
	if err != nil {
		return nil, err
	}
	if q.Kind == backend.FeedRecommend {
		b.storeCursor(key, q.Page, feedEntityCursor(objects))
	} else {
		b.storeCursor(key, q.Page, feedCursor(objects))
	}
	return objects, nil
}

// productTag returns the tag a product's posts are listed under: tagFeedList
// finds none by the product's title, which holds spaces, but by its alias.
func (b *coolapkBackend) productTag(ctx context.Context, id, title string) (string, error) {
	b.mu.Lock()
	tag, ok := b.productTags[id]
	b.mu.Unlock()
	if ok {
		return tag, nil
	}
	if _, err := strconv.Atoi(id); err != nil {
		return "", fmt.Errorf("无效的产品编号")
	}
	response, err := b.client.GetProductDetailWithCtx(ctx, id)
	if err != nil {
		return "", err
	}
	object, err := decodeAPIObject(response.Data)
	if err != nil {
		return "", err
	}
	tag = firstAPIString(object, "alias_title", "index_title", "title")
	if tag == "" {
		tag = title
	}
	b.mu.Lock()
	b.productTags[id] = tag
	b.mu.Unlock()
	return tag, nil
}

func (b *coolapkBackend) FeedDetail(ctx context.Context, id string) (backend.Feed, error) {
	feedID, err := strconv.Atoi(id)
	if err != nil || feedID <= 0 {
		return backend.Feed{}, fmt.Errorf("无效的帖子编号")
	}
	response, err := b.client.GetFeedDetailRawWithCtx(ctx, feedID, 0)
	if err != nil {
		return backend.Feed{}, err
	}
	object, err := decodeAPIObject(response.Data)
	if err != nil {
		return backend.Feed{}, err
	}
	feed := feedFromObject(object)
	if feed.ID == "" {
		return backend.Feed{}, fmt.Errorf("帖子响应缺少编号")
	}
	return feed, nil
}

func (b *coolapkBackend) Replies(ctx context.Context, q backend.ReplyQuery) (backend.ReplyPage, error) {
	key := fmt.Sprintf("reply:%s:%s:%d:%d", q.FeedID, q.ParentID, q.Sort, q.Of)
	cursor := b.cursor(key, q.Page)
	id := q.FeedID
	if q.ParentID != "" {
		id = q.ParentID
	}
	numericID, err := strconv.Atoi(id)
	if err != nil || numericID <= 0 {
		return backend.ReplyPage{}, fmt.Errorf("无效的评论编号")
	}

	var response *coolapk.RawResponse
	entity := "feed_reply"
	list := coolapk.ListRequest{Page: q.Page, FirstItem: cursor.first, LastItem: cursor.last}
	switch {
	case q.ParentID != "":
		response, err = b.client.GetChildRepliesWithCtx(ctx, numericID, coolapk.ListRequest{
			Page: q.Page, FirstItem: cursor.first, LastItem: cursor.last,
		})
	case q.Of == backend.RepliesOfVote:
		entity = "feed"
		response, err = b.client.GetVoteCommentListWithCtx(ctx, id, "", list)
	case q.Of == backend.RepliesOfQuestion:
		entity = "feed"
		sort := map[backend.ReplySort]string{backend.RepliesLatest: "dateline", backend.RepliesHot: "like"}[q.Sort]
		if sort == "" {
			sort = "reply"
		}
		response, err = b.client.GetAnswerListWithCtx(ctx, id, sort, list)
	default:
		listType := map[backend.ReplySort]string{
			backend.RepliesRecent: "lastupdate_desc", backend.RepliesLatest: "dateline_desc", backend.RepliesHot: "popular",
		}[q.Sort]
		response, err = b.client.GetFeedRepliesWithCtx(ctx, numericID, coolapk.ListRequest{
			Page: q.Page, FirstItem: cursor.first, LastItem: cursor.last,
		}, listType, q.Sort == backend.RepliesAuthor)
	}
	objects, err := listObjects(response, err)
	if err != nil {
		return backend.ReplyPage{}, err
	}
	// A page can end with a card that is no comment.
	objects = slices.DeleteFunc(objects, func(o map[string]any) bool { return firstAPIString(o, "entityType") != entity })
	replies := make([]backend.Reply, 0, len(objects))
	for _, object := range objects {
		replies = append(replies, replyFromObject(object))
	}
	b.storeCursor(key, q.Page, feedCursor(objects))
	return backend.ReplyPage{Replies: replies, HasMore: len(objects) > 0}, nil
}

func (b *coolapkBackend) cursor(key string, page int) apiCursor {
	if page == 1 {
		return apiCursor{}
	}
	return b.lastCursor(key)
}

func (b *coolapkBackend) lastCursor(key string) apiCursor {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cursors[key]
}

func (b *coolapkBackend) storeCursor(key string, page int, cursor apiCursor) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cursors[key] = cursor
}

// listObjects decodes the list that a request answered with, or returns the
// error of the request.
func listObjects(response *coolapk.RawResponse, err error) ([]map[string]any, error) {
	if err != nil {
		return nil, err
	}
	return decodeAPIObjects(response.Data)
}

func decodeAPIObjects(data json.RawMessage) ([]map[string]any, error) {
	var objects []map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&objects); err != nil {
		return nil, fmt.Errorf("解析列表响应: %w", err)
	}
	return objects, nil
}

func decodeAPIObject(data json.RawMessage) (map[string]any, error) {
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return nil, fmt.Errorf("解析帖子响应: %w", err)
	}
	if object == nil {
		return nil, fmt.Errorf("帖子响应为空")
	}
	return object, nil
}

func feedsFromObjects(objects []map[string]any) []backend.Feed {
	feeds := make([]backend.Feed, 0, len(objects))
	seen := make(map[string]bool)
	var appendObject func(map[string]any)
	appendObject = func(object map[string]any) {
		if firstAPIString(object, "entityType") == "feed" {
			feed := feedFromObject(object)
			if feed.ID != "" && !seen[feed.ID] {
				seen[feed.ID] = true
				feeds = append(feeds, feed)
			}
		}
		for _, child := range apiObjects(object, "entities") {
			appendObject(child)
		}
	}
	for _, object := range objects {
		appendObject(object)
	}
	return feeds
}

func feedFromObject(object map[string]any) backend.Feed {
	action := apiObject(object, "userAction")
	content, rich := backend.RichTextFromAPI(firstAPIString(object, "message", "message_raw_output"))
	feed := backend.Feed{
		ID:          apiID(object),
		Title:       firstAPIString(object, "message_title"),
		Content:     content,
		Rich:        rich,
		IsArticle:   firstAPIString(object, "feedType") == "feedArticle" || apiInt(object, "is_html_article") == 1,
		Device:      firstAPIString(object, "device_title", "device_name"),
		Location:    firstAPIString(object, "ip_location", "location"),
		Target:      targetFromObject(object),
		Tags:        tagsFromAPI(firstAPIString(object, "tags")),
		PublishedAt: apiTime(object, "dateline"),
		Author:      authorFromObject(object),
		Stats: backend.Stats{
			Likes:    apiInt(object, "likenum"),
			Comments: apiInt(object, "commentnum", "replynum"),
			Forwards: apiInt(object, "forwardnum"),
		},
		Liked:     apiInt(action, "like") != 0,
		Favorited: apiInt(action, "collect") == 1 || apiInt(action, "favorite") == 1,
	}
	feed.Kind = postKindFromAPI(firstAPIString(object, "feedType"))
	if feed.Title == "" && feed.Kind != backend.PostText {
		feed.Title = firstAPIString(object, "title")
	}
	switch feed.Kind {
	case backend.PostArticle, backend.PostTrade:
		feed.Blocks = blocksFromObject(object)
		if feed.Kind == backend.PostTrade && !slices.ContainsFunc(feed.Blocks, func(b backend.Block) bool { return b.Kind == backend.BlockImage }) {
			feed.Images = imagesFromObject(object)
			break
		}
		if covers := imagesFromURLs(firstAPIString(object, "message_cover"), firstAPIString(object, "pic")); len(covers) > 0 {
			// pic only repeats the body's first picture for some articles,
			// whose detail draws it again right below the cover.
			cover := covers[0]
			dupe := false
			for _, block := range feed.Blocks {
				if block.Kind == backend.BlockImage && block.Image.URL == cover.URL {
					dupe = true
					break
				}
			}
			if !dupe {
				feed.Cover = &covers[0]
			}
		}
	case backend.PostVideo:
		// Its picture is the video's cover, read from media_info below.
	default:
		feed.Images = imagesFromObject(object)
	}
	feed.Answers, feed.Followers = apiInt(object, "question_answer_num"), apiInt(object, "question_follow_num")
	if feed.Kind == backend.PostRating {
		// A rating's title is made of its author's name ("某某的点评"),
		// which the official client leaves out.
		feed.Title, feed.Rating = "", ratingFromObject(object)
	}
	if trade := apiObject(object, "ershou_info"); trade != nil {
		value := firstAPIString(trade, "product_price")
		if amount, err := strconv.ParseFloat(value, 64); err == nil && amount > 0 {
			feed.Price = "¥" + value
		} else if value == "价格面议" {
			feed.Price = value
		}
	}
	if link := firstAPIString(object, "extra_url"); link != "" {
		feed.Link = &backend.Link{
			URL: link, Title: firstAPIString(object, "extra_title"),
			Subtitle: firstAPIString(object, "extra_info"), CoverURL: normalizeCoolapkImage(firstAPIString(object, "extra_pic")),
		}
	}
	if feed.Kind == backend.PostVideo || apiInt(object, "media_type") == 2 {
		feed.Video = videoFromObject(object)
	}
	if vote := apiObject(object, "vote"); vote != nil {
		feed.Vote = voteFromObject(vote)
	}
	if forward := apiObject(object, "forwardSourceFeed"); forward != nil {
		forwardContent, forwardRich := backend.RichTextFromAPI(firstAPIString(forward, "message"))
		feed.Forward = &backend.Forward{
			Author: authorFromObject(forward), Content: forwardContent, Rich: forwardRich,
			Images: imagesFromObject(forward),
		}
	}
	for _, top := range apiObjects(object, "topReplyRows") {
		reply := replyFromObject(top)
		reply.Pinned = true
		feed.Pinned = append(feed.Pinned, reply)
	}
	return feed
}

// blocksFromObject reads an article's body: message_raw_output is a JSON
// list of blocks, each a paragraph (type text, message), a picture (image,
// url, description) or a link card (shareUrl, url, title, subTitle, logo).
func blocksFromObject(object map[string]any) []backend.Block {
	raw := firstAPIString(object, "message_raw_output")
	if raw == "" {
		return nil
	}
	var items []struct {
		Type        string `json:"type"`
		Message     string `json:"message"`
		URL         string `json:"url"`
		Description string `json:"description"`
		Title       string `json:"title"`
		SubTitle    string `json:"subTitle"`
		Logo        string `json:"logo"`
	}
	if json.Unmarshal([]byte(raw), &items) != nil {
		return nil
	}
	var blocks []backend.Block
	for _, item := range items {
		switch item.Type {
		case "text":
			// <!--break--> marks where the summary ends.
			text, rich := backend.RichTextFromAPI(strings.ReplaceAll(item.Message, "<!--break-->", ""))
			if text != "" {
				blocks = append(blocks, backend.Block{Kind: backend.BlockText, Text: text, Rich: rich})
			}
		case "image":
			if images := imagesFromURLs(item.URL); len(images) > 0 {
				blocks = append(blocks, backend.Block{Kind: backend.BlockImage, Image: images[0], Caption: strings.TrimSpace(item.Description)})
			}
		case "shareUrl":
			if item.URL != "" {
				blocks = append(blocks, backend.Block{Kind: backend.BlockLink, Link: &backend.Link{
					URL: item.URL, Title: item.Title, Subtitle: item.SubTitle, CoverURL: normalizeCoolapkImage(item.Logo),
				}})
			}
		}
	}
	return blocks
}

// postKindFromAPI maps the feedType of a post.
func postKindFromAPI(feedType string) backend.PostKind {
	switch feedType {
	case "feedArticle":
		return backend.PostArticle
	case "picture":
		return backend.PostPicture
	case "video":
		return backend.PostVideo
	case "question":
		return backend.PostQuestion
	case "answer":
		return backend.PostAnswer
	case "vote":
		return backend.PostVote
	case "trade", "ershou":
		return backend.PostTrade
	case "rating":
		return backend.PostRating
	}
	return backend.PostText
}

// videoFromObject reads a video post: media_info is JSON of the cover, the
// length in milliseconds and the name, and media_url the page it plays on.
func videoFromObject(object map[string]any) *backend.Video {
	v := &backend.Video{URL: firstAPIString(object, "media_url"), CoverURL: normalizeCoolapkImage(firstAPIString(object, "media_pic"))}
	var info struct {
		Name     string      `json:"name"`
		Cover    string      `json:"cover"`
		Duration json.Number `json:"duration"`
	}
	if raw := firstAPIString(object, "media_info"); raw != "" && json.Unmarshal([]byte(raw), &info) == nil {
		if v.CoverURL == "" {
			v.CoverURL = info.Cover
		}
		if ms, err := info.Duration.Int64(); err == nil {
			v.Duration = time.Duration(ms) * time.Millisecond
		}
		v.Title = info.Name
	}
	if v.CoverURL == "" {
		v.CoverURL = normalizeCoolapkImage(firstAPIString(object, "pic"))
	}
	return v
}

// imagesFromURLs makes the pictures of urls, skipping empty ones and
// repeats.
func imagesFromURLs(urls ...string) []backend.Image {
	var images []backend.Image
	seen := make(map[string]bool, len(urls))
	for _, value := range urls {
		if value = normalizeCoolapkImage(value); value == "" || seen[value] {
			continue
		}
		seen[value] = true
		image := backend.Image{URL: value}
		if match := imageSizePattern.FindStringSubmatch(value); len(match) == 3 {
			image.Width, _ = strconv.Atoi(match[1])
			image.Height, _ = strconv.Atoi(match[2])
		}
		images = append(images, image)
	}
	return images
}

func replyFromObject(object map[string]any) backend.Reply {
	content, rich := backend.RichTextFromAPI(firstAPIString(object, "message"))
	reply := backend.Reply{
		ID:            apiID(object),
		Author:        authorFromObject(object),
		Content:       content,
		Rich:          rich,
		CreatedAt:     apiTime(object, "dateline"),
		ReplyToName:   firstAPIString(object, "rusername"),
		ReplyToID:     firstAPIString(object, "ruid"),
		Images:        imagesFromObject(object),
		Likes:         apiInt(object, "likenum"),
		Liked:         apiInt(apiObject(object, "userAction"), "like") != 0,
		IsAuthor:      apiInt(object, "isFeedAuthor") == 1,
		ChildrenTotal: apiInt(object, "replynum"),
	}
	reply.Post = firstAPIString(object, "entityType") == "feed"
	if firstAPIString(object, "feedType") == "voteComment" {
		reply.Stance = firstAPIString(object, "extra_title")
	}
	for _, child := range apiObjects(object, "replyRows") {
		reply.Children = append(reply.Children, replyFromObject(child))
	}
	reply.ChildrenHasMore = apiInt(object, "replyRowsMore") > 0 || reply.ChildrenTotal > len(reply.Children)
	return reply
}

func authorFromObject(object map[string]any) backend.Author {
	user := apiObject(object, "userInfo")
	if user == nil {
		user = object
	}
	return backend.Author{
		ID:        firstAPIString(user, "uid", "entityId"),
		Name:      firstAPIString(user, "displayUsername", "username"),
		AvatarURL: normalizeCoolapkImage(firstAPIString(user, "userAvatar", "userSmallAvatar")),
		Verified:  apiInt(user, "verify_status") > 0,
	}
}

var imageSizePattern = regexp.MustCompile(`@(\d+)x(\d+)`)

func imagesFromObject(object map[string]any) []backend.Image {
	values := apiStrings(object, "picArr")
	if len(values) == 0 {
		for _, key := range []string{"pic", "message_cover", "cover"} {
			if value := firstAPIString(object, key); value != "" {
				values = append(values, value)
				break
			}
		}
	}
	return imagesFromURLs(values...)
}

func feedCursor(objects []map[string]any) apiCursor {
	if len(objects) == 0 {
		return apiCursor{}
	}
	return apiCursor{first: apiID(objects[0]), last: apiID(objects[len(objects)-1])}
}

// feedEntityCursor is the cursor of the first and last post of a page that
// mixes posts with cards, which hold no post id of their own.
func feedEntityCursor(objects []map[string]any) apiCursor {
	var cursor apiCursor
	var visit func(map[string]any)
	visit = func(object map[string]any) {
		if firstAPIString(object, "entityType") == "feed" {
			if id := apiID(object); id != "" {
				if cursor.first == "" {
					cursor.first = id
				}
				cursor.last = id
			}
		}
		for _, child := range apiObjects(object, "entities") {
			visit(child)
		}
	}
	for _, object := range objects {
		visit(object)
	}
	return cursor
}

func apiID(object map[string]any) string {
	for _, key := range []string{"id", "fid", "entityId"} {
		if value := firstAPIString(object, key); value != "" && value != "0" {
			return value
		}
	}
	return ""
}

func apiObject(object map[string]any, key string) map[string]any {
	value, _ := object[key].(map[string]any)
	return value
}

func apiObjects(object map[string]any, key string) []map[string]any {
	array, _ := object[key].([]any)
	objects := make([]map[string]any, 0, len(array))
	for _, value := range array {
		if child, ok := value.(map[string]any); ok {
			objects = append(objects, child)
		}
	}
	return objects
}

func apiStrings(object map[string]any, key string) []string {
	array, _ := object[key].([]any)
	values := make([]string, 0, len(array))
	for _, item := range array {
		if value, ok := item.(string); ok && value != "" {
			values = append(values, value)
		}
	}
	return values
}

func firstAPIString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			switch value := value.(type) {
			case string:
				if value != "" {
					return value
				}
			case json.Number:
				return value.String()
			case float64:
				return strconv.FormatFloat(value, 'f', -1, 64)
			}
		}
	}
	return ""
}

func apiInt(object map[string]any, keys ...string) int {
	for _, key := range keys {
		value := firstAPIString(object, key)
		if value == "" {
			continue
		}
		if number, err := strconv.ParseFloat(value, 64); err == nil {
			return int(number)
		}
	}
	return 0
}

func apiTime(object map[string]any, key string) time.Time {
	seconds := int64(apiInt(object, key))
	if seconds <= 0 {
		return time.Time{}
	}
	return time.Unix(seconds, 0)
}

func cleanAPIText(raw string) string {
	text, _ := backend.RichTextFromAPI(raw)
	return text
}

// targetFromObject reads the topic or product a post is filed under: the
// targetRow with its logo, else the bare ttitle.
func targetFromObject(object map[string]any) *backend.Target {
	if row := apiObject(object, "targetRow"); row != nil {
		if title := firstAPIString(row, "title"); title != "" {
			t := &backend.Target{
				Title:   title,
				LogoURL: normalizeCoolapkImage(firstAPIString(row, "logo")),
				Product: firstAPIString(row, "targetType") == "product",
			}
			if t.Product {
				t.ID = apiID(row)
			}
			return t
		}
	}
	if title := firstAPIString(object, "ttitle"); title != "" {
		return &backend.Target{Title: title}
	}
	return nil
}

// tagsFromAPI splits "#a#,#b#" into topic names.
func tagsFromAPI(raw string) []string {
	var tags []string
	for _, part := range strings.Split(raw, ",") {
		if name := strings.Trim(strings.TrimSpace(part), "#"); name != "" {
			tags = append(tags, name)
		}
	}
	return tags
}

// voteFromObject reads the vote of a post.
func voteFromObject(vote map[string]any) *backend.Vote {
	v := &backend.Vote{
		Title:  firstAPIString(vote, "message_title"),
		EndsAt: apiTime(vote, "end_time"), Voters: apiInt(vote, "total_vote_num"),
		MaxSelect: max(apiInt(vote, "max_select_num"), 1),
	}
	v.Expired = !v.EndsAt.IsZero() && v.EndsAt.Before(time.Now())
	// The counts are told once the account has voted or the vote is over:
	// before that the detail sends -999, and a list sends none.
	options := apiObjects(vote, "options")
	v.Counted = len(options) > 0
	// user_selected_option names the options the account voted for.
	chosen := strings.Split(firstAPIString(vote, "user_selected_option"), ",")
	for _, option := range options {
		o := backend.VoteOption{ID: apiID(option), Text: firstAPIString(option, "title")}
		o.Chosen = slices.Contains(chosen, o.ID)
		if _, ok := option["total_select_num"]; ok && apiInt(option, "total_select_num") >= 0 {
			o.Votes = apiInt(option, "total_select_num")
		} else {
			v.Counted = false
		}
		if c, err := strconv.ParseUint(strings.TrimPrefix(firstAPIString(option, "color"), "#"), 16, 32); err == nil {
			o.Color = uint32(c)
		}
		v.Options = append(v.Options, o)
	}
	v.Versus = apiInt(vote, "type") == 0 && len(v.Options) == 2
	return v
}

// ratingWords name a rating's stars, one to five, as the official client
// words them.
var ratingWords = []string{"很差", "较差", "一般", "不错", "非常好"}

// ratingFromObject reads a rating post: its stars (star, else rating_score
// out of ten), the sides it scores out of ten with words for their stars
// (rating_item_info), and its text, a JSON object of sections whose order
// v4_rating_message_key gives.
func ratingFromObject(object map[string]any) *backend.FeedRating {
	stars := apiInt(object, "star")
	if stars <= 0 {
		stars = (apiInt(object, "rating_score") + 1) / 2
	}
	r := &backend.FeedRating{Stars: min(stars, 5), Owner: apiInt(object, "is_owner") == 1}
	if r.Stars > 0 {
		r.Word = ratingWords[r.Stars-1]
	}
	for _, item := range apiObjects(object, "rating_item_info") {
		side := backend.RatedSide{Name: firstAPIString(item, "name"), Stars: min((apiInt(item, "v4_score")+1)/2, 5)}
		if words := apiStrings(item, "star_desc"); side.Stars > 0 && side.Stars <= len(words) {
			side.Word = words[side.Stars-1]
		}
		if side.Name != "" && side.Stars > 0 {
			r.Sides = append(r.Sides, side)
		}
	}
	var texts map[string]string
	var order []string
	_ = json.Unmarshal([]byte(firstAPIString(object, "v4_rating_message")), &texts)
	_ = json.Unmarshal([]byte(firstAPIString(object, "v4_rating_message_key")), &order)
	for _, title := range order {
		if text := strings.TrimSpace(texts[title]); text != "" {
			plain, rich := backend.RichTextFromAPI(text)
			r.Sections = append(r.Sections, backend.RatingSection{Title: title, Text: plain, Rich: rich})
		}
	}
	return r
}

package backend

import (
	"context"
	"errors"
	"net/url"
)

// Backend is the data source the native UI reads. The Coolapk adapter
// implements it; the UI never performs HTTP requests itself.
//
// Pages are 1-based. Backends must be safe for concurrent calls. Text
// fields (Feed.Content, Reply.Content, Link.Title and so on) are plain
// text: HTML markup is shown literally.
type Backend interface {
	// Feed returns one page of a home, headline or topic list.
	Feed(ctx context.Context, q FeedQuery) (FeedPage, error)
	// Event returns the details and discussion tabs of an activity.
	Event(ctx context.Context, id string) (EventDetail, error)
	// FeedDetail returns one post.
	FeedDetail(ctx context.Context, id string) (Feed, error)
	// FeedSummary returns the AI summary coolapk1s.com writes of a post,
	// passing its pieces to delta as they arrive. ErrNoSummary means the
	// post has none, or the service cannot write one now.
	FeedSummary(ctx context.Context, feedID string, delta func(string)) (FeedSummary, error)
	// Replies returns comments of a post, or of one comment when
	// ReplyQuery.ParentID is set, in the order ReplyQuery.Sort names.
	Replies(ctx context.Context, q ReplyQuery) (ReplyPage, error)
	// Search returns global feed or user results. An empty keyword loads hot searches.
	Search(ctx context.Context, q SearchQuery) (SearchPage, error)
	// SearchSuggestions returns matching search keywords.
	SearchSuggestions(ctx context.Context, keyword string) ([]string, error)
	// User returns a public user profile.
	User(ctx context.Context, uid string) (UserProfile, error)
	// CastVote votes for options of the vote of a post.
	CastVote(ctx context.Context, feedID string, optionIDs []string) error
	// UserFeeds returns posts of kind from one public user.
	UserFeeds(ctx context.Context, uid string, kind UserFeedKind, page int) (FeedPage, error)
	// UserConnections returns followed accounts or followers.
	UserConnections(ctx context.Context, uid string, kind UserConnectionKind, page int) (UserConnectionPage, error)

	// PageEntries returns one page of a server list: the posts, products and
	// product categories in it; cards that are only decoration are left out.
	PageEntries(ctx context.Context, q PageQuery) (PageEntriesResult, error)
	// TopicCategories returns the groups of the topic page.
	TopicCategories(ctx context.Context) ([]TopicCategory, error)
	// TopicList returns one page of a category's topics.
	TopicList(ctx context.Context, category TopicCategory, page int, cursor ListCursor) (TopicPage, error)

	// SessionUID returns the signed-in uid, or "" for a guest. It reads
	// local state only.
	SessionUID() string
	// UserAgent is the app identity the web login page sees.
	UserAgent() string
	// ImportSession validates the credentials the web login set, requests
	// CDID and persists the session.
	ImportSession(ctx context.Context, uid, username, token string) error
	// Logout clears the account's credentials; the device stays as it is.
	Logout() error
	// Device returns the identity requests carry now.
	Device() DeviceInfo
	// SetDeviceProfile changes the model fields; a blank one is kept.
	SetDeviceProfile(model, hardwareID, salesModel, systemVersion string) error
	// RegenerateDeviceProfile picks another iPhone model, keeping the ID.
	RegenerateDeviceProfile() error
	// ResetDeviceID makes the installation a new device, and for a signed-in
	// account asks the device service for its CDID.
	ResetDeviceID(ctx context.Context) error
	// UnreadCounts returns message-center badges.
	UnreadCounts(ctx context.Context) (UnreadCounts, error)
	// Notifications returns one page of message-center entries.
	Notifications(ctx context.Context, kind NoticeKind, page int) (NotificationPage, error)
	// MarkNotificationsRead clears the account's unread counts.
	MarkNotificationsRead(ctx context.Context) error
	// ClearNotifications deletes the entries of the main notification list.
	ClearNotifications(ctx context.Context) error
	// NoticeSenders returns the senders of service notices.
	NoticeSenders(ctx context.Context) ([]NoticeSender, error)
	// ServiceNotices returns one page of a sender's notices, by its kind.
	ServiceNotices(ctx context.Context, sender string, page int) (NotificationPage, error)
	// Topic returns the page of a topic by name, or of a product by id.
	Topic(ctx context.Context, name, productID string) (TopicDetail, error)
	TopicRatingTrends(ctx context.Context, id string) (RatingTrends, error)
	TopicNodeRatings(ctx context.Context, q NodeRatingQuery) (NodeRatingPage, error)
	// SetTopicFollow follows or leaves the topic, or the product when id is set.
	SetTopicFollow(ctx context.Context, name, productID string, follow bool) error
	// Conversations returns one page of private-message threads.
	Conversations(ctx context.Context, page int) (ConversationPage, error)
	// Chat returns one page of the thread ukey names, newest first, and
	// marks the thread read with its first page.
	Chat(ctx context.Context, ukey string, page int) (ChatPage, error)
	// SendMessage sends text to the account uid.
	SendMessage(ctx context.Context, uid, text string) error
	// SetFeedLike likes or unlikes a feed and returns its like count.
	SetFeedLike(ctx context.Context, id string, liked bool) (int, error)
	// SetReplyLike likes or unlikes a reply and returns its like count.
	SetReplyLike(ctx context.Context, id string, liked bool) (int, error)
	// SetUserFollow follows or unfollows a user.
	SetUserFollow(ctx context.Context, uid string, follow bool) error
	// Collections lists favorite folders and whether feedID is in each.
	Collections(ctx context.Context, feedID string, page int) (CollectionPage, error)
	// SetCollection adds a feed to a favorite folder or removes it.
	SetCollection(ctx context.Context, collectionID, feedID string, add bool) error
	// DeleteReply removes a comment or a nested reply of the account's.
	DeleteReply(ctx context.Context, id string) error
	// CreateReply publishes a comment or a nested reply.
	CreateReply(ctx context.Context, draft ReplyDraft) (Reply, error)
	// CreateFeed publishes a text feed and returns its ID.
	CreateFeed(ctx context.Context, draft FeedDraft) (string, error)
}

// ErrNoSummary is FeedSummary's error when the post has no AI summary:
// it is too short for one, or the service cannot write one now.
var ErrNoSummary = errors.New("backend: no summary for this post")

// FeedSummary is the AI summary coolapk1s.com writes of a post: its text
// and the model that wrote it.
type FeedSummary struct {
	Text  string
	Model string
}

// TopicCategory is a group of topics on the topic page: its title and the
// page url its topics are listed under.
type TopicCategory struct {
	Title   string
	URL     string
	Default bool
}

// TopicItem is one topic in a category.
type TopicItem struct {
	ID       string
	Title    string
	LogoURL  string
	Subtitle string // such as "93.9万热度"
}

// TopicPage is one page of a category's topics.
type TopicPage struct {
	Items   []TopicItem
	HasMore bool
	Cursor  ListCursor
}

// ProductCategoriesURL is the PageQuery.URL of the product categories, which
// is no page url but an endpoint of its own.
const ProductCategoriesURL = "/product/categoryList"

const (
	DigitalHomeURL  = "/page?url=V10_DIGITAL_HOME"
	DigitalPhoneURL = "/page?url=V10_CHANNEL_SJB"
	DigitalRankURL  = "/page?url=V10_CHANNEL_SMB_TOP"
)

// TradeBrandsURL is the PageQuery.URL of the kinds of second-hand goods, an
// endpoint of its own as the product categories are.
const TradeBrandsURL = "/ershou/ershouProductBrandList"

// TradeCityURL is the route of the second-hand posts of one city.
const TradeCityURL = "/ershou/location"

// TradeListURL is the page url of the second-hand posts of a product, a city
// or both, each "" for any.
func TradeListURL(product, city string) string {
	return "#/feed/ershouList?brand=&productId=" + url.QueryEscape(product) + "&cityId=" + url.QueryEscape(city) + "&ershouType=&dataListType=staggered"
}

// HeadlineURL and FollowURL are the PageQuery.URL of the home's headline and
// follow tabs, which are no page urls: the headline has an endpoint of its
// own, and the follow tab's url depends on the account.
const (
	HeadlineURL = "headline"
	FollowURL   = "/page?url=V15_HOME_TAB_FOLLOW"
)

// The signed-in account's own lists. The my: ones are no page urls: what it
// liked, its comments, the posts it opened on any device, the accounts it
// blocks and its collections.
const (
	MyLikesURL       = "my:likes"
	MyRepliesURL     = "my:replies"
	MyFootprintsURL  = "my:footprints"
	MyBlocksURL      = "my:blocks"
	MyCollectionsURL = "my:collections"
	// MyTopicsURL and MyProductsURL are server lists of what the account follows.
	MyTopicsURL   = "#/topic/userFollowTagList"
	MyProductsURL = "#/product/followProductList"
	// CollectionURLPrefix leads the url of what a collection holds, before its id.
	CollectionURLPrefix = "collection:"
	ChannelURLPrefix    = "channel:"
)

// PageQuery names a page of a server list: the url the clients ask
// /v6/page/dataList for, or ProductCategoriesURL.
type PageQuery struct {
	URL    string
	Title  string
	Page   int
	Cursor ListCursor
	// ContentURL is the separate list named by a page's configCard.
	ContentURL string
}

// ListCursor identifies the first and last scrollable entities already loaded.
type ListCursor struct {
	FirstItem string
	LastItem  string
}

// PageEntriesResult is one page of a server list.
type PageEntriesResult struct {
	Entries    []PageEntry
	HasMore    bool
	Cursor     ListCursor
	ContentURL string
	// Title is the server page's configured title, when provided.
	Title string
}

// FeedQuery names a feed page.
type FeedQuery struct {
	Kind  FeedKind
	Topic string // required when Kind is FeedTopic
	// ProductID is set when Topic names a product: its feed is listed under
	// the product's alias, not its title.
	ProductID string
	Page      int
}

// FeedPage is one page of Feed cards.
type FeedPage struct {
	Feeds   []Feed
	HasMore bool
}

// ReplySort is the order of a comment page.
type ReplySort int

const (
	// RepliesRecent is the default: the comments answered last first.
	RepliesRecent ReplySort = iota
	RepliesLatest
	RepliesHot
	// RepliesAuthor lists only the comments of the post's author.
	RepliesAuthor
)

// ReplyOf is what a post lists under it.
type ReplyOf int

const (
	// RepliesOfPost are comments.
	RepliesOfPost ReplyOf = iota
	// RepliesOfVote are the opinions of a vote, each a post of its own that
	// stands for an option.
	RepliesOfVote
	// RepliesOfQuestion are the answers to a question, each a post of its own.
	RepliesOfQuestion
)

// ReplyQuery names a comment page. ParentID is empty for top-level
// comments, and names a comment whose nested replies are wanted otherwise.
// Of says what the post keeps as its comments.
type ReplyQuery struct {
	FeedID   string
	ParentID string
	Of       ReplyOf
	Sort     ReplySort
	Page     int
}

// ReplyPage is one page of comments.
type ReplyPage struct {
	Replies []Reply
	HasMore bool
}

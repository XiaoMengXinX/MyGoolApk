package backend

import "context"

// Backend is the data source the native UI reads. The Coolapk adapter
// implements it; the UI never performs HTTP requests itself.
//
// Pages are 1-based. Backends must be safe for concurrent calls. Text
// fields (Feed.Content, Reply.Content, Link.Title and so on) are plain
// text: HTML markup is shown literally.
type Backend interface {
	// Feed returns one page of a home, headline or topic list.
	Feed(ctx context.Context, q FeedQuery) (FeedPage, error)
	// FeedDetail returns one post.
	FeedDetail(ctx context.Context, id string) (Feed, error)
	// Replies returns comments of a post, or of one comment when
	// ReplyQuery.ParentID is set, in the order ReplyQuery.Sort names.
	Replies(ctx context.Context, q ReplyQuery) (ReplyPage, error)
	// Search returns global feed or user results. An empty keyword loads hot searches.
	Search(ctx context.Context, q SearchQuery) (SearchPage, error)
	// SearchSuggestions returns matching search keywords.
	SearchSuggestions(ctx context.Context, keyword string) ([]string, error)
	// User returns a public user profile.
	User(ctx context.Context, uid string) (UserProfile, error)
	// UserFeeds returns posts from one public user.
	UserFeeds(ctx context.Context, uid string, page int) (FeedPage, error)
	// UserConnections returns followed accounts or followers.
	UserConnections(ctx context.Context, uid string, kind UserConnectionKind, page int) (UserConnectionPage, error)

	// PageEntries returns one page of a server list: the posts, products and
	// product categories in it; cards that are only decoration are left out.
	PageEntries(ctx context.Context, q PageQuery) (PageEntriesResult, error)
	// TopicCategories returns the groups of the topic page.
	TopicCategories(ctx context.Context) ([]TopicCategory, error)
	// TopicList returns one page of a category's topics.
	TopicList(ctx context.Context, category TopicCategory, page int) (TopicPage, error)

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
	Notifications(ctx context.Context, page int) (NotificationPage, error)
	// Conversations returns one page of private-message threads.
	Conversations(ctx context.Context, page int) (ConversationPage, error)
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
	// CreateReply publishes a comment or a nested reply.
	CreateReply(ctx context.Context, draft ReplyDraft) (Reply, error)
	// CreateFeed publishes a text feed and returns its ID.
	CreateFeed(ctx context.Context, draft FeedDraft) (string, error)
}

// TopicCategory is a group of topics on the topic page: its title and the
// page url its topics are listed under.
type TopicCategory struct {
	Title string
	URL   string
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
}

// ProductCategoriesURL is the PageQuery.URL of the product categories, which
// is no page url but an endpoint of its own.
const ProductCategoriesURL = "/product/categoryList"

// HeadlineURL and FollowURL are the PageQuery.URL of the home's headline and
// follow tabs, which are no page urls: the headline has an endpoint of its
// own, and the follow tab's url depends on the account.
const (
	HeadlineURL = "headline"
	FollowURL   = "follow"
)

// PageQuery names a page of a server list: the url the clients ask
// /v6/page/dataList for, or ProductCategoriesURL.
type PageQuery struct {
	URL   string
	Title string
	Page  int
}

// PageEntriesResult is one page of a server list.
type PageEntriesResult struct {
	Entries []PageEntry
	HasMore bool
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
	RepliesHot ReplySort = iota
	RepliesLatest
)

// ReplyQuery names a comment page. ParentID is empty for top-level
// comments, and names a comment whose nested replies are wanted otherwise.
type ReplyQuery struct {
	FeedID   string
	ParentID string
	Sort     ReplySort
	Page     int
}

// ReplyPage is one page of comments.
type ReplyPage struct {
	Replies []Reply
	HasMore bool
}

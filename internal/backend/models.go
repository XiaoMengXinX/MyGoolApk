package backend

import "time"

// FeedKind names the source of a feed page.
type FeedKind int

const (
	// FeedFollow is the feed of accounts the user follows. A backend
	// without credentials returns an empty page rather than an error.
	FeedFollow FeedKind = iota
	// FeedRecommend is the mixed feed of the headline tab.
	FeedRecommend
	// FeedHot is the home feed ranked by activity.
	FeedHot
	// FeedTopic is the feed of one topic, named by FeedQuery.Topic.
	FeedTopic
)

// Author is the account that wrote a feed or a reply.
type Author struct {
	ID        string
	Name      string
	AvatarURL string
	Verified  bool
}

// Image is one picture of a feed, reply or repost.
type Image struct {
	URL    string
	Width  int
	Height int
}

// Video is the cover of a video feed. The native client shows its cover
// and title; it does not play the stream.
type Video struct {
	CoverURL string
	Title    string
	Duration time.Duration
	// URL is the page the video plays on, as the poster linked it: the
	// client has no player for these sources, so it opens the page.
	URL string
}

// Link is a page a feed links to.
type Link struct {
	Title    string
	Subtitle string
	URL      string
	CoverURL string
}

// Vote is a poll attached to a feed. It is shown read-only.
type Vote struct {
	Title   string
	Options []VoteOption
	Expired bool
}

// VoteOption is one choice of a Vote.
type VoteOption struct {
	Text  string
	Votes int
}

// RichSpanKind names one kind of inline rich text.
type RichSpanKind int

const (
	// SpanText is plain text.
	SpanText RichSpanKind = iota
	// SpanTopic is a topic tag, shown as "#topic#".
	SpanTopic
	// SpanUser is a mention of an account.
	SpanUser
	// SpanLink is a link to a page inside or outside Coolapk.
	SpanLink
	// SpanEmoji is one Coolapk emoji, named by its asset.
	SpanEmoji
)

// RichSpan is one inline piece of a message. Text is always the visible
// text, so joining every span reproduces the plain message; URL is the
// target of a topic, mention or link, and Name the asset of an emoji.
type RichSpan struct {
	Kind RichSpanKind
	Text string
	URL  string
	Name string
}

// Forward is the post a feed reposts.
type Forward struct {
	Author  Author
	Content string
	Rich    []RichSpan
	Images  []Image
}

// Stats are the interaction counts of a feed.
type Stats struct {
	Likes    int
	Comments int
	Forwards int
}

// Target is the topic or product a post is filed under, shown as the chip
// at the bottom left of a card. Its title opens the feed of that name.
type Target struct {
	ID      string // of a product, which has its own feed key
	Title   string
	LogoURL string
	Product bool
}

// EntryKind is what a row of a server page (the tabs of the home, digital and
// discovery pages, and the lists they open) shows.
type EntryKind int

const (
	EntryFeed EntryKind = iota
	EntryProduct
	// EntryCategory is a group of products, which opens its own list.
	EntryCategory
	// EntryCard is one of the cards a server page leads with: a banner, a
	// row of entries, a selector of what the list below shows.
	EntryCard
)

// CardKind is how a card of a server page shows.
type CardKind int

const (
	// CardBanner is pictures that page sideways (imageCarouselCard_1,
	// imageScaleCard).
	CardBanner CardKind = iota
	// CardIcons is a grid of icons with their names (iconLinkGridCard,
	// iconGridCard, iconLongTitleGridCard).
	CardIcons
	// CardChips is a row of chips that scrolls (iconMiniScrollCard,
	// capsuleListCard).
	CardChips
	// CardList is a few rows under a title (iconListCard,
	// productTimelineListCard, iconMiniGridCard).
	CardList
	// CardScroll is pictures with titles that scroll sideways
	// (imageTextScrollCard, imageSquareScrollCard, feedScrollCard).
	CardScroll
	// CardTitle heads the rows after it (titleCard).
	CardTitle
	// CardSelector chooses what the list below it shows (selectorLinkCard):
	// each item's URL is a page url of its own.
	CardSelector
)

// Card is a card of a server page.
type Card struct {
	Kind  CardKind
	Title string
	Items []CardItem
}

// CardItem is one entry of a card.
type CardItem struct {
	Title    string
	Subtitle string
	Pic      string
	// URL is where the item leads, as ResolveLink reads it; for a selector,
	// the page url of the list it shows.
	URL string
	// Round is set for people, whose pictures are avatars.
	Round bool
	// Ratio is the picture's width over its height when the server says,
	// else 0.
	Ratio float32
}

// PageEntry is one row of a server page: a post, a product, or a category.
type PageEntry struct {
	Kind     EntryKind
	ID       string
	Feed     *Feed
	Title    string
	Subtitle string
	LogoURL  string
	// URL is the page url of the list a category opens.
	URL  string
	Card *Card
}

// BlockKind names one part of an article's body.
type BlockKind int

const (
	BlockText BlockKind = iota
	BlockImage
	BlockLink
)

// Block is one paragraph, picture or link card of an article, in the order
// the body shows them (message_raw_output of the post's detail).
type Block struct {
	Kind    BlockKind
	Text    string
	Rich    []RichSpan
	Image   Image
	Caption string
	Link    *Link
}

// PostKind is what a post is, which decides how it shows: a text post with
// pictures, an article with a cover, a video, a question or its answer, a
// vote, a picture post or a trade.
type PostKind int

const (
	PostText PostKind = iota
	PostArticle
	PostPicture
	PostVideo
	PostQuestion
	PostAnswer
	PostVote
	PostTrade
	// PostComment is a comment shown as a card of the post it is on, as the
	// account's own comments list: its text is the comment's, not the post's.
	PostComment
)

// Label names the kind in the badge of a card; plain posts have none.
func (k PostKind) Label() string {
	switch k {
	case PostArticle:
		return "图文"
	case PostPicture:
		return "酷图"
	case PostVideo:
		return "视频"
	case PostQuestion:
		return "提问"
	case PostAnswer:
		return "回答"
	case PostVote:
		return "投票"
	case PostTrade:
		return "二手"
	case PostComment:
		return "评论"
	}
	return ""
}

// Feed is one card of a feed list or one post's detail.
type Feed struct {
	ID          string
	Author      Author
	PublishedAt time.Time
	Device      string
	Location    string
	Target      *Target
	Tags        []string
	Title       string
	Content     string
	Rich        []RichSpan
	IsArticle   bool
	Kind        PostKind
	// Blocks is an article's body, where the detail has one.
	Blocks []Block
	// Cover is the picture of an article or a trade, which has no others.
	Cover *Image
	// Answers and Followers count a question's answers and followers.
	Answers, Followers int
	Images             []Image
	Video              *Video
	Link               *Link
	Vote               *Vote
	Forward            *Forward
	Stats              Stats
	Liked              bool
	Favorited          bool
}

// Reply is one comment. Children holds the first level of nested replies.
type Reply struct {
	ID              string
	Author          Author
	Content         string
	Rich            []RichSpan
	CreatedAt       time.Time
	ReplyToName     string
	ReplyToID       string
	Images          []Image
	Likes           int
	Liked           bool
	IsAuthor        bool
	Children        []Reply
	ChildrenTotal   int
	ChildrenHasMore bool
}

// SearchKind selects a global search result tab.
type SearchKind int

const (
	SearchFeeds SearchKind = iota
	SearchUsers
)

// SearchQuery selects one page of global search results.
type SearchQuery struct {
	Keyword string
	Kind    SearchKind
	Page    int
}

// SearchPage contains heterogeneous global search results. HotSearches is
// populated for an empty keyword.
type SearchPage struct {
	Feeds       []Feed
	Users       []Author
	HasMore     bool
	HotSearches []string
}

// UserProfile is the public header of a Coolapk user page.
type UserProfile struct {
	Author      Author
	Bio         string
	CoverURL    string
	Level       int
	FeedCount   int
	FollowCount int
	FansCount   int
	IsFollowed  bool
	VerifyTitle string
}

// UserConnectionKind selects followers or followed accounts.
type UserConnectionKind int

const (
	UserFollowing UserConnectionKind = iota
	UserFans
)

// UserConnectionPage is one page of public user connections.
type UserConnectionPage struct {
	Users   []Author
	HasMore bool
}

// UnreadCounts are the notification badges of a logged-in account.
type UnreadCounts struct {
	Notifications int
	Messages      int
}

// Notification is one message-center entry.
type Notification struct {
	ID        string
	Title     string
	Summary   string
	AvatarURL string
	URL       string
	CreatedAt time.Time
	Unread    bool
}

// NotificationPage is one page of message-center entries.
type NotificationPage struct {
	Items   []Notification
	HasMore bool
}

// Conversation is one private-message thread.
type Conversation struct {
	Ukey        string
	Title       string
	AvatarURL   string
	LastMessage string
	LastTime    time.Time
	Unread      int
}

// ConversationPage is one page of private-message threads.
type ConversationPage struct {
	Items   []Conversation
	HasMore bool
}

// Collection is one favorite folder of the logged-in user. Contains says
// whether the feed the folders were listed for is in it.
type Collection struct {
	ID       string
	Name     string
	Count    int
	Contains bool
}

// CollectionPage lists favorite folders.
type CollectionPage struct {
	Items   []Collection
	HasMore bool
}

// ReplyDraft submits a comment to a feed or to one reply.
type ReplyDraft struct {
	FeedID   string
	ParentID string
	Message  string
}

// FeedDraft publishes a text feed.
type FeedDraft struct {
	Message string
	Private bool
}

// DeviceInfo is the identity the app presents to Coolapk.
type DeviceInfo struct {
	ID            string
	HasCDID       bool
	Model         string
	HardwareID    string
	SalesModel    string
	SystemVersion string
	AppDevice     string
	AppToken      string
}

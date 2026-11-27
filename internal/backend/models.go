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

// Vote is a poll attached to a feed.
type Vote struct {
	Title   string
	Options []VoteOption
	Expired bool
	EndsAt  time.Time
	// Voters is how many accounts voted.
	Voters int
	// MaxSelect is how many options one account may choose.
	MaxSelect int
	// Versus is set for a vote between two sides, each with its color.
	Versus bool
	// Counted is whether the options' votes are known: the server tells them
	// to an account that has voted.
	Counted bool
}

// VoteOption is one choice of a Vote.
type VoteOption struct {
	ID    string
	Text  string
	Votes int
	// Color is the side's color of a Versus vote, as 0xRRGGBB; 0 when not
	// given.
	Color uint32
	// Chosen is set for the options the account voted for.
	Chosen bool
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
	Likes     int
	Comments  int
	Forwards  int
	Favorites int
}

// Target is the topic or product a post is filed under, shown as the chip
// at the bottom left of a card. Its title opens the feed of that name.
type Target struct {
	ID      string // of a product, which has its own feed key
	Title   string
	LogoURL string
	Product bool
	Rating  ItemRating
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
	// EntryLink is a row that leads where its URL says, as ResolveLink reads
	// it: a person of a ranking, an offline event.
	EntryLink
	// EntryNews is an editorial headline and pictures (articleNews).
	EntryNews
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
	// listCard).
	CardList
	// CardScroll is a horizontal strip of posts (feedScrollCard).
	CardScroll
	// CardTitle heads the rows after it (titleCard).
	CardTitle
	// CardSelector chooses what the list below it shows (selectorLinkCard):
	// each item's URL is a page url of its own.
	CardSelector
	// CardTabs chooses as a selector does, and shows as the tabs of the page
	// (iconTabLinkGridCard).
	CardTabs
	// CardButtons is pictures side by side, each a link (iconButtonGridCard).
	CardButtons
	// CardNote is a notice in rich text (messageCard).
	CardNote
	// CardConfigs is the versions a product sells in, each with its price
	// and the page of its specifications (productConfigList).
	CardConfigs
	// CardAvatars is people in a row that scrolls, as the followed accounts
	// with news (iconScrollCard); the card's URL lists them all.
	CardAvatars
	// CardPairs is names with small icons, two to a row (iconMiniGridCard).
	CardPairs
	// CardRanking is a grid of products in the order of their rank
	// (iconLongTitleGridCard with a ranking).
	CardRanking
	// CardTiles is square pictures under their names, in a row that scrolls
	// (imageSquareScrollCard).
	CardTiles
	// CardLogos is the logos of brands in two rows that scroll sideways
	// (iconGridHorizonCard).
	CardLogos
	// CardEditorial presents article covers and headlines in a horizontal strip.
	CardEditorial
	// CardImageGrid shows titled pictures in two columns (imageTextGridCard).
	CardImageGrid
	CardTimeline
	CardProductList
	CardProductHighlights
	CardEntries
	CardMiniLinks
	CardFollowedProducts
	CardDeviceEnvironment
	CardTopicStrip
)

// Card is a card of a server page.
type Card struct {
	Kind     CardKind
	Title    string
	Subtitle string
	// URL, when set, is where the card's title leads, as ResolveLink reads
	// it, and More what the title says of it.
	URL        string
	More       string
	Items      []CardItem
	Columns    int
	Ranked     bool
	Continues  bool
	DefaultTab int
	Inset      bool
	// Results holds the typed rows of a combined search preview.
	Results []PageEntry
	// Note is the text of a CardNote.
	Note []RichSpan
	// Ratio is the width over the height the server asks a banner to show
	// at, 0 without one.
	Ratio float32
}

// Chooses reports whether the card's items choose what the list below it
// shows.
func (c *Card) Chooses() bool { return c.Kind == CardSelector || c.Kind == CardTabs }

// CardItem is one entry of a card.
type CardItem struct {
	Title            string
	Subtitle         string
	Pic              string
	Hot, Discussions string
	Rating           ItemRating
	// URL is where the item leads, as ResolveLink reads it; for a selector,
	// the page url of the list it shows.
	URL string
	// Round is set for people, whose pictures are avatars.
	Round bool
	// Ratio is the picture's width over its height when the server says,
	// else 0.
	Ratio float32
}

// ItemRating is the score or release status shown on a device or topic row.
type ItemRating struct {
	Score, Label string
	Count        string
	OwnerCount   string
	ReleaseDate  string
	Unreleased   bool
}

// PageEntry is one row of a server page: a post, a product, a category, a
// link, or a card.
type PageEntry struct {
	Kind     EntryKind
	ID       string
	Feed     *Feed
	Title    string
	Subtitle string
	LogoURL  string
	// Round is set for people, whose pictures are avatars.
	Round bool
	// Hot is how much a product is talked of, as the server words it
	// ("201.1万").
	Hot         string
	Discussions string
	Score       string
	Rating      ItemRating
	Followed    bool
	Ranked      bool
	Timeline    bool
	RatingRow   bool
	// URL is the page url of the list a category opens, or where a link
	// leads.
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
	Markup  string
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
	// PostRating rates a product: its stars and text are in Feed.Rating.
	PostRating
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
	case PostRating:
		return "点评"
	}
	return ""
}

// FeedRating is what a rating post says of its product: its stars out of
// five and the word for them, whether the author owns the product, the
// score of each side, and its text in sections, as 点评 or 优点.
type FeedRating struct {
	Stars    int
	Word     string
	Owner    bool
	Sides    []RatedSide
	Sections []RatingSection
}

// RatedSide is the stars a rating gives one side of a product, as its
// battery, and the word for them.
type RatedSide struct {
	Name  string
	Stars int
	Word  string
}

// RatingSection is a part of a rating's text under its heading.
type RatingSection struct {
	Title  string
	Text   string
	Rich   []RichSpan
	Markup string
}

// Feed is one card of a feed list or one post's detail.
type Feed struct {
	ID string
	// BigCover presents a picture carousel before the text in post details.
	BigCover    bool
	Author      Author
	PublishedAt time.Time
	Device      string
	Location    string
	Target      *Target
	Relation    *CardItem
	Tags        []string
	Title       string
	// Price is what a second-hand post asks, as "¥2100", "" for the others.
	Price   string
	Content string
	Rich    []RichSpan
	// Markup is the original body of a detail response, before whitespace
	// and formatting are flattened for a feed card.
	Markup    string
	IsArticle bool
	Kind      PostKind
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
	Rating             *FeedRating
	Stats              Stats
	// Pinned are the comments the author keeps at the top, which the post's
	// own page tells.
	Pinned         []Reply
	Liked          bool
	Favorited      bool
	AuthorFollowed bool
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
	// Pinned says the post's author keeps the comment at the top.
	Pinned bool
	// Post is set for an opinion on a vote and an answer to a question, which
	// are posts of their own that ID names.
	Post bool
	// Stance is the option an opinion on a vote stands for.
	Stance string
}

// SearchKind selects a global search result tab.
type SearchKind int

const (
	SearchAll SearchKind = iota
	SearchProducts
	SearchFeeds
	SearchTopics
	SearchUsers
	SearchTrades
	SearchQuestions
	SearchChannels
)

// SearchQuery selects one page of global search results.
type SearchQuery struct {
	Keyword  string
	Kind     SearchKind
	Page     int
	Sort     string
	FeedType string
	City     string
}

const SearchTradeNegotiable = "negotiable"

// SearchPage contains heterogeneous global search results. HotSearches is
// populated for an empty keyword.
type SearchPage struct {
	Entries     []PageEntry
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
	// LikeCount is the likes the account's posts have got.
	LikeCount int
	// Gender is 1 for male and 0 for female, -1 when not given.
	Gender int
	// Region is the province and city of the profile.
	Region     string
	LastActive time.Time
}

// UserFeedKind selects which of an account's posts a list shows.
type UserFeedKind int

const (
	UserFeedAll UserFeedKind = iota
	// UserFeedArticles are the long posts with a title (图文).
	UserFeedArticles
	// UserFeedQA are questions and answers.
	UserFeedQA
)

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
	Notifications int // what other accounts wrote to this one
	Likes         int
	Follows       int
	Messages      int
	Service       int // the notices of the site itself
	Badge         int // the server's own total
}

// Total is the count the entry of the message center shows.
func (u UnreadCounts) Total() int {
	return max(u.Badge, u.Notifications+u.Likes+u.Follows+u.Messages+u.Service)
}

// TopicDetail is the page of a topic or a product: its head and the lists
// under it, which PageEntries reads by their urls.
type TopicDetail struct {
	ID        string
	LottoLogo string
	Title     string
	LogoURL   string
	Intro     string
	Hot       string // as the server words it, "94.4万"
	Comments  int
	Followers int
	Followed  bool
	Tabs      []TopicTab
	Selected  int // the tab shown first
	// Rating is the aggregate score of the topic or product.
	Rating *Rating
	Tags   []string
}

// Rating is the score out of 10, its breakdown, and how many rated it.
type Rating struct {
	Score    string
	Sides    []RatingSide
	Owners   int
	Users    int
	Detailed int
	Stats    *TopicRatingStats
}

// RatingSide is one side of a product, as its battery, and its score.
type RatingSide struct {
	Name  string
	Score float32
}

type TopicRatingStats struct {
	Stars          [5]int
	RecentScore    string
	RecentCount    int
	RecentPositive string
	Weekly         RatingTrend
}

type RatingTrends struct {
	Day   RatingTrend
	Week  RatingTrend
	Month RatingTrend
}

type RatingTrend struct {
	Periods                  []RatingPeriod
	ScoreFloor, ScoreCeiling float32
	CountCeiling             int
}

type RatingPeriod struct {
	Date  string
	Score float32
	Count int
}

type NodeRating struct {
	ID, FeedID string
	Author     Author
	Stars      int
	Detailed   bool
}

type NodeRatingPage struct {
	Items   []NodeRating
	Cursor  ListCursor
	HasMore bool
}

type NodeRatingQuery struct {
	TopicID string
	Page    int
	Cursor  ListCursor
	Filter  NodeRatingFilter
}

type NodeRatingFilter struct {
	Star    int
	Detail  RatingDetailFilter
	Comment RatingCommentFilter
}

type RatingDetailFilter uint8

const (
	RatingDetailDefault RatingDetailFilter = iota
	RatingDetailOnly
	RatingDetailOff
)

type RatingCommentFilter uint8

const (
	RatingCommentDefault RatingCommentFilter = iota
	RatingCommentAll
	RatingWithComment
	RatingWithoutComment
)

type EventDetail struct {
	Title, CoverURL, ActionURL, WinnersURL string
	Description, Rules                     []RichSpan
	Sponsors                               []Author
	Prizes                                 []CardItem
	RegistrationStart, RegistrationEnd     time.Time
	Stage                                  int
	Joined                                 bool
	Tabs                                   []TopicTab
}

// TopicTab is one list or webpage of a topic's page.
type TopicTab struct {
	Title string
	URL   string
}

// NoticeKind is a list of the message center.
type NoticeKind int

const (
	// NoticesAll are what other accounts wrote to this one: comments,
	// replies and mentions.
	NoticesAll NoticeKind = iota
	NoticesLikes
	NoticesFollows
)

// Notification is one message-center entry. Action words the button of a
// service notice that leads to URL.
type Notification struct {
	ID        string
	Title     string
	Summary   string
	AvatarURL string
	URL       string
	Action    string
	CreatedAt time.Time
	Unread    bool
}

// NoticeSender is a sender of service notices, as the system or the
// activities: its note type and its name.
type NoticeSender struct {
	Kind string
	Name string
}

// NotificationPage is one page of message-center entries.
type NotificationPage struct {
	Items   []Notification
	HasMore bool
}

// Conversation is one private-message thread.
type Conversation struct {
	Ukey string
	// UID is the other account's, which a message to it names.
	UID         string
	Title       string
	AvatarURL   string
	LastMessage string
	LastTime    time.Time
	Unread      int
}

// ChatMessage is one message of a private-message thread. A picture
// message has no text.
type ChatMessage struct {
	ID        string
	Author    Author
	Mine      bool
	Text      string
	Picture   bool
	CreatedAt time.Time
}

// ChatPage is one page of a thread's messages, newest first.
type ChatPage struct {
	Items   []ChatMessage
	HasMore bool
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

// DeviceProfile describes the model and OS reported in requests.
type DeviceProfile struct {
	Model, HardwareID, SystemVersion string
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

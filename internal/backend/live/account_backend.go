package live

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"app/internal/backend"
	coolapk "github.com/XiaoMengXinX/CoolapkApi-Go"
)

func (b *coolapkBackend) SessionUID() string { return b.client.Session().UID }

func (b *coolapkBackend) UserAgent() string { return b.client.Device.UserAgent() }

func (b *coolapkBackend) ImportSession(ctx context.Context, uid, username, token string) error {
	previous := b.client.Session()
	err := b.importSession(ctx, coolapk.Session{
		UID: uid, Username: username, Token: token,
		CDID: previous.CDID, IDFV: previous.IDFV, InstallID: previous.InstallID,
	})
	if err != nil {
		// The device keeps the IDs this attempt made, so that the next one is
		// the same installation.
		failed := b.client.Session()
		previous.IDFV, previous.InstallID = failed.IDFV, failed.InstallID
		b.client.SetSession(previous)
		_ = b.saveIdentity()
	}
	return err
}

func (b *coolapkBackend) importSession(ctx context.Context, session coolapk.Session) error {
	b.client.SetSession(session)
	if _, err := b.client.CheckLoginInfoWithCtx(ctx); err != nil {
		return err
	}
	if _, err := b.client.EnsureCDIDWithCtx(ctx); err != nil {
		return err
	}
	return b.saveIdentity()
}

func (b *coolapkBackend) Logout() error {
	b.client.Logout()
	return b.saveIdentity()
}

func (b *coolapkBackend) Device() backend.DeviceInfo {
	d := b.client.DeviceInfo()
	return backend.DeviceInfo{
		ID: d.ID, HasCDID: d.HasCDID, Model: d.Model, HardwareID: d.HardwareID, SalesModel: d.SalesModel,
		SystemVersion: d.SystemVersion, AppDevice: d.AppDevice, AppToken: d.AppToken,
	}
}

func (b *coolapkBackend) SetDeviceProfile(model, hardwareID, salesModel, systemVersion string) error {
	b.client.SetDeviceProfile(model, hardwareID, salesModel, systemVersion)
	return b.saveIdentity()
}

func (b *coolapkBackend) RegenerateDeviceProfile() error {
	b.client.RegenerateDeviceProfile()
	return b.saveIdentity()
}

func (b *coolapkBackend) ResetDeviceID(ctx context.Context) error {
	err := b.client.ResetDeviceID(ctx)
	if saveErr := b.saveIdentity(); err == nil {
		err = saveErr
	}
	return err
}

func (b *coolapkBackend) saveIdentity() error {
	return saveCoolapkIdentity(b.dir, coolapkIdentity{
		Device:      b.client.Device,
		InstallTime: b.installTime,
		Session:     b.client.Session(),
	})
}

func (b *coolapkBackend) UnreadCounts(ctx context.Context) (backend.UnreadCounts, error) {
	response, err := b.client.GetUnreadCountWithCtx(ctx)
	if err != nil {
		return backend.UnreadCounts{}, err
	}
	object, err := decodeAPIObject(response.Data)
	if err != nil {
		return backend.UnreadCounts{}, err
	}
	return backend.UnreadCounts{
		Notifications: apiInt(object, "notification_v18"),
		Likes:         apiInt(object, "feedlike"),
		Follows:       apiInt(object, "contacts_follow"),
		Messages:      apiInt(object, "message"),
		Service:       apiInt(object, "notification_v18_service"),
		Badge:         apiInt(object, "badge_v18"),
	}, nil
}

// noticeLists are the lists of the message center.
var noticeLists = map[backend.NoticeKind]coolapk.NotificationKind{
	backend.NoticesAll:     coolapk.NotificationAll,
	backend.NoticesLikes:   coolapk.NotificationFeedLike,
	backend.NoticesFollows: coolapk.NotificationFollow,
}

// serviceNotice says whether an entry of the main list stands for a sender
// of service notices, whose notices are a list of their own.
func serviceNotice(object map[string]any) bool {
	return apiInt(object, "subitem_notify") != 0
}

func (b *coolapkBackend) Notifications(ctx context.Context, kind backend.NoticeKind, page int) (backend.NotificationPage, error) {
	key := fmt.Sprintf("notifications:%d", kind)
	cursor := b.cursor(key, page)
	objects, err := listObjects(b.client.GetNotificationListWithCtx(ctx, noticeLists[kind], coolapk.ListRequest{Page: page, LastItem: cursor.last}))
	if err != nil {
		return backend.NotificationPage{}, err
	}
	b.storeCursor(key, page, feedCursor(objects))
	items := make([]backend.Notification, 0, len(objects))
	for _, object := range objects {
		if kind == backend.NoticesAll && serviceNotice(object) {
			continue
		}
		items = append(items, noticeFromObject(object, kind))
	}
	return backend.NotificationPage{Items: items, HasMore: len(objects) > 0}, nil
}

func (b *coolapkBackend) MarkNotificationsRead(ctx context.Context) error {
	_, err := b.client.GetRawWithCtx(ctx, "/notification/clearCount", map[string]interface{}{"type": "all"})
	return err
}

func (b *coolapkBackend) ClearNotifications(ctx context.Context) error {
	_, err := b.client.PostFormWithCtx(ctx, "/notificationV18/deleteAll", url.Values{"type": {"notification"}}, nil)
	return err
}

func (b *coolapkBackend) NoticeSenders(ctx context.Context) ([]backend.NoticeSender, error) {
	objects, err := listObjects(b.client.GetRawWithCtx(ctx, "/notificationV18/subGroupList", nil))
	if err != nil {
		return nil, err
	}
	var senders []backend.NoticeSender
	seen := map[string]bool{}
	for _, object := range objects {
		kind := firstAPIString(object, "note_type")
		if kind == "" || seen[kind] {
			continue
		}
		seen[kind] = true
		senders = append(senders, backend.NoticeSender{Kind: kind, Name: firstAPIString(object, "from_username", "fromusername")})
	}
	return senders, nil
}

func (b *coolapkBackend) ServiceNotices(ctx context.Context, sender string, page int) (backend.NotificationPage, error) {
	key := "notifications:service:" + sender
	cursor := b.cursor(key, page)
	objects, err := listObjects(b.client.GetServiceNotificationsWithCtx(ctx, sender, coolapk.ListRequest{
		Page: page, FirstItem: cursor.first, LastItem: cursor.last,
	}))
	if err != nil {
		return backend.NotificationPage{}, err
	}
	next := feedCursor(objects)
	if page > 1 {
		next.first = cursor.first
	}
	b.storeCursor(key, page, next)
	items := make([]backend.Notification, 0, len(objects))
	for _, object := range objects {
		n := noticeFromObject(object, backend.NoticesAll)
		n.Action = firstAPIString(object, "url_btn_title")
		items = append(items, n)
	}
	return backend.NotificationPage{Items: items, HasMore: len(objects) > 0}, nil
}

// noticeFromObject reads a notice: who it is from, what it says after its
// kind, and where it leads. A like says what was liked.
func noticeFromObject(object map[string]any, kind backend.NoticeKind) backend.Notification {
	said := cleanAPIText(firstAPIString(object, "note", "message"))
	if kind == backend.NoticesLikes {
		// What was liked is its text, or for an article its cover's address.
		said = cleanAPIText(firstAPIString(object, "addition_info", "note"))
		if strings.HasPrefix(said, "http") {
			said = ""
		}
	}
	n := backend.Notification{
		ID:        apiID(object),
		Title:     firstAPIString(object, "fromusername", "from_username", "username", "title"),
		Summary:   joinNonEmpty("：", firstAPIString(object, "noteTypeTitle"), said),
		AvatarURL: normalizeCoolapkImage(firstAPIString(object, "fromUserAvatar", "userAvatar")),
		URL:       firstAPIString(object, "url"),
		CreatedAt: apiTime(object, "dateline"),
		Unread:    apiInt(object, "isnew") != 0,
	}
	if n.URL == "" {
		if uid := firstAPIString(object, "fromuid", "from_uid", "uid"); uid != "" {
			n.URL = "/u/" + uid
		}
	}
	return n
}

func (b *coolapkBackend) Conversations(ctx context.Context, page int) (backend.ConversationPage, error) {
	cursor := b.cursor("conversations", page)
	objects, err := listObjects(b.client.GetMessageListWithCtx(ctx, coolapk.ListRequest{Page: page, LastItem: cursor.last}))
	if err != nil {
		return backend.ConversationPage{}, err
	}
	b.storeCursor("conversations", page, feedCursor(objects))
	items := make([]backend.Conversation, 0, len(objects))
	for _, object := range objects {
		last := cleanAPIText(firstAPIString(object, "message"))
		if last == "" && firstAPIString(object, "message_pic") != "" {
			last = "[图片]"
		}
		items = append(items, backend.Conversation{
			Ukey:        firstAPIString(object, "ukey"),
			UID:         firstAPIString(object, "messageUid"),
			Title:       firstAPIString(object, "messageUsername", "username"),
			AvatarURL:   normalizeCoolapkImage(firstAPIString(object, "messageUserAvatar", "userAvatar")),
			LastMessage: last,
			LastTime:    apiTime(object, "dateline"),
			Unread:      apiInt(object, "unreadNum"),
		})
	}
	return backend.ConversationPage{Items: items, HasMore: len(objects) > 0}, nil
}

func (b *coolapkBackend) Chat(ctx context.Context, ukey string, page int) (backend.ChatPage, error) {
	objects, err := listObjects(b.client.GetChatHistoryWithCtx(ctx, ukey, page))
	if err != nil {
		return backend.ChatPage{}, err
	}
	if page == 1 {
		// The thread is read once shown; a failure leaves it marked unread.
		b.client.MarkChatReadWithCtx(ctx, ukey)
	}
	me := b.SessionUID()
	items := make([]backend.ChatMessage, 0, len(objects))
	for _, object := range objects {
		if firstAPIString(object, "entityType") != "message" {
			continue // the server's notes between the messages
		}
		from := firstAPIString(object, "fromuid")
		items = append(items, backend.ChatMessage{
			ID: apiID(object),
			Author: backend.Author{ID: from, Name: firstAPIString(object, "fromusername"),
				AvatarURL: normalizeCoolapkImage(firstAPIString(object, "fromUserAvatar"))},
			Mine:      from == me,
			Text:      cleanAPIText(firstAPIString(object, "message")),
			Picture:   firstAPIString(object, "message_pic") != "",
			CreatedAt: apiTime(object, "dateline"),
		})
	}
	return backend.ChatPage{Items: items, HasMore: len(objects) > 0}, nil
}

func (b *coolapkBackend) SendMessage(ctx context.Context, uid, text string) error {
	_, err := b.client.SendMessageWithCtx(ctx, coolapk.MessageRequest{UID: uid, Message: text})
	return err
}

func (b *coolapkBackend) SetFeedLike(ctx context.Context, id string, liked bool) (int, error) {
	feedID, err := strconv.Atoi(id)
	if err != nil {
		return 0, fmt.Errorf("无效的帖子编号")
	}
	var response *coolapk.RawResponse
	if liked {
		response, err = b.client.LikeFeedWithCtx(ctx, feedID)
	} else {
		response, err = b.client.UnlikeFeedWithCtx(ctx, feedID)
	}
	if err != nil {
		return 0, err
	}
	return decodeLikeCount(response)
}

func (b *coolapkBackend) DeleteReply(ctx context.Context, id string) error {
	_, err := b.client.DeleteReplyWithCtx(ctx, id)
	return err
}

func (b *coolapkBackend) SetReplyLike(ctx context.Context, id string, liked bool) (int, error) {
	replyID, err := strconv.Atoi(id)
	if err != nil {
		return 0, fmt.Errorf("无效的评论编号")
	}
	var response *coolapk.RawResponse
	if liked {
		response, err = b.client.LikeReplyWithCtx(ctx, replyID)
	} else {
		response, err = b.client.UnlikeReplyWithCtx(ctx, replyID)
	}
	if err != nil {
		return 0, err
	}
	return decodeLikeCount(response)
}

// decodeLikeCount reads the count a like or unlike returns: an object
// with "count", or the bare number.
func decodeLikeCount(response *coolapk.RawResponse) (int, error) {
	if object, err := decodeAPIObject(response.Data); err == nil {
		if value := firstAPIString(object, "count", "likenum"); value != "" {
			return apiInt(object, "count", "likenum"), nil
		}
	}
	raw := strings.Trim(strings.TrimSpace(string(response.Data)), `"`)
	count, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("点赞响应缺少数量")
	}
	return count, nil
}

func (b *coolapkBackend) SetUserFollow(ctx context.Context, uid string, follow bool) error {
	var err error
	if follow {
		_, err = b.client.FollowUserWithCtx(ctx, uid)
	} else {
		_, err = b.client.UnfollowUserWithCtx(ctx, uid)
	}
	return err
}

func (b *coolapkBackend) Collections(ctx context.Context, feedID string, page int) (backend.CollectionPage, error) {
	objects, err := listObjects(b.client.GetCollectionsWithCtx(ctx, coolapk.CollectionListRequest{
		UID: b.client.Session().UID, FeedID: feedID, Type: "feed", ShowDefault: 1, Page: page,
	}))
	if err != nil {
		return backend.CollectionPage{}, err
	}
	items := make([]backend.Collection, 0, len(objects))
	for _, object := range objects {
		items = append(items, backend.Collection{
			ID:       apiID(object),
			Name:     firstAPIString(object, "title"),
			Count:    apiInt(object, "item_num"),
			Contains: apiInt(object, "isBeCollected") == 1,
		})
	}
	return backend.CollectionPage{Items: items, HasMore: len(objects) > 0}, nil
}

func (b *coolapkBackend) SetCollection(ctx context.Context, collectionID, feedID string, add bool) error {
	var err error
	if add {
		_, err = b.client.AddToCollectionWithCtx(ctx, collectionID, feedID, "feed")
	} else {
		_, err = b.client.RemoveFromCollectionWithCtx(ctx, collectionID, feedID, "feed")
	}
	return err
}

func (b *coolapkBackend) CreateReply(ctx context.Context, draft backend.ReplyDraft) (backend.Reply, error) {
	request := coolapk.ReplyRequest{Message: draft.Message}
	var response *coolapk.RawResponse
	var err error
	if draft.ParentID != "" {
		parentID, convErr := strconv.Atoi(draft.ParentID)
		if convErr != nil {
			return backend.Reply{}, fmt.Errorf("无效的评论编号")
		}
		response, err = b.client.ReplyReplyWithCtx(ctx, parentID, request)
	} else {
		feedID, convErr := strconv.Atoi(draft.FeedID)
		if convErr != nil {
			return backend.Reply{}, fmt.Errorf("无效的帖子编号")
		}
		response, err = b.client.ReplyFeedWithCtx(ctx, feedID, request)
	}
	if err != nil {
		return backend.Reply{}, err
	}
	object, err := decodeAPIObject(response.Data)
	if err != nil {
		return backend.Reply{}, err
	}
	return replyFromObject(object), nil
}

func (b *coolapkBackend) CreateFeed(ctx context.Context, draft backend.FeedDraft) (string, error) {
	status := 0
	if draft.Private {
		status = 1
	}
	response, err := b.client.CreateFeedWithCtx(ctx, coolapk.CreateFeedRequest{Message: draft.Message, PublishStatus: status})
	if err != nil {
		return "", err
	}
	object, err := decodeAPIObject(response.Data)
	if err != nil {
		return "", err
	}
	id := apiID(object)
	if id == "" {
		return "", fmt.Errorf("发帖响应缺少编号")
	}
	return id, nil
}

func (b *coolapkBackend) CastVote(ctx context.Context, feedID string, optionIDs []string) error {
	_, err := b.client.VoteWithCtx(ctx, feedID, optionIDs)
	return err
}

func (b *coolapkBackend) Topic(ctx context.Context, name, productID string) (backend.TopicDetail, error) {
	var response *coolapk.RawResponse
	var err error
	if productID != "" {
		response, err = b.client.GetProductDetailWithCtx(ctx, productID)
	} else {
		response, err = b.client.GetTopicDetailWithCtx(ctx, name)
	}
	if err != nil {
		return backend.TopicDetail{}, err
	}
	object, err := decodeAPIObject(response.Data)
	if err != nil {
		return backend.TopicDetail{}, err
	}
	d := backend.TopicDetail{
		Title:     firstAPIString(object, "title"),
		LogoURL:   normalizeCoolapkImage(firstAPIString(object, "logo")),
		Intro:     strings.TrimSpace(firstAPIString(object, "intro", "description")),
		Hot:       firstAPIString(object, "hot_num_txt"),
		Comments:  apiInt(object, "commentnum", "feed_comment_num"),
		Followers: apiInt(object, "follownum", "follow_num"),
		Followed:  apiInt(apiObject(object, "userAction"), "follow") != 0,
	}
	if firstAPIString(object, "relation_addition_title") == "抽奖" {
		d.LottoLogo = normalizeCoolapkImage(firstAPIString(object, "relation_addition_logo"))
	}
	if productID != "" {
		if release := firstAPIString(object, "release_time"); release != "" {
			d.Tags = append(d.Tags, release+"上市")
		}
		d.Tags = append(d.Tags, apiStrings(object, "tagArr")...)
		if n := apiInt(object, "owner_star_total_count"); n > 0 {
			d.Rating = &backend.Rating{Score: firstAPIString(object, "owner_star_average_score"), Owners: n,
				Detailed: apiInt(object, "v4_rating_owner_total_count")}
			for _, side := range apiObjects(object, "rating_item_info") {
				score, _ := strconv.ParseFloat(firstAPIString(side, "owner_average_score"), 32)
				d.Rating.Sides = append(d.Rating.Sides, backend.RatingSide{Name: firstAPIString(side, "name"), Score: float32(score)})
			}
		}
		b.mu.Lock()
		b.configs[productID] = apiObjects(object, "configRows")
		b.mu.Unlock()
	}
	d.Tabs, d.Selected = topicTabsFromObject(object)
	return d, nil
}

func topicTabsFromObject(object map[string]any) (tabs []backend.TopicTab, at int) {
	selected := firstAPIString(object, "selectedTab")
	for _, tab := range apiObjects(object, "tabList") {
		if link := firstAPIString(tab, "url"); link != "" {
			if firstAPIString(tab, "page_name") == selected {
				at = len(tabs)
			}
			tabs = append(tabs, backend.TopicTab{Title: firstAPIString(tab, "title"), URL: link})
		}
	}
	return tabs, at
}

func (b *coolapkBackend) SetTopicFollow(ctx context.Context, name, productID string, follow bool) error {
	var err error
	switch {
	case productID != "":
		_, err = b.client.ChangeProductFollowWithCtx(ctx, productID, follow)
	case follow:
		_, err = b.client.FollowTopicWithCtx(ctx, name)
	default:
		_, err = b.client.UnfollowTopicWithCtx(ctx, name)
	}
	return err
}

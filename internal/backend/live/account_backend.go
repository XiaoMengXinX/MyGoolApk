package live

import (
	"context"
	"fmt"
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
		Notifications: apiInt(object, "notification_v18", "notification"),
		Messages:      apiInt(object, "message"),
	}, nil
}

func (b *coolapkBackend) Notifications(ctx context.Context, page int) (backend.NotificationPage, error) {
	cursor := b.cursor("notifications", page)
	objects, err := listObjects(b.client.GetNotificationV18WithCtx(ctx, page, cursor.last))
	if err != nil {
		return backend.NotificationPage{}, err
	}
	b.storeCursor("notifications", page, feedCursor(objects))
	items := make([]backend.Notification, 0, len(objects))
	for _, object := range objects {
		items = append(items, backend.Notification{
			ID:        apiID(object),
			Title:     firstAPIString(object, "noteTypeTitle", "title"),
			Summary:   cleanAPIText(firstAPIString(object, "note", "message")),
			AvatarURL: normalizeCoolapkImage(firstAPIString(object, "fromUserAvatar", "userAvatar")),
			URL:       firstAPIString(object, "url"),
			CreatedAt: apiTime(object, "dateline"),
			Unread:    apiInt(object, "isnew") != 0,
		})
	}
	return backend.NotificationPage{Items: items, HasMore: len(objects) > 0}, nil
}

func (b *coolapkBackend) Conversations(ctx context.Context, page int) (backend.ConversationPage, error) {
	cursor := b.cursor("conversations", page)
	objects, err := listObjects(b.client.GetMessageListWithCtx(ctx, page, cursor.last))
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
			Title:       firstAPIString(object, "messageUsername", "username"),
			AvatarURL:   normalizeCoolapkImage(firstAPIString(object, "messageUserAvatar", "userAvatar")),
			LastMessage: last,
			LastTime:    apiTime(object, "dateline"),
			Unread:      apiInt(object, "unreadNum"),
		})
	}
	return backend.ConversationPage{Items: items, HasMore: len(objects) > 0}, nil
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

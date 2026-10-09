// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCForumsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) MessagesGetForumTopics(in *mtproto.TLMessagesGetForumTopics) (*mtproto.Messages_ForumTopics, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	key, err := c.forumPeerKey(uid, in.GetPeer(), false)
	if err != nil {
		return nil, err
	}
	q := ""
	if in.GetQ() != nil {
		q = in.GetQ().GetValue()
	}
	topics, err := forumList(key, uid, q, in.GetLimit(), nil, false)
	if err != nil {
		return nil, err
	}
	topics, err = forumWithUserTitle(uid, key, topics)
	if err != nil {
		return nil, err
	}
	return forumTopicsReply(topics), nil
}

func (c *ApiFullCore) MessagesGetForumTopicsByID(in *mtproto.TLMessagesGetForumTopicsByID) (*mtproto.Messages_ForumTopics, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	key, err := c.forumPeerKey(uid, in.GetPeer(), false)
	if err != nil {
		return nil, err
	}
	topics, err := forumList(key, uid, "", 0, in.GetTopics(), true)
	if err != nil {
		return nil, err
	}
	return forumTopicsReply(topics), nil
}

func (c *ApiFullCore) MessagesEditForumTopic(in *mtproto.TLMessagesEditForumTopic) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	key, err := c.forumPeerKey(uid, in.GetPeer(), true)
	if err != nil {
		return nil, err
	}
	if err := forumEditTopic(key, in.GetTopicId(), in.GetTitle(), in.GetIconEmojiId(), in.GetClosed(), in.GetHidden()); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) MessagesUpdatePinnedForumTopic(in *mtproto.TLMessagesUpdatePinnedForumTopic) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	key, err := c.forumPeerKey(uid, in.GetPeer(), true)
	if err != nil {
		return nil, err
	}
	pinned := mtproto.FromBool(in.GetPinned())
	if err := forumSetPinned(key, in.GetTopicId(), pinned); err != nil {
		return nil, err
	}
	peer, err := forumPeerFromKey(key)
	if err != nil {
		return nil, err
	}
	updates := emptyUpdates()
	updates.Updates = []*mtproto.Update{mtproto.MakeTLUpdatePinnedForumTopic(&mtproto.Update{
		Pinned:        pinned,
		Peer_PEER:     peer,
		TopicId_INT32: in.GetTopicId(),
	}).To_Update()}
	return updates, nil
}

func (c *ApiFullCore) MessagesReorderPinnedForumTopics(in *mtproto.TLMessagesReorderPinnedForumTopics) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	key, err := c.forumPeerKey(uid, in.GetPeer(), true)
	if err != nil {
		return nil, err
	}
	if err := forumReorder(key, in.GetOrder()); err != nil {
		return nil, err
	}
	peer, err := forumPeerFromKey(key)
	if err != nil {
		return nil, err
	}
	updates := emptyUpdates()
	updates.Updates = []*mtproto.Update{mtproto.MakeTLUpdatePinnedForumTopics(&mtproto.Update{
		Peer_PEER:         peer,
		Order_VECTORINT32: in.GetOrder(),
	}).To_Update()}
	return updates, nil
}

func (c *ApiFullCore) MessagesCreateForumTopic(in *mtproto.TLMessagesCreateForumTopic) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetTitle() == "" {
		return nil, mtproto.ErrTitleInvalid
	}
	key, err := c.forumPeerKey(uid, in.GetPeer(), true)
	if err != nil {
		return nil, err
	}
	if err := forumPut(key, uid, in.GetTitle()); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) MessagesDeleteTopicHistory(in *mtproto.TLMessagesDeleteTopicHistory) (*mtproto.Messages_AffectedHistory, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	key, err := c.forumPeerKey(uid, in.GetPeer(), true)
	if err != nil {
		return nil, err
	}
	deleted, err := forumDelete(key, in.GetTopMsgId())
	if err != nil {
		return nil, err
	}
	if err := forumClearUserTitleIfEmpty(uid, key); err != nil {
		return nil, err
	}
	return forumAffected(deleted), nil
}

func (c *ApiFullCore) ChannelsToggleForum(in *mtproto.TLChannelsToggleForum) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	if _, err := c.forumChannelKey(uid, in.GetChannel(), true); err != nil {
		return nil, err
	}
	if err := domain.UpdateForumChannelSettings(in.GetChannel().GetChannelId(), func(settings *domain.ForumChannelSettings) {
		settings.Enabled = mtproto.FromBool(in.GetEnabled())
		settings.Tabs = mtproto.FromBool(in.GetTabs())
	}); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) ChannelsToggleViewForumAsMessages(in *mtproto.TLChannelsToggleViewForumAsMessages) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	if _, err := c.forumChannelKey(uid, in.GetChannel(), true); err != nil {
		return nil, err
	}
	enabled := mtproto.FromBool(in.GetEnabled())
	if err := domain.UpdateForumChannelSettings(in.GetChannel().GetChannelId(), func(settings *domain.ForumChannelSettings) {
		settings.ViewAsMessages = enabled
	}); err != nil {
		return nil, err
	}
	updates := emptyUpdates()
	updates.Updates = []*mtproto.Update{mtproto.MakeTLUpdateChannelViewForumAsMessages(&mtproto.Update{
		ChannelId: in.GetChannel().GetChannelId(),
		Enabled:   mtproto.ToBool(enabled),
	}).To_Update()}
	return updates, nil
}

func (c *ApiFullCore) ChannelsCreateForumTopic(in *mtproto.TLChannelsCreateForumTopic) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetTitle() == "" {
		return nil, mtproto.ErrTitleInvalid
	}
	key, err := c.forumChannelKey(uid, in.GetChannel(), true)
	if err != nil {
		return nil, err
	}
	if err := forumPut(key, uid, in.GetTitle()); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) ChannelsGetForumTopics(in *mtproto.TLChannelsGetForumTopics) (*mtproto.Messages_ForumTopics, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	key, err := c.forumChannelKey(uid, in.GetChannel(), false)
	if err != nil {
		return nil, err
	}
	q := ""
	if in.GetQ() != nil {
		q = in.GetQ().GetValue()
	}
	topics, err := forumList(key, uid, q, in.GetLimit(), nil, false)
	if err != nil {
		return nil, err
	}
	topics, err = forumWithUserTitle(uid, key, topics)
	if err != nil {
		return nil, err
	}
	return forumTopicsReply(topics), nil
}

func (c *ApiFullCore) ChannelsGetForumTopicsByID(in *mtproto.TLChannelsGetForumTopicsByID) (*mtproto.Messages_ForumTopics, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	key, err := c.forumChannelKey(uid, in.GetChannel(), false)
	if err != nil {
		return nil, err
	}
	topics, err := forumList(key, uid, "", 0, in.GetTopics(), true)
	if err != nil {
		return nil, err
	}
	return forumTopicsReply(topics), nil
}

func (c *ApiFullCore) ChannelsEditForumTopic(in *mtproto.TLChannelsEditForumTopic) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	key, err := c.forumChannelKey(uid, in.GetChannel(), true)
	if err != nil {
		return nil, err
	}
	if err := forumEditTopic(key, in.GetTopicId(), in.GetTitle(), in.GetIconEmojiId(), in.GetClosed(), in.GetHidden()); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

func (c *ApiFullCore) ChannelsUpdatePinnedForumTopic(in *mtproto.TLChannelsUpdatePinnedForumTopic) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	key, err := c.forumChannelKey(uid, in.GetChannel(), true)
	if err != nil {
		return nil, err
	}
	pinned := mtproto.FromBool(in.GetPinned())
	if err := forumSetPinned(key, in.GetTopicId(), pinned); err != nil {
		return nil, err
	}
	peer, err := forumPeerFromKey(key)
	if err != nil {
		return nil, err
	}
	updates := emptyUpdates()
	updates.Updates = []*mtproto.Update{mtproto.MakeTLUpdatePinnedForumTopic(&mtproto.Update{
		Pinned:        pinned,
		Peer_PEER:     peer,
		TopicId_INT32: in.GetTopicId(),
	}).To_Update()}
	return updates, nil
}

func (c *ApiFullCore) ChannelsDeleteTopicHistory(in *mtproto.TLChannelsDeleteTopicHistory) (*mtproto.Messages_AffectedHistory, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	key, err := c.forumChannelKey(uid, in.GetChannel(), true)
	if err != nil {
		return nil, err
	}
	deleted, err := forumDelete(key, in.GetTopMsgId())
	if err != nil {
		return nil, err
	}
	if err := forumClearUserTitleIfEmpty(uid, key); err != nil {
		return nil, err
	}
	return forumAffected(deleted), nil
}

func (c *ApiFullCore) ChannelsReorderPinnedForumTopics(in *mtproto.TLChannelsReorderPinnedForumTopics) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrChannelInvalid
	}
	key, err := c.forumChannelKey(uid, in.GetChannel(), true)
	if err != nil {
		return nil, err
	}
	if err := forumReorder(key, in.GetOrder()); err != nil {
		return nil, err
	}
	peer, err := forumPeerFromKey(key)
	if err != nil {
		return nil, err
	}
	updates := emptyUpdates()
	updates.Updates = []*mtproto.Update{mtproto.MakeTLUpdatePinnedForumTopics(&mtproto.Update{
		Peer_PEER:         peer,
		Order_VECTORINT32: in.GetOrder(),
	}).To_Update()}
	return updates, nil
}

// forumWithUserTitle fills an empty list from the caller's most recent topic title.
func forumWithUserTitle(uid int64, key string, topics []*mtproto.ForumTopic) ([]*mtproto.ForumTopic, error) {
	title, err := domain.ForumUserTitle(uid)
	if err != nil || title == "" {
		return topics, err
	}
	for _, t := range topics {
		if t != nil && t.GetTitle() == title {
			return topics, nil
		}
	}
	if len(topics) == 0 {
		peer, err := forumPeerFromKey(key)
		if err != nil {
			return nil, err
		}
		return []*mtproto.ForumTopic{forumTopicFromDomain(peer, domain.ForumTopic{Title: title, CreatorID: uid}, uid)}, nil
	}
	for _, t := range topics {
		if t != nil && t.GetTitle() == "" {
			t.Title = title
		}
	}
	return topics, nil
}

func forumClearUserTitleIfEmpty(uid int64, key string) error {
	return domain.ClearForumUserTitleIfNoTopics(uid, key)
}

func forumPeerKey(peer *mtproto.InputPeer) (string, error) {
	if peer == nil {
		return "", mtproto.ErrPeerIdInvalid
	}
	if id := peer.GetChannelId(); id != 0 {
		return fmt.Sprintf("channel:%d", id), nil
	}
	if id := peer.GetChatId(); id != 0 {
		return fmt.Sprintf("chat:%d", id), nil
	}
	if id := peer.GetUserId(); id != 0 {
		return fmt.Sprintf("user:%d", id), nil
	}
	return "", mtproto.ErrPeerIdInvalid
}

func forumPeerFromKey(key string) (*mtproto.Peer, error) {
	kind, rawID, ok := strings.Cut(key, ":")
	if !ok {
		return nil, mtproto.ErrPeerIdInvalid
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch kind {
	case "channel":
		return mtproto.MakeTLPeerChannel(&mtproto.Peer{ChannelId: id}).To_Peer(), nil
	case "chat":
		return mtproto.MakeTLPeerChat(&mtproto.Peer{ChatId: id}).To_Peer(), nil
	case "user":
		return mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: id}).To_Peer(), nil
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
}

// forumPeerKey validates the channel variant of InputPeer before using its
// identifier as a forum storage key. Non-channel forum peers keep their
// existing peer-specific behavior.
func (c *ApiFullCore) forumPeerKey(userID int64, peer *mtproto.InputPeer, write bool) (string, error) {
	if peer == nil {
		return "", mtproto.ErrPeerIdInvalid
	}
	if peer.GetChannelId() == 0 {
		return forumPeerKey(peer)
	}
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  peer.GetChannelId(),
		AccessHash: peer.GetAccessHash(),
	}).To_InputChannel()
	return c.forumChannelKey(userID, input, write)
}

func (c *ApiFullCore) forumChannelKey(userID int64, input *mtproto.InputChannel, write bool) (string, error) {
	if input == nil || input.GetChannelId() == 0 {
		return "", mtproto.ErrChannelInvalid
	}
	channel, err := c.resolveMemberChannel(userID, input, input.GetChannelId())
	if err != nil {
		return "", err
	}
	if err = c.requireChannelMember(userID, channel.ID); err != nil {
		return "", err
	}
	if write && channel.Creator != userID {
		member, found, err := domain.LoadChannelMember(channel.ID, userID)
		if err != nil {
			return "", c.mapChannelMemberError(err)
		}
		if !found || member.AdminRights == nil || !member.AdminRights.ManageTopics {
			return "", mtproto.ErrChatAdminRequired
		}
	}
	return fmt.Sprintf("channel:%d", channel.ID), nil
}

func forumTopicFromDomain(peer *mtproto.Peer, v domain.ForumTopic, userID int64) *mtproto.ForumTopic {
	creatorID := v.CreatorID
	if creatorID == 0 {
		creatorID = userID
	}
	return mtproto.MakeTLForumTopic(&mtproto.ForumTopic{
		Id:          v.ID,
		Title:       v.Title,
		My:          creatorID == userID,
		Closed:      v.Closed,
		Pinned:      v.Pinned,
		Hidden:      v.Hidden,
		Date:        v.Date,
		Peer:        peer,
		IconEmojiId: mtproto.MakeFlagsInt64(v.IconEmojiID),
		TopMessage:  v.TopMessage,
		FromId:      mtproto.MakePeerUser(creatorID),
		NotifySettings: mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{}).
			To_PeerNotifySettings(),
	}).To_ForumTopic()
}

func forumPut(key string, userID int64, title string) error {
	return domain.CreateForumTopic(key, userID, title)
}

func forumList(key string, userID int64, q string, limit int32, ids []int32, byID bool) ([]*mtproto.ForumTopic, error) {
	topics, err := domain.ListForumTopics(key)
	if err != nil {
		return nil, err
	}
	peer, err := forumPeerFromKey(key)
	if err != nil {
		return nil, err
	}
	want := map[int32]struct{}{}
	for _, id := range ids {
		want[id] = struct{}{}
	}
	out := make([]*mtproto.ForumTopic, 0)
	for _, t := range topics {
		if byID {
			if _, ok := want[t.ID]; !ok {
				continue
			}
		}
		if q != "" && !strings.Contains(t.Title, q) {
			continue
		}
		out = append(out, forumTopicFromDomain(peer, t, userID))
		if limit > 0 && int32(len(out)) >= limit {
			break
		}
	}
	return out, nil
}

func forumUpdateTopic(key string, id int32, update func(*domain.ForumTopic) error) error {
	return domain.UpdateForumTopic(key, id, update)
}

func forumEditTopic(key string, id int32, title *wrapperspb.StringValue, iconEmojiID *wrapperspb.Int64Value, closed, hidden *mtproto.Bool) error {
	return forumUpdateTopic(key, id, func(topic *domain.ForumTopic) error {
		if title != nil {
			if title.GetValue() == "" {
				return mtproto.ErrTopicTitleEmpty
			}
			topic.Title = title.GetValue()
		}
		if iconEmojiID != nil {
			topic.IconEmojiID = iconEmojiID.GetValue()
		}
		if closed != nil {
			topic.Closed = mtproto.FromBool(closed)
		}
		if hidden != nil {
			topic.Hidden = mtproto.FromBool(hidden)
		}
		return nil
	})
}

func forumSetPinned(key string, id int32, pinned bool) error {
	return forumUpdateTopic(key, id, func(topic *domain.ForumTopic) error {
		topic.Pinned = pinned
		return nil
	})
}

func forumReorder(key string, order []int32) error {
	return domain.ReorderForumTopics(key, order)
}

func forumDelete(key string, id int32) (bool, error) {
	return domain.DeleteForumTopic(key, id)
}

func forumTopicsReply(topics []*mtproto.ForumTopic) *mtproto.Messages_ForumTopics {
	if topics == nil {
		topics = []*mtproto.ForumTopic{}
	}
	return mtproto.MakeTLMessagesForumTopics(&mtproto.Messages_ForumTopics{
		Count:    int32(len(topics)),
		Topics:   topics,
		Messages: []*mtproto.Message{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_ForumTopics()
}

func forumAffected(deleted bool) *mtproto.Messages_AffectedHistory {
	pts := int32(0)
	if deleted {
		pts = 1
	}
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      pts,
		PtsCount: pts,
	}).To_Messages_AffectedHistory()
}

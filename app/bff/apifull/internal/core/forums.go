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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
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
	if err := persist.Default.Set(forumUserKey(uid), in.GetTitle()); err != nil {
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
	key, err := c.forumChannelKey(uid, in.GetChannel(), true)
	if err != nil {
		return nil, err
	}
	if err := updateForumSettings(key, func(settings *forumChannelSettings) {
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
	key, err := c.forumChannelKey(uid, in.GetChannel(), true)
	if err != nil {
		return nil, err
	}
	enabled := mtproto.FromBool(in.GetEnabled())
	if err := updateForumSettings(key, func(settings *forumChannelSettings) {
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
	if err := persist.Default.Set(forumUserKey(uid), in.GetTitle()); err != nil {
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

// Forum topics are JSON blobs under forum:. persist.Default is MySQL in tests.

const forumKeyPrefix = "forum:"

type forumTopicJSON struct {
	Id          int32  `json:"id"`
	Title       string `json:"title"`
	My          bool   `json:"my"`
	Closed      bool   `json:"closed"`
	Pinned      bool   `json:"pinned"`
	Hidden      bool   `json:"hidden"`
	Date        int32  `json:"date"`
	CreatorID   int64  `json:"creatorId"`
	IconEmojiID int64  `json:"iconEmojiId"`
	TopMessage  int32  `json:"topMessage"`
}

func forumUserKey(uid int64) string {
	return fmt.Sprintf("%s%d", forumKeyPrefix, uid)
}

func forumStoreKey(key string) string {
	return forumKeyPrefix + key
}

// forumWithUserTitle fills an empty list from the title stored at forum:<userId>.
func forumWithUserTitle(uid int64, key string, topics []*mtproto.ForumTopic) ([]*mtproto.ForumTopic, error) {
	title, err := persist.Default.Get(forumUserKey(uid))
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
		return []*mtproto.ForumTopic{forumTopicFromJSON(peer, forumTopicJSON{Title: title, My: true, CreatorID: uid}, uid)}, nil
	}
	for _, t := range topics {
		if t != nil && t.GetTitle() == "" {
			t.Title = title
		}
	}
	return topics, nil
}

func forumClearUserTitleIfEmpty(uid int64, key string) error {
	left, err := loadForumTopics(key)
	if err != nil || len(left) > 0 {
		return err
	}
	return persist.Default.Set(forumUserKey(uid), "")
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

func loadForumTopics(key string) ([]forumTopicJSON, error) {
	raw, err := persist.Default.Get(forumStoreKey(key))
	if err != nil || raw == "" {
		return nil, err
	}
	var stored []forumTopicJSON
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, err
	}
	return stored, nil
}

func saveForumTopics(key string, topics []forumTopicJSON) error {
	b, err := json.Marshal(topics)
	if err != nil {
		return err
	}
	return persist.Default.Set(forumStoreKey(key), string(b))
}

func nextForumSeq() (int32, error) {
	raw, err := persist.Default.Get(forumKeyPrefix + "seq")
	if err != nil {
		return 0, err
	}
	var seq int32
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &seq); err != nil {
			return 0, err
		}
	}
	seq++
	b, err := json.Marshal(seq)
	if err != nil {
		return 0, err
	}
	if err := persist.Default.Set(forumKeyPrefix+"seq", string(b)); err != nil {
		return 0, err
	}
	return seq, nil
}

func forumTopicFromJSON(peer *mtproto.Peer, v forumTopicJSON, userID int64) *mtproto.ForumTopic {
	creatorID := v.CreatorID
	if creatorID == 0 {
		creatorID = userID
	}
	my := v.My
	if v.CreatorID != 0 {
		my = v.CreatorID == userID
	}
	return mtproto.MakeTLForumTopic(&mtproto.ForumTopic{
		Id:          v.Id,
		Title:       v.Title,
		My:          my,
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
	topics, err := loadForumTopics(key)
	if err != nil {
		return err
	}
	seq, err := nextForumSeq()
	if err != nil {
		return err
	}
	topics = append(topics, forumTopicJSON{
		Id:         seq,
		Title:      title,
		My:         true,
		Date:       int32(time.Now().Unix()),
		CreatorID:  userID,
		TopMessage: seq,
	})
	return saveForumTopics(key, topics)
}

func forumList(key string, userID int64, q string, limit int32, ids []int32, byID bool) ([]*mtproto.ForumTopic, error) {
	topics, err := loadForumTopics(key)
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
			if _, ok := want[t.Id]; !ok {
				continue
			}
		}
		if q != "" && !strings.Contains(t.Title, q) {
			continue
		}
		out = append(out, forumTopicFromJSON(peer, t, userID))
		if limit > 0 && int32(len(out)) >= limit {
			break
		}
	}
	return out, nil
}

func forumUpdateTopic(key string, id int32, update func(*forumTopicJSON) error) error {
	if id == 0 {
		return mtproto.ErrTopicIdInvalid
	}
	topics, err := loadForumTopics(key)
	if err != nil {
		return err
	}
	for i := range topics {
		if topics[i].Id != id {
			continue
		}
		if err := update(&topics[i]); err != nil {
			return err
		}
		return saveForumTopics(key, topics)
	}
	return mtproto.ErrTopicIdInvalid
}

func forumEditTopic(key string, id int32, title *wrapperspb.StringValue, iconEmojiID *wrapperspb.Int64Value, closed, hidden *mtproto.Bool) error {
	return forumUpdateTopic(key, id, func(topic *forumTopicJSON) error {
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
	return forumUpdateTopic(key, id, func(topic *forumTopicJSON) error {
		topic.Pinned = pinned
		return nil
	})
}

func forumReorder(key string, order []int32) error {
	topics, err := loadForumTopics(key)
	if err != nil {
		return err
	}
	byID := make(map[int32]forumTopicJSON, len(topics))
	for _, topic := range topics {
		byID[topic.Id] = topic
	}
	reordered := make([]forumTopicJSON, 0, len(topics))
	seen := make(map[int32]struct{}, len(order))
	for _, id := range order {
		topic, ok := byID[id]
		if !ok {
			return mtproto.ErrTopicIdInvalid
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		reordered = append(reordered, topic)
	}
	for _, topic := range topics {
		if _, ordered := seen[topic.Id]; !ordered {
			reordered = append(reordered, topic)
		}
	}
	return saveForumTopics(key, reordered)
}

type forumChannelSettings struct {
	Enabled        bool `json:"enabled"`
	Tabs           bool `json:"tabs"`
	ViewAsMessages bool `json:"viewAsMessages"`
}

func loadForumSettings(key string) (forumChannelSettings, error) {
	raw, err := persist.Default.Get(forumStoreKey(key) + ":settings")
	if err != nil || raw == "" {
		return forumChannelSettings{}, err
	}
	var settings forumChannelSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return forumChannelSettings{}, err
	}
	return settings, nil
}

func updateForumSettings(key string, update func(*forumChannelSettings)) error {
	settings, err := loadForumSettings(key)
	if err != nil {
		return err
	}
	update(&settings)
	b, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return persist.Default.Set(forumStoreKey(key)+":settings", string(b))
}

func forumDelete(key string, id int32) (bool, error) {
	topics, err := loadForumTopics(key)
	if err != nil {
		return false, err
	}
	for i, t := range topics {
		if t.Id == id {
			topics = append(topics[:i], topics[i+1:]...)
			return true, saveForumTopics(key, topics)
		}
	}
	return false, nil
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

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
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RPCMessageThreadsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

const discussionReadKeyPrefix = "discussion:read:"

var discussionReadMu sync.Mutex

func discussionReadKey(userID int64, peer *mtproto.InputPeer, msgID int32) string {
	peerType, peerID := apifullPeerTypeID(userID, peer)
	return fmt.Sprintf("%s%d:%d:%d:%d", discussionReadKeyPrefix, userID, peerType, peerID, msgID)
}

func loadDiscussionReadMaxID(key string) (int32, error) {
	raw, err := persist.Default.Get(key)
	if err != nil || raw == "" {
		return 0, err
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(value), nil
}

func (c *ApiFullCore) ContactsBlockFromReplies(in *mtproto.TLContactsBlockFromReplies) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if in.GetMsgId() <= 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.PollMessageReader == nil ||
		c.svcCtx.Dao.UserClient == nil || c.svcCtx.Dao.SyncClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	box, err := c.svcCtx.Dao.PollMessageReader.MessageGetUserMessage(c.ctx, &messagepb.TLMessageGetUserMessage{
		UserId: uid,
		Id:     in.GetMsgId(),
	})
	if err != nil {
		return nil, err
	}
	if box == nil || box.GetSenderUserId() <= 0 || box.GetSenderUserId() == uid ||
		box.GetPeerType() != mtproto.PEER_USER || box.GetPeerId() != box.GetSenderUserId() {
		return nil, mtproto.ErrMessageIdInvalid
	}
	senderID := box.GetSenderUserId()
	if (in.GetDeleteMessage() || in.GetDeleteHistory()) && c.svcCtx.Dao.MessageMutator == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	users, err := c.svcCtx.Dao.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: []int64{uid, senderID}})
	if err != nil {
		return nil, err
	}
	if users == nil {
		return nil, mtproto.ErrContactIdInvalid
	}
	if sender, ok := users.GetImmutableUser(senderID); !ok || sender == nil || sender.GetUser() == nil || sender.GetUser().GetDeleted() {
		return nil, mtproto.ErrContactIdInvalid
	}
	if in.GetReportSpam() {
		if !domain.Ready() {
			return nil, mtproto.ErrMethodNotImpl
		}
		if err = c.recordReport(uid, "contacts.blockFromReplies", reportTarget{typ: "user", id: senderID}, in); err != nil {
			return nil, err
		}
	}
	blocked, err := c.svcCtx.Dao.UserBlockPeer(c.ctx, &userpb.TLUserBlockPeer{
		UserId:   uid,
		PeerType: mtproto.PEER_USER,
		PeerId:   senderID,
	})
	if err != nil {
		return nil, err
	}
	if !mtproto.FromBool(blocked) {
		return nil, mtproto.ErrInternalServerError
	}

	if in.GetDeleteMessage() {
		if _, err = c.svcCtx.Dao.MessageMutator.MsgDeleteMessages(c.ctx, &msgpb.TLMsgDeleteMessages{
			UserId:    uid,
			AuthKeyId: c.MD.PermAuthKeyId,
			PeerType:  mtproto.PEER_EMPTY,
			PeerId:    0,
			Revoke:    true,
			Id:        []int32{in.GetMsgId()},
		}); err != nil {
			return nil, err
		}
	}
	if in.GetDeleteHistory() {
		if _, err = c.svcCtx.Dao.MessageMutator.MsgDeleteHistory(c.ctx, &msgpb.TLMsgDeleteHistory{
			UserId:    uid,
			AuthKeyId: c.MD.PermAuthKeyId,
			PeerType:  mtproto.PEER_USER,
			PeerId:    senderID,
			Revoke:    true,
		}); err != nil {
			return nil, err
		}
	}

	syncUpdates := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdatePeerBlocked(&mtproto.Update{
		Blocked_BOOL:        mtproto.BoolTrue,
		Blocked_FLAGBOOLEAN: true,
		PeerId:              mtproto.MakePeerUser(senderID),
	}).To_Update())
	syncUpdates.Users = users.GetUserListByIdList(uid, senderID)
	if _, err = c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &syncpb.TLSyncUpdatesNotMe{
		UserId:        uid,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       syncUpdates,
	}); err != nil {
		return nil, err
	}
	return syncUpdates, nil
}

func validThreadInputPeer(selfID int64, peer *mtproto.InputPeer) bool {
	if peer == nil {
		return false
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		return selfID > 0
	case mtproto.Predicate_inputPeerUser:
		return peer.GetUserId() > 0
	case mtproto.Predicate_inputPeerChat:
		return peer.GetChatId() > 0
	case mtproto.Predicate_inputPeerChannel:
		return peer.GetChannelId() > 0
	case mtproto.Predicate_inputPeerUserFromMessage:
		return peer.GetUserId() > 0 && peer.GetMsgId() > 0 && validThreadInputPeer(selfID, peer.GetPeer())
	case mtproto.Predicate_inputPeerChannelFromMessage:
		return peer.GetChannelId() > 0 && peer.GetMsgId() > 0 && validThreadInputPeer(selfID, peer.GetPeer())
	case mtproto.Predicate_inputPeerUsername:
		return peer.GetUsername() != ""
	case "":
		peerType, peerID := apifullPeerTypeID(selfID, peer)
		return peerID > 0 && (peerType == mtproto.PEER_SELF || peerType == mtproto.PEER_USER || peerType == mtproto.PEER_CHAT || peerType == mtproto.PEER_CHANNEL)
	default:
		return false
	}
}

func validateThreadRequest(selfID int64, peer *mtproto.InputPeer, msgID int32) error {
	if !validThreadInputPeer(selfID, peer) {
		return mtproto.ErrPeerIdInvalid
	}
	if msgID <= 0 {
		return mtproto.ErrMsgIdInvalid
	}
	return nil
}

func (c *ApiFullCore) MessagesGetReplies(in *mtproto.TLMessagesGetReplies) (*mtproto.Messages_Messages, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if err = validateThreadRequest(uid, in.GetPeer(), in.GetMsgId()); err != nil {
		return nil, err
	}
	if in.GetLimit() <= 0 || in.GetLimit() > 100 || in.GetAddOffset() < 0 || in.GetOffsetId() < 0 || in.GetOffsetDate() < 0 || in.GetMaxId() < 0 || in.GetMinId() < 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	peer := mtproto.FromInputPeer2(uid, in.GetPeer())
	if peer == nil || peer.PeerType == mtproto.PEER_EMPTY || peer.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.PeerType == mtproto.PEER_CHANNEL {
		return c.channelReplies(in)
	}
	if _, err = c.loadThreadRoot(peer.PeerType, peer.PeerId, in.GetMsgId()); err != nil {
		return nil, err
	}
	boxes, err := c.loadThreadReplies(peer.PeerType, peer.PeerId, in.GetMsgId(), in.GetOffsetId(), in.GetOffsetDate(), in.GetAddOffset(), in.GetLimit(), in.GetMaxId(), in.GetMinId())
	if err != nil {
		return nil, err
	}
	return c.threadMessagesResult(uid, boxes, in.GetLimit())
}

func (c *ApiFullCore) MessagesGetDiscussionMessage(in *mtproto.TLMessagesGetDiscussionMessage) (*mtproto.Messages_DiscussionMessage, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if err = validateThreadRequest(uid, in.GetPeer(), in.GetMsgId()); err != nil {
		return nil, err
	}
	peer := mtproto.FromInputPeer2(uid, in.GetPeer())
	if peer == nil || peer.PeerType == mtproto.PEER_EMPTY || peer.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.PeerType == mtproto.PEER_CHANNEL {
		return c.channelDiscussionMessage(in)
	}
	root, err := c.loadThreadRoot(peer.PeerType, peer.PeerId, in.GetMsgId())
	if err != nil {
		return nil, err
	}
	message := root.ToMessage(uid)
	readMax, err := loadDiscussionReadMaxID(discussionReadKey(uid, in.GetPeer(), in.GetMsgId()))
	if err != nil {
		return nil, err
	}
	users, chats, err := c.hydrateThreadMessages(uid, []*mtproto.Message{message})
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesDiscussionMessage(&mtproto.Messages_DiscussionMessage{
		Messages:        []*mtproto.Message{message},
		MaxId:           wrapperspb.Int32(in.GetMsgId()),
		ReadInboxMaxId:  wrapperspb.Int32(readMax),
		ReadOutboxMaxId: wrapperspb.Int32(readMax),
		UnreadCount:     0,
		Chats:           chats,
		Users:           users,
	}).To_Messages_DiscussionMessage(), nil
}

// channelReplyTarget resolves a broadcast channel's linked discussion group.
// When no group is linked, replies are kept in the channel's native message
// store, which also makes the method useful for megagroups and local tests.
func channelReplyTarget(channelID int64) (int64, error) {
	ch, ok, err := domain.LoadChannel(channelID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, mtproto.ErrChannelInvalid
	}
	if ch.DiscussionGroupID > 0 {
		return ch.DiscussionGroupID, nil
	}
	return channelID, nil
}

func (c *ApiFullCore) channelReplies(in *mtproto.TLMessagesGetReplies) (*mtproto.Messages_Messages, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetPeer().GetPredicateName() != mtproto.Predicate_inputPeerChannel {
		return nil, mtproto.ErrChannelInvalid
	}
	channelID, err := channelview.ValidateInputPeer(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if _, err = channelview.MessagesBox(uid, channelID, []int32{in.GetMsgId()}); err != nil {
		return nil, err
	}
	targetID, err := channelReplyTarget(channelID)
	if err != nil {
		return nil, err
	}
	rows, count, err := domain.ChannelMessageReplies(uid, targetID, in.GetMsgId(), in.GetOffsetId(), in.GetOffsetDate(), in.GetAddOffset(), in.GetMinId(), in.GetMaxId(), in.GetLimit())
	if err != nil {
		return nil, err
	}
	ids := make([]int32, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.MessageID)
	}
	box, err := channelview.MessagesBox(uid, targetID, ids)
	if err != nil {
		return nil, err
	}
	if count > int32(len(ids)) {
		return mtproto.MakeTLMessagesMessagesSlice(&mtproto.Messages_Messages{
			Inexact: true, Count: count, Messages: box.GetMessages(), Users: box.GetUsers(), Chats: box.GetChats(),
		}).To_Messages_Messages(), nil
	}
	return box, nil
}

func (c *ApiFullCore) channelDiscussionMessage(in *mtproto.TLMessagesGetDiscussionMessage) (*mtproto.Messages_DiscussionMessage, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetPeer() == nil || in.GetPeer().GetPredicateName() != mtproto.Predicate_inputPeerChannel {
		return nil, mtproto.ErrChannelInvalid
	}
	channelID, err := channelview.ValidateInputPeer(uid, in.GetPeer())
	if err != nil {
		return nil, err
	}
	box, err := channelview.MessagesBox(uid, channelID, []int32{in.GetMsgId()})
	if err != nil {
		return nil, err
	}
	if len(box.GetMessages()) != 1 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	targetID, err := channelReplyTarget(channelID)
	if err != nil {
		return nil, err
	}
	maxID, err := domain.TopChannelMessage(targetID)
	if err != nil {
		return nil, err
	}
	if maxID == 0 {
		maxID = in.GetMsgId()
	}
	readMax, err := loadDiscussionReadMaxID(discussionReadKey(uid, in.GetPeer(), in.GetMsgId()))
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesDiscussionMessage(&mtproto.Messages_DiscussionMessage{
		Messages:        box.GetMessages(),
		MaxId:           wrapperspb.Int32(maxID),
		ReadInboxMaxId:  wrapperspb.Int32(readMax),
		ReadOutboxMaxId: wrapperspb.Int32(readMax),
		UnreadCount:     0,
		Chats:           box.GetChats(),
		Users:           box.GetUsers(),
	}).To_Messages_DiscussionMessage(), nil
}

// loadThreadRoot reads the requested message from the message service and
// verifies that it belongs to the exact peer supplied by the caller. The
// message service is the source of truth for ordinary users and basic groups.
func (c *ApiFullCore) loadThreadRoot(peerType int32, peerID int64, messageID int32) (*mtproto.MessageBox, error) {
	d := c.apifullDao()
	if d == nil || d.PollMessageReader == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	box, err := d.PollMessageReader.MessageGetUserMessage(c.ctx, &messagepb.TLMessageGetUserMessage{UserId: uid, Id: messageID})
	if err != nil {
		return nil, err
	}
	if box == nil || box.GetMessage() == nil || box.GetMessageId() != messageID || box.GetPeerType() != peerType || box.GetPeerId() != peerID {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return box, nil
}

func (c *ApiFullCore) loadThreadReplies(peerType int32, peerID int64, rootID, offsetID, offsetDate, addOffset, limit, maxID, minID int32) ([]*mtproto.MessageBox, error) {
	d := c.apifullDao()
	if d == nil || d.MessageHistoryReader == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	pageLimit := int32(100)
	if limit > pageLimit {
		pageLimit = limit
	}
	if pageLimit > 100 {
		pageLimit = 100
	}
	result := make([]*mtproto.MessageBox, 0, limit)
	nextOffset := offsetID
	lastMinID := int32(0)
	for pageIndex := 0; pageIndex < 32 && int32(len(result)) < limit; pageIndex++ {
		historyPage, err := d.MessageHistoryReader.MessageGetHistoryMessages(c.ctx, &messagepb.TLMessageGetHistoryMessages{
			UserId: uid, PeerType: peerType, PeerId: peerID,
			OffsetId: nextOffset, OffsetDate: offsetDate, AddOffset: addOffset,
			Limit: pageLimit, MaxId: maxID, MinId: minID,
		})
		if err != nil {
			return nil, err
		}
		if historyPage == nil || len(historyPage.GetDatas()) == 0 {
			break
		}
		minSeen := int32(0)
		for _, box := range historyPage.GetDatas() {
			if box == nil || box.GetMessage() == nil || box.GetPeerType() != peerType || box.GetPeerId() != peerID {
				continue
			}
			if !messageBoxRepliesTo(box, rootID) {
				continue
			}
			if int32(len(result)) < limit {
				result = append(result, box)
			}
			if minSeen == 0 || box.GetMessageId() < minSeen {
				minSeen = box.GetMessageId()
			}
		}
		if int32(len(historyPage.GetDatas())) < pageLimit || minSeen == 0 || minSeen == lastMinID {
			break
		}
		lastMinID = minSeen
		nextOffset = minSeen
		addOffset = 0
	}
	return result, nil
}

func messageBoxRepliesTo(box *mtproto.MessageBox, rootID int32) bool {
	if box == nil || rootID <= 0 {
		return false
	}
	if box.GetReplyToMsgId() == rootID || box.GetReplyToTopId() == rootID {
		return true
	}
	replyTo := box.GetMessage().GetReplyTo()
	return replyTo != nil && (replyTo.GetReplyToMsgId() == rootID || replyTo.GetReplyToTopId().GetValue() == rootID)
}

func (c *ApiFullCore) threadMessagesResult(userID int64, boxes []*mtproto.MessageBox, limit int32) (*mtproto.Messages_Messages, error) {
	messages := make([]*mtproto.Message, 0, len(boxes))
	for _, box := range boxes {
		if box == nil || box.GetMessage() == nil {
			continue
		}
		messages = append(messages, box.ToMessage(userID))
	}
	users, chats, err := c.hydrateThreadMessages(userID, messages)
	if err != nil {
		return nil, err
	}
	if int32(len(messages)) == limit {
		return mtproto.MakeTLMessagesMessagesSlice(&mtproto.Messages_Messages{
			Inexact: true, Count: int32(len(messages)), Messages: messages, Users: users, Chats: chats,
		}).To_Messages_Messages(), nil
	}
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: messages, Users: users, Chats: chats,
	}).To_Messages_Messages(), nil
}

func (c *ApiFullCore) hydrateThreadMessages(userID int64, messages []*mtproto.Message) ([]*mtproto.User, []*mtproto.Chat, error) {
	userIDs := make(map[int64]struct{})
	chatIDs := make(map[int64]struct{})
	channelIDs := make(map[int64]struct{})
	for _, message := range messages {
		if message == nil {
			continue
		}
		if from := message.GetFromId(); from != nil && from.GetUserId() > 0 {
			userIDs[from.GetUserId()] = struct{}{}
		}
		if peer := message.GetPeerId(); peer != nil {
			switch {
			case peer.GetUserId() > 0:
				userIDs[peer.GetUserId()] = struct{}{}
			case peer.GetChatId() > 0:
				chatIDs[peer.GetChatId()] = struct{}{}
			case peer.GetChannelId() > 0:
				channelIDs[peer.GetChannelId()] = struct{}{}
			}
		}
	}
	d := c.apifullDao()
	if d == nil {
		return nil, nil, mtproto.ErrMethodNotImpl
	}
	users := make([]*mtproto.User, 0, len(userIDs))
	if len(userIDs) > 0 && d.UserClient != nil {
		ids := make([]int64, 0, len(userIDs))
		for id := range userIDs {
			ids = append(ids, id)
		}
		mutable, err := d.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: ids})
		if err != nil {
			return nil, nil, err
		}
		if mutable != nil {
			users = append(users, mutable.GetUserListByIdList(userID, ids...)...)
		}
	}
	chats := make([]*mtproto.Chat, 0, len(chatIDs)+len(channelIDs))
	if len(chatIDs) > 0 && d.ChatClient != nil {
		ids := make([]int64, 0, len(chatIDs))
		for id := range chatIDs {
			ids = append(ids, id)
		}
		mutable, err := d.ChatClient.ChatGetChatListByIdList(c.ctx, &chatpb.TLChatGetChatListByIdList{IdList: ids})
		if err != nil {
			return nil, nil, err
		}
		if mutable != nil {
			chats = append(chats, mutable.GetChatListByIdList(userID, ids...)...)
		}
	}
	if len(channelIDs) > 0 {
		ids := make([]int64, 0, len(channelIDs))
		for id := range channelIDs {
			ids = append(ids, id)
		}
		chats = append(chats, channelview.ChatsByID(userID, ids)...)
	}
	return mtproto.ToSafeUsers(users), mtproto.ToSafeChats(chats), nil
}

func (c *ApiFullCore) MessagesReadDiscussion(in *mtproto.TLMessagesReadDiscussion) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if err = validateThreadRequest(uid, in.GetPeer(), in.GetMsgId()); err != nil {
		return nil, err
	}
	if in.GetReadMaxId() < 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	peerType, peerID := apifullPeerTypeID(uid, in.GetPeer())
	if peerType == mtproto.PEER_EMPTY || peerType == mtproto.PEER_UNKNOWN || peerID <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}

	key := discussionReadKey(uid, in.GetPeer(), in.GetMsgId())
	discussionReadMu.Lock()
	defer discussionReadMu.Unlock()
	current, err := loadDiscussionReadMaxID(key)
	if err != nil {
		return nil, err
	}
	if in.GetReadMaxId() > current {
		if err = persist.Default.Set(key, strconv.FormatInt(int64(in.GetReadMaxId()), 10)); err != nil {
			return nil, err
		}
	}
	return mtproto.BoolTrue, nil
}

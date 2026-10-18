// Copyright 2022 Teamgram Authors
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
	"time"

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// MessagesSendScreenshotNotification
// messages.sendScreenshotNotification#c97df020 peer:InputPeer reply_to_msg_id:int random_id:long = Updates;
func (c *DialogsCore) MessagesSendScreenshotNotification(in *mtproto.TLMessagesSendScreenshotNotification) (*mtproto.Updates, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		c.Logger.Errorf("messages.sendScreenshotNotification - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetRandomId() == 0 {
		return nil, mtproto.ErrRandomIdEmpty
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	if peer == nil || peer.PeerId == 0 {
		c.Logger.Errorf("messages.sendScreenshotNotification - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
	default:
		c.Logger.Errorf("messages.sendScreenshotNotification - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}
	if err := c.validateScreenshotPeer(peer, in.GetPeer()); err != nil {
		return nil, err
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil ||
		c.svcCtx.Dao.MessageClient == nil || c.svcCtx.Dao.MsgClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	replyID, topID, err := screenshotReplyTarget(in)
	if err != nil {
		return nil, err
	}
	if err = c.validateScreenshotReply(peer, replyID); err != nil {
		return nil, err
	}

	peerType := peer.PeerType
	if peerType == mtproto.PEER_SELF {
		peerType = mtproto.PEER_USER
		peer = mtproto.MakePeerUtil(peerType, c.MD.UserId)
	}
	message := mtproto.MakeTLMessageService(&mtproto.Message{
		Out:    true,
		FromId: mtproto.MakePeerUser(c.MD.UserId),
		PeerId: peer.ToPeer(),
		Date:   int32(time.Now().Unix()),
		ReplyTo: &mtproto.MessageReplyHeader{
			ReplyToMsgId_INT32: replyID,
			ReplyToMsgId:       replyID,
			ReplyToTopId:       topID,
		},
		Action: mtproto.MakeMessageActionScreenshotTaken(),
	}).To_Message()
	updates, err := c.svcCtx.Dao.MsgClient.MsgSendMessageV2(c.ctx, &msgpb.TLMsgSendMessageV2{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  peerType,
		PeerId:    peer.PeerId,
		Message: []*msgpb.OutboxMessage{msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			RandomId: in.GetRandomId(),
			Message:  message,
		}).To_OutboxMessage()},
	})
	if err != nil {
		return nil, err
	}
	if updates == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return updates, nil
}

func screenshotReplyTarget(in *mtproto.TLMessagesSendScreenshotNotification) (int32, *wrapperspb.Int32Value, error) {
	if in == nil {
		return 0, nil, mtproto.ErrInputRequestInvalid
	}
	replyTo := in.GetReplyTo()
	if replyTo == nil {
		if in.GetReplyToMsgId() <= 0 {
			return 0, nil, mtproto.ErrInputRequestInvalid
		}
		return in.GetReplyToMsgId(), nil, nil
	}
	if replyTo.GetReplyToMsgId() <= 0 {
		return 0, nil, mtproto.ErrMessageIdInvalid
	}
	if replyTo.GetReplyToPeerId() != nil || replyTo.GetQuoteText() != nil || len(replyTo.GetQuoteEntities()) > 0 || replyTo.GetQuoteOffset() != nil {
		return 0, nil, mtproto.ErrMethodNotImpl
	}
	return replyTo.GetReplyToMsgId(), replyTo.GetTopMsgId(), nil
}

func (c *DialogsCore) validateScreenshotReply(peer *mtproto.PeerUtil, replyID int32) error {
	if peer == nil || replyID <= 0 || c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return mtproto.ErrInternalServerError
	}
	boxes, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(c.ctx, &messagepb.TLMessageGetUserMessageList{
		UserId: c.MD.UserId,
		IdList: []int32{replyID},
	})
	if err != nil {
		return err
	}
	if boxes == nil {
		return mtproto.ErrInternalServerError
	}
	expectedType := peer.PeerType
	if expectedType == mtproto.PEER_SELF {
		expectedType = mtproto.PEER_USER
	}
	for _, box := range boxes.GetDatas() {
		if box == nil || box.GetMessage() == nil || box.GetMessageId() != replyID || box.GetMessage().GetId() != replyID {
			continue
		}
		boxType := box.GetPeerType()
		if boxType == mtproto.PEER_SELF {
			boxType = mtproto.PEER_USER
		}
		if boxType == expectedType && box.GetPeerId() == peer.PeerId {
			return nil
		}
	}
	return mtproto.ErrMessageIdInvalid
}

func (c *DialogsCore) validateScreenshotPeer(peer *mtproto.PeerUtil, input *mtproto.InputPeer) error {
	if peer == nil || input == nil || peer.PeerId <= 0 {
		return mtproto.ErrPeerIdInvalid
	}

	switch peer.PeerType {
	case mtproto.PEER_SELF:
		return nil
	case mtproto.PEER_USER:
		if input.GetUserId() <= 0 || input.GetAccessHash() == 0 || c.svcCtx.Dao.UserClient == nil {
			return mtproto.ErrUserIdInvalid
		}
		users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: []int64{peer.PeerId},
			To: []int64{c.MD.UserId},
		})
		if err != nil {
			return err
		}
		if users == nil {
			return mtproto.ErrInternalServerError
		}
		for _, user := range users.GetDatas() {
			if user == nil || user.GetUser() == nil {
				continue
			}
			data := user.GetUser()
			if data.GetId() == peer.PeerId && !data.GetDeleted() && data.GetAccessHash() == input.GetAccessHash() {
				return nil
			}
		}
		return mtproto.ErrUserIdInvalid
	case mtproto.PEER_CHAT:
		if input.GetChatId() <= 0 || c.svcCtx.Dao.ChatClient == nil {
			return mtproto.ErrChatIdInvalid
		}
		chat, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{ChatId: peer.PeerId})
		if err != nil {
			return err
		}
		if chat == nil || chat.GetChat() == nil || chat.GetChat().GetId() != peer.PeerId {
			return mtproto.ErrChatIdInvalid
		}
		actorIsMember := false
		for _, participant := range chat.GetChatParticipants() {
			if participant == nil || participant.GetUserId() <= 0 {
				return mtproto.ErrInternalServerError
			}
			if !participant.IsChatMemberStateNormal() {
				continue
			}
			userID := participant.GetUserId()
			if userID == c.MD.UserId {
				actorIsMember = true
				continue
			}
		}
		if !actorIsMember {
			return mtproto.ErrUserNotParticipant
		}
		return nil
	case mtproto.PEER_CHANNEL:
		if input.GetChannelId() <= 0 || input.GetAccessHash() == 0 {
			return mtproto.ErrChannelInvalid
		}
		return mtproto.ErrMethodNotImpl
	default:
		return mtproto.ErrPeerIdInvalid
	}
}

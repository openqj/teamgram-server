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
	"github.com/teamgram/proto/mtproto"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessagesGetMessageReadParticipants31C1C44F
// messages.getMessageReadParticipants#31c1c44f peer:InputPeer msg_id:int = Vector<ReadParticipantDate>;
func (c *ChatsCore) MessagesGetMessageReadParticipants31C1C44F(in *mtproto.TLMessagesGetMessageReadParticipants31C1C44F) (*mtproto.Vector_ReadParticipantDate, error) {
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	var (
		peer = mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	)
	if err := validateMessageReadParticipantsPeer(peer); err != nil {
		return nil, err
	}

	switch peer.PeerType {
	case mtproto.PEER_CHANNEL:
		if err := validateMessageReadParticipantsDependencies(c, peer.PeerType); err != nil {
			return nil, err
		}
		msgBox, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessage(c.ctx, &message.TLMessageGetUserMessage{
			UserId: c.MD.UserId,
			Id:     in.MsgId,
		})
		if err != nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", err)
			return nil, err
		} else if msgBox == nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - empty message reply")
			return nil, mtproto.ErrInternalServerError
		}
		if err := validateReadParticipantChannelMessage(c.MD.UserId, peer.PeerId, in.MsgId, msgBox); err != nil {
			return nil, err
		}

		callerDialog, err := c.svcCtx.Dao.DialogClient.DialogGetDialogById(c.ctx, &dialog.TLDialogGetDialogById{
			UserId:   c.MD.UserId,
			PeerType: mtproto.PEER_CHANNEL,
			PeerId:   peer.PeerId,
		})
		if err != nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", err)
			return nil, err
		} else if err := validateReadParticipantCallerDialog(callerDialog, peer.PeerId); err != nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - empty caller dialog")
			return nil, err
		}

		participants, err := c.svcCtx.Dao.DialogClient.DialogGetChannelMessageReadParticipants(c.ctx, &dialog.TLDialogGetChannelMessageReadParticipants{
			UserId:    c.MD.UserId,
			ChannelId: peer.PeerId,
			MsgId:     in.MsgId,
		})
		if err != nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", err)
			return nil, err
		} else if participants == nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - empty channel participant reply")
			return nil, mtproto.ErrInternalServerError
		}
		return buildMessageReadParticipantDates(participants.GetDatas())
	case mtproto.PEER_CHAT:
		if err := validateMessageReadParticipantsDependencies(c, peer.PeerType); err != nil {
			return nil, err
		}
		msgBox, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessage(c.ctx, &message.TLMessageGetUserMessage{
			UserId: c.MD.UserId,
			Id:     in.MsgId,
		})
		if err != nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", err)
			return nil, err
		} else if msgBox == nil {
			err = mtproto.ErrInternalServerError
			c.Logger.Errorf("messages.getMessageReadParticipants - empty message reply")
			return nil, err
		}
		if err := validateReadParticipantMessage(c.MD.UserId, peer.PeerId, in.MsgId, msgBox); err != nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - message does not belong to requested peer: %v", err)
			return nil, err
		}

		chat, err := c.svcCtx.Dao.ChatClient.Client().ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
			ChatId: peer.PeerId,
		})
		if err != nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", err)
			return nil, err
		} else if chat == nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - empty chat reply")
			return nil, mtproto.ErrInternalServerError
		}
		me, ok := chat.GetImmutableChatParticipant(c.MD.UserId)
		if !ok || me == nil || !me.IsChatMemberStateNormal() {
			return nil, mtproto.ErrPeerIdInvalid
		}

		pIdList, err := c.svcCtx.Dao.ChatClient.Client().ChatGetChatParticipantIdList(c.ctx, &chatpb.TLChatGetChatParticipantIdList{
			ChatId: peer.PeerId,
		})
		if err != nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", err)
			return nil, err
		} else if pIdList == nil {
			err = mtproto.ErrInternalServerError
			c.Logger.Errorf("messages.getMessageReadParticipants - empty participant reply")
			return nil, err
		}

		participantIdList := pIdList.GetDatas()
		if len(participantIdList) == 0 {
			return buildMessageReadParticipantDates(nil)
		}

		boxList, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessageListByDataIdUserIdList(c.ctx, &message.TLMessageGetUserMessageListByDataIdUserIdList{
			Id:         msgBox.DialogMessageId,
			UserIdList: participantIdList,
		})
		if err != nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", err)
			return nil, err
		} else if boxList == nil {
			err = mtproto.ErrInternalServerError
			c.Logger.Errorf("messages.getMessageReadParticipants - empty message list reply")
			return nil, err
		}

		// Read receipts are persisted against the sender's local cursor. Resolve
		// that cursor from the participant copies, then inspect the persisted
		// receipt rows. Their max-date is not an exact per-message timestamp.
		senderMessageID := int32(0)
		boxList.Walk(func(_ int, v *mtproto.MessageBox) {
			if v != nil && v.UserId == msgBox.SenderUserId {
				senderMessageID = v.MessageId
			}
		})
		if senderMessageID == 0 {
			return &mtproto.Vector_ReadParticipantDate{Datas: []*mtproto.ReadParticipantDate{}}, nil
		}
		readDates, err := c.svcCtx.Dao.MessageClient.MessageGetOutboxReadDate(c.ctx, &message.TLMessageGetOutboxReadDate{
			UserId: msgBox.SenderUserId, PeerType: mtproto.PEER_CHAT, PeerId: peer.PeerId, MsgId: senderMessageID,
		})
		if err != nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", err)
			return nil, err
		}
		if readDates == nil {
			return nil, mtproto.ErrInternalServerError
		}
		return buildMessageReadParticipantDatesFromStoredCursor(readDates.GetDatas())
	default:
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", err)
		return nil, err
	}
}

func validateReadParticipantMessage(userID, chatID int64, messageID int32, box *mtproto.MessageBox) error {
	if box == nil || box.GetMessage() == nil || box.GetUserId() != userID || box.GetMessageId() != messageID || box.GetPeerType() != mtproto.PEER_CHAT || box.GetPeerId() != chatID {
		return mtproto.ErrPeerIdInvalid
	}
	return nil
}

func validateReadParticipantChannelMessage(userID, channelID int64, messageID int32, box *mtproto.MessageBox) error {
	if box == nil || box.GetMessage() == nil || box.GetUserId() != userID || box.GetMessageId() != messageID || box.GetPeerType() != mtproto.PEER_CHANNEL || box.GetPeerId() != channelID {
		return mtproto.ErrPeerIdInvalid
	}
	return nil
}

func validateMessageReadParticipantsPeer(peer *mtproto.PeerUtil) error {
	if peer == nil || peer.GetPeerId() <= 0 {
		return mtproto.ErrPeerIdInvalid
	}
	if peer.GetPeerType() == mtproto.PEER_CHANNEL && peer.GetAccessHash() == 0 {
		return mtproto.ErrChannelInvalid
	}
	return nil
}

func validateMessageReadParticipantsDependencies(c *ChatsCore, peerType int32) error {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return mtproto.ErrInternalServerError
	}
	if peerType == mtproto.PEER_CHANNEL && c.svcCtx.Dao.DialogClient == nil {
		return mtproto.ErrInternalServerError
	}
	if peerType == mtproto.PEER_CHAT && (c.svcCtx.Dao.ChatClient == nil || c.svcCtx.Dao.ChatClient.Client() == nil) {
		return mtproto.ErrInternalServerError
	}
	return nil
}

func validateMessageReadParticipantIDs(ids []int64, callerID int64) error {
	for _, id := range ids {
		if id <= 0 || id == callerID {
			return mtproto.ErrInternalServerError
		}
	}
	return nil
}

func validateReadParticipantDates(dates []*mtproto.ReadParticipantDate) error {
	for _, date := range dates {
		if date == nil || date.GetUserId() <= 0 || date.GetDate() <= 0 {
			return mtproto.ErrInternalServerError
		}
	}
	return nil
}

func buildMessageReadParticipantDatesFromStoredCursor(dates []*mtproto.ReadParticipantDate) (*mtproto.Vector_ReadParticipantDate, error) {
	if err := validateReadParticipantDates(dates); err != nil {
		return nil, err
	}
	if len(dates) > 0 {
		// message_read_outbox stores a reader's maximum sender-local
		// message cursor and the latest timestamp for that cursor. A
		// request for an earlier message cannot recover that message's
		// exact read time, so do not return a later timestamp as if it
		// were authoritative for this message.
		return nil, mtproto.ErrMethodNotImpl
	}
	return &mtproto.Vector_ReadParticipantDate{Datas: []*mtproto.ReadParticipantDate{}}, nil
}

func validateReadParticipantCallerDialog(callerDialog *dialog.DialogExt, channelID int64) error {
	if callerDialog == nil || callerDialog.GetDialog() == nil || callerDialog.GetDialog().GetPeer() == nil {
		return mtproto.ErrPeerIdInvalid
	}
	peer := callerDialog.GetDialog().GetPeer()
	if peer.GetPredicateName() != mtproto.Predicate_peerChannel || peer.GetChannelId() != channelID {
		return mtproto.ErrPeerIdInvalid
	}
	return nil
}

func buildMessageReadParticipantDates(readParticipantIdList []int64) (*mtproto.Vector_ReadParticipantDate, error) {
	if len(readParticipantIdList) > 0 {
		// Read history stores a per-dialog message cursor, but no read timestamp.
		// Message and dialog dates describe different events and cannot satisfy
		// ReadParticipantDate.date, so fail closed instead of returning date: 0.
		return nil, mtproto.ErrMethodNotImpl
	}

	return &mtproto.Vector_ReadParticipantDate{Datas: []*mtproto.ReadParticipantDate{}}, nil
}

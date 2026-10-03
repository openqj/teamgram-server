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
	var (
		peer = mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	)

	switch peer.PeerType {
	case mtproto.PEER_CHANNEL:
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
		if msgBox.GetPeerType() != mtproto.PEER_CHANNEL || msgBox.GetPeerId() != peer.PeerId || msgBox.GetMessageId() != in.MsgId {
			return nil, mtproto.ErrPeerIdInvalid
		}

		callerDialog, err := c.svcCtx.Dao.DialogClient.DialogGetDialogById(c.ctx, &dialog.TLDialogGetDialogById{
			UserId:   c.MD.UserId,
			PeerType: mtproto.PEER_CHANNEL,
			PeerId:   peer.PeerId,
		})
		if err != nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", err)
			return nil, err
		} else if callerDialog == nil || callerDialog.GetDialog() == nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - empty caller dialog")
			return nil, mtproto.ErrPeerIdInvalid
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

		var (
			readParticipantIdList []int64
			lookupErr             error
		)
		// TODO: performance optimization
		boxList.Walk(func(idx int, v *mtproto.MessageBox) {
			if lookupErr != nil {
				return
			}
			if v == nil {
				lookupErr = mtproto.ErrInternalServerError
				return
			}
			if v.UserId == c.MD.UserId {
				return
			}

			dialogList, err := c.svcCtx.Dao.DialogClient.DialogGetDialogsByIdList(c.ctx, &dialog.TLDialogGetDialogsByIdList{
				UserId: v.UserId,
				IdList: []int64{mtproto.MakePeerDialogId(peer.PeerType, peer.PeerId)},
			})
			if err != nil {
				lookupErr = err
				return
			} else if dialogList == nil {
				lookupErr = mtproto.ErrInternalServerError
				return
			}

			for _, d := range dialogList.GetDatas() {
				if d == nil || d.GetDialog() == nil {
					lookupErr = mtproto.ErrInternalServerError
					return
				}
				if d.GetDialog().GetReadInboxMaxId() >= v.MessageId {
					readParticipantIdList = append(readParticipantIdList, v.UserId)
					return
				}
			}
		})
		if lookupErr != nil {
			c.Logger.Errorf("messages.getMessageReadParticipants - error: %v", lookupErr)
			return nil, lookupErr
		}

		return buildMessageReadParticipantDates(readParticipantIdList)
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

func buildMessageReadParticipantDates(readParticipantIdList []int64) (*mtproto.Vector_ReadParticipantDate, error) {
	if len(readParticipantIdList) > 0 {
		// Read history stores a per-dialog message cursor, but no read timestamp.
		// Message and dialog dates describe different events and cannot satisfy
		// ReadParticipantDate.date, so fail closed instead of returning date: 0.
		return nil, mtproto.ErrMethodNotImpl
	}

	return &mtproto.Vector_ReadParticipantDate{Datas: []*mtproto.ReadParticipantDate{}}, nil
}

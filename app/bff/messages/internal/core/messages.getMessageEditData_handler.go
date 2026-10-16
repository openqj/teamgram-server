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
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

/*
// android client source code.

```
if (!messageObject.scheduled) {
	TLRPC.TL_messages_getMessageEditData req = new TLRPC.TL_messages_getMessageEditData();
	req.peer = getMessagesController().getInputPeer((int) dialog_id);
	req.id = messageObject.getId();
	editingMessageObjectReqId = getConnectionsManager().sendRequest(req, (response, error) -> AndroidUtilities.runOnUIThread(() -> {
		editingMessageObjectReqId = 0;
		if (response == null) {
			if (getParentActivity() == null) {
				return;
			}
			AlertDialog.Builder builder = new AlertDialog.Builder(getParentActivity());
			builder.setTitle(LocaleController.getString("AppName", R.string.AppName));
			builder.setMessage(LocaleController.getString("EditMessageError", R.string.EditMessageError));
			builder.setPositiveButton(LocaleController.getString("OK", R.string.OK), null);
			showDialog(builder.create());

			if (chatActivityEnterView != null) {
				chatActivityEnterView.setEditingMessageObject(null, false);
				hideFieldPanel(true);
			}
		} else {
			if (chatActivityEnterView != null) {
				chatActivityEnterView.showEditDoneProgress(false, true);
			}
		}
	}));

```
*/

// MessagesGetMessageEditData
// messages.getMessageEditData#fda68d36 peer:InputPeer id:int = messages.MessageEditData;
func (c *MessagesCore) MessagesGetMessageEditData(in *mtproto.TLMessagesGetMessageEditData) (*mtproto.Messages_MessageEditData, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}

	// 400	CHAT_ADMIN_REQUIRED	You must be an admin in this chat to do this.
	// 403	MESSAGE_AUTHOR_REQUIRED	Message author required.
	// 400	MESSAGE_ID_INVALID	The provided message id is invalid.
	// 400	PEER_ID_INVALID	The provided peer id is invalid.

	if c.MD.IsBot {
		return nil, mtproto.ErrBotMethodInvalid
	}
	if in.GetId() <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}

	switch peer.PeerType {
	case mtproto.PEER_SELF,
		mtproto.PEER_USER,
		mtproto.PEER_CHAT:
		if peer.PeerType == mtproto.PEER_USER && in.Peer.GetAccessHash() == 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
		if peer.PeerType == mtproto.PEER_SELF {
			peer.PeerType = mtproto.PEER_USER
			peer.PeerId = c.MD.UserId
		}
		if peer.PeerId <= 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
		if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
			return nil, mtproto.ErrInternalServerError
		}
		boxMsg, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessage(c.ctx, &message.TLMessageGetUserMessage{
			UserId: c.MD.UserId,
			Id:     in.Id,
		})
		if err != nil {
			return nil, err
		}
		if boxMsg == nil || boxMsg.Message == nil {
			return nil, mtproto.ErrInternalServerError
		}
		if boxMsg.MessageId != in.Id || boxMsg.Message.GetId() != in.Id {
			return nil, mtproto.ErrMessageIdInvalid
		}
		if boxMsg.PeerType != peer.PeerType || boxMsg.PeerId != peer.PeerId {
			return nil, mtproto.ErrMessageAuthorRequired
		}

		if peer.PeerType == mtproto.PEER_CHAT {
			if c.MD.UserId != boxMsg.SenderUserId {
				if c.svcCtx.Dao.ChatClient == nil || c.svcCtx.Dao.ChatClient.Client() == nil {
					return nil, mtproto.ErrInternalServerError
				}
				mChat, err := c.svcCtx.Dao.ChatClient.Client().ChatGetChatBySelfId(c.ctx, &chatpb.TLChatGetChatBySelfId{
					SelfId: c.MD.UserId,
					ChatId: peer.PeerId,
				})
				if err != nil {
					return nil, mtproto.ErrChatAdminRequired
				}
				if mChat == nil {
					return nil, mtproto.ErrInternalServerError
				}
				for _, participant := range mChat.GetChatParticipants() {
					if participant == nil {
						return nil, mtproto.ErrInternalServerError
					}
				}
				me, ok := mChat.GetImmutableChatParticipant(c.MD.UserId)
				if !ok || me == nil {
					return nil, mtproto.ErrChatAdminRequired
				}
				if !me.CanAdminEditMessages() {
					return nil, mtproto.ErrChatAdminRequired
				}
			}
		} else {
			if c.MD.UserId != boxMsg.SenderUserId {
				return nil, mtproto.ErrMessageAuthorRequired
			}
		}
		caption := false
		if media := boxMsg.Message.GetMedia(); media != nil {
			caption = media.GetPredicateName() == mtproto.Predicate_messageMediaPhoto ||
				media.GetPredicateName() == mtproto.Predicate_messageMediaDocument
		}
		return mtproto.MakeTLMessagesMessageEditData(&mtproto.Messages_MessageEditData{
			Caption: caption,
		}).To_Messages_MessageEditData(), nil
	case mtproto.PEER_CHANNEL:
		editData, err := channelview.GetMessageEditData(c.MD.UserId, in.Peer, in.Id)
		if err != nil {
			return nil, err
		}
		return editData, nil
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
}

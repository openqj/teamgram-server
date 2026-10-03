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
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// MessagesReadMentions
// messages.readMentions#36e5bf4d flags:# peer:InputPeer top_msg_id:flags.0?int = messages.AffectedHistory;
func (c *MessagesCore) MessagesReadMentions(in *mtproto.TLMessagesReadMentions) (*mtproto.Messages_AffectedHistory, error) {
	if c == nil || c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetTopMsgId() != nil && in.GetTopMsgId().GetValue() <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_CHAT:
		if peer.PeerId <= 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
	case mtproto.PEER_USER:
		if peer.PeerId <= 0 || peer.AccessHash == 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
	case mtproto.PEER_CHANNEL:
		if peer.PeerId <= 0 || peer.AccessHash == 0 {
			return nil, mtproto.ErrChannelInvalid
		}
	case mtproto.PEER_UNKNOWN:
		switch in.GetPeer().GetPredicateName() {
		case mtproto.Predicate_inputPeerUserFromMessage:
			if in.GetPeer().GetUserId() <= 0 || in.GetPeer().GetMsgId() <= 0 || in.GetPeer().GetPeer() == nil {
				return nil, mtproto.ErrPeerIdInvalid
			}
			return nil, mtproto.ErrMethodNotImpl
		case mtproto.Predicate_inputPeerChannelFromMessage:
			if in.GetPeer().GetChannelId() <= 0 || in.GetPeer().GetMsgId() <= 0 || in.GetPeer().GetPeer() == nil {
				return nil, mtproto.ErrChannelInvalid
			}
			return nil, mtproto.ErrMethodNotImpl
		}
		return nil, mtproto.ErrPeerIdInvalid
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.PeerType != mtproto.PEER_CHAT {
		return nil, mtproto.ErrMethodNotImpl
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MsgClient == nil || c.svcCtx.Dao.ChatClient == nil || c.svcCtx.Dao.ChatClient.Client() == nil {
		return nil, mtproto.ErrInternalServerError
	}
	chat, err := c.svcCtx.Dao.ChatClient.Client().ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{ChatId: peer.PeerId})
	if err != nil {
		return nil, err
	}
	if chat == nil || chat.GetChat() == nil || chat.Id() != peer.PeerId {
		return nil, mtproto.ErrChatIdInvalid
	}
	for _, participant := range chat.GetChatParticipants() {
		if participant == nil {
			return nil, mtproto.ErrInternalServerError
		}
	}
	member, ok := chat.GetImmutableChatParticipant(c.MD.UserId)
	if !ok || member == nil || !member.IsChatMemberStateNormal() {
		return nil, mtproto.ErrUserNotParticipant
	}
	r, err := c.svcCtx.Dao.MsgClient.MsgReadMentions(c.ctx, &msgpb.TLMsgReadMentions{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		TopMsgId:  in.GetTopMsgId(),
	})
	if err != nil {
		c.Logger.Errorf("messages.readMentions - error: %v", err)
		return nil, err
	}
	if r == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return r, nil
}

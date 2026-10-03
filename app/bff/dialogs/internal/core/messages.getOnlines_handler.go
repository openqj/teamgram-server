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
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesGetOnlines
// messages.getOnlines#6e2be050 peer:InputPeer = ChatOnlines;
func (c *DialogsCore) MessagesGetOnlines(in *mtproto.TLMessagesGetOnlines) (*mtproto.ChatOnlines, error) {
	if in == nil || in.GetPeer() == nil {
		c.Logger.Errorf("messages.getOnlines - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	if peer == nil || peer.PeerType != mtproto.PEER_CHAT || peer.PeerId == 0 {
		c.Logger.Errorf("messages.getOnlines - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}
	onlines, ok := c.chatOnlineCount(peer.PeerId)
	if !ok {
		c.Logger.Errorf("messages.getOnlines - error: online count unavailable")
		return nil, mtproto.ErrInternalServerError
	}

	return mtproto.MakeTLChatOnlines(&mtproto.ChatOnlines{
		Onlines: onlines,
	}).To_ChatOnlines(), nil
}

// chatOnlineCount uses the same online window as user.MakeUserStatus (last seen within 60s).
func (c *DialogsCore) chatOnlineCount(chatID int64) (int32, bool) {
	chat, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
		ChatId: chatID,
	})
	if err != nil || chat == nil {
		c.Logger.Errorf("messages.getOnlines - error: %v", err)
		return 0, false
	}
	ids := chat.ParticipantIdList()
	if len(ids) == 0 {
		return 0, true
	}
	seens, err := c.svcCtx.Dao.UserClient.UserGetLastSeens(c.ctx, &userpb.TLUserGetLastSeens{
		Id: ids,
	})
	if err != nil || seens == nil {
		c.Logger.Errorf("messages.getOnlines - error: %v", err)
		return 0, false
	}
	now := time.Now().Unix()
	var n int32
	for _, v := range seens.GetDatas() {
		if v != nil && now <= v.LastSeenAt+60 {
			n++
		}
	}
	return n, true
}

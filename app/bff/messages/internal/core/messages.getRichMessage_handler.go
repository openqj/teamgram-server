// Copyright (c) 2026 The Teamgram Authors (https://teamgram.net).
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
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessagesGetRichMessage
// messages.getRichMessage#501569cf peer:InputPeer id:int = messages.Messages;
func (c *MessagesCore) MessagesGetRichMessage(in *mtproto.TLMessagesGetRichMessage) (*mtproto.Messages_Messages, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		c.Logger.Errorf("messages.getRichMessage - error: nil request")
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if in.GetId() <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if in.GetPeer() == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}

	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
	default:
		c.Logger.Errorf("messages.getRichMessage - error: peer invalid")
		return nil, mtproto.ErrPeerIdInvalid
	}

	// Same client as messages.getMessages. An unknown id is messageEmpty, not an RPC error.
	boxList, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(c.ctx, &message.TLMessageGetUserMessageList{
		UserId: c.MD.UserId,
		IdList: []int32{in.GetId()},
	})
	if err != nil {
		c.Logger.Errorf("messages.getRichMessage - error: %v", err)
		return nil, err
	}
	if boxList == nil {
		return nil, mtproto.ErrInternalServerError
	}

	var found *mtproto.MessageBox
	for _, box := range boxList.GetDatas() {
		if box != nil && box.GetMessageId() == in.GetId() && richPeerMatches(peer, box) {
			found = box
			break
		}
	}
	if found == nil {
		return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
			Messages: []*mtproto.Message{
				mtproto.MakeTLMessageEmpty(&mtproto.Message{Id: in.GetId()}).To_Message(),
			},
			Users: []*mtproto.User{},
			Chats: []*mtproto.Chat{},
		}).To_Messages_Messages(), nil
	}

	return c.messagesOfBoxes([]*mtproto.MessageBox{found})
}

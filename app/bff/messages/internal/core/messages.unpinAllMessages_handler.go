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
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

// MessagesUnpinAllMessages
// messages.unpinAllMessages#f025bc8b peer:InputPeer = messages.AffectedHistory;
func (c *MessagesCore) MessagesUnpinAllMessages(in *mtproto.TLMessagesUnpinAllMessages) (*mtproto.Messages_AffectedHistory, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	var (
		peer = mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	)
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.IsChannel() {
		channelID, err := channelview.ValidateInputPeer(c.MD.UserId, in.GetPeer())
		if err != nil {
			return nil, err
		}
		return channelview.UnpinAll(c.MD.UserId, channelID)
	}
	switch peer.PeerType {
	case mtproto.PEER_SELF:
	case mtproto.PEER_USER:
	case mtproto.PEER_CHAT:
	default:
		c.Logger.Errorf("invalid peer: %v", in.Peer)
		err := mtproto.ErrPeerIdInvalid
		return nil, err
	}

	rValues, err := c.svcCtx.Dao.MsgClient.MsgUnpinAllMessages(c.ctx, &msgpb.TLMsgUnpinAllMessages{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
	})
	if err != nil {
		c.Logger.Errorf("messages.unpinAllMessages - error: %v", in.Peer)
		return nil, err
	}

	return rValues, nil
}

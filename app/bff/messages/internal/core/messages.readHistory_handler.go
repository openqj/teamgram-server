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

// MessagesReadHistory
// messages.readHistory#e306d3a peer:InputPeer max_id:int = messages.AffectedMessages;
func (c *MessagesCore) MessagesReadHistory(in *mtproto.TLMessagesReadHistory) (*mtproto.Messages_AffectedMessages, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetMaxId() < 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}

	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	if peer == nil || peer.PeerId <= 0 {
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("messages.readHistory - error: %v", err)
		return nil, err
	}
	if peer.PeerType == mtproto.PEER_CHANNEL {
		// APIFull owns channel read cursors. Standalone messages workers do not
		// have that store, so keep the historical peer error there rather than
		// claiming a successful read without persistence.
		if !channelview.Ready() {
			return nil, mtproto.ErrPeerIdInvalid
		}
		return channelview.ReadHistoryForInputPeer(c.MD.UserId, in.GetPeer(), in.GetMaxId())
	}
	if !peer.IsChatOrUser() {
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("messages.readHistory - error: %v", err)
		return nil, err
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MsgClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	rV, err := c.svcCtx.Dao.MsgClient.MsgReadHistoryV2(
		c.ctx,
		&msgpb.TLMsgReadHistoryV2{
			UserId:    c.MD.UserId,
			AuthKeyId: c.MD.PermAuthKeyId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			MaxId:     in.GetMaxId(),
		})
	if err != nil {
		c.Logger.Errorf("messages.readHistory - error: %v", err)
		return nil, err
	}
	if rV == nil {
		return nil, mtproto.ErrInternalServerError
	}

	return rV, nil
}

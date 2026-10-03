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

	"google.golang.org/grpc/status"
)

// MessagesUpdatePinnedMessage
// messages.updatePinnedMessage#d2aaf7ec flags:# silent:flags.0?true unpin:flags.1?true pm_oneside:flags.2?true peer:InputPeer id:int = Updates;
func (c *MessagesCore) MessagesUpdatePinnedMessage(in *mtproto.TLMessagesUpdatePinnedMessage) (*mtproto.Updates, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var (
		peer     = mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
		rUpdates *mtproto.Updates
	)

	if peer == nil || (!peer.IsChannel() && !peer.IsChatOrUser()) {
		c.Logger.Errorf("invalid peer")
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.IsChannel() {
		rUpdates, err := channelview.Pin(c.MD.UserId, peer.PeerId, in.Id, in.Unpin)
		if err != nil {
			c.Logger.Errorf("messages.updatePinnedMessage - channel pin failed")
			if _, ok := status.FromError(err); !ok {
				err = mtproto.ErrInternalServerError
			}
			return nil, err
		}
		return rUpdates, nil
	}
	if c.svcCtx.Dao.MsgClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	rUpdates, err := c.svcCtx.Dao.MsgClient.MsgUpdatePinnedMessage(c.ctx, &msgpb.TLMsgUpdatePinnedMessage{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		Silent:    in.Silent,
		Unpin:     in.Unpin,
		PmOneside: in.PmOneside,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		Id:        in.Id,
	})
	if err != nil {
		c.Logger.Errorf("messages.updatePinnedMessage - error: %v", in.Peer)
		return nil, err
	}
	if rUpdates == nil {
		return nil, mtproto.ErrInternalServerError
	}

	return rUpdates, nil
}

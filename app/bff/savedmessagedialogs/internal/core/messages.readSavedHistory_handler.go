// Copyright 2025 Teamgram Authors
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
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// MessagesReadSavedHistory
// messages.readSavedHistory#ba4a3b5b parent_peer:InputPeer peer:InputPeer max_id:int = Bool;
func (c *SavedMessageDialogsCore) MessagesReadSavedHistory(in *mtproto.TLMessagesReadSavedHistory) (*mtproto.Bool, error) {
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if c == nil || c.MD == nil || c.MD.GetUserId() <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in.GetParentPeer() == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}

	parent := mtproto.FromInputPeer2(c.MD.GetUserId(), in.GetParentPeer())
	if !savedPeerAllowed(parent) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	parentKey := savedPeerKeyOf(parent.PeerType, parent.PeerId, c.MD.GetUserId())
	if parentKey.peerType != mtproto.PEER_USER || parentKey.peerId != c.MD.GetUserId() {
		return nil, mtproto.ErrPeerIdInvalid
	}

	peer := mtproto.FromInputPeer2(c.MD.GetUserId(), in.GetPeer())
	if !savedPeerAllowed(peer) || peer.PeerId <= 0 {
		if c.Logger != nil {
			c.Logger.Errorf("messages.readSavedHistory - error: invalid peer")
		}
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetMaxId() < 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}

	result, err := c.svcCtx.Dao.DialogClient.DialogMarkSavedHistoryRead(c.ctx, &dialog.TLDialogInsertOrUpdateDialog{
		UserId:         c.MD.GetUserId(),
		PeerType:       peer.PeerType,
		PeerId:         peer.PeerId,
		ReadInboxMaxId: wrapperspb.Int32(in.GetMaxId()),
	})
	if err != nil {
		if c.Logger != nil {
			c.Logger.Errorf("messages.readSavedHistory - error: %v", err)
		}
		return nil, err
	}
	if result == nil || !mtproto.FromBool(result) {
		return nil, mtproto.ErrInternalServerError
	}
	return mtproto.BoolTrue, nil
}

// Copyright 2024 Teamgram Authors
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
)

// MessagesReorderPinnedSavedDialogs
// messages.reorderPinnedSavedDialogs#8b716587 flags:# force:flags.0?true order:Vector<InputDialogPeer> = Bool;
func (c *SavedMessageDialogsCore) MessagesReorderPinnedSavedDialogs(in *mtproto.TLMessagesReorderPinnedSavedDialogs) (*mtproto.Bool, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if len(in.GetOrder()) == 0 && !in.GetForce() {
		c.Logger.Errorf("messages.reorderPinnedDialogs - len(order) == 0")
		return mtproto.BoolTrue, nil
	}

	var (
		order []*mtproto.PeerUtil
	)
	for _, peer := range in.GetOrder() {
		if peer == nil {
			return nil, mtproto.ErrPeerIdInvalid
		}
		switch peer.PredicateName {
		case mtproto.Predicate_inputDialogPeer:
			if peer.Peer == nil {
				return nil, mtproto.ErrPeerIdInvalid
			}
			p := mtproto.FromInputPeer2(c.MD.UserId, peer.Peer)
			if !savedPeerAllowed(p) {
				err := mtproto.ErrPeerIdInvalid
				c.Logger.Errorf("messages.reorderPinnedSavedDialogs - error: %v", err)
				return nil, err
			}
			order = append(order, p)
		default:
			err := mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("messages.reorderPinnedSavedDialogs - error: %v", err)
			return nil, err
		}
	}

	result, err := c.svcCtx.Dao.DialogClient.DialogReorderPinnedSavedDialogs(c.ctx, &dialog.TLDialogReorderPinnedSavedDialogs{
		UserId: c.MD.UserId,
		Force:  mtproto.ToBool(in.Force),
		Order:  order,
	})
	if err != nil {
		c.Logger.Errorf("messages.reorderPinnedSavedDialogs - error: %v", err)
		return nil, err
	}
	if result == nil || !mtproto.FromBool(result) {
		return nil, mtproto.ErrInternalServerError
	}

	return mtproto.BoolTrue, nil
}

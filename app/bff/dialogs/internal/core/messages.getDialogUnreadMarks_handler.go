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
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// MessagesGetDialogUnreadMarks
// messages.getDialogUnreadMarks#22e24e22 = Vector<DialogPeer>;
func (c *DialogsCore) MessagesGetDialogUnreadMarks(in *mtproto.TLMessagesGetDialogUnreadMarks) (*mtproto.Vector_DialogPeer, error) {
	_ = in
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	dialogPeerList, err := c.svcCtx.DialogClient.DialogGetDialogUnreadMarkList(
		c.ctx,
		&dialog.TLDialogGetDialogUnreadMarkList{
			UserId: c.MD.UserId,
		})
	if err != nil {
		if c.Logger != nil {
			c.Logger.Errorf("messages.getDialogUnreadMarks - error: %v", err)
		}
		return nil, err
	}
	if dialogPeerList == nil {
		return nil, mtproto.ErrInternalServerError
	}
	for _, dialogPeer := range dialogPeerList.GetDatas() {
		if dialogPeer == nil || dialogPeer.GetPeer() == nil {
			return nil, mtproto.ErrInternalServerError
		}
		peer := dialogPeer.GetPeer()
		switch peer.GetPredicateName() {
		case mtproto.Predicate_peerUser:
			if peer.GetUserId() <= 0 {
				return nil, mtproto.ErrInternalServerError
			}
		case mtproto.Predicate_peerChat:
			if peer.GetChatId() <= 0 {
				return nil, mtproto.ErrInternalServerError
			}
		case mtproto.Predicate_peerChannel:
			if peer.GetChannelId() <= 0 {
				return nil, mtproto.ErrInternalServerError
			}
		default:
			return nil, mtproto.ErrInternalServerError
		}
	}

	return &mtproto.Vector_DialogPeer{
		Datas: dialogPeerList.GetDatas(),
	}, nil
}

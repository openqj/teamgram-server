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

// DialogGetMyDialogsData
// dialog.getMyDialogsData flags:# user:flags.0?true chat:flags.1?true channel:flags.2?true = Vector<PeerUtil>;
func (c *DialogCore) DialogGetMyDialogsData(in *dialog.TLDialogGetMyDialogsData) (*dialog.DialogsData, error) {
	store, err := c.pgStore()
	if err != nil || store.Dialogs == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	var (
		uIdList  []int64
		cIdList  []int64
		chIdList []int64
	)

	if in.User {
		rows, err := store.Dialogs.SelectDialogsByPeerType(c.ctx, in.UserId, []int32{mtproto.PEER_USER})
		if err != nil {
			return nil, err
		}
		for i := range rows {
			uIdList = append(uIdList, rows[i].PeerId)
		}
	}

	if in.Chat {
		rows, err := store.Dialogs.SelectDialogsByPeerType(c.ctx, in.UserId, []int32{mtproto.PEER_CHAT})
		if err != nil {
			return nil, err
		}
		for i := range rows {
			cIdList = append(cIdList, rows[i].PeerId)
		}
	}

	if in.Channel {
		rows, err := store.Dialogs.SelectDialogsByPeerType(c.ctx, in.UserId, []int32{mtproto.PEER_CHANNEL})
		if err != nil {
			return nil, err
		}
		for i := range rows {
			chIdList = append(chIdList, rows[i].PeerId)
		}
	}

	return dialog.MakeTLSimpleDialogsData(&dialog.DialogsData{
		Users:    uIdList,
		Chats:    cIdList,
		Channels: chIdList,
	}).To_DialogsData(), nil
}

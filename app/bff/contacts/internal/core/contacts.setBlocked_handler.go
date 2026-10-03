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
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsSetBlocked
// contacts.setBlocked#94c65c76 flags:# my_stories_from:flags.0?true id:Vector<InputPeer> limit:int = Bool;
func (c *ContactsCore) ContactsSetBlocked(in *mtproto.TLContactsSetBlocked) (*mtproto.Bool, error) {
	// user.blockPeer / unblockPeer have no my_stories_from or stories field, so the
	// existing contacts.block and contacts.unblock paths are the storage.
	want := make(map[int64]*mtproto.InputPeer, len(in.GetId()))
	for _, id := range in.GetId() {
		peer := mtproto.FromInputPeer2(c.MD.UserId, id)
		if !peer.IsUser() || peer.IsSelf() || peer.PeerId == c.MD.UserId {
			err := mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("contacts.setBlocked - error: %v", err)
			return nil, err
		}
		want[peer.PeerId] = id
	}

	blockedList, err := c.svcCtx.Dao.UserClient.UserGetBlockedList(c.ctx, &userpb.TLUserGetBlockedList{
		UserId: c.MD.UserId,
		Offset: 0,
		Limit:  10000,
	})
	if err != nil {
		c.Logger.Errorf("contacts.setBlocked - error: %v", err)
		return nil, err
	}

	have := make(map[int64]struct{}, len(blockedList.GetDatas()))
	for _, blocked := range blockedList.GetDatas() {
		uid := blocked.GetPeerId().GetUserId()
		if uid == 0 {
			continue
		}
		have[uid] = struct{}{}
		if _, ok := want[uid]; ok {
			continue
		}
		if _, err = c.ContactsUnblock(&mtproto.TLContactsUnblock{
			MyStoriesFrom: in.GetMyStoriesFrom(),
			Id:            mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: uid}).To_InputPeer(),
		}); err != nil {
			c.Logger.Errorf("contacts.setBlocked - error: %v", err)
			return nil, err
		}
	}

	for uid, id := range want {
		if _, ok := have[uid]; ok {
			continue
		}
		if _, err = c.ContactsBlock(&mtproto.TLContactsBlock{
			MyStoriesFrom: in.GetMyStoriesFrom(),
			Id:            id,
		}); err != nil {
			c.Logger.Errorf("contacts.setBlocked - error: %v", err)
			return nil, err
		}
	}

	return mtproto.BoolTrue, nil
}

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

// UsersGetUsers
// users.getUsers#d91a548 id:Vector<InputUser> = Vector<User>;
func (c *UsersCore) UsersGetUsers(in *mtproto.TLUsersGetUsers) (*mtproto.Vector_User, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	type requestedUser struct {
		id         int64
		accessHash int64
		self       bool
	}

	requested := make([]requestedUser, 0, len(in.GetId()))
	idList := make([]int64, 0, len(in.GetId()))

	for _, inputUser := range in.Id {
		if inputUser == nil {
			return nil, mtproto.ErrUserIdInvalid
		}
		switch inputUser.GetPredicateName() {
		case mtproto.Predicate_inputUserSelf:
			requested = append(requested, requestedUser{id: c.MD.UserId, self: true})
			idList = append(idList, c.MD.UserId)
		case mtproto.Predicate_inputUser:
			if inputUser.GetUserId() <= 0 || inputUser.GetAccessHash() == 0 {
				return nil, mtproto.ErrUserIdInvalid
			}
			requested = append(requested, requestedUser{
				id:         inputUser.GetUserId(),
				accessHash: inputUser.GetAccessHash(),
			})
			idList = append(idList, inputUser.GetUserId())
		default:
			c.Logger.Errorf("invalid userId")
			return nil, mtproto.ErrUserIdInvalid
		}
	}
	if len(requested) == 0 {
		return &mtproto.Vector_User{Datas: []*mtproto.User{}}, nil
	}

	mUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: idList,
		To: []int64{c.MD.UserId},
	})
	if err != nil {
		c.Logger.Errorf("users.getUsers - error: %v", err)
		return nil, err
	}
	if mUsers == nil {
		c.Logger.Errorf("users.getUsers - error: nil response")
		return nil, mtproto.ErrInternalServerError
	}

	canonical := make(map[int64]*mtproto.ImmutableUser, len(mUsers.GetDatas()))
	for _, immutable := range mUsers.GetDatas() {
		if immutable == nil || immutable.GetUser() == nil || immutable.Id() <= 0 {
			return nil, mtproto.ErrInternalServerError
		}
		if _, exists := canonical[immutable.Id()]; exists {
			return nil, mtproto.ErrInternalServerError
		}
		canonical[immutable.Id()] = immutable
	}
	me, ok := canonical[c.MD.UserId]
	if !ok || me.Deleted() {
		return nil, mtproto.ErrInternalServerError
	}

	users := make([]*mtproto.User, 0, len(requested))
	for _, request := range requested {
		immutable, ok := canonical[request.id]
		if !ok || immutable.Deleted() || (!request.self && immutable.AccessHash() != request.accessHash) {
			return nil, mtproto.ErrUserIdInvalid
		}
		if request.self || request.id == c.MD.UserId {
			users = append(users, immutable.ToSelfUser())
		} else {
			users = append(users, immutable.ToUnsafeUser(me))
		}
	}

	return &mtproto.Vector_User{Datas: users}, nil
}

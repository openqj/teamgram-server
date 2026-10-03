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
	"sort"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsGetContactIDs
// contacts.getContactIDs#7adc669d hash:long = Vector<int>;
func (c *ContactsCore) ContactsGetContactIDs(in *mtproto.TLContactsGetContactIDs) (*mtproto.Vector_Int, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	contactList, err := c.svcCtx.Dao.UserClient.UserGetContactList(c.ctx, &userpb.TLUserGetContactList{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("contacts.getContactIDs - error: %v", err)
		return nil, err
	}
	if contactList == nil {
		return nil, mtproto.ErrInternalServerError
	}

	ids := make([]int64, 0, len(contactList.GetDatas()))
	for _, contact := range contactList.GetDatas() {
		if contact == nil || contact.GetContactUserId() <= 0 || contact.GetContactUserId() > int64(^uint32(0)>>1) {
			return nil, mtproto.ErrUserIdInvalid
		}
		ids = append(ids, contact.GetContactUserId())
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	// contacts.getContacts reports a saved count of zero, so the matching
	// contacts hash is based on the sorted contact IDs alone.
	const hashMod = uint64(0x80000000)
	hash := uint64(0)
	for _, id := range ids {
		hash = (hash*20261 + hashMod + uint64(id)) % hashMod
	}

	result := &mtproto.Vector_Int{Datas: []int32{}}
	if in.GetHash() == int64(hash) {
		return result, nil
	}
	result.Datas = make([]int32, 0, len(ids))
	for _, id := range ids {
		result.Datas = append(result.Datas, int32(id))
	}

	return result, nil
}

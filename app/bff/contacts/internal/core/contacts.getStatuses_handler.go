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

// ContactsGetStatuses
// contacts.getStatuses#c4a353ee = Vector<ContactStatus>;
func (c *ContactsCore) ContactsGetStatuses(in *mtproto.TLContactsGetStatuses) (*mtproto.Vector_ContactStatus, error) {
	cList, err := c.svcCtx.Dao.UserClient.UserGetContactList(c.ctx, &userpb.TLUserGetContactList{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("contacts.getStatuses - user.getContactList error: %v", err)
		return nil, err
	}
	if cList == nil {
		c.Logger.Errorf("contacts.getStatuses - user.getContactList returned no response")
		return nil, mtproto.ErrInternalServerError
	}

	rList := &mtproto.Vector_ContactStatus{
		Datas: make([]*mtproto.ContactStatus, 0, len(cList.GetDatas())),
	}

	idList := make([]int64, 0, len(cList.GetDatas()))
	for _, id := range cList.GetDatas() {
		if id == nil {
			c.Logger.Errorf("contacts.getStatuses - user.getContactList returned a nil contact")
			return nil, mtproto.ErrInternalServerError
		}
		idList = append(idList, id.ContactUserId)
	}
	if len(idList) == 0 {
		return rList, nil
	}

	lastSeenList, err := c.svcCtx.Dao.UserGetLastSeens(c.ctx, &userpb.TLUserGetLastSeens{
		Id: idList,
	})
	if err != nil {
		c.Logger.Errorf("contacts.getStatuses - user.getLastSeens error: %v", err)
		return nil, err
	}
	if lastSeenList == nil {
		c.Logger.Errorf("contacts.getStatuses - user.getLastSeens returned no response")
		return nil, mtproto.ErrInternalServerError
	}

	lastSeenByID := make(map[int64]int64, len(lastSeenList.GetDatas()))
	for _, v := range lastSeenList.GetDatas() {
		if v == nil {
			c.Logger.Errorf("contacts.getStatuses - user.getLastSeens returned a nil last seen")
			return nil, mtproto.ErrInternalServerError
		}
		lastSeenByID[v.GetUserId()] = v.GetLastSeenAt()
	}

	for _, userID := range idList {
		status := mtproto.MakeTLUserStatusEmpty(nil).To_UserStatus()
		if lastSeenAt, ok := lastSeenByID[userID]; ok {
			status = userpb.MakeUserStatus(lastSeenAt, true)
		}
		rList.Datas = append(rList.Datas, mtproto.MakeTLContactStatus(&mtproto.ContactStatus{
			UserId: userID,
			Status: status,
		}).To_ContactStatus())
	}

	return rList, nil
}

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
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserGetBirthdays
// user.getBirthdays user_id:long = Vector<ContactBirthday>;
func (c *UserCore) UserGetBirthdays(in *user.TLUserGetBirthdays) (*user.Vector_ContactBirthday, error) {
	rV := &user.Vector_ContactBirthday{
		Datas: make([]*mtproto.ContactBirthday, 0),
	}
	if in == nil || in.GetUserId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}

	self, err := c.svcCtx.Dao.GetImmutableUser(c.ctx, in.GetUserId(), false)
	if err != nil {
		return nil, err
	}
	ids, err := c.svcCtx.Dao.SelectUserContactIDs(c.ctx, self.Id())
	if err != nil {
		return nil, err
	}
	for _, contactID := range ids {
		if contactID <= 0 {
			continue
		}
		contact, err := c.svcCtx.Dao.GetImmutableUser(c.ctx, contactID, true, self.Id())
		if err == mtproto.ErrUserIdInvalid {
			continue
		}
		if err != nil {
			return nil, err
		}
		if contact == nil || contact.Deleted() {
			continue
		}
		birthday := contact.Birthday()
		if birthday == nil {
			continue
		}
		allowed, err := c.svcCtx.Dao.CheckUserPrivacy(c.ctx, contactID, mtproto.BIRTHDAY, self.Id())
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}
		rV.Datas = append(rV.Datas, mtproto.MakeTLContactBirthday(&mtproto.ContactBirthday{
			ContactId: contactID,
			Birthday:  birthday,
		}).To_ContactBirthday())
	}

	return rV, nil
}

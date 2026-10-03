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

// ContactsResetSaved
// contacts.resetSaved#879537f1 = Bool;
func (c *ContactsCore) ContactsResetSaved(in *mtproto.TLContactsResetSaved) (*mtproto.Bool, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	// The user service reserves id=0 for an atomic full contact-book reset.
	deleted, err := c.svcCtx.Dao.UserClient.UserDeleteContact(c.ctx, &userpb.TLUserDeleteContact{
		UserId: c.MD.UserId,
		Id:     0,
	})
	if err != nil {
		c.Logger.Errorf("contacts.resetSaved - reset user contacts error: %v", err)
		return nil, err
	}
	if deleted == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if !mtproto.FromBool(deleted) {
		return mtproto.BoolFalse, nil
	}

	if err = saveSavedContacts(c.MD.UserId, []savedPhoneContact{}); err != nil {
		c.Logger.Errorf("contacts.resetSaved - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}

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
	"time"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UsersGetRequirementsToContact
// users.getRequirementsToContact#d89a83a3 id:Vector<InputUser> = Vector<RequirementToContact>;
func (c *PrivacySettingsCore) UsersGetRequirementsToContact(in *mtproto.TLUsersGetRequirementsToContact) (*mtproto.Vector_RequirementToContact, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	selfPremium, premiumKnown, err := c.selfPremiumKnown()
	if err != nil {
		c.Logger.Errorf("users.getRequirementsToContact - error: %v", err)
		return nil, err
	}
	datas := make([]*mtproto.RequirementToContact, 0, len(in.GetId()))
	for _, id := range in.GetId() {
		requirement, err := c.requirementToContact(id, selfPremium, premiumKnown)
		if err != nil {
			c.Logger.Errorf("users.getRequirementsToContact - error: %v", err)
			return nil, err
		}
		datas = append(datas, requirement)
	}
	return &mtproto.Vector_RequirementToContact{Datas: datas}, nil
}

func (c *PrivacySettingsCore) selfPremiumKnown() (premium bool, known bool, err error) {
	me, err := c.svcCtx.Dao.UserGetUserDataById(c.ctx, &userpb.TLUserGetUserDataById{
		UserId: c.MD.UserId,
	})
	if err != nil {
		return false, false, err
	}
	if me == nil {
		return false, false, mtproto.ErrInternalServerError
	}
	if me.GetId() != c.MD.UserId || me.GetDeleted() {
		return false, false, mtproto.ErrUserIdInvalid
	}
	if !me.GetPremium() {
		return false, true, nil
	}
	exp := me.GetPremiumExpireDate().GetValue()
	if exp == 0 {
		return true, true, nil
	}
	return time.Now().Unix() < exp, true, nil
}

func (c *PrivacySettingsCore) requirementToContact(id *mtproto.InputUser, selfPremium, premiumKnown bool) (*mtproto.RequirementToContact, error) {
	empty := mtproto.MakeTLRequirementToContactEmpty(&mtproto.RequirementToContact{}).To_RequirementToContact()
	if id == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	if id.GetPredicateName() == mtproto.Predicate_inputUserSelf {
		if id.GetUserId() != 0 || id.GetAccessHash() != 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
		return empty, nil
	}
	if id.GetPredicateName() != mtproto.Predicate_inputUser || id.GetUserId() <= 0 || id.GetAccessHash() == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	owner, err := c.svcCtx.Dao.UserGetUserDataById(c.ctx, &userpb.TLUserGetUserDataById{
		UserId: id.GetUserId(),
	})
	if err != nil {
		return nil, err
	}
	if owner == nil || owner.GetId() != id.GetUserId() || owner.GetDeleted() || owner.GetAccessHash() != id.GetAccessHash() {
		return nil, mtproto.ErrUserIdInvalid
	}
	if owner.GetId() == c.MD.UserId {
		return empty, nil
	}
	settings, err := c.svcCtx.Dao.UserGetGlobalPrivacySettings(c.ctx, &userpb.TLUserGetGlobalPrivacySettings{
		UserId: owner.GetId(),
	})
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return nil, mtproto.ErrInternalServerError
	}

	stars := settings.GetNoncontactPeersPaidStars().GetValue()
	needPremium := settings.GetNewNoncontactPeersRequirePremium()
	if stars <= 0 && !needPremium {
		return empty, nil
	}

	contact, err := c.svcCtx.Dao.UserCheckContact(c.ctx, &userpb.TLUserCheckContact{
		UserId: owner.GetId(),
		Id:     c.MD.UserId,
	})
	if err != nil {
		return nil, err
	}
	if contact == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if mtproto.FromBool(contact) {
		return empty, nil
	}

	if stars > 0 {
		allowed, err := c.svcCtx.Dao.UserCheckPrivacy(c.ctx, &userpb.TLUserCheckPrivacy{
			UserId: owner.GetId(), KeyType: mtproto.NO_PAID_MESSAGES, PeerId: c.MD.UserId,
		})
		if err != nil {
			return nil, err
		}
		if allowed == nil {
			return nil, mtproto.ErrInternalServerError
		}
		if mtproto.FromBool(allowed) {
			return empty, nil
		}
		return mtproto.MakeTLRequirementToContactPaidMessages(&mtproto.RequirementToContact{
			StarsAmount: stars,
		}).To_RequirementToContact(), nil
	}

	if needPremium {
		if !premiumKnown {
			return nil, mtproto.ErrInternalServerError
		}
		if !selfPremium {
			return mtproto.MakeTLRequirementToContactPremium(&mtproto.RequirementToContact{}).To_RequirementToContact(), nil
		}
	}
	return empty, nil
}

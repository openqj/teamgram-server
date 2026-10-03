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
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// AccountSetPrivacy
// account.setPrivacy#c9f81ce8 key:InputPrivacyKey rules:Vector<InputPrivacyRule> = account.PrivacyRules;
func (c *PrivacySettingsCore) AccountSetPrivacy(in *mtproto.TLAccountSetPrivacy) (*mtproto.Account_PrivacyRules, error) {
	var (
		key = mtproto.FromInputPrivacyKeyType(in.Key)
	)

	// TODO(@benqi): Check request valid.
	if key == mtproto.KEY_TYPE_INVALID {
		err := mtproto.ErrPrivacyKeyInvalid
		c.Logger.Errorf("account.setPrivacy - error: %v", err)
		return nil, err
	}

	ruleList := mtproto.ToPrivacyRuleListByInput(c.MD.UserId, in.Rules)
	users, chats, err := c.hydratePrivacyRuleObjects(ruleList)
	if err != nil {
		c.Logger.Errorf("account.setPrivacy - error: %v", err)
		return nil, fmt.Errorf("account.setPrivacy: %w", err)
	}

	saved, err := c.svcCtx.Dao.UserClient.UserSetPrivacy(c.ctx, &userpb.TLUserSetPrivacy{
		UserId:  c.MD.UserId,
		KeyType: int32(key),
		Rules:   ruleList,
	})
	if err != nil {
		c.Logger.Errorf("account.setPrivacy - error: %v", err)
		return nil, err
	}
	if saved == nil || saved.GetPredicateName() != mtproto.Predicate_boolTrue {
		err = fmt.Errorf("account.setPrivacy: saving privacy rules returned false")
		c.Logger.Errorf("account.setPrivacy - error: %v", err)
		return nil, err
	}

	rValue := mtproto.MakeTLAccountPrivacyRules(&mtproto.Account_PrivacyRules{
		Rules: ruleList,
		Users: users,
		Chats: chats,
	}).To_Account_PrivacyRules()
	syncUpdates := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdatePrivacy(&mtproto.Update{
		Key:   mtproto.ToPrivacyKey(key),
		Rules: ruleList,
	}).To_Update())

	syncUpdates.PushUser(rValue.Users...)
	syncUpdates.PushChat(rValue.Chats...)

	// Persist the rules before publishing their update; delivery cannot be rolled back.
	synced, err := c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       syncUpdates,
	})
	if err != nil {
		c.Logger.Errorf("account.setPrivacy - sync error: %v", err)
		return nil, fmt.Errorf("account.setPrivacy: privacy rules were saved but sync update failed: %w", err)
	}
	if synced == nil {
		return nil, fmt.Errorf("account.setPrivacy: privacy rules were saved but sync returned an empty response")
	}

	return rValue, nil
}

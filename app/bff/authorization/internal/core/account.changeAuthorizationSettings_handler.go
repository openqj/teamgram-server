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
	"encoding/json"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
)

const authorizationSettingsKey = "authorization_settings"

type authorizationSettingsFlags struct {
	Confirmed                 bool  `json:"confirmed"`
	EncryptedRequestsDisabled *bool `json:"encrypted_requests_disabled,omitempty"`
	CallRequestsDisabled      *bool `json:"call_requests_disabled,omitempty"`
}

// AccountChangeAuthorizationSettings
// account.changeAuthorizationSettings#40f48462 flags:# confirmed:flags.3?true hash:long encrypted_requests_disabled:flags.0?Bool call_requests_disabled:flags.1?Bool = Bool;
func (c *AuthorizationCore) AccountChangeAuthorizationSettings(in *mtproto.TLAccountChangeAuthorizationSettings) (*mtproto.Bool, error) {
	// AuthsessionClient has no RPC that updates encrypted_requests_disabled,
	// call_requests_disabled, or confirmed. Persist per auth key instead of
	// resetting any session.
	keyId := c.MD.PermAuthKeyId
	if in.GetHash() != 0 {
		keyId = in.GetHash()
	}

	flags := authorizationSettingsFlags{}
	if prev, err := c.svcCtx.Dao.GetCachePhoneCode(c.ctx, keyId, authorizationSettingsKey); err == nil && prev != nil && prev.PhoneNumber == authorizationSettingsKey && prev.PhoneCodeExtraData != "" {
		_ = json.Unmarshal([]byte(prev.PhoneCodeExtraData), &flags)
	}
	flags.Confirmed = in.GetConfirmed()
	if v := in.GetEncryptedRequestsDisabled(); v != nil {
		b := mtproto.FromBool(v)
		flags.EncryptedRequestsDisabled = &b
	}
	if v := in.GetCallRequestsDisabled(); v != nil {
		b := mtproto.FromBool(v)
		flags.CallRequestsDisabled = &b
	}

	raw, err := json.Marshal(flags)
	if err != nil {
		c.Logger.Errorf("account.changeAuthorizationSettings - encode error: %v", err)
		return nil, err
	}
	codeData := &model.PhoneCodeTransaction{
		AuthKeyId:          keyId,
		SessionId:          c.MD.SessionId,
		PhoneNumber:        authorizationSettingsKey,
		PhoneCodeExtraData: string(raw),
		State:              model.CodeStateOk,
	}
	if in.GetHash() != 0 {
		codeData.SessionId = 0
	}
	if err = c.svcCtx.Dao.PutCachePhoneCode(c.ctx, keyId, authorizationSettingsKey, codeData); err != nil {
		c.Logger.Errorf("account.changeAuthorizationSettings - store error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

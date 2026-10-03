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
	"encoding/json"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	webauthn "github.com/go-webauthn/webauthn/webauthn"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/passkey/internal/dao"
	authsession "github.com/teamgram/teamgram-server/app/service/authsession/authsession"
)

// AuthFinishPasskeyLogin
// auth.finishPasskeyLogin#9857ad07 flags:# credential:InputPasskeyCredential from_dc_id:flags.0?int from_auth_key_id:flags.0?long = auth.Authorization;
func (c *PasskeyCore) AuthFinishPasskeyLogin(in *mtproto.TLAuthFinishPasskeyLogin) (*mtproto.Auth_Authorization, error) {
	if in == nil || in.GetCredential() == nil {
		return nil, mtproto.ErrAuthTokenInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	err := c.requireProvider()
	if err != nil {
		return nil, err
	}
	if fromDC := in.GetFromDcId(); fromDC != nil {
		if err = validatePasskeySourceDc(c.svcCtx.Config.DcId, fromDC.GetValue()); err != nil {
			return nil, err
		}
	}
	payload, err := credentialResponsePayload(in.GetCredential(), false)
	if err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(payload)
	if err != nil {
		return nil, mtproto.ErrAuthTokenInvalid
	}
	challenge := sessionChallengeLogin(parsed)
	record, err := c.svcCtx.Dao.GetSession(c.ctx, challenge, dao.SessionLogin)
	if err != nil {
		return nil, passkeyStorageError(err)
	}
	var session webauthn.SessionData
	if err = json.Unmarshal(record.Data, &session); err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	w, err := c.webAuthn()
	if err != nil {
		return nil, passkeyStorageError(err)
	}
	var authenticatedUser *passkeyUser
	user, credential, err := w.ValidatePasskeyLogin(func(_ []byte, userHandle []byte) (webauthn.User, error) {
		userID, ok := userHandleID(userHandle)
		if !ok {
			return nil, mtproto.ErrAuthTokenInvalid
		}
		loaded, loadErr := c.loadUser(c.ctx, userID)
		if loadErr == nil {
			authenticatedUser = loaded
		}
		return loaded, loadErr
	}, session, parsed)
	if err != nil || user == nil || credential == nil || authenticatedUser == nil {
		return nil, mtproto.ErrAuthTokenInvalid
	}
	if credential.Authenticator.CloneWarning {
		return nil, mtproto.ErrAuthTokenInvalid
	}
	stored, err := c.svcCtx.Dao.GetCredential(c.ctx, credential.ID)
	if err != nil {
		return nil, passkeyStorageError(err)
	}
	if stored.UserID != authenticatedUser.id {
		return nil, mtproto.ErrAuthTokenInvalid
	}
	data, err := json.Marshal(credential)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if err = c.svcCtx.Dao.ConsumeLoginAndUpdateCredential(c.ctx, challenge, dao.SessionLogin, credential.ID, stored.SignCount, credential.Authenticator.SignCount, data, time.Now().Unix()); err != nil {
		return nil, passkeyStorageError(err)
	}
	if c.svcCtx.Dao.AuthsessionClient == nil || c.MD == nil {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	authKeyID := int64(0)
	if in.GetFromAuthKeyId() != nil {
		authKeyID = in.GetFromAuthKeyId().GetValue()
	}
	if authKeyID == 0 {
		authKeyID = c.MD.PermAuthKeyId
	}
	if authKeyID == 0 {
		authKeyID = c.MD.AuthId
	}
	if authKeyID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if _, err = c.svcCtx.Dao.AuthsessionClient.AuthsessionBindAuthKeyUser(c.ctx, &authsession.TLAuthsessionBindAuthKeyUser{
		AuthKeyId: authKeyID,
		UserId:    authenticatedUser.id,
	}); err != nil {
		return nil, err
	}
	request := &structUserRequest{Id: authenticatedUser.id}
	immutable, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, request.TL())
	if err != nil || immutable == nil || immutable.GetUser() == nil {
		return nil, passkeyStorageError(err)
	}
	return mtproto.MakeTLAuthAuthorization(&mtproto.Auth_Authorization{
		User: immutable.ToSelfUser(),
	}).To_Auth_Authorization(), nil
}

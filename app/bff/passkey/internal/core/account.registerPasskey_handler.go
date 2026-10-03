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
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/go-webauthn/webauthn/protocol"
	webauthn "github.com/go-webauthn/webauthn/webauthn"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/passkey/internal/dao"
)

// AccountRegisterPasskey
// account.registerPasskey#55b41fd6 credential:InputPasskeyCredential = Passkey;
func (c *PasskeyCore) AccountRegisterPasskey(in *mtproto.TLAccountRegisterPasskey) (*mtproto.Passkey, error) {
	userID, err := passkeyRequireUser(c)
	if err != nil {
		return nil, err
	}
	if in == nil || in.GetCredential() == nil {
		return nil, mtproto.ErrAuthTokenInvalid
	}
	if err = c.requireProvider(); err != nil {
		return nil, err
	}
	if c.svcCtx.Dao == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	payload, err := credentialResponsePayload(in.GetCredential(), true)
	if err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(payload)
	if err != nil {
		return nil, mapPasskeyVerificationError(err)
	}
	challenge := sessionChallengeRegistration(parsed)
	record, err := c.svcCtx.Dao.GetSession(c.ctx, challenge, dao.SessionRegistration)
	if err != nil {
		return nil, passkeyStorageError(err)
	}
	if record.UserID != userID {
		return nil, mtproto.ErrAuthTokenInvalid
	}
	var session webauthn.SessionData
	if err = json.Unmarshal(record.Data, &session); err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	user, err := c.loadUser(c.ctx, userID)
	if err != nil {
		return nil, passkeyStorageError(err)
	}
	w, err := c.webAuthn()
	if err != nil {
		return nil, passkeyStorageError(err)
	}
	credential, err := w.CreateCredential(user, session, parsed)
	if err != nil {
		return nil, mapPasskeyVerificationError(err)
	}
	if credential == nil || len(credential.ID) == 0 || len(credential.PublicKey) == 0 {
		return nil, mtproto.ErrAuthTokenInvalid
	}
	stored, err := recordFromCredential(userID, credential, "Passkey")
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if err = c.svcCtx.Dao.ConsumeAndInsertCredential(c.ctx, challenge, dao.SessionRegistration, &stored); err != nil {
		// A duplicate credential is an invalid replay, never a successful
		// registration response.
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, mtproto.ErrInternalServerError
		}
		return nil, mtproto.ErrAuthTokenInvalid
	}
	return mtproto.MakeTLPasskey(&mtproto.Passkey{
		Id:            base64.RawURLEncoding.EncodeToString(credential.ID),
		Name:          stored.Name,
		Date:          int32(stored.Date),
		LastUsageDate: nil,
	}).To_Passkey(), nil
}

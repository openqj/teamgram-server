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
	"errors"

	"github.com/go-webauthn/webauthn/protocol"
	webauthn "github.com/go-webauthn/webauthn/webauthn"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/passkey/internal/dao"
)

// AccountInitPasskeyRegistration
// account.initPasskeyRegistration#429547e8 = account.PasskeyRegistrationOptions;
func (c *PasskeyCore) AccountInitPasskeyRegistration(in *mtproto.TLAccountInitPasskeyRegistration) (*mtproto.Account_PasskeyRegistrationOptions, error) {
	userID, err := passkeyRequireUser(c)
	if err != nil {
		return nil, err
	}
	_ = in
	if err = c.requireProvider(); err != nil {
		return nil, err
	}
	if c.svcCtx.Dao == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	user, err := c.loadUser(c.ctx, userID)
	if err != nil {
		return nil, passkeyStorageError(err)
	}
	w, err := c.webAuthn()
	if err != nil {
		return nil, passkeyStorageError(err)
	}
	exclusions := make([]protocol.CredentialDescriptor, 0, len(user.credentials))
	for _, credential := range user.credentials {
		exclusions = append(exclusions, credential.Descriptor())
	}
	creation, session, err := w.BeginRegistration(
		user,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(exclusions),
	)
	if err != nil {
		return nil, mtproto.ErrAuthTokenInvalid
	}
	options, err := json.Marshal(creation)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	sessionData, err := json.Marshal(session)
	if err != nil {
		return nil, mtproto.ErrInternalServerError
	}
	if err = c.svcCtx.Dao.PutSession(c.ctx, &dao.Session{
		Challenge: session.Challenge,
		UserID:    userID,
		Kind:      dao.SessionRegistration,
		Data:      sessionData,
		ExpiresAt: session.Expires.Unix(),
	}); err != nil {
		return nil, passkeyStorageError(err)
	}
	data := &mtproto.DataJSON{Data: string(options)}
	return mtproto.MakeTLAccountPasskeyRegistrationOptions(&mtproto.Account_PasskeyRegistrationOptions{
		Options: data,
	}).To_Account_PasskeyRegistrationOptions(), nil
}

func passkeyStorageError(err error) error {
	if errors.Is(err, dao.ErrUnavailable) || errors.Is(err, mtproto.ErrMethodNotImpl) {
		return mtproto.ErrMethodNotImpl
	}
	if errors.Is(err, dao.ErrSessionNotFound) || errors.Is(err, dao.ErrSessionUsed) || errors.Is(err, dao.ErrCredentialNotFound) || errors.Is(err, dao.ErrCounterReplay) {
		return mtproto.ErrAuthTokenInvalid
	}
	return mtproto.ErrInternalServerError
}

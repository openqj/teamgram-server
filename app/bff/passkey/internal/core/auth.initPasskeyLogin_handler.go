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

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/passkey/internal/dao"
)

// AuthInitPasskeyLogin
// auth.initPasskeyLogin#518ad0b7 api_id:int api_hash:string = auth.PasskeyLoginOptions;
func (c *PasskeyCore) AuthInitPasskeyLogin(in *mtproto.TLAuthInitPasskeyLogin) (*mtproto.Auth_PasskeyLoginOptions, error) {
	if in == nil || in.GetApiId() <= 0 || !validPasskeyAPIHash(in.GetApiHash()) {
		return nil, mtproto.ErrApiIdInvalid
	}
	if err := c.requireProvider(); err != nil {
		return nil, err
	}
	if c.svcCtx.Dao == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if !passkeyTrustedApp(c.svcCtx.Config.Provider, in.GetApiId(), in.GetApiHash()) {
		return nil, mtproto.ErrApiIdInvalid
	}
	w, err := (&PasskeyCore{svcCtx: c.svcCtx, ctx: c.ctx}).webAuthn()
	if err != nil {
		return nil, err
	}
	creation, session, err := w.BeginDiscoverableLogin(
		webauthn.WithUserVerification("preferred"),
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
		Kind:      dao.SessionLogin,
		Data:      sessionData,
		ExpiresAt: session.Expires.Unix(),
	}); err != nil {
		return nil, passkeyStorageError(err)
	}
	return mtproto.MakeTLAuthPasskeyLoginOptions(&mtproto.Auth_PasskeyLoginOptions{
		Options: passkeyDataJSON(string(options)),
	}).To_Auth_PasskeyLoginOptions(), nil
}

func validPasskeyAPIHash(apiHash string) bool {
	if len(apiHash) != 32 {
		return false
	}
	for _, ch := range apiHash {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return false
		}
	}
	return true
}

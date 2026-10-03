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
	"encoding/binary"
	"fmt"
	"time"

	"github.com/teamgram/proto/mtproto"
	qrcodeconfig "github.com/teamgram/teamgram-server/app/bff/qrcode/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/model"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/rpc/dccontext"

	"google.golang.org/grpc/status"
)

// AuthImportLoginToken
// auth.importLoginToken#95ac5ce4 token:bytes = auth.LoginToken;
// A token exported on another configured DC is returned as auth.LoginTokenMigrateTo;
// the token is consumed only on the DC that owns the exported auth key.
func (c *QrCodeCore) AuthImportLoginToken(in *mtproto.TLAuthImportLoginToken) (*mtproto.Auth_LoginToken, error) {
	c.ensureLogger()
	if in == nil || len(in.GetToken()) != 24 {
		err := mtproto.ErrAuthTokenInvalid
		if c != nil {
			c.Logger.Errorf("auth.importLoginToken - error: %v", err)
		}
		return nil, err
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrMethodNotImpl
	}

	token := in.GetToken()
	keyId := int64(binary.BigEndian.Uint64(token))
	qrCode, err := c.svcCtx.Dao.GetCacheQRLoginCode(c.ctx, keyId)
	if err != nil || qrCode == nil {
		err := mtproto.ErrAuthTokenExpired
		c.Logger.Errorf("auth.importLoginToken - error: %v", err)
		return nil, err
	}
	if !qrCode.CheckByToken(token) {
		err := mtproto.ErrAuthTokenInvalid
		c.Logger.Errorf("auth.importLoginToken - error: %v", err)
		return nil, err
	}
	currentDCID := c.svcCtx.Config.DcId
	if actualDCID, ok := dccontext.DCID(c.ctx); ok {
		currentDCID = actualDCID
	}
	if migrationDCID, migrationErr := qrLoginTokenMigrationTarget(c.svcCtx.Config, currentDCID, qrCode.DcId); migrationErr != nil {
		return nil, migrationErr
	} else if migrationDCID > 0 {
		return mtproto.MakeTLAuthLoginTokenMigrateTo(&mtproto.Auth_LoginToken{
			DcId:  migrationDCID,
			Token: qrCode.Token(),
		}).To_Auth_LoginToken(), nil
	}

	switch qrCode.State {
	case model.QRCodeStateSuccess:
		if c.svcCtx.Dao.AuthsessionClient == nil || c.svcCtx.Dao.UserClient == nil {
			return nil, mtproto.ErrMethodNotImpl
		}
		boundUser, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionGetUserId(c.ctx, &authsession.TLAuthsessionGetUserId{
			AuthKeyId: qrCode.AuthKeyId,
		})
		if err != nil {
			c.Logger.Errorf("auth.importLoginToken - auth key binding lookup error: %v", err)
			return nil, err
		}
		if err = verifyQRLoginAuthKeyBinding(qrCode.UserId, boundUser); err != nil {
			return nil, err
		}
		user, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
			Id: qrCode.UserId,
		})
		if err != nil {
			c.Logger.Errorf("auth.importLoginToken - error: %v", err)
			return nil, err
		}
		if user == nil || user.GetUser() == nil || user.GetUser().GetId() != qrCode.UserId {
			return nil, mtproto.ErrUserIdInvalid
		}
		passwordNeeded, err := c.svcCtx.Dao.CheckSessionPasswordNeeded(user.User.Id)
		if err != nil {
			return nil, err
		}
		if c.svcCtx.Plugin != nil && c.svcCtx.Plugin.CheckSessionPasswordNeeded(c.ctx, user.User.Id) {
			passwordNeeded = true
		}
		if passwordNeeded {
			err = status.Error(mtproto.ErrUnauthorized, fmt.Sprintf("SESSION_PASSWORD_NEEDED_%d", user.Id()))
			c.Logger.Infof("auth.importLoginToken - registered, next step auth.checkPassword: %v", err)
			return nil, err
		}
		if err = c.svcCtx.Dao.DeleteCacheQRLoginCode(c.ctx, keyId); err != nil {
			c.Logger.Errorf("auth.importLoginToken - error: %v", err)
			return nil, err
		}
		return mtproto.MakeTLAuthLoginTokenSuccess(&mtproto.Auth_LoginToken{
			Authorization: mtproto.MakeTLAuthAuthorization(&mtproto.Auth_Authorization{
				SetupPasswordRequired: false,
				User:                  user.ToSelfUser(),
			}).To_Auth_Authorization(),
		}).To_Auth_LoginToken(), nil
	case model.QRCodeStateNew, model.QRCodeStateAccepted:
		if qrCode.ExpireAt < time.Now().Unix() {
			if deleteErr := c.svcCtx.Dao.DeleteCacheQRLoginCode(c.ctx, keyId); deleteErr != nil {
				c.Logger.Errorf("auth.importLoginToken - expired token cleanup error: %v", deleteErr)
				return nil, deleteErr
			}
			err := mtproto.ErrAuthTokenExpired
			c.Logger.Errorf("auth.importLoginToken - error: %v", err)
			return nil, err
		}
		return mtproto.MakeTLAuthLoginToken(&mtproto.Auth_LoginToken{
			Expires: int32(qrCode.ExpireAt),
			Token:   qrCode.Token(),
		}).To_Auth_LoginToken(), nil
	default:
		err := mtproto.ErrAuthTokenInvalid
		c.Logger.Errorf("auth.importLoginToken - error: %v", err)
		return nil, err
	}
}

// qrLoginTokenMigrationTarget returns the DC that owns the QR auth key when a
// poll arrives at another configured DC. A zero result preserves legacy
// single-DC deployments that do not set a DC identity.
func qrLoginTokenMigrationTarget(cfg qrcodeconfig.Config, currentDCID, sourceDCID int32) (int32, error) {
	if sourceDCID == 0 && cfg.DcId == 0 {
		return 0, nil
	}
	if sourceDCID <= 0 || currentDCID <= 0 || !cfg.SupportsDc(currentDCID) || !cfg.SupportsDc(sourceDCID) {
		return 0, mtproto.ErrDcIdInvalid
	}
	if sourceDCID == currentDCID {
		return 0, nil
	}
	return sourceDCID, nil
}

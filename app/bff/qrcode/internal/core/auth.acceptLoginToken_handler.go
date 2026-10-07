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
	"context"
	"encoding/binary"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/model"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/rpc/dccontext"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

type qrAuthKeyBinder interface {
	AuthsessionGetUserId(context.Context, *authsession.TLAuthsessionGetUserId) (*mtproto.Int64, error)
	AuthsessionBindAuthKeyUser(context.Context, *authsession.TLAuthsessionBindAuthKeyUser) (*mtproto.Int64, error)
}

// AuthAcceptLoginToken
// auth.acceptLoginToken#e894ad4d token:bytes = Authorization;
func (c *QrCodeCore) AuthAcceptLoginToken(in *mtproto.TLAuthAcceptLoginToken) (*mtproto.Authorization, error) {
	c.ensureLogger()
	// 8 + 16
	if in == nil || len(in.GetToken()) != 24 {
		err := mtproto.ErrAuthTokenInvalid
		if c != nil {
			c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		}
		return nil, err
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if c.MD == nil || c.MD.GetUserId() <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx.Dao.AuthsessionClient == nil || c.svcCtx.Dao.UserClient == nil || c.svcCtx.Dao.SyncClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}

	token := in.GetToken()
	var (
		keyId = int64(binary.BigEndian.Uint64(token))
	)

	qrCode, err := c.svcCtx.Dao.GetCacheQRLoginCode(c.ctx, keyId)
	if err != nil || qrCode == nil {
		err := mtproto.ErrAuthTokenExpired
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	}

	c.Logger.Infof("auth.acceptLoginToken - state=%d expires_at=%d", qrCode.State, qrCode.ExpireAt)

	if !qrCode.CheckByToken(token) {
		err := mtproto.ErrAuthTokenInvalid
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	}

	now := time.Now().Unix()
	if qrCode.ExpireAt < now {
		return nil, mtproto.ErrAuthTokenExpired
	}

	switch qrCode.State {
	case model.QRCodeStateNew, model.QRCodeStateAccepted:
		// ok
	case model.QRCodeStateSuccess:
		err := mtproto.ErrAuthTokenAccepted
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	default:
		err := mtproto.ErrAuthTokenInvalid
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	}

	userID := c.MD.GetUserId()
	if userID <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if err = checkQRLoginExceptIDs(qrCode, userID); err != nil {
		return nil, err
	}

	user, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
		Id: userID,
	})
	if err != nil {
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	}
	if user == nil || user.Id() != userID {
		return nil, mtproto.ErrUserIdInvalid
	}

	acceptDCID := c.svcCtx.Config.DcId
	if actualDCID, ok := dccontext.DCID(c.ctx); ok {
		acceptDCID = actualDCID
	}
	if c.svcCtx.Config.DcId > 0 && !c.svcCtx.Config.SupportsDc(acceptDCID) {
		return nil, mtproto.ErrDcIdInvalid
	}
	var acceptResult int64
	if acceptDCID > 0 {
		acceptResult, err = c.svcCtx.Dao.AcceptCacheQRLoginCodeAtDC(c.ctx, keyId, qrCode.CodeHash, userID, now, acceptDCID)
	} else {
		acceptResult, err = c.svcCtx.Dao.AcceptCacheQRLoginCode(c.ctx, keyId, qrCode.CodeHash, userID, now)
	}
	if err != nil {
		c.Logger.Errorf("auth.acceptLoginToken - claim error: %v", err)
		return nil, err
	}
	switch acceptResult {
	case 1:
		// The QR transaction is now claimed by exactly this account.
	case 2:
		// A previous bind attempt had an ambiguous result. The same account can
		// resume it until authsession ownership is confirmed.
	case 0:
		return nil, mtproto.ErrAuthTokenAccepted
	case -1:
		return nil, mtproto.ErrAuthTokenInvalid
	case -2:
		return nil, mtproto.ErrAuthTokenExpired
	default:
		return nil, mtproto.ErrInternalServerError
	}

	if err = ensureQRLoginAuthKeyBinding(c.ctx, c.svcCtx.Dao.AuthsessionClient, qrCode.AuthKeyId, user.Id()); err != nil {
		c.Logger.Errorf("auth.acceptLoginToken - bind pending: %v", err)
		return nil, err
	}
	commitResult, err := c.svcCtx.Dao.CommitCacheQRLoginCode(c.ctx, keyId, qrCode.CodeHash, userID)
	if err != nil {
		c.Logger.Errorf("auth.acceptLoginToken - commit error: %v", err)
		return nil, err
	}
	if commitResult != 1 && commitResult != 2 {
		return nil, mtproto.ErrAuthTokenInvalid
	}

	authorization, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionGetAuthorization(c.ctx, &authsession.TLAuthsessionGetAuthorization{
		AuthKeyId: qrCode.AuthKeyId,
	})
	if err != nil {
		c.Logger.Errorf("auth.acceptLoginToken - error: %v", err)
		return nil, err
	}
	if authorization == nil {
		return nil, mtproto.ErrInternalServerError
	}

	authorization.DateCreated = int32(time.Now().Unix())
	authorization.DateActive = authorization.DateCreated

	if c.svcCtx.Dao.SyncClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if _, err = c.svcCtx.Dao.SyncClient.SyncUpdatesMe(
		c.ctx,
		&sync.TLSyncUpdatesMe{
			UserId:        user.Id(),
			PermAuthKeyId: qrCode.PermAuthKeyId,
			ServerId:      &wrapperspb.StringValue{Value: qrCode.ServerId},
			AuthKeyId:     &wrapperspb.Int64Value{Value: qrCode.AuthKeyId},
			SessionId:     &wrapperspb.Int64Value{Value: qrCode.SessionId},
			Updates: mtproto.MakeTLUpdateShort(&mtproto.Updates{
				Update: mtproto.MakeTLUpdateLoginToken(nil).To_Update(),
				Date:   int32(time.Now().Unix()),
			}).To_Updates(),
		}); err != nil {
		c.Logger.Errorf("auth.acceptLoginToken - sync error: %v", err)
		return nil, err
	}

	return authorization, nil
}

func ensureQRLoginAuthKeyBinding(ctx context.Context, binder qrAuthKeyBinder, authKeyID, userID int64) error {
	boundUser, err := binder.AuthsessionGetUserId(ctx, &authsession.TLAuthsessionGetUserId{AuthKeyId: authKeyID})
	if err != nil {
		return err
	}
	if boundUser != nil && boundUser.GetV() == userID {
		return nil
	}
	if boundUser != nil && boundUser.GetV() != 0 {
		return mtproto.ErrAuthTokenInvalid
	}

	_, bindErr := binder.AuthsessionBindAuthKeyUser(ctx, &authsession.TLAuthsessionBindAuthKeyUser{
		AuthKeyId: authKeyID,
		UserId:    userID,
	})
	if bindErr == nil {
		return nil
	}

	// A transport error can arrive after authsession committed. Confirm the
	// owner before leaving the Redis claim pending for an idempotent retry.
	boundUser, confirmErr := binder.AuthsessionGetUserId(ctx, &authsession.TLAuthsessionGetUserId{AuthKeyId: authKeyID})
	if confirmErr == nil && boundUser != nil && boundUser.GetV() == userID {
		return nil
	}
	return bindErr
}

func checkQRLoginExceptIDs(qrCode *model.QRCodeTransaction, userID int64) error {
	if qrCode.ExcludesUser(userID) {
		return mtproto.ErrAuthTokenInvalid
	}
	return nil
}

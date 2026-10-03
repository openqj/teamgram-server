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
	"sort"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/crypto"
	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/model"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/rpc/dccontext"

	"google.golang.org/grpc/status"
)

const (
	qrCodeTimeout = 60 // salt timeout
)

// AuthExportLoginToken
// auth.exportLoginToken#b7e085fe api_id:int api_hash:string except_ids:Vector<long> = auth.LoginToken;
func (c *QrCodeCore) AuthExportLoginToken(in *mtproto.TLAuthExportLoginToken) (*mtproto.Auth_LoginToken, error) {
	c.ensureLogger()
	if in == nil {
		return nil, mtproto.ErrApiIdInvalid
	}
	apiHash, err := canonicalQRAppHash(in.GetApiId(), in.GetApiHash())
	if err != nil {
		return nil, err
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.MD == nil {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx.Dao.AuthsessionClient == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	dcID := c.svcCtx.Config.DcId
	if actualDCID, ok := dccontext.DCID(c.ctx); ok {
		dcID = actualDCID
	}
	if c.svcCtx.Config.DcId > 0 && !c.svcCtx.Config.SupportsDc(dcID) {
		return nil, mtproto.ErrDcIdInvalid
	}
	exceptIDs, err := normalizeQRExceptIDs(in.GetExceptIds())
	if err != nil {
		return nil, err
	}
	if err := validateQRPermAuthKeyID(c.MD.PermAuthKeyId); err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	qrCode, err := c.svcCtx.Dao.GetCacheQRLoginCode(c.ctx, c.MD.PermAuthKeyId)
	if err != nil {
		c.Logger.Errorf("getQRCode - error: %v", err)
		return nil, err
	}
	if qrCode != nil && c.svcCtx.Config.DcId > 0 && (qrCode.DcId <= 0 || qrCode.DcId != dcID) {
		return nil, mtproto.ErrDcIdInvalid
	}
	newQRCode := func() *model.QRCodeTransaction {
		return &model.QRCodeTransaction{
			PermAuthKeyId: c.MD.PermAuthKeyId,
			DcId:          dcID,
			AuthKeyId:     c.MD.AuthId,
			SessionId:     c.MD.SessionId,
			ServerId:      c.MD.ServerId,
			ApiId:         in.GetApiId(),
			ApiHash:       apiHash,
			ExceptIDs:     exceptIDs,
			CodeHash:      crypto.GenerateStringNonce(16),
			ExpireAt:      now + qrCodeTimeout,
			UserId:        0,
			State:         model.QRCodeStateNew,
		}
	}
	if qrCode == nil {
		qrCode = newQRCode()
		c.Logger.Infof("putQRCode - state=%d expires_at=%d", qrCode.State, qrCode.ExpireAt)
		created, createErr := c.svcCtx.Dao.CreateCacheQRLoginCode(c.ctx, c.MD.PermAuthKeyId, qrCode, qrCodeTimeout+2)
		if createErr != nil {
			err = createErr
			c.Logger.Errorf("putQRCode - error: %v", err)
			return nil, err
		}
		if created != 1 {
			return nil, mtproto.ErrAuthTokenInvalid
		}
	} else if qrCode.State == model.QRCodeStateNew &&
		(qrCode.ExpireAt < now || qrCode.ApiId != in.GetApiId() || qrCode.ApiHash != apiHash || !sameQRIDs(qrCode.ExceptIDs, exceptIDs)) {
		oldCodeHash := qrCode.CodeHash
		qrCode = newQRCode()
		c.Logger.Infof("rotateQRCode - state=%d expires_at=%d", qrCode.State, qrCode.ExpireAt)
		rotated, rotateErr := c.svcCtx.Dao.RotateCacheQRLoginCode(c.ctx, c.MD.PermAuthKeyId, oldCodeHash, qrCode, qrCodeTimeout+2)
		if rotateErr != nil {
			c.Logger.Errorf("rotateQRCode - error: %v", rotateErr)
			return nil, rotateErr
		}
		if rotated != 1 {
			// Another accept or refresh won the compare-and-set. Do not issue this stale token.
			return nil, mtproto.ErrAuthTokenInvalid
		}
	}

	var (
		rQRLoginToken *mtproto.Auth_LoginToken
	)

	switch qrCode.State {
	case model.QRCodeStateSuccess:
		if qrCode.ApiId != in.GetApiId() || qrCode.ApiHash != apiHash {
			return nil, mtproto.ErrApiIdInvalid
		}
		if qrCode.UserId <= 0 || qrCode.ExcludesUser(qrCode.UserId) || containsQRID(exceptIDs, qrCode.UserId) {
			return nil, mtproto.ErrAuthTokenInvalid
		}
		boundUser, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionGetUserId(c.ctx, &authsession.TLAuthsessionGetUserId{
			AuthKeyId: qrCode.AuthKeyId,
		})
		if err != nil {
			c.Logger.Errorf("auth.exportLoginToken - auth key binding lookup error: %v", err)
			return nil, err
		}
		if err = verifyQRLoginAuthKeyBinding(qrCode.UserId, boundUser); err != nil {
			return nil, err
		}
		//// Check SESSION_PASSWORD_NEEDED
		//if sessionPasswordNeeded, _ := c.svcCtx.Dao.TwofaClient.TwofaCheckSessionPasswordNeeded(c.ctx, &twofa.TLTwofaCheckSessionPasswordNeeded{
		//	UserId: qrCode.UserId,
		//}); mtproto.FromBool(sessionPasswordNeeded) {
		//	err = mtproto.ErrSessionPasswordNeeded
		//	c.Logger.Infof("auth.exportLoginToken - registered, next step auth.checkPassword: %v", err)
		//	return nil, err
		//}

		user, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
			Id: qrCode.UserId,
		})
		if err != nil {
			c.Logger.Errorf("auth.exportLoginToken - error: %v", err)
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
			c.Logger.Infof("auth.exportLoginToken - registered, next step auth.checkPassword: %v", err)
			return nil, err
		}

		rQRLoginToken = mtproto.MakeTLAuthLoginTokenSuccess(&mtproto.Auth_LoginToken{
			Authorization: mtproto.MakeTLAuthAuthorization(&mtproto.Auth_Authorization{
				SetupPasswordRequired: false,
				OtherwiseReloginDays:  nil,
				TmpSessions:           nil,
				FutureAuthToken:       nil,
				User:                  user.ToSelfUser(),
			}).To_Auth_Authorization(),
		}).To_Auth_LoginToken()
	case model.QRCodeStateNew, model.QRCodeStateAccepted:
		rQRLoginToken = mtproto.MakeTLAuthLoginToken(&mtproto.Auth_LoginToken{
			Expires: int32(qrCode.ExpireAt),
			Token:   qrCode.Token(),
		}).To_Auth_LoginToken()
	default:
		return nil, mtproto.ErrAuthTokenInvalid
	}

	return rQRLoginToken, nil
}

func verifyQRLoginAuthKeyBinding(expectedUserID int64, boundUser *mtproto.Int64) error {
	if expectedUserID <= 0 || boundUser == nil || boundUser.GetV() != expectedUserID {
		return mtproto.ErrAuthTokenInvalid
	}
	return nil
}

func validateQRPermAuthKeyID(authKeyID int64) error {
	if authKeyID == 0 {
		return mtproto.ErrAuthKeyInvalid
	}
	return nil
}

func validateQRAppCredentials(apiID int32, apiHash string) error {
	if apiID <= 0 || len(apiHash) != 32 {
		return mtproto.ErrApiIdInvalid
	}
	for _, ch := range apiHash {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return mtproto.ErrApiIdInvalid
		}
	}
	// This service has no trusted app registry, so this is format validation only.
	return nil
}

func canonicalQRAppHash(apiID int32, apiHash string) (string, error) {
	if err := validateQRAppCredentials(apiID, apiHash); err != nil {
		return "", err
	}
	return strings.ToLower(apiHash), nil
}

func normalizeQRExceptIDs(ids []int64) ([]int64, error) {
	out := append([]int64(nil), ids...)
	for _, id := range out {
		if id <= 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	if len(out) == 0 {
		return []int64{}, nil
	}
	unique := out[:1]
	for _, id := range out[1:] {
		if id != unique[len(unique)-1] {
			unique = append(unique, id)
		}
	}
	return unique, nil
}

func sameQRIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsQRID(ids []int64, id int64) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

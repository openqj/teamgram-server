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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/rpc/dccontext"
)

type exportedAuthorizationImporter interface {
	GetExportedAuthorization(context.Context, []byte) (*dao.ExportedAuthorization, error)
	ClaimExportedAuthorization(context.Context, []byte, int64, int32) (*dao.ExportedAuthorization, error)
	CompleteExportedAuthorization(context.Context, []byte, int64) (bool, error)
	AuthsessionGetUserId(context.Context, *authsession.TLAuthsessionGetUserId) (*mtproto.Int64, error)
	UserGetImmutableUser(context.Context, *userpb.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error)
	AuthsessionBindAuthKeyUser(context.Context, *authsession.TLAuthsessionBindAuthKeyUser) (*mtproto.Int64, error)
}

// AuthImportAuthorization
// auth.importAuthorization#a57a7dad id:long bytes:bytes = auth.Authorization;
func (c *AuthorizationCore) AuthImportAuthorization(in *mtproto.TLAuthImportAuthorization) (*mtproto.Auth_Authorization, error) {
	if in == nil || in.GetId() <= 0 || len(in.GetBytes()) != dao.ExportedAuthorizationTokenSize {
		return nil, mtproto.ErrAuthBytesInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.MD == nil || c.MD.GetPermAuthKeyId() == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	currentDCID := c.svcCtx.Config.DcId
	if actualDCID, ok := dccontext.DCID(c.ctx); ok {
		currentDCID = actualDCID
	}
	if currentDCID <= 0 || !c.svcCtx.Config.SupportsDc(currentDCID) {
		return nil, mtproto.ErrDcIdInvalid
	}
	// Validate both ends before consuming the one-time credential. The target
	// must be the DC selected by the incoming transport, and the source must be
	// one of the configured authorities for this shared deployment.
	exported, err := c.svcCtx.Dao.GetExportedAuthorization(c.ctx, in.GetBytes())
	if err != nil {
		return nil, err
	}
	if exported == nil || exported.TargetDCID != currentDCID || exported.SourceDCID <= 0 || !c.svcCtx.Config.SupportsDc(exported.SourceDCID) || exported.SourceDCID == currentDCID {
		return nil, mtproto.ErrAuthBytesInvalid
	}

	return c.importAuthorization(in, currentDCID, c.svcCtx.Dao)
}

func (c *AuthorizationCore) importAuthorization(in *mtproto.TLAuthImportAuthorization, currentDCID int32, importer exportedAuthorizationImporter) (*mtproto.Auth_Authorization, error) {
	return c.importAuthorizationWithSupport(in, currentDCID, nil, importer)
}

// importAuthorizationWithSupport validates a transfer against the configured
// shared-DC set. A nil supportsDc callback retains the single-DC behavior used
// by focused callers and older deployments.
func (c *AuthorizationCore) importAuthorizationWithSupport(in *mtproto.TLAuthImportAuthorization, currentDCID int32, supportsDc func(int32) bool, importer exportedAuthorizationImporter) (*mtproto.Auth_Authorization, error) {
	targetAuthKeyID := c.MD.GetPermAuthKeyId()
	exported, err := importer.GetExportedAuthorization(c.ctx, in.GetBytes())
	if err != nil {
		c.Logger.Errorf("auth.importAuthorization - read credential: %v", err)
		return nil, err
	}
	if exported == nil || exported.ID != in.GetId() {
		return nil, mtproto.ErrAuthBytesInvalid
	}
	if supportsDc == nil {
		if exported.TargetDCID != currentDCID {
			return nil, mtproto.ErrAuthBytesInvalid
		}
	} else if !supportsDc(exported.TargetDCID) {
		return nil, mtproto.ErrAuthBytesInvalid
	}
	targetDCID := currentDCID
	if supportsDc != nil {
		targetDCID = exported.TargetDCID
	}

	targetUserID, err := importer.AuthsessionGetUserId(c.ctx, &authsession.TLAuthsessionGetUserId{
		AuthKeyId: targetAuthKeyID,
	})
	if err != nil {
		c.Logger.Errorf("auth.importAuthorization - target auth key lookup: %v", err)
		return nil, err
	}
	if targetUserID == nil {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if targetUserID.GetV() != 0 && (targetUserID.GetV() != exported.UserID || (exported.State != dao.ExportedAuthorizationBinding && exported.State != dao.ExportedAuthorizationComplete) || exported.TargetAuthKeyID != targetAuthKeyID) {
		return nil, mtproto.ErrAuthBytesInvalid
	}
	if exported.State == dao.ExportedAuthorizationComplete && (targetUserID.GetV() == 0 || exported.TargetAuthKeyID != targetAuthKeyID) {
		return nil, mtproto.ErrAuthBytesInvalid
	}

	sourceUserID, err := importer.AuthsessionGetUserId(c.ctx, &authsession.TLAuthsessionGetUserId{
		AuthKeyId: exported.SourceAuthKeyID,
	})
	if err != nil {
		c.Logger.Errorf("auth.importAuthorization - source auth key lookup: %v", err)
		return nil, err
	}
	if sourceUserID == nil || sourceUserID.GetV() != exported.UserID {
		return nil, mtproto.ErrAuthBytesInvalid
	}

	user, err := importer.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
		Id: exported.UserID,
	})
	if err != nil {
		c.Logger.Errorf("auth.importAuthorization - user lookup: %v", err)
		return nil, err
	}
	if user == nil {
		return nil, mtproto.ErrAuthBytesInvalid
	}

	claimed := exported
	if exported.State != dao.ExportedAuthorizationComplete {
		claimed, err = importer.ClaimExportedAuthorization(c.ctx, in.GetBytes(), targetAuthKeyID, targetDCID)
		if err != nil {
			c.Logger.Errorf("auth.importAuthorization - claim credential: %v", err)
			return nil, err
		}
	}
	if claimed == nil || claimed.ID != exported.ID || claimed.UserID != exported.UserID || claimed.SourceAuthKeyID != exported.SourceAuthKeyID || claimed.TargetDCID != targetDCID || claimed.TargetAuthKeyID != targetAuthKeyID {
		return nil, mtproto.ErrAuthBytesInvalid
	}

	if err = ensureImportedAuthorizationBinding(c.ctx, importer, targetAuthKeyID, exported.UserID); err != nil {
		c.Logger.Errorf("auth.importAuthorization - bind target auth key: %v", err)
		return nil, err
	}
	completed, err := importer.CompleteExportedAuthorization(c.ctx, in.GetBytes(), targetAuthKeyID)
	if err != nil {
		c.Logger.Errorf("auth.importAuthorization - complete credential: %v", err)
		return nil, err
	}
	if !completed {
		return nil, mtproto.ErrAuthBytesInvalid
	}

	return authAuthorization(user), nil
}

func ensureImportedAuthorizationBinding(ctx context.Context, importer exportedAuthorizationImporter, targetAuthKeyID, userID int64) error {
	bound, err := importer.AuthsessionGetUserId(ctx, &authsession.TLAuthsessionGetUserId{AuthKeyId: targetAuthKeyID})
	if err != nil {
		return err
	}
	if bound != nil && bound.GetV() == userID {
		return nil
	}
	if bound != nil && bound.GetV() != 0 {
		return mtproto.ErrAuthBytesInvalid
	}
	bindHash, bindErr := importer.AuthsessionBindAuthKeyUser(ctx, &authsession.TLAuthsessionBindAuthKeyUser{
		AuthKeyId: targetAuthKeyID,
		UserId:    userID,
	})
	if bindErr == nil && (bindHash == nil || bindHash.GetV() == 0) {
		// A nil or zero bind result is not proof that authsession committed the
		// ownership. Keep the Redis claim recoverable until a later retry returns
		// the durable bind hash.
		return mtproto.ErrInternalServerError
	}
	// Confirm the durable owner after a non-zero bind result and after transport
	// errors before completing the Redis transfer claim.
	bound, confirmErr := importer.AuthsessionGetUserId(ctx, &authsession.TLAuthsessionGetUserId{AuthKeyId: targetAuthKeyID})
	if confirmErr != nil {
		if bindErr != nil {
			return bindErr
		}
		return confirmErr
	}
	if bound != nil && bound.GetV() == userID {
		return nil
	}
	if bound != nil && bound.GetV() != 0 {
		return mtproto.ErrAuthBytesInvalid
	}
	if bindErr != nil {
		// A transport error can race with a successful authsession commit. The
		// zero owner readback means this attempt is still safe to retry.
		return bindErr
	}
	return mtproto.ErrInternalServerError
}

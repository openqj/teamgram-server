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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/crypto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/pkg/rpc/dccontext"
)

// AuthExportAuthorization
// auth.exportAuthorization#e5bfffcd dc_id:int = auth.ExportedAuthorization;
func (c *AuthorizationCore) AuthExportAuthorization(in *mtproto.TLAuthExportAuthorization) (*mtproto.Auth_ExportedAuthorization, error) {
	if in == nil || in.GetDcId() <= 0 {
		return nil, mtproto.ErrDcIdInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.MD == nil || c.MD.GetPermAuthKeyId() == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	sourceDCID := c.svcCtx.Config.DcId
	if actualDCID, ok := dccontext.DCID(c.ctx); ok {
		sourceDCID = actualDCID
	}
	if !c.svcCtx.Config.SupportsDc(sourceDCID) || !c.svcCtx.Config.SupportsDc(in.GetDcId()) || in.GetDcId() == sourceDCID {
		return nil, mtproto.ErrDcIdInvalid
	}

	sourceAuthKeyID := c.MD.GetPermAuthKeyId()
	userID, err := c.svcCtx.Dao.AuthsessionGetUserId(c.ctx, &authsession.TLAuthsessionGetUserId{
		AuthKeyId: sourceAuthKeyID,
	})
	if err != nil {
		c.Logger.Errorf("auth.exportAuthorization - source auth key lookup: %v", err)
		return nil, err
	}
	if userID == nil || userID.GetV() <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}

	token := crypto.RandomBytes(dao.ExportedAuthorizationTokenSize)
	id := int64(binary.BigEndian.Uint64(token[:8]) & 0x7fffffffffffffff)
	if id == 0 {
		return nil, mtproto.ErrInternalServerError
	}
	if err = c.svcCtx.Dao.PutExportedAuthorization(c.ctx, token, &dao.ExportedAuthorization{
		ID:              id,
		UserID:          userID.GetV(),
		SourceAuthKeyID: sourceAuthKeyID,
		SourceDCID:      sourceDCID,
		TargetDCID:      in.GetDcId(),
		State:           dao.ExportedAuthorizationReady,
	}); err != nil {
		c.Logger.Errorf("auth.exportAuthorization - store credential: %v", err)
		return nil, err
	}

	return mtproto.MakeTLAuthExportedAuthorization(&mtproto.Auth_ExportedAuthorization{
		Id:    id,
		Bytes: token,
	}).To_Auth_ExportedAuthorization(), nil
}

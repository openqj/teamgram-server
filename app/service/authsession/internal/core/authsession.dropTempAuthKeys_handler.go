/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"errors"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/svc"
)

// AuthsessionDropTempAuthKeys
// authsession.dropTempAuthKeys except_auth_keys:Vector<long> = Bool;
func (c *AuthsessionCore) AuthsessionDropTempAuthKeys(in *authsession.TLAuthsessionDropTempAuthKeys) (*mtproto.Bool, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.MD == nil || c.MD.GetPermAuthKeyId() == 0 {
		return nil, mtproto.ErrAuthKeyInvalid
	}
	keyID := c.MD.GetPermAuthKeyId()
	key, err := c.svcCtx.Dao.QueryAuthKeyV2(c.ctx, keyID)
	if errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		return nil, mtproto.ErrAuthKeyInvalid
	}
	if err != nil {
		return nil, err
	}
	if key == nil || key.GetAuthKeyType() != mtproto.AuthKeyTypePerm {
		return nil, mtproto.ErrAuthKeyInvalid
	}
	dropper, ok := c.svcCtx.Dao.(svc.TempAuthKeyDropper)
	if !ok {
		return nil, mtproto.ErrMethodNotImpl
	}
	if err := dropper.DropTempAuthKeys(c.ctx, keyID, in.GetExceptAuthKeys()); err != nil {
		c.Logger.Errorf("authsession.dropTempAuthKeys - error: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	return mtproto.BoolTrue, nil
}

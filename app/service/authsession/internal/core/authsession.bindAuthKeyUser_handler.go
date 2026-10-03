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
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dao"
)

// AuthsessionBindAuthKeyUser
// authsession.bindAuthKeyUser auth_key_id:long user_id:long = Int64;
func (c *AuthsessionCore) AuthsessionBindAuthKeyUser(in *authsession.TLAuthsessionBindAuthKeyUser) (*mtproto.Int64, error) {
	if in == nil || in.GetAuthKeyId() == 0 {
		return nil, mtproto.ErrAuthKeyInvalid
	}
	if in.GetUserId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	var (
		inKeyId = in.GetAuthKeyId()
	)

	keyData, err := c.svcCtx.Dao.QueryAuthKeyV2(c.ctx, inKeyId)
	if err != nil {
		c.Logger.Errorf("queryAuthKeyV2(%d) is error: %v", inKeyId, err)
		return nil, err
	} else if keyData.PermAuthKeyId == 0 {
		c.Logger.Errorf("queryAuthKeyV2(%d) - PermAuthKeyId is empty", inKeyId)
		return nil, mtproto.ErrAuthKeyPermEmpty
	}

	hash, err := c.svcCtx.Dao.BindAuthKeyUser(c.ctx, keyData.PermAuthKeyId, in.GetUserId())
	if errors.Is(err, dao.ErrAuthKeyOwnedByAnotherUser) {
		return nil, mtproto.ErrAuthKeyInvalid
	}
	if err != nil {
		c.Logger.Errorf("bindAuthKeyUser(%d, %d) is error: %v", keyData.PermAuthKeyId, in.GetUserId(), err)
		return nil, mtproto.ErrInternalServerError
	}

	return &mtproto.Int64{V: hash}, nil
}

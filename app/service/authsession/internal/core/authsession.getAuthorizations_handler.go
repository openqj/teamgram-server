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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
)

// AuthsessionGetAuthorizations
// authsession.getAuthorizations user_id:long exclude_auth_keyId:long = account.Authorizations;
func (c *AuthsessionCore) AuthsessionGetAuthorizations(in *authsession.TLAuthsessionGetAuthorizations) (*mtproto.Account_Authorizations, error) {
	if in == nil || in.GetExcludeAuthKeyId() == 0 {
		return nil, mtproto.ErrAuthKeyInvalid
	}
	if in.GetUserId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	var (
		inKeyId = in.GetExcludeAuthKeyId()
	)

	keyData, err := c.svcCtx.Dao.QueryAuthKeyV2(c.ctx, inKeyId)
	if err != nil {
		c.Logger.Errorf("queryAuthKeyV2(%d) is error: %v", inKeyId, err)
		return nil, err
	} else if keyData.PermAuthKeyId == 0 {
		c.Logger.Errorf("queryAuthKeyV2(%d) - PermAuthKeyId is empty", inKeyId)
		return nil, mtproto.ErrAuthKeyPermEmpty
	}
	// The exclusion key is the caller's current authorization. Verify its
	// durable owner before using the user_id supplied to this internal RPC;
	// otherwise a trusted downstream caller with a mismatched user_id could
	// enumerate another user's active sessions.
	if ownerID := c.svcCtx.Dao.GetAuthKeyUserId(c.ctx, keyData.PermAuthKeyId); ownerID != in.GetUserId() {
		c.Logger.Errorf("auth key %d belongs to user %d, requested user %d", keyData.PermAuthKeyId, ownerID, in.GetUserId())
		return nil, mtproto.ErrAuthKeyUnregistered
	}

	authorizationList := c.svcCtx.Dao.GetAuthorizations(c.ctx, in.GetUserId(), keyData.PermAuthKeyId)

	return mtproto.MakeTLAccountAuthorizations(&mtproto.Account_Authorizations{
		Authorizations: authorizationList,
	}).To_Account_Authorizations(), nil
}

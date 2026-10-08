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
	"math"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessageSearchGlobal
// message.searchGlobal user_id:long q:string offset:int limit:int = Vector<MessageBox>;
func (c *MessageCore) MessageSearchGlobal(in *message.TLMessageSearchGlobal) (*mtproto.MessageBoxList, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in.Q == "" {
		return nil, mtproto.ErrSearchQueryEmpty
	}
	if in.Limit < 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.Messages == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var (
		offset  = in.Offset
		rValues []*mtproto.MessageBox
		limit   = in.Limit
	)

	if offset == 0 {
		offset = math.MaxInt32
	}
	if limit > 50 {
		limit = 50
	}

	rows, err := c.svcCtx.Dao.SearchGlobalMessagesForSearch(c.ctx, in.UserId, offset, "%"+in.Q+"%", limit)
	if err != nil {
		c.Logger.Errorf("message.searchGlobal - error: %v", err)
		return nil, err
	}
	for i := range rows {
		rValues = append(rValues, c.svcCtx.Dao.MakeMessageBox(c.ctx, in.UserId, &rows[i]))
	}

	if rValues == nil {
		rValues = []*mtproto.MessageBox{}
	}

	return mtproto.MakeTLMessageBoxList(&mtproto.MessageBoxList{
		BoxList: rValues,
	}).To_MessageBoxList(), nil
}

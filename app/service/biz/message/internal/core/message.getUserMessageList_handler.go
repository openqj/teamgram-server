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
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessageGetUserMessageList
// message.getUserMessageList user_id:long id_list:Vector<int> = Vector<MessageBox>;
func (c *MessageCore) MessageGetUserMessageList(in *message.TLMessageGetUserMessageList) (*message.Vector_MessageBox, error) {
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil || c.svcCtx.Dao.Postgres.Store == nil || c.svcCtx.Dao.Postgres.Store.Messages == nil {
		return nil, mtproto.ErrInternalServerError
	}
	rValueList := &message.Vector_MessageBox{
		Datas: make([]*mtproto.MessageBox, 0, len(in.IdList)),
	}

	list, err := c.svcCtx.Dao.SelectMessageByIdList(c.ctx, in.UserId, in.IdList)
	for i := range list {
		rValueList.Datas = append(rValueList.GetDatas(), c.svcCtx.Dao.MakeMessageBox(c.ctx, in.UserId, &list[i]))
	}
	if err != nil {
		c.Logger.Errorf("message.getUserMessageList - error: %v", err)
		return nil, err
	}

	return rValueList, nil
}

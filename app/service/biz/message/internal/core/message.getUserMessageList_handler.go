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
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessageGetUserMessageList
// message.getUserMessageList user_id:long id_list:Vector<int> = Vector<MessageBox>;
func (c *MessageCore) MessageGetUserMessageList(in *message.TLMessageGetUserMessageList) (*message.Vector_MessageBox, error) {
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessagesDAO == nil {
		return nil, mtproto.ErrInternalServerError
	}
	rValueList := &message.Vector_MessageBox{
		Datas: make([]*mtproto.MessageBox, 0, len(in.IdList)),
	}

	_, err := c.svcCtx.Dao.MessagesDAO.SelectByMessageIdListWithCB(
		c.ctx,
		in.UserId,
		in.IdList,
		func(sz, i int, v *dataobject.MessagesDO) {
			rValueList.Datas = append(rValueList.GetDatas(), c.svcCtx.Dao.MakeMessageBox(c.ctx, in.UserId, v))
		})
	if err != nil {
		c.Logger.Errorf("message.getUserMessageList - error: %v", err)
		return nil, err
	}

	return rValueList, nil
}

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
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

// ChatGetUsersChatIdList
// chat.getUsersChatIdList id:Vector<long> = Vector<UserChatIdList>;
func (c *ChatCore) ChatGetUsersChatIdList(in *chat.TLChatGetUsersChatIdList) (*chat.Vector_UserChatIdList, error) {
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	selector := c.svcCtx.Dao.SelectUsersChatIdListWithCB
	r, err := collectUsersChatIdList(c.ctx, in.Id, selector)
	if err != nil {
		c.Logger.Errorf("chat.getUsersChatIdList - error: %v", err)
		return nil, err
	}
	return r, nil
}

func collectUsersChatIdList(ctx context.Context, ids []int64, selectRows func(context.Context, []int64, func(sz, i int, v *dataobject.ChatParticipantsDO)) ([]dataobject.ChatParticipantsDO, error)) (*chat.Vector_UserChatIdList, error) {
	rows, err := selectRows(ctx, ids, nil)
	if err != nil {
		return nil, err
	}

	var (
		rValueList = make([]*chat.UserChatIdList, 0, len(ids))
	)

	for _, row := range rows {
		found := false
		for _, ch := range rValueList {
			if ch.UserId == row.UserId {
				ch.ChatIdList = append(ch.ChatIdList, row.ChatId)
				found = true
				break
			}
		}
		if !found {
			rValueList = append(rValueList, &chat.UserChatIdList{
				UserId:     row.UserId,
				ChatIdList: []int64{row.ChatId},
			})
		}
	}

	return &chat.Vector_UserChatIdList{
		Datas: rValueList,
	}, nil
}

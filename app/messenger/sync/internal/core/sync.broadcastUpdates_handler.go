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
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// SyncBroadcastUpdates
// sync.broadcastUpdates broadcast_type:int chat_id:long exclude_id_list:Vector<long> updates:Updates = Void;
func (c *SyncCore) SyncBroadcastUpdates(in *sync.TLSyncBroadcastUpdates) (*mtproto.Void, error) {
	if in == nil || in.GetChatId() <= 0 || in.GetUpdates() == nil || in.GetBroadcastType() != sync.BroadcastTypeChat {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	pushUpdates := &sync.TLSyncPushUpdates{
		UserId:  0,
		Updates: in.Updates,
	}

	idList, err := c.svcCtx.Dao.ChatClient.ChatGetChatParticipantIdList(c.ctx, &chatpb.TLChatGetChatParticipantIdList{
		ChatId: in.ChatId,
	})
	if err != nil {
		return nil, err
	}
	if idList == nil {
		return nil, mtproto.ErrInternalServerError
	}
	excludes := make(map[int64]struct{}, len(in.GetExcludeIdList()))
	for _, id := range in.GetExcludeIdList() {
		excludes[id] = struct{}{}
	}

	for _, id := range idList.GetDatas() {
		if _, excluded := excludes[id]; excluded {
			continue
		}
		pushUpdates.UserId = id
		if _, err := c.SyncPushUpdates(pushUpdates); err != nil {
			return nil, err
		}
	}

	return mtproto.EmptyVoid, nil
}

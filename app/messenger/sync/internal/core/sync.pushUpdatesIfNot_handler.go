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
	"github.com/teamgram/teamgram-server/app/interface/session/session"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/status/status"
)

// SyncPushUpdatesIfNot
// sync.pushUpdatesIfNot user_id:long excludes:Vector<int64> updates:Updates = Void;
func (c *SyncCore) SyncPushUpdatesIfNot(in *sync.TLSyncPushUpdatesIfNot) (*mtproto.Void, error) {
	if in == nil || in.GetUserId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if in.GetUpdates() == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.StatusClient == nil {
		if c != nil {
			c.Logger.Errorf("sync.pushUpdatesIfNot - status provider is unavailable")
		}
		return nil, mtproto.ErrMethodNotImpl
	}

	notification, err := c.processUpdates(syncTypeUserNotMe, in.GetUserId(), false, in.GetUpdates())
	if err != nil {
		c.Logger.Errorf("sync.pushUpdatesIfNot - process updates error: %v", err)
		return nil, err
	}

	statusList, err := c.svcCtx.Dao.StatusClient.StatusGetUserOnlineSessions(c.ctx, &status.TLStatusGetUserOnlineSessions{
		UserId: in.GetUserId(),
	})
	if err != nil {
		c.Logger.Errorf("sync.pushUpdatesIfNot - get online sessions error: %v", err)
		return nil, err
	}
	if statusList == nil {
		c.Logger.Errorf("sync.pushUpdatesIfNot - status provider returned nil sessions")
		return nil, mtproto.ErrMethodNotImpl
	}

	excludes := make(map[int64]struct{}, len(in.GetExcludes()))
	for _, id := range in.GetExcludes() {
		if id != 0 {
			excludes[id] = struct{}{}
		}
	}

	for _, sess := range statusList.GetUserSessions() {
		if sess == nil {
			continue
		}
		permAuthKeyId := sess.GetPermAuthKeyId()
		if permAuthKeyId == 0 {
			permAuthKeyId = sess.GetAuthKeyId()
		}
		if _, ok := excludes[permAuthKeyId]; ok {
			continue
		}
		if permAuthKeyId == 0 || sess.GetGateway() == "" {
			c.Logger.Errorf("sync.pushUpdatesIfNot - invalid online session: %s", sess)
			return nil, mtproto.ErrMethodNotImpl
		}

		if err = c.svcCtx.Dao.PushUpdatesToSession(c.ctx, sess.GetGateway(), &session.TLSessionPushUpdatesData{
			PermAuthKeyId: permAuthKeyId,
			Notification:  notification,
			Updates:       in.GetUpdates(),
		}); err != nil {
			c.Logger.Errorf("sync.pushUpdatesIfNot - push updates to gateway %s error: %v", sess.GetGateway(), err)
			return nil, err
		}
	}

	return mtproto.EmptyVoid, nil
}

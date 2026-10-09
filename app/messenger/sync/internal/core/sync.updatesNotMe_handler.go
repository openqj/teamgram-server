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
)

// SyncUpdatesNotMe
// sync.updatesNotMe user_id:long auth_key_id:long updates:Updates = Void;
func (c *SyncCore) SyncUpdatesNotMe(in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	if in == nil || in.GetUserId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if in.GetUpdates() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var (
		userId    = in.GetUserId()
		authKeyId = in.GetPermAuthKeyId()
		updates   = in.GetUpdates()
	)

	notification, err := c.processUpdates(syncTypeUserNotMe, userId, false, updates)
	if err != nil {
		c.Logger.Errorf("sync.updatesNotMe - error: %v", err)
		return nil, err
	}

	if err := c.pushUpdatesToSession(syncTypeUserNotMe, userId, authKeyId, nil, nil, nil, updates, notification); err != nil {
		return nil, err
	}

	return mtproto.EmptyVoid, nil
}

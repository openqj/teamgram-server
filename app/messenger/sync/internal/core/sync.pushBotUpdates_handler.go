// Copyright 2025 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
)

// SyncPushBotUpdates
// sync.pushBotUpdates user_id:long updates:Updates = Void;
func (c *SyncCore) SyncPushBotUpdates(in *sync.TLSyncPushBotUpdates) (*mtproto.Void, error) {
	if in == nil || in.GetUserId() <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if in.GetUpdates() == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.StatusClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}

	notification, err := c.processUpdates(syncTypeUser, in.GetUserId(), true, in.GetUpdates())
	if err != nil {
		return nil, err
	}
	c.pushUpdatesToSession(syncTypeUser, in.GetUserId(), 0, nil, nil, nil, in.GetUpdates(), notification)

	return mtproto.EmptyVoid, nil
}

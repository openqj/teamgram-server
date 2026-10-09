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
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserToggleUsername
// user.toggleUsername peer_type:int peer_id:long username:string active:Bool = Bool;
func (c *UserCore) UserToggleUsername(in *user.TLUserToggleUsername) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.GetUserId() <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in.GetActive() == nil || (in.GetActive().GetPredicateName() != mtproto.Predicate_boolTrue && in.GetActive().GetPredicateName() != mtproto.Predicate_boolFalse) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err := c.svcCtx.Dao.TogglePeerUsername(c.ctx, c.MD.GetUserId(), in.GetPeerType(), in.GetPeerId(), in.GetUsername(), mtproto.FromBool(in.GetActive())); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

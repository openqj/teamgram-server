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
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UserSaveMusic
// user.saveMusic flags:# unsave:flags.0?true user_id:long id:long after_id:flags.15?long = Bool;
func (c *UserCore) UserSaveMusic(in *user.TLUserSaveMusic) (*mtproto.Bool, error) {
	if in == nil {
		return mtproto.BoolFalse, mtproto.ErrInputRequestInvalid
	}
	err := c.svcCtx.Dao.SaveUserMusic(c.ctx, in.GetUserId(), in.GetId(), in.GetUnsave())
	return mtproto.ToBool(err == nil), err
}

func savedMusicEntryMatches(entry *dataobject.UserSavedMusicDO, musicID int64) bool {
	return entry != nil && musicID > 0 && entry.SavedMusicId == musicID
}

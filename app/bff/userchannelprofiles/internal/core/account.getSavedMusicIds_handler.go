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
	"sort"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// AccountGetSavedMusicIds
// account.getSavedMusicIds#e09d5faf hash:long = account.SavedMusicIds;
func (c *UserChannelProfilesCore) AccountGetSavedMusicIds(in *mtproto.TLAccountGetSavedMusicIds) (*mtproto.Account_SavedMusicIds, error) {
	if c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	idList, err := c.svcCtx.Dao.UserClient.UserGetSavedMusicIdList(c.ctx, &user.TLUserGetSavedMusicIdList{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("account.getSavedMusicIds - error: %v", err)
		return nil, err
	}
	if idList == nil {
		return nil, mtproto.ErrInternalServerError
	}

	ids := append([]int64(nil), idList.GetDatas()...)
	hash := savedMusicIDsHash(ids)
	if in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLAccountSavedMusicIdsNotModified(nil).To_Account_SavedMusicIds(), nil
	}

	return mtproto.MakeTLAccountSavedMusicIds(&mtproto.Account_SavedMusicIds{
		Ids: ids,
	}).To_Account_SavedMusicIds(), nil
}

func savedMusicIDsHash(ids []int64) int64 {
	// account.getSavedMusicIds returns an unordered list, so order changes
	// must not invalidate a cached result.
	sortedIDs := append([]int64(nil), ids...)
	sort.Slice(sortedIDs, func(i, j int) bool { return sortedIDs[i] < sortedIDs[j] })

	var hash uint64
	for _, id := range sortedIDs {
		hash ^= hash >> 21
		hash ^= hash << 35
		hash ^= hash >> 4
		hash += uint64(id)
	}
	return int64(hash)
}

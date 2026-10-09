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
// limitations under the License.s
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/app/service/media/media"
)

// UsersGetSavedMusic
// users.getSavedMusic#788d7fe3 id:InputUser offset:int limit:int hash:long = users.SavedMusic;
func (c *UserChannelProfilesCore) UsersGetSavedMusic(in *mtproto.TLUsersGetSavedMusic) (*mtproto.Users_SavedMusic, error) {
	if in == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	targetID, targetAccessHash, selfInput, err := savedMusicTarget(c.MD.UserId, in.GetId())
	if err != nil {
		return nil, err
	}

	target, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &user.TLUserGetImmutableUser{
		Id:       targetID,
		Contacts: []int64{c.MD.UserId},
	})
	if err != nil {
		return nil, err
	}
	if target == nil || target.GetUser() == nil || target.GetUser().GetId() != targetID || target.GetUser().GetDeleted() {
		return nil, mtproto.ErrUserIdInvalid
	}
	if !selfInput && target.GetUser().GetAccessHash() != targetAccessHash {
		return nil, mtproto.ErrUserIdInvalid
	}

	if targetID != c.MD.UserId {
		allowed, err := c.svcCtx.Dao.UserClient.UserCheckPrivacy(c.ctx, &user.TLUserCheckPrivacy{
			UserId:  targetID,
			KeyType: mtproto.SAVED_MUSIC,
			PeerId:  c.MD.UserId,
		})
		if err != nil {
			return nil, err
		}
		if allowed == nil {
			return nil, mtproto.ErrInternalServerError
		}
		if !mtproto.FromBool(allowed) {
			return nil, mtproto.ErrUserPrivacyRestricted
		}
	}

	idList, err := c.svcCtx.Dao.UserClient.UserGetSavedMusicIdList(c.ctx, &user.TLUserGetSavedMusicIdList{
		UserId: targetID,
	})
	if err != nil {
		c.Logger.Errorf("users.getSavedMusic - error: %v", err)
		return nil, err
	}
	if idList == nil {
		return nil, mtproto.ErrInternalServerError
	}

	documents := make([]*mtproto.Document, 0)
	if len(idList.GetDatas()) > 0 {
		dList, err := c.svcCtx.Dao.MediaClient.MediaGetDocumentList(c.ctx, &media.TLMediaGetDocumentList{
			IdList: idList.GetDatas(),
		})
		if err != nil {
			c.Logger.Errorf("users.getSavedMusic - error: %v", err)
			return nil, err
		}
		if dList == nil {
			return nil, mtproto.ErrInternalServerError
		}
		byID := make(map[int64]*mtproto.Document, len(dList.GetDatas()))
		for _, document := range dList.GetDatas() {
			if document != nil {
				byID[document.GetId()] = document
			}
		}
		seen := make(map[int64]struct{}, len(idList.GetDatas()))
		for _, id := range idList.GetDatas() {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			if document := byID[id]; document != nil {
				documents = append(documents, document)
			}
		}
	}

	count := int32(len(documents))
	var hash int64 = 1
	for _, document := range documents {
		hash = hash*31 + document.GetId()
	}
	if hash < 0 {
		hash = -hash
	}
	if in.GetHash() != 0 && in.GetHash() == hash {
		return mtproto.MakeTLUsersSavedMusicNotModified(&mtproto.Users_SavedMusic{
			Count: count,
		}).To_Users_SavedMusic(), nil
	}

	offset := int(in.GetOffset())
	if offset < 0 {
		offset = 0
	}
	if offset >= len(documents) {
		documents = []*mtproto.Document{}
	} else {
		limit := int(in.GetLimit())
		if limit < 0 {
			limit = 0
		}
		end := offset + limit
		if end > len(documents) {
			end = len(documents)
		}
		documents = documents[offset:end]
	}

	return mtproto.MakeTLUsersSavedMusic(&mtproto.Users_SavedMusic{
		Count:     count,
		Documents: documents,
	}).To_Users_SavedMusic(), nil
}

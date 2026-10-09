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
	"github.com/teamgram/teamgram-server/app/service/media/media"
)

// UsersGetSavedMusicByID
// users.getSavedMusicByID#7573a4e9 id:InputUser documents:Vector<InputDocument> = users.SavedMusic;
func (c *UserChannelProfilesCore) UsersGetSavedMusicByID(in *mtproto.TLUsersGetSavedMusicByID) (*mtproto.Users_SavedMusic, error) {
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

	requested := make(map[int64]*mtproto.InputDocument, len(in.GetDocuments()))
	requestedIDs := make([]int64, 0, len(in.GetDocuments()))
	for _, document := range in.GetDocuments() {
		if document == nil || document.GetPredicateName() != mtproto.Predicate_inputDocument || document.GetId() <= 0 || document.GetAccessHash() == 0 {
			return nil, mtproto.ErrDocumentInvalid
		}
		if previous, ok := requested[document.GetId()]; ok {
			if previous.GetAccessHash() != document.GetAccessHash() {
				return nil, mtproto.ErrDocumentInvalid
			}
			continue
		}
		requested[document.GetId()] = document
		requestedIDs = append(requestedIDs, document.GetId())
	}

	saved, err := c.svcCtx.Dao.UserClient.UserGetSavedMusicIdList(c.ctx, &user.TLUserGetSavedMusicIdList{
		UserId: targetID,
	})
	if err != nil {
		return nil, err
	}
	if saved == nil {
		return nil, mtproto.ErrInternalServerError
	}
	savedIDs := make(map[int64]struct{}, len(saved.GetDatas()))
	for _, id := range saved.GetDatas() {
		savedIDs[id] = struct{}{}
	}
	eligibleIDs := make([]int64, 0, len(requestedIDs))
	for _, id := range requestedIDs {
		if _, ok := savedIDs[id]; ok {
			eligibleIDs = append(eligibleIDs, id)
		}
	}

	documents := make([]*mtproto.Document, 0, len(eligibleIDs))
	if len(eligibleIDs) > 0 {
		result, err := c.svcCtx.Dao.MediaClient.MediaGetDocumentList(c.ctx, &media.TLMediaGetDocumentList{IdList: eligibleIDs})
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, mtproto.ErrInternalServerError
		}
		byID := make(map[int64]*mtproto.Document, len(result.GetDatas()))
		for _, document := range result.GetDatas() {
			if document != nil {
				byID[document.GetId()] = document
			}
		}
		for _, id := range eligibleIDs {
			document := byID[id]
			if document == nil {
				continue
			}
			if document.GetAccessHash() != requested[id].GetAccessHash() {
				return nil, mtproto.ErrDocumentInvalid
			}
			documents = append(documents, document)
		}
	}

	return mtproto.MakeTLUsersSavedMusic(&mtproto.Users_SavedMusic{
		Count:     int32(len(documents)),
		Documents: documents,
	}).To_Users_SavedMusic(), nil
}

func savedMusicTarget(selfID int64, input *mtproto.InputUser) (int64, int64, bool, error) {
	if input == nil {
		return 0, 0, false, mtproto.ErrUserIdInvalid
	}
	switch input.GetPredicateName() {
	case mtproto.Predicate_inputUserSelf:
		if input.GetUserId() != 0 || input.GetAccessHash() != 0 {
			return 0, 0, false, mtproto.ErrUserIdInvalid
		}
		return selfID, 0, true, nil
	case mtproto.Predicate_inputUser:
		if input.GetUserId() <= 0 || input.GetAccessHash() == 0 {
			return 0, 0, false, mtproto.ErrUserIdInvalid
		}
		return input.GetUserId(), input.GetAccessHash(), false, nil
	default:
		return 0, 0, false, mtproto.ErrUserIdInvalid
	}
}

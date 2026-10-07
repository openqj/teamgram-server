// Copyright 2022 Teamgram Authors
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
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
)

// PhotosGetUserPhotos
// photos.getUserPhotos#91cd32a8 user_id:InputUser offset:int max_id:long limit:int = photos.Photos;
func (c *UserChannelProfilesCore) PhotosGetUserPhotos(in *mtproto.TLPhotosGetUserPhotos) (*mtproto.Photos_Photos, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetUserId() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetOffset() < 0 {
		return nil, mtproto.ErrOffsetInvalid
	}
	userId := mtproto.FromInputUser(c.MD.UserId, in.UserId)
	switch userId.PeerType {
	case mtproto.PEER_SELF:
	case mtproto.PEER_USER:
		if in.GetUserId().GetUserId() <= 0 || in.GetUserId().GetAccessHash() == 0 {
			return nil, mtproto.ErrUserIdInvalid
		}
	default:
		err := mtproto.ErrUserIdInvalid
		c.Logger.Errorf("photos.getUserPhotos - error: %v", err)
		return nil, err
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil ||
		c.svcCtx.Dao.UserClient == nil || c.svcCtx.Dao.MediaClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	cachePhotos, err := c.svcCtx.Dao.UserClient.UserGetProfilePhotos(c.ctx, &userpb.TLUserGetProfilePhotos{
		UserId: userId.PeerId,
	})
	if err != nil {
		c.Logger.Errorf("photos.getUserPhotos - error: %v", err)
		return nil, err
	}
	if cachePhotos == nil {
		return nil, mtproto.ErrInternalServerError
	}

	// The user service returns the profile-photo ids in newest-first order. Apply
	// the Layer 229 window locally because the service contract predates the
	// photos.getUserPhotos pagination fields.
	ids := cachePhotos.GetDatas()
	filtered := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if in.GetMaxId() > 0 && id > in.GetMaxId() {
			continue
		}
		filtered = append(filtered, id)
	}
	offset := int(in.GetOffset())
	if offset >= len(filtered) {
		filtered = nil
	} else if offset > 0 {
		filtered = filtered[offset:]
	}
	limit := int(in.GetLimit())
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	photos := mtproto.MakeTLPhotosPhotos(&mtproto.Photos_Photos{
		Photos: make([]*mtproto.Photo, 0, len(filtered)),
		Users:  []*mtproto.User{},
	}).To_Photos_Photos()

	for _, id := range filtered {
		photo, err := c.svcCtx.Dao.MediaClient.MediaGetPhoto(c.ctx,
			&mediapb.TLMediaGetPhoto{
				PhotoId: id,
			})
		if err != nil {
			c.Logger.Errorf("photos.getUserPhotos - error: %v", err)
			return nil, err
		}
		if photo == nil {
			return nil, mtproto.ErrInternalServerError
		}
		photos.Photos = append(photos.Photos, photo)
	}

	return photos, nil
}

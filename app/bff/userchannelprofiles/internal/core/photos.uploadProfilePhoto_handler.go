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
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
)

// PhotosUploadProfilePhoto
// photos.uploadProfilePhoto#89f30f69 flags:# file:flags.0?InputFile video:flags.1?InputFile video_start_ts:flags.2?double = photos.Photo;
func (c *UserChannelProfilesCore) PhotosUploadProfilePhoto(in *mtproto.TLPhotosUploadProfilePhoto) (*mtproto.Photos_Photo, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || (inputFileEmpty(in.GetFile()) && inputFileEmpty(in.GetVideo())) {
		return nil, mtproto.ErrPhotoInvalid
	}
	if c.MD.PermAuthKeyId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil ||
		c.svcCtx.Dao.MediaClient == nil || c.svcCtx.Dao.UserClient == nil ||
		c.svcCtx.Dao.SyncClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	photo, err := c.svcCtx.Dao.MediaClient.MediaUploadProfilePhotoFile(c.ctx, &mediapb.TLMediaUploadProfilePhotoFile{
		OwnerId:          c.MD.PermAuthKeyId,
		File:             in.GetFile(),
		Video:            in.GetVideo(),
		VideoStartTs:     in.GetVideoStartTs(),
		VideoEmojiMarkup: in.GetVideoEmojiMarkup(),
	})
	if err != nil {
		c.Logger.Errorf("photos.uploadProfilePhoto - error: %v", err)
		return nil, err
	}
	if photo == nil || photo.GetId() <= 0 {
		return nil, mtproto.ErrInternalServerError
	}

	// TODO: ALBUM_PHOTOS_TOO_MANY
	updatedPhotoId, err := c.svcCtx.Dao.UserClient.UserUpdateProfilePhoto(c.ctx, &userpb.TLUserUpdateProfilePhoto{
		UserId: c.MD.UserId,
		Id:     photo.GetId(),
	})
	if err != nil {
		c.Logger.Errorf("photos.uploadProfilePhoto - error: %v", err)
		return nil, err
	}
	if updatedPhotoId == nil || updatedPhotoId.GetV() <= 0 {
		return nil, mtproto.ErrInternalServerError
	}

	me, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(
		c.ctx,
		&userpb.TLUserGetImmutableUser{
			Id:       c.MD.UserId,
			Privacy:  false,
			Contacts: nil,
		})
	if err != nil {
		c.Logger.Errorf("photos.uploadProfilePhoto - error: %v", err)
		return nil, err
	}
	if me == nil {
		return nil, mtproto.ErrInternalServerError
	}

	syncReply, err := c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
		UserId: c.MD.UserId,
		Updates: mtproto.MakeUpdatesByUpdatesUsers(
			[]*mtproto.User{me.ToSelfUser()},
			mtproto.MakeTLUpdateUser(&mtproto.Update{
				UserId: c.MD.UserId,
			}).To_Update()),
	})
	if err != nil {
		c.Logger.Errorf("photos.uploadProfilePhoto - sync error: %v", err)
		return nil, err
	}
	if syncReply == nil {
		return nil, mtproto.ErrInternalServerError
	}

	return mtproto.MakeTLPhotosPhoto(&mtproto.Photos_Photo{
		Photo: photo,
		Users: []*mtproto.User{},
	}).To_Photos_Photo(), nil
}

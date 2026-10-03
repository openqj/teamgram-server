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
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
)

// PhotosUploadContactProfilePhoto
// photos.uploadContactProfilePhoto#e14c4a71 flags:# suggest:flags.3?true save:flags.4?true user_id:InputUser file:flags.0?InputFile video:flags.1?InputFile video_start_ts:flags.2?double video_emoji_markup:flags.5?VideoSize = photos.Photo;
func (c *UserChannelProfilesCore) PhotosUploadContactProfilePhoto(in *mtproto.TLPhotosUploadContactProfilePhoto) (*mtproto.Photos_Photo, error) {
	if inputFileEmpty(in.GetFile()) {
		c.Logger.Errorf("photos.uploadContactProfilePhoto - error: empty file")
		return nil, mtproto.ErrPhotoInvalid
	}
	userID, err := otherUserPeer(c.MD.UserId, in.GetUserId())
	if err != nil {
		c.Logger.Errorf("photos.uploadContactProfilePhoto - error: %v", err)
		return nil, err
	}

	photo, err := c.svcCtx.Dao.MediaClient.MediaUploadProfilePhotoFile(c.ctx, &mediapb.TLMediaUploadProfilePhotoFile{
		OwnerId:          c.MD.PermAuthKeyId,
		File:             in.GetFile(),
		Video:            in.GetVideo(),
		VideoStartTs:     in.GetVideoStartTs(),
		VideoEmojiMarkup: in.GetVideoEmojiMarkup(),
	})
	if err != nil {
		c.Logger.Errorf("photos.uploadContactProfilePhoto - error: %v", err)
		return nil, err
	}

	// Contact photo only. Do not call UserUpdateProfilePhoto (that sets the caller's own photo).
	if err = persist.Default.Set(contactPhotoKey(c.MD.UserId, userID), strconv.FormatInt(photo.GetId(), 10)); err != nil {
		c.Logger.Errorf("photos.uploadContactProfilePhoto - error: %v", err)
		return nil, err
	}

	return mtproto.MakeTLPhotosPhoto(&mtproto.Photos_Photo{
		Photo: photo,
		Users: []*mtproto.User{},
	}).To_Photos_Photo(), nil
}

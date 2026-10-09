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
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/media/media"
)

// MediaUploadProfilePhotoFile
// media.uploadProfilePhotoFile flags:# owner_id:long file:flags.0?InputFile video:flags.1?InputFile video_start_ts:flags.2?double = Photo;
func (c *MediaCore) MediaUploadProfilePhotoFile(in *media.TLMediaUploadProfilePhotoFile) (*mtproto.Photo, error) {
	if in == nil || in.GetOwnerId() <= 0 || (in.GetFile() == nil && in.GetVideo() == nil) ||
		(in.GetFile() != nil && (in.GetFile().GetId_INT64() <= 0 || in.GetFile().GetParts() <= 0)) ||
		(in.GetVideo() != nil && (in.GetVideo().GetId_INT64() <= 0 || in.GetVideo().GetParts() <= 0)) {
		c.Logger.Errorf("media.uploadProfilePhotoFile - error: file is nil")
		return nil, mtproto.ErrMediaInvalid
	}
	fileName := ""
	if in.GetFile() != nil {
		fileName = in.GetFile().GetName()
	} else if in.GetVideo() != nil {
		fileName = in.GetVideo().GetName()
	}

	photo, err := c.svcCtx.Dao.DfsClient.DfsUploadProfilePhotoFileV2(c.ctx, &dfs.TLDfsUploadProfilePhotoFileV2{
		Creator:      in.OwnerId,
		File:         in.GetFile(),
		Video:        in.GetVideo(),
		VideoStartTs: in.GetVideoStartTs(),
	})
	if err != nil {
		c.Logger.Error("media.uploadProfilePhotoFile - error: %v", err.Error())
		return nil, err
	}
	if photo == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if photo.GetId() <= 0 {
		return nil, mtproto.ErrInternalServerError
	}

	hasVideo := len(photo.GetVideoSizes()) > 0

	if err = c.svcCtx.Dao.SavePhotoAggregateV2(c.ctx,
		photo.GetId(),
		photo.GetAccessHash(),
		photo.GetHasStickers(),
		hasVideo,
		fileName,
		photo.GetSizes(),
		photo.GetVideoSizes()); err != nil {
		c.Logger.Errorf("media.uploadProfilePhotoFile - save photo: %v", err)
		return nil, err
	}

	return photo, nil
}

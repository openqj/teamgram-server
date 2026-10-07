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

// MediaUploadEncryptedFile
// media.uploadEncryptedFile owner_id:long file:InputEncryptedFile = EncryptedFile;
func (c *MediaCore) MediaUploadEncryptedFile(in *media.TLMediaUploadEncryptedFile) (*mtproto.EncryptedFile, error) {
	if in == nil || in.GetOwnerId() <= 0 || in.GetFile() == nil || in.GetFile().GetId() <= 0 || in.GetFile().GetParts() <= 0 {
		return nil, mtproto.ErrMediaInvalid
	}

	file, err := c.svcCtx.Dao.DfsClient.DfsUploadEncryptedFileV2(c.ctx, &dfs.TLDfsUploadEncryptedFileV2{
		Creator: in.GetOwnerId(),
		File:    in.GetFile(),
	})
	if err != nil {
		c.Logger.Errorf("media.uploadEncryptedFile - error: %v", err)
		return nil, err
	}
	if file == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	return file, nil
}

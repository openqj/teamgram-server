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

// MediaUploadWallPaperFile
// media.uploadWallPaperFile owner_id:long file:InputFile mime_type:string admin:Bool = Document;
func (c *MediaCore) MediaUploadWallPaperFile(in *media.TLMediaUploadWallPaperFile) (*mtproto.Document, error) {
	if in == nil || in.GetOwnerId() <= 0 || in.GetFile() == nil || in.GetFile().GetId_INT64() <= 0 || in.GetFile().GetParts() <= 0 {
		return nil, mtproto.ErrWallpaperFileInvalid
	}

	document, err := c.svcCtx.Dao.DfsClient.DfsUploadWallPaperFile(c.ctx, &dfs.TLDfsUploadWallPaperFile{
		Creator:  in.GetOwnerId(),
		File:     in.GetFile(),
		MimeType: in.GetMimeType(),
		Admin:    in.GetAdmin(),
	})
	if err != nil {
		c.Logger.Errorf("media.uploadWallPaperFile - error: %v", err)
		return nil, err
	}
	if document == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	if err = c.svcCtx.Dao.SaveDocumentV2(c.ctx, in.GetFile().GetName(), document); err != nil {
		return nil, err
	}
	return document, nil
}

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

// MediaUploadThemeFile
// media.uploadThemeFile flags:# owner_id:long file:InputFile thumb:flags.0?InputFile mime_type:string file_name:string = Document;
func (c *MediaCore) MediaUploadThemeFile(in *media.TLMediaUploadThemeFile) (*mtproto.Document, error) {
	if in == nil || in.GetOwnerId() <= 0 || in.GetFile() == nil || in.GetFile().GetId_INT64() <= 0 || in.GetFile().GetParts() <= 0 {
		return nil, mtproto.ErrThemeFileInvalid
	}

	document, err := c.svcCtx.Dao.DfsClient.DfsUploadThemeFile(c.ctx, &dfs.TLDfsUploadThemeFile{
		Creator:  in.GetOwnerId(),
		File:     in.GetFile(),
		Thumb:    in.GetThumb(),
		MimeType: in.GetMimeType(),
		FileName: in.GetFileName(),
	})
	if err != nil {
		c.Logger.Errorf("media.uploadThemeFile - error: %v", err)
		return nil, err
	}
	if document == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	if err = c.svcCtx.Dao.SaveDocumentV2(c.ctx, in.GetFileName(), document); err != nil {
		return nil, err
	}
	return document, nil
}

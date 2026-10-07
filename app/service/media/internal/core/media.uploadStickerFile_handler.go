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

// MediaUploadStickerFile
// media.uploadStickerFile flags:# owner_id:long file:InputFile thumb:flags.0?InputFile mime_type:string file_name:string document_attribute_sticker:DocumentAttribute = Document;
func stickerUploadMedia(in *media.TLMediaUploadStickerFile) (*mtproto.InputMedia, error) {
	if in == nil || in.GetFile() == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	if in.GetDocumentAttributeSticker() == nil || in.GetDocumentAttributeSticker().GetPredicateName() != mtproto.Predicate_documentAttributeSticker {
		return nil, mtproto.ErrDocumentInvalid
	}

	// The DFS document path already persists uploaded bytes and returns the
	// authoritative document ID/access hash. Keep the sticker attribute on the
	// media passed to DFS so the document can be used as a sticker later.
	file := *in.GetFile()
	if in.GetFileName() != "" {
		file.Name = in.GetFileName()
	}
	inputMedia := mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
		File:       &file,
		Thumb:      in.GetThumb(),
		MimeType:   in.GetMimeType(),
		Attributes: []*mtproto.DocumentAttribute{in.GetDocumentAttributeSticker()},
	}).To_InputMedia()
	return inputMedia, nil
}

func (c *MediaCore) MediaUploadStickerFile(in *media.TLMediaUploadStickerFile) (*mtproto.Document, error) {
	if in == nil || in.GetOwnerId() <= 0 {
		return nil, mtproto.ErrMediaInvalid
	}
	inputMedia, err := stickerUploadMedia(in)
	if err != nil {
		return nil, err
	}

	document, err := c.svcCtx.Dao.DfsClient.DfsUploadDocumentFileV2(c.ctx, &dfs.TLDfsUploadDocumentFileV2{
		Creator: in.GetOwnerId(),
		Media:   inputMedia,
	})
	if err != nil {
		return nil, err
	}
	if document == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	if err = c.svcCtx.Dao.SaveDocumentV2(c.ctx, inputMedia.GetFile().GetName(), document); err != nil {
		return nil, err
	}

	return document, nil
}

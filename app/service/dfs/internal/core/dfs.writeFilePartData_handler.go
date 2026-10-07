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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/model"
)

const maxFilePartBytes = 512 * 1024

// validateWriteFilePartData keeps malformed upload envelopes out of SSDB. The
// BFF performs the same checks, but this service boundary is also reachable by
// other callers and must not trust those callers to enforce protocol limits.
func validateWriteFilePartData(in *dfs.TLDfsWriteFilePartData) error {
	if in == nil || in.GetCreator() <= 0 || in.GetFileId() <= 0 {
		return mtproto.ErrInputRequestInvalid
	}
	if err := model.CheckFilePart(in.GetFilePart()); err != nil {
		return err
	}
	if len(in.GetBytes()) == 0 {
		return mtproto.ErrFilePartEmpty
	}
	if len(in.GetBytes()) > maxFilePartBytes {
		return mtproto.ErrFilePartTooBig
	}
	if in.GetBig() {
		total := in.GetFileTotalParts()
		if total == nil {
			return mtproto.ErrFilePartsInvalid
		}
		if err := model.CheckFileParts(total.GetValue()); err != nil {
			return err
		}
		if in.GetFilePart() >= total.GetValue() {
			return mtproto.ErrFilePartInvalid
		}
	}
	return nil
}

// DfsWriteFilePartData
// dfs.writeFilePartData flags:# creator:long file_id:long bytes:bytes big:flags.0?true file_total_parts:flags.1?int = Bool;
func (c *DfsCore) DfsWriteFilePartData(in *dfs.TLDfsWriteFilePartData) (*mtproto.Bool, error) {
	if err := validateWriteFilePartData(in); err != nil {
		c.Logger.Errorf("dfs.writeFilePartData - error: %v", err)
		return nil, err
	}

	var err error

	err = c.svcCtx.Dao.WriteFilePartData(c.ctx, in.Creator, in.FileId, in.FilePart, in.Bytes)
	if err != nil {
		c.Logger.Errorf("dfs.writeFilePartData - error: %v", err)
		return nil, err
	}

	if in.FilePart == 0 {
		fileTotalParts := 0
		if totalParts := in.GetFileTotalParts(); totalParts != nil {
			fileTotalParts = int(totalParts.GetValue())
		}
		err = c.svcCtx.Dao.SetFileInfo(c.ctx, &model.DfsFileInfo{
			Creator:           in.Creator,
			FileId:            in.FileId,
			Big:               in.Big,
			FileName:          "",
			FileTotalParts:    fileTotalParts,
			FirstFilePartSize: len(in.Bytes),
			FilePartSize:      0,
			LastFilePartSize:  0,
			MimeType:          "",
			Mtime:             time.Now().Unix(),
		})

		// TODO(@benqi): error
		if err != nil {
			c.Logger.Errorf("dfs.writeFilePartData - error: %v", err)
			return nil, mtproto.ErrFilePartInvalid
		}
	} else if in.FilePart == 1 {
		err = c.svcCtx.Dao.SetFileInfo(c.ctx, &model.DfsFileInfo{
			Creator:      in.Creator,
			FileId:       in.FileId,
			FilePartSize: len(in.Bytes),
		})

		// TODO(@benqi): error
		if err != nil {
			c.Logger.Errorf("dfs.writeFilePartData - error: %v", err)
			return nil, mtproto.ErrFilePartInvalid
		}
	}

	if in.GetFileTotalParts() != nil {
		if in.GetFileTotalParts().GetValue() == in.FilePart+1 {
			err = c.svcCtx.Dao.SetFileInfo(
				c.ctx,
				&model.DfsFileInfo{
					Creator:          in.Creator,
					FileId:           in.FileId,
					LastFilePartSize: len(in.Bytes),
				})

			// TODO(@benqi): error
			if err != nil {
				c.Logger.Errorf("dfs.writeFilePartData - error: %v", err)
				return nil, mtproto.ErrFilePartInvalid
			}
		}
	}

	return mtproto.BoolTrue, nil
}

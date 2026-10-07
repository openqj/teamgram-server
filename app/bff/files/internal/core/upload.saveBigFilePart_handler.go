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
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// UploadSaveBigFilePart
// upload.saveBigFilePart#de7b673d file_id:long file_part:int file_total_parts:int bytes:bytes = Bool;
func (c *FilesCore) UploadSaveBigFilePart(in *mtproto.TLUploadSaveBigFilePart) (*mtproto.Bool, error) {
	if c == nil || c.MD == nil || c.MD.PermAuthKeyId == 0 || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DfsClient == nil {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetFileId() <= 0 {
		return nil, mtproto.ErrFileIdInvalid
	}
	if in.GetFilePart() < 0 {
		return nil, mtproto.ErrFilePartInvalid
	}
	if in.GetFilePart() > 2999 {
		return nil, mtproto.ErrFilePartInvalid
	}
	if in.GetFileTotalParts() < 1 || in.GetFileTotalParts() > 3000 {
		return nil, mtproto.ErrFilePartsInvalid
	}
	if in.GetFilePart() >= in.GetFileTotalParts() {
		return nil, mtproto.ErrFilePartInvalid
	}
	if len(in.GetBytes()) == 0 {
		return nil, mtproto.ErrFilePartEmpty
	}
	if len(in.GetBytes()) > 512*1024 {
		return nil, mtproto.ErrFilePartTooBig
	}
	_, err := c.svcCtx.Dao.DfsClient.DfsWriteFilePartData(c.ctx, &dfs.TLDfsWriteFilePartData{
		Creator:        c.MD.PermAuthKeyId,
		FileId:         in.FileId,
		FilePart:       in.FilePart,
		Bytes:          in.Bytes,
		Big:            true,
		FileTotalParts: &wrapperspb.Int32Value{Value: in.FileTotalParts},
	})
	if err != nil {
		c.Logger.Errorf("upload.saveFilePart - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}

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
	"crypto/sha256"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
)

const fileHashPartSize = 128 * 1024

// UploadGetFileHashes
// upload.getFileHashes#9156982a location:InputFileLocation offset:long = Vector<FileHash>;
func (c *FilesCore) UploadGetFileHashes(in *mtproto.TLUploadGetFileHashes) (*mtproto.Vector_FileHash, error) {
	location := in.GetLocation()
	if location == nil {
		c.Logger.Errorf("upload.getFileHashes - empty location")
		return nil, mtproto.ErrLocationInvalid
	}
	offset := in.GetOffset_INT64()
	if offset == 0 {
		offset = int64(in.GetOffset_INT32())
	}
	if offset < 0 {
		c.Logger.Errorf("upload.getFileHashes - offset: %d", offset)
		return nil, mtproto.ErrOffsetInvalid
	}

	// Same DFS download as upload.getFile. There is no hash RPC; hash 128KB parts.
	out := &mtproto.Vector_FileHash{Datas: []*mtproto.FileHash{}}
	for n := 0; n < 64; n++ {
		file, err := c.svcCtx.Dao.DfsClient.DfsDownloadFile(c.ctx, &dfs.TLDfsDownloadFile{
			Location: location,
			Offset:   offset,
			Limit:    fileHashPartSize,
		})
		if err != nil {
			c.Logger.Errorf("upload.getFileHashes - error: %v", err)
			if len(out.Datas) == 0 {
				return nil, err
			}
			break
		}
		part := file.GetBytes()
		if len(part) == 0 {
			break
		}
		sum := sha256.Sum256(part)
		hash := make([]byte, len(sum))
		copy(hash, sum[:])
		fh := mtproto.MakeTLFileHash(&mtproto.FileHash{
			Offset_INT64: offset,
			Limit:        int32(len(part)),
			Hash:         hash,
		}).To_FileHash()
		if offset <= int64(^uint32(0)>>1) {
			fh.Offset_INT32 = int32(offset)
		}
		out.Datas = append(out.Datas, fh)
		offset += int64(len(part))
		if len(part) < fileHashPartSize {
			break
		}
	}
	return out, nil
}

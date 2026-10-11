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
)

// UploadGetCdnFileHashes
// upload.getCdnFileHashes#91dc3f31 file_token:bytes offset:long = Vector<FileHash>;
func (c *FilesCore) UploadGetCdnFileHashes(in *mtproto.TLUploadGetCdnFileHashes) (*mtproto.Vector_FileHash, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if len(in.GetFileToken()) == 0 {
		return nil, mtproto.ErrFileTokenInvalid
	}
	offset := in.GetOffset_INT64()
	if offset == 0 {
		offset = int64(in.GetOffset_INT32())
	}
	if offset < 0 {
		return nil, mtproto.ErrOffsetInvalid
	}
	result := new(mtproto.Vector_FileHash)
	if err := c.callCDNProvider("upload.getCdnFileHashes", map[string]any{"file_token": in.GetFileToken(), "offset": offset}, result); err != nil {
		return nil, err
	}
	if result == nil || len(result.GetDatas()) > 10000 {
		return nil, mtproto.ErrCdnMethodInvalid
	}
	if !validCDNFileHashVector(result) {
		return nil, mtproto.ErrCdnMethodInvalid
	}
	return result, nil
}

func validCDNFileHashVector(result *mtproto.Vector_FileHash) bool {
	if result == nil || len(result.GetDatas()) > 10000 {
		return false
	}
	for _, fileHash := range result.GetDatas() {
		if fileHash == nil || fileHash.GetPredicateName() != mtproto.Predicate_fileHash || fileHash.GetOffset_INT64() < 0 || fileHash.GetLimit() <= 0 || fileHash.GetLimit() > fileHashPartSize || len(fileHash.GetHash()) != 32 {
			return false
		}
	}
	return true
}

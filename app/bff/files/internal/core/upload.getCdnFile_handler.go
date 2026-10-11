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

import "github.com/teamgram/proto/mtproto"

const maxCDNFileChunk = 1 << 20

// UploadGetCdnFile
// upload.getCdnFile#395f69da file_token:bytes offset:long limit:int = upload.CdnFile;
func (c *FilesCore) UploadGetCdnFile(in *mtproto.TLUploadGetCdnFile) (*mtproto.Upload_CdnFile, error) {
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
	if in.GetLimit() <= 0 || in.GetLimit() > maxCDNFileChunk {
		return nil, mtproto.ErrLimitInvalid
	}
	result := new(mtproto.Upload_CdnFile)
	if err := c.callCDNProvider("upload.getCdnFile", map[string]any{"file_token": in.GetFileToken(), "offset": offset, "limit": in.GetLimit()}, result); err != nil {
		return nil, err
	}
	if !validCDNFileResult(result) {
		return nil, mtproto.ErrCdnMethodInvalid
	}
	return result, nil
}

func validCDNFileResult(result *mtproto.Upload_CdnFile) bool {
	if result == nil {
		return false
	}
	bytesLen := len(result.GetBytes())
	requestTokenLen := len(result.GetRequestToken())
	switch result.GetPredicateName() {
	case mtproto.Predicate_upload_cdnFile:
		return bytesLen > 0 && bytesLen <= maxCDNFileChunk && requestTokenLen == 0
	case mtproto.Predicate_upload_cdnFileReuploadNeeded:
		return requestTokenLen > 0 && bytesLen == 0
	default:
		return false
	}
}

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
	if in.GetLimit() <= 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	if c != nil && c.Logger != nil {
		c.Logger.Errorf("upload.getCdnFile - error: %v", mtproto.ErrCdnMethodInvalid)
	}
	return nil, mtproto.ErrCdnMethodInvalid
}

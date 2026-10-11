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

// UploadReuploadCdnFile
// upload.reuploadCdnFile#9b2754a8 file_token:bytes request_token:bytes = Vector<FileHash>;
func (c *FilesCore) UploadReuploadCdnFile(in *mtproto.TLUploadReuploadCdnFile) (*mtproto.Vector_FileHash, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if len(in.GetFileToken()) == 0 {
		return nil, mtproto.ErrFileTokenInvalid
	}
	if len(in.GetRequestToken()) == 0 {
		return nil, mtproto.ErrTokenInvalid
	}
	result := new(mtproto.Vector_FileHash)
	if err := c.callCDNProvider("upload.reuploadCdnFile", map[string]any{"file_token": in.GetFileToken(), "request_token": in.GetRequestToken()}, result); err != nil {
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

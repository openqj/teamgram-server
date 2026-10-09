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
	"github.com/teamgram/teamgram-server/app/service/media/media/hashrpc"
)

// MessagesGetDocumentByHash
// messages.getDocumentByHash#b1f2061f sha256:bytes size:long mime_type:string = Document;
func (c *FilesCore) MessagesGetDocumentByHash(in *mtproto.TLMessagesGetDocumentByHash) (*mtproto.Document, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	size := in.GetSize2_INT64()
	if size == 0 {
		size = int64(in.GetSize2_INT32())
	}
	if len(in.GetSha256()) != sha256.Size || in.GetMimeType() == "" || size < 0 {
		if c != nil && c.Logger != nil {
			c.Logger.Errorf("messages.getDocumentByHash - invalid request")
		}
		return nil, mtproto.ErrDocumentInvalid
	}

	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MediaClient == nil {
		if c != nil && c.Logger != nil {
			c.Logger.Errorf("messages.getDocumentByHash - media provider unavailable")
		}
		return nil, mtproto.ErrMethodNotImpl
	}
	document, err := c.svcCtx.Dao.MediaClient.MediaGetDocumentByHash(c.ctx, &hashrpc.DocumentHashRequest{
		Sha256:   in.GetSha256(),
		Size:     size,
		MimeType: in.GetMimeType(),
	})
	if err != nil {
		return nil, err
	}
	if document == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return document, nil
}

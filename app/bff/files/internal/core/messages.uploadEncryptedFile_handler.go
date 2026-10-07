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
)

// MessagesUploadEncryptedFile
// messages.uploadEncryptedFile#5057c497 peer:InputEncryptedChat file:InputEncryptedFile = EncryptedFile;
func (c *FilesCore) MessagesUploadEncryptedFile(in *mtproto.TLMessagesUploadEncryptedFile) (*mtproto.EncryptedFile, error) {
	if in == nil || in.GetPeer() == nil || in.GetPeer().GetChatId() <= 0 || in.GetPeer().GetAccessHash() == 0 {
		c.Logger.Errorf("messages.uploadEncryptedFile - invalid encrypted chat")
		return nil, mtproto.ErrEncryptionIdInvalid
	}
	file := in.GetFile()
	if file == nil || file.GetPredicateName() == mtproto.Predicate_inputEncryptedFileEmpty || file.GetId() == 0 {
		c.Logger.Errorf("messages.uploadEncryptedFile - empty file")
		return nil, mtproto.ErrFileIdInvalid
	}
	switch file.GetPredicateName() {
	case mtproto.Predicate_inputEncryptedFileUploaded, mtproto.Predicate_inputEncryptedFileBigUploaded:
		if file.GetParts() <= 0 {
			c.Logger.Errorf("messages.uploadEncryptedFile - invalid parts")
			return nil, mtproto.ErrFilePartsInvalid
		}
	case mtproto.Predicate_inputEncryptedFile:
		if file.GetAccessHash() == 0 {
			c.Logger.Errorf("messages.uploadEncryptedFile - missing access hash")
			return nil, mtproto.ErrFileIdInvalid
		}
	default:
		c.Logger.Errorf("messages.uploadEncryptedFile - invalid file: %s", file.GetPredicateName())
		return nil, mtproto.ErrFileIdInvalid
	}

	// Same part store as upload.saveFilePart; DFS assembles it into an EncryptedFile.
	encrypted, err := c.svcCtx.Dao.DfsClient.DfsUploadEncryptedFileV2(c.ctx, &dfs.TLDfsUploadEncryptedFileV2{
		Creator: c.MD.PermAuthKeyId,
		File:    file,
	})
	if err != nil {
		c.Logger.Errorf("messages.uploadEncryptedFile - error: %v", err)
		return nil, err
	}
	if encrypted == nil {
		c.Logger.Errorf("messages.uploadEncryptedFile - empty encrypted file")
		return nil, mtproto.ErrFileIdInvalid
	}
	if encrypted.GetKeyFingerprint() == 0 {
		encrypted.KeyFingerprint = file.GetKeyFingerprint()
	}

	return encrypted, nil
}

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
	"fmt"
	"math/rand"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/dfs/dfs"
)

// DfsUploadEncryptedFileV2
// dfs.uploadEncryptedFileV2 creator:long file:InputEncryptedFile = EncryptedFile;
func (c *DfsCore) DfsUploadEncryptedFileV2(in *dfs.TLDfsUploadEncryptedFileV2) (*mtproto.EncryptedFile, error) {
	if in == nil || in.GetFile() == nil {
		return nil, mtproto.ErrMediaInvalid
	}

	var (
		file            = in.GetFile()
		creatorId       = in.GetCreator()
		encryptedFileId = c.svcCtx.Dao.IDGenClient2.NextId(c.ctx)
		accessHash      = int64(mtproto.CRC32_storage_filePartial)<<32 | int64(rand.Uint32())
	)

	fileInfo, err := c.svcCtx.Dao.GetFileInfo(c.ctx, creatorId, file.Id)
	if err != nil {
		c.Logger.Errorf("dfs.uploadDocumentFile - error: %v", err)
		return nil, err
	}
	path := fmt.Sprintf("%d.dat", encryptedFileId)

	// The RPC response is the durable upload acknowledgement. Do not return an
	// EncryptedFile while the object write is still running: a failed async
	// write would leave clients with an identity that cannot be downloaded.
	if _, err = c.svcCtx.Dao.PutEncryptedFile(c.ctx, path, c.svcCtx.Dao.NewSSDBReader(fileInfo)); err != nil {
		c.Logger.Errorf("dfs.uploadEncryptedFile - error: %v", err)
		return nil, err
	}
	if err = c.svcCtx.Dao.SetCacheFileInfo(c.ctx, encryptedFileId, fileInfo); err != nil {
		c.Logger.Errorf("dfs.uploadEncryptedFile - cache metadata: %v", err)
	}

	encryptedFile := mtproto.MakeTLEncryptedFile(&mtproto.EncryptedFile{
		Id:             encryptedFileId,
		AccessHash:     accessHash,
		Size2_INT32:    int32(fileInfo.GetFileSize()),
		Size2_INT64:    fileInfo.GetFileSize(),
		DcId:           1,
		KeyFingerprint: 0,
	}).To_EncryptedFile()

	return encryptedFile, nil
}

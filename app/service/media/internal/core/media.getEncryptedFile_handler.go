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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/media/media"
)

// MediaGetEncryptedFile
// media.getEncryptedFile id:long access_hash:long = EncryptedFile;
func (c *MediaCore) MediaGetEncryptedFile(in *media.TLMediaGetEncryptedFile) (*mtproto.EncryptedFile, error) {
	if in == nil || in.GetId() <= 0 || in.GetAccessHash() == 0 {
		return nil, mtproto.ErrMediaInvalid
	}

	file, err := c.svcCtx.Dao.GetEncryptedFile(c.ctx, in.GetId(), in.GetAccessHash())
	if err != nil {
		c.Logger.Errorf("media.getEncryptedFile - lookup: %v", err)
		return nil, err
	}
	return file, nil
}

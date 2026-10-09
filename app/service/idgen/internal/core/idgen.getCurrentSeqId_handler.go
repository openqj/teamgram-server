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
	"github.com/teamgram/teamgram-server/app/service/idgen/idgen"
)

// IdgenGetCurrentSeqId
// idgen.getCurrentSeqId key:string = Int64;
func (c *IdgenCore) IdgenGetCurrentSeqId(in *idgen.TLIdgenGetCurrentSeqId) (*mtproto.Int64, error) {
	id, err := c.svcCtx.Dao.Current(c.ctx, in.GetKey())
	if err != nil {
		c.Logger.Errorf("dgen.getCurrentSeqId(%s) error: %v", in.GetKey(), err)
		return nil, err
	}

	return &mtproto.Int64{
		V: id,
	}, nil
}
